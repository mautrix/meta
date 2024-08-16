package connector

import (
	"context"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

type MetaConnector struct {
	Bridge      *bridgev2.Bridge
	Config      Config
	MsgConv     *msgconv.MessageConverter
	DeviceStore *sqlstore.Container
}

var (
	_ bridgev2.NetworkConnector      = (*MetaConnector)(nil)
	_ bridgev2.MaxFileSizeingNetwork = (*MetaConnector)(nil)
)

func (m *MetaConnector) Init(bridge *bridgev2.Bridge) {
	m.Bridge = bridge
	m.MsgConv = msgconv.New(bridge)
	m.DeviceStore = sqlstore.NewWithDB(
		m.Bridge.DB.RawDB,
		m.Bridge.DB.Dialect.String(),
		waLog.Zerolog(m.Bridge.Log.With().Str("db_section", "whatsmeow").Logger()),
	)
}

func (m *MetaConnector) Start(ctx context.Context) error {
	err := m.DeviceStore.Upgrade()
	if err != nil {
		return bridgev2.DBUpgradeError{Err: err, Section: "whatsmeow"}
	}
	return nil
}

func (m *MetaConnector) SetMaxFileSize(maxSize int64) {
	m.MsgConv.MaxFileSize = maxSize
}

var metaGeneralCaps = &bridgev2.NetworkGeneralCapabilities{
	DisappearingMessages: false,
	AggressiveUpdateInfo: false,
}

func (m *MetaConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return metaGeneralCaps
}

func (m *MetaConnector) GetName() bridgev2.BridgeName {
	switch m.Config.Mode {
	case types.Facebook, types.FacebookTor, types.Messenger:
		return bridgev2.BridgeName{
			DisplayName:      "Facebook Messenger",
			NetworkURL:       "https://www.facebook.com/messenger",
			NetworkIcon:      "mxc://maunium.net/ygtkteZsXnGJLJHRchUwYWak",
			NetworkID:        "facebook",
			BeeperBridgeType: "facebookgo",
			DefaultPort:      29319,
		}
	case types.Instagram:
		return bridgev2.BridgeName{
			DisplayName:      "Instagram",
			NetworkURL:       "https://instagram.com",
			NetworkIcon:      "mxc://maunium.net/JxjlbZUlCPULEeHZSwleUXQv",
			NetworkID:        "instagram",
			BeeperBridgeType: "instagramgo",
			DefaultPort:      29319,
		}
	default:
		return bridgev2.BridgeName{
			DisplayName:      "Meta",
			NetworkURL:       "https://meta.com",
			NetworkIcon:      "mxc://maunium.net/DxpVrwwzPUwaUSazpsjXgcKB",
			NetworkID:        "meta",
			BeeperBridgeType: "meta",
			DefaultPort:      29319,
		}
	}
}
