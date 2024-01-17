// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"go.mau.fi/util/configupgrade"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridge"
	"maunium.net/go/mautrix/bridge/commands"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/config"
	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/types"
)

//go:embed example-config.yaml
var ExampleConfig string

// Information to find out exactly which commit the bridge was built from.
// These are filled at build time with the -X linker flag.
var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type MetaBridge struct {
	bridge.Bridge

	Config *config.Config
	DB     *database.Database

	provisioning *ProvisioningAPI

	usersByMXID   map[id.UserID]*User
	usersByMetaID map[int64]*User
	usersLock     sync.Mutex

	managementRooms     map[id.RoomID]*User
	managementRoomsLock sync.Mutex

	portalsByMXID map[id.RoomID]*Portal
	portalsByID   map[database.PortalKey]*Portal
	portalsLock   sync.Mutex

	puppets             map[int64]*Puppet
	puppetsByCustomMXID map[id.UserID]*Puppet
	puppetsLock         sync.Mutex
}

var _ bridge.ChildOverride = (*MetaBridge)(nil)

func (br *MetaBridge) GetExampleConfig() string {
	return ExampleConfig
}

func (br *MetaBridge) GetConfigPtr() interface{} {
	br.Config = &config.Config{
		BaseConfig: &br.Bridge.Config,
	}
	br.Config.BaseConfig.Bridge = &br.Config.Bridge
	return br.Config
}

func (br *MetaBridge) ValidateConfig() error {
	switch br.Config.Meta.Mode {
	case config.ModeInstagram, config.ModeFacebook:
		// ok
	default:
		return fmt.Errorf("invalid meta bridge mode %q", br.Config.Meta.Mode)
	}
	return nil
}

func (br *MetaBridge) Init() {
	var defaultCommandPrefix string
	switch br.Config.Meta.Mode {
	case config.ModeInstagram:
		br.ProtocolName = "Instagram DM"
		br.BeeperServiceName = "instagram"
		br.BeeperNetworkName = "instagram"
		defaultCommandPrefix = "!ig"
		MessagixPlatform = types.Instagram
		database.NewCookies = func() cookies.Cookies {
			return &cookies.InstagramCookies{}
		}
	case config.ModeFacebook:
		br.ProtocolName = "Facebook Messenger"
		br.BeeperServiceName = "facebook"
		br.BeeperNetworkName = "facebook"
		defaultCommandPrefix = "!fb"
		MessagixPlatform = types.Facebook
		database.NewCookies = func() cookies.Cookies {
			return &cookies.FacebookCookies{}
		}
	}
	if br.Config.Bridge.CommandPrefix == "default" {
		br.Config.Bridge.CommandPrefix = defaultCommandPrefix
	}
	br.Config.Bridge.ManagementRoomText.Welcome = fmt.Sprintf(br.Config.Bridge.ManagementRoomText.Welcome, br.ProtocolName)
	br.CommandProcessor = commands.NewProcessor(&br.Bridge)
	br.RegisterCommands()

	br.DB = database.New(br.Bridge.DB)

	ss := br.Config.Bridge.Provisioning.SharedSecret
	if len(ss) > 0 && ss != "disable" {
		br.provisioning = &ProvisioningAPI{bridge: br, log: br.ZLog.With().Str("component", "provisioning").Logger()}
	}
}

func (br *MetaBridge) Start() {
	if br.provisioning != nil {
		br.ZLog.Debug().Msg("Initializing provisioning API")
		br.provisioning.Init()
	}
	go br.StartUsers()
}

func (br *MetaBridge) Stop() {
	for _, user := range br.usersByMXID {
		br.Log.Debugln("Disconnecting", user.MXID)
		user.Disconnect()
	}
}

func (br *MetaBridge) GetIPortal(mxid id.RoomID) bridge.Portal {
	p := br.GetPortalByMXID(mxid)
	if p == nil {
		return nil
	}
	return p
}

func (br *MetaBridge) GetIUser(mxid id.UserID, create bool) bridge.User {
	p := br.GetUserByMXID(mxid)
	if p == nil {
		return nil
	}
	return p
}

func (br *MetaBridge) IsGhost(mxid id.UserID) bool {
	_, isGhost := br.ParsePuppetMXID(mxid)
	return isGhost
}

func (br *MetaBridge) GetIGhost(mxid id.UserID) bridge.Ghost {
	p := br.GetPuppetByMXID(mxid)
	if p == nil {
		return nil
	}
	return p
}

func (br *MetaBridge) CreatePrivatePortal(roomID id.RoomID, _ bridge.User, ghost bridge.Ghost) {
	_, _ = ghost.DefaultIntent().LeaveRoom(context.TODO(), roomID, &mautrix.ReqLeave{
		Reason: "This bridge doesn't support creating direct chats yet",
	})
	//TODO implement?
}

func main() {
	br := &MetaBridge{
		usersByMXID:   make(map[id.UserID]*User),
		usersByMetaID: make(map[int64]*User),

		managementRooms: make(map[id.RoomID]*User),

		portalsByMXID: make(map[id.RoomID]*Portal),
		portalsByID:   make(map[database.PortalKey]*Portal),

		puppets:             make(map[int64]*Puppet),
		puppetsByCustomMXID: make(map[id.UserID]*Puppet),
	}
	br.Bridge = bridge.Bridge{
		Name:        "mautrix-meta",
		URL:         "https://github.com/mautrix/meta",
		Description: "A Matrix-Facebook Messenger and Instagram DM puppeting bridge.",
		Version:     "0.1.0",

		CryptoPickleKey: "mautrix.bridge.e2ee",

		ConfigUpgrader: &configupgrade.StructUpgrader{
			SimpleUpgrader: configupgrade.SimpleUpgrader(config.DoUpgrade),
			Blocks:         config.SpacedBlocks,
			Base:           ExampleConfig,
		},

		Child: br,
	}
	br.InitVersion(Tag, Commit, BuildTime)

	br.Main()
}
