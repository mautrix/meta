// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
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

package igconnector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/metadb"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

type IGConnector struct {
	Bridge *bridgev2.Bridge
	Config Config
	//MsgConv *msgconv.MessageConverter
	DB *metadb.MetaDB
}

var (
	_ bridgev2.NetworkConnector        = (*IGConnector)(nil)
	_ bridgev2.MaxFileSizeingNetwork   = (*IGConnector)(nil)
	_ bridgev2.NetworkResettingNetwork = (*IGConnector)(nil)
)

func (ic *IGConnector) Init(bridge *bridgev2.Bridge) {
	ic.Bridge = bridge
	ic.DB = metadb.New(bridge.ID, bridge.DB.Database, ic.Bridge.Log.With().Str("db_section", "meta").Logger())
	//ic.MsgConv = msgconv.New(bridge, ic.DB)
	//ic.MsgConv.DisableViewOnce = ic.Config.DisableViewOnce
}

func (ic *IGConnector) Start(ctx context.Context) error {
	ic.ResetHTTPTransport()
	err := ic.DB.Upgrade(ctx)
	if err != nil {
		return bridgev2.DBUpgradeError{Err: err, Section: "meta"}
	}
	return nil
}

func (ic *IGConnector) SetMaxFileSize(maxSize int64) {
	//ic.MsgConv.MaxFileSize = maxSize
}

func (ic *IGConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:      "Instagram",
		NetworkURL:       "https://instagram.com",
		NetworkIcon:      "mxc://maunium.net/JxjlbZUlCPULEeHZSwleUXQv",
		NetworkID:        "instagram",
		BeeperBridgeType: "instagramgo",
		DefaultPort:      29319,
	}
}

func (ic *IGConnector) ResetHTTPTransport() {
	cfg := ic.Bridge.GetHTTPClientSettings()
	mediadl.SetHTTP(cfg)
	if ic.Config.ProxyMedia && ic.Config.Proxy != "" {
		mediadl.SetProxy(ic.Config.Proxy)
	}
	for _, login := range ic.Bridge.GetAllCachedUserLogins() {
		login.Client.(*IGClient).Client.GetHTTP().SetConfig(cfg)
	}
}

func (ic *IGConnector) ResetNetworkConnections() {
	for _, login := range ic.Bridge.GetAllCachedUserLogins() {
		login.Client.(*IGClient).Client.ForceReconnect()
	}
}
