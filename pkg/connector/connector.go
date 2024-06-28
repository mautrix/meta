package connector

import (
	"context"
	
	"maunium.net/go/mautrix/bridgev2"
)

type MetaConnector struct {
	Bridge *bridgev2.Bridge
	Config  *MetaConfig
}

func NewConnector() *MetaConnector {
	return &MetaConnector{}
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
	// Would be nice if we could use MetaConfig.Mode here, but it's not available yet?
	return bridgev2.BridgeName{
		DisplayName:      "Meta",
		NetworkURL:       "https://meta.com",
		NetworkIcon:      "mxc://maunium.net/JxjlbZUlCPULEeHZSwleUXQv", // Instagram icon
		NetworkID:        "meta",
		BeeperBridgeType: "meta",
		DefaultPort:      29328,
	}
}

func (m *MetaConnector) Init(*bridgev2.Bridge) {
	println("Connector Init unimplemented")
}

func (m *MetaConnector) Start(context.Context) error {
	println("Connector Start unimplemented")
	return nil
}

func (m *MetaConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	panic("unimplemented")
}