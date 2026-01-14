package connector

import (
	"context"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"

	"go.mau.fi/mautrix-meta/pkg/metadb"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

type MetaConnector struct {
	Bridge      *bridgev2.Bridge
	Config      Config
	MsgConv     *msgconv.MessageConverter
	DeviceStore *sqlstore.Container
	DB          *metadb.MetaDB
}

var (
	_ bridgev2.NetworkConnector        = (*MetaConnector)(nil)
	_ bridgev2.MaxFileSizeingNetwork   = (*MetaConnector)(nil)
	_ bridgev2.NetworkResettingNetwork = (*MetaConnector)(nil)
)

func (m *MetaConnector) Init(bridge *bridgev2.Bridge) {
	m.Bridge = bridge
	m.DeviceStore = sqlstore.NewWithDB(
		m.Bridge.DB.RawDB,
		m.Bridge.DB.Dialect.String(),
		waLog.Zerolog(m.Bridge.Log.With().Str("db_section", "whatsmeow").Logger()),
	)
	m.Bridge.Commands.(*commands.Processor).AddHandlers(cmdToggleEncryption)
	m.DB = metadb.New(bridge.ID, bridge.DB.Database, m.Bridge.Log.With().Str("db_section", "meta").Logger())
	m.MsgConv = msgconv.New(bridge, m.DB)
	m.MsgConv.DisableViewOnce = m.Config.DisableViewOnce
}

func (m *MetaConnector) Start(ctx context.Context) error {
	m.ResetHTTPTransport()
	err := m.DeviceStore.Upgrade(ctx)
	if err != nil {
		return bridgev2.DBUpgradeError{Err: err, Section: "whatsmeow"}
	}
	err = m.DB.Upgrade(ctx)
	if err != nil {
		return bridgev2.DBUpgradeError{Err: err, Section: "meta"}
	}
	return nil
}

func (m *MetaConnector) SetMaxFileSize(maxSize int64) {
	m.MsgConv.MaxFileSize = maxSize
}

func (m *MetaConnector) GetName() bridgev2.BridgeName {
	if m.Config.Mode.IsMessenger() {
		return bridgev2.BridgeName{
			DisplayName:      "Facebook Messenger",
			NetworkURL:       "https://www.facebook.com/messenger",
			NetworkIcon:      "mxc://maunium.net/ygtkteZsXnGJLJHRchUwYWak",
			NetworkID:        "facebook",
			BeeperBridgeType: "facebookgo",
			DefaultPort:      29319,
		}
	}
	if m.Config.Mode.IsInstagram() {
		return bridgev2.BridgeName{
			DisplayName:      "Instagram",
			NetworkURL:       "https://instagram.com",
			NetworkIcon:      "mxc://maunium.net/JxjlbZUlCPULEeHZSwleUXQv",
			NetworkID:        "instagram",
			BeeperBridgeType: "instagramgo",
			DefaultPort:      29319,
		}
	}
	return bridgev2.BridgeName{
		DisplayName:      "Meta",
		NetworkURL:       "https://meta.com",
		NetworkIcon:      "mxc://maunium.net/DxpVrwwzPUwaUSazpsjXgcKB",
		NetworkID:        "meta",
		BeeperBridgeType: "meta",
		DefaultPort:      29319,
	}
}

func (m *MetaConnector) ResetHTTPTransport() {
	cfg := m.Bridge.GetHTTPClientSettings()
	msgconv.SetHTTP(cfg)
	if m.Config.ProxyMedia && m.Config.Proxy != "" {
		msgconv.SetProxy(m.Config.Proxy)
	}
	for _, login := range m.Bridge.GetAllCachedUserLogins() {
		login.Client.(*MetaClient).Client.SetHTTP(cfg)
	}
}

func (m *MetaConnector) ResetNetworkConnections() {
	for _, login := range m.Bridge.GetAllCachedUserLogins() {
		login.Client.(*MetaClient).Client.ForceReconnect()
		login.Client.(*MetaClient).E2EEClient.ResetConnection()
	}
}
