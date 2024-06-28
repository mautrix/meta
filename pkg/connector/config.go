package connector

import (
	_ "embed"

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

func (s *MetaConnector) GetConfig() (string, any, up.Upgrader) {
	return ExampleConfig, &s.Config, up.SimpleUpgrader(upgradeConfig)
}
