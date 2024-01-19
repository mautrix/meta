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

	"github.com/rs/zerolog"
	"go.mau.fi/util/configupgrade"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridge"
	"maunium.net/go/mautrix/bridge/commands"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/config"
	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
	"go.mau.fi/mautrix-meta/msgconv"
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
		msgconv.MediaReferer = "https://www.instagram.com/"
		br.ProtocolName = "Instagram DM"
		br.BeeperServiceName = "instagramgo"
		br.BeeperNetworkName = "instagram"
		defaultCommandPrefix = "!ig"
		MessagixPlatform = types.Instagram
		database.NewCookies = func() cookies.Cookies {
			return &cookies.InstagramCookies{}
		}
	case config.ModeFacebook:
		msgconv.MediaReferer = "https://www.facebook.com/"
		br.ProtocolName = "Facebook Messenger"
		br.BeeperServiceName = "facebookgo"
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

func (br *MetaBridge) CreatePrivatePortal(roomID id.RoomID, brInviter bridge.User, brGhost bridge.Ghost) {
	inviter := brInviter.(*User)
	puppet := brGhost.(*Puppet)

	log := br.ZLog.With().
		Str("action", "create private portal").
		Stringer("target_room_id", roomID).
		Stringer("inviter_mxid", inviter.MXID).
		Int64("invitee_fbid", puppet.ID).
		Logger()
	ctx := log.WithContext(context.TODO())
	log.Debug().Msg("Creating private chat portal")

	portal := br.GetPortalByThreadID(database.PortalKey{
		ThreadID: puppet.ID,
		Receiver: inviter.MetaID,
	}, table.ONE_TO_ONE)
	if len(portal.MXID) == 0 {
		resp, err := inviter.Client.ExecuteTasks([]socket.Task{
			&socket.CreateThreadTask{
				ThreadFBID:                portal.ThreadID,
				ForceUpsert:               0,
				UseOpenMessengerTransport: 0,
				SyncGroup:                 1,
				MetadataOnly:              0,
				PreviewOnly:               0,
			},
		})
		log.Trace().Any("response_data", resp).Msg("DM thread create response")
		if err != nil {
			log.Err(err).Msg("Failed to create DM thread")
			_, _ = puppet.DefaultIntent().LeaveRoom(ctx, roomID, &mautrix.ReqLeave{Reason: "Failed to create DM thread"})
			return
		}
		br.createPrivatePortalFromInvite(ctx, roomID, inviter, puppet, portal)
		return
	}
	log.Debug().
		Str("existing_room_id", portal.MXID.String()).
		Msg("Existing private chat portal found, trying to invite user")

	ok := portal.ensureUserInvited(ctx, inviter)
	if !ok {
		log.Warn().Msg("Failed to invite user to existing private chat portal. Redirecting portal to new room")
		br.createPrivatePortalFromInvite(ctx, roomID, inviter, puppet, portal)
		return
	}
	intent := puppet.DefaultIntent()
	errorMessage := fmt.Sprintf("You already have a private chat portal with me at [%[1]s](https://matrix.to/#/%[1]s)", portal.MXID)
	errorContent := format.RenderMarkdown(errorMessage, true, false)
	_, _ = intent.SendMessageEvent(ctx, roomID, event.EventMessage, errorContent)
	log.Debug().Msg("Leaving ghost from private chat room after accepting invite because we already have a chat with the user")
	_, _ = intent.LeaveRoom(ctx, roomID)
}

func (br *MetaBridge) createPrivatePortalFromInvite(ctx context.Context, roomID id.RoomID, inviter *User, puppet *Puppet, portal *Portal) {
	log := zerolog.Ctx(ctx)
	log.Debug().Msg("Creating private portal from invite")

	// Check if room is already encrypted
	var existingEncryption event.EncryptionEventContent
	var encryptionEnabled bool
	err := portal.MainIntent().StateEvent(ctx, roomID, event.StateEncryption, "", &existingEncryption)
	if err != nil {
		log.Err(err).Msg("Failed to check if encryption is enabled in private chat room")
	} else {
		encryptionEnabled = existingEncryption.Algorithm == id.AlgorithmMegolmV1
	}
	portal.MXID = roomID
	br.portalsLock.Lock()
	br.portalsByMXID[portal.MXID] = portal
	br.portalsLock.Unlock()
	intent := puppet.DefaultIntent()

	if br.Config.Bridge.Encryption.Default || encryptionEnabled {
		log.Debug().Msg("Adding bridge bot to new private chat portal as encryption is enabled")
		_, err = intent.InviteUser(ctx, roomID, &mautrix.ReqInviteUser{UserID: br.Bot.UserID})
		if err != nil {
			log.Err(err).Msg("Failed to invite bridge bot to enable e2be")
		}
		err = br.Bot.EnsureJoined(ctx, roomID)
		if err != nil {
			log.Err(err).Msg("Failed to join as bridge bot to enable e2be")
		}
		if !encryptionEnabled {
			_, err = intent.SendStateEvent(ctx, roomID, event.StateEncryption, "", portal.getEncryptionEventContent())
			if err != nil {
				log.Err(err).Msg("Failed to enable e2be")
			}
		}
		br.AS.StateStore.SetMembership(ctx, roomID, inviter.MXID, event.MembershipJoin)
		br.AS.StateStore.SetMembership(ctx, roomID, puppet.MXID, event.MembershipJoin)
		br.AS.StateStore.SetMembership(ctx, roomID, br.Bot.UserID, event.MembershipJoin)
		portal.Encrypted = true
	}
	portal.UpdateInfoFromPuppet(ctx, puppet)
	_, _ = intent.SendNotice(ctx, roomID, "Private chat portal created")
	log.Info().Msg("Created private chat portal after invite")
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
