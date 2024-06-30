package connector

import (
	_ "embed"
	"fmt"

	up "go.mau.fi/util/configupgrade"
)

//go:embed example-config.yaml
var ExampleConfig string

type MetaConfig struct {
	Mode string `yaml:"mode"`
}

func upgradeConfig(helper up.Helper) {
	helper.Copy(up.Str, "mode")
}

func (m *MetaConnector) GetConfig() (string, any, up.Upgrader) {
	return ExampleConfig, m.Config, up.SimpleUpgrader(upgradeConfig)
}

func (m *MetaConnector) ValidateConfig() error {
	if m.Config.Mode != "facebook" && m.Config.Mode != "instagram" && m.Config.Mode != "" {
		return fmt.Errorf("invalid mode %q", m.Config.Mode)
	}
	return nil
}
