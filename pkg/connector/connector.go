package connector

import (
	"context"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/store"
)

type MetaConnector struct {
	Bridge *bridgev2.Bridge
	Config *MetaConfig

	store *store.Container
}

func NewConnector() *MetaConnector {
	return &MetaConnector{
		Config: &MetaConfig{},
	}
}

var _ bridgev2.NetworkConnector = (*MetaConnector)(nil)
var _ bridgev2.MaxFileSizeingNetwork = (*MetaConnector)(nil)

func (m *MetaConnector) SetMaxFileSize(maxSize int64) {
	println("SetMaxFileSize unimplemented")
}

var metaGeneralCaps = &bridgev2.NetworkGeneralCapabilities{
	DisappearingMessages: false,
	AggressiveUpdateInfo: false,
}

func (m *MetaConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return metaGeneralCaps
}

func (s *MetaConnector) GetName() bridgev2.BridgeName {
	if s.Config == nil || s.Config.Mode == "" {
		return bridgev2.BridgeName{
			DisplayName:      "Meta",
			NetworkURL:       "https://meta.com",
			NetworkIcon:      "mxc://maunium.net/DxpVrwwzPUwaUSazpsjXgcKB",
			NetworkID:        "meta",
			BeeperBridgeType: "meta",
			DefaultPort:      29319,
		}
	} else {
		if s.Config.Mode == "instagram" {
			return bridgev2.BridgeName{
				DisplayName:      "Instagram",
				NetworkURL:       "https://instagram.com",
				NetworkIcon:      "mxc://maunium.net/JxjlbZUlCPULEeHZSwleUXQv",
				NetworkID:        "instagram",
				BeeperBridgeType: "meta",
				DefaultPort:      29319,
			}
		} else if s.Config.Mode == "facebook" {
			return bridgev2.BridgeName{
				DisplayName:      "Facebook",
				NetworkURL:       "https://www.facebook.com/messenger",
				NetworkIcon:      "mxc://maunium.net/ygtkteZsXnGJLJHRchUwYWak",
				NetworkID:        "facebook",
				BeeperBridgeType: "meta",
				DefaultPort:      29319,
			}
		} else {
			panic("unknown mode in config") // This should never happen if ValidateConfig is implemented correctly
		}
	}
}

func (m *MetaConnector) Init(bridge *bridgev2.Bridge) {
	m.store = store.NewStore(bridge.DB.Database, dbutil.ZeroLogger(bridge.Log.With().Str("db_section", "meta").Logger()))
	m.Bridge = bridge
}

func (m *MetaConnector) Start(ctx context.Context) error {
	return m.store.Upgrade(ctx)
}

func (m *MetaConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	client, err := NewMetaClient(ctx, m, login)
	if err != nil {
		return err
	}
	login.Client = client
	return nil
}
