package connector

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	up "go.mau.fi/util/configupgrade"
	"gopkg.in/yaml.v3"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

//go:embed example-config.yaml
var ExampleConfig string

type Config struct {
	RawMode string         `yaml:"mode"`
	Mode    types.Platform `yaml:"-"`
	IGE2EE  bool           `yaml:"ig_e2ee"`

	Proxy        string `yaml:"proxy"`
	GetProxyFrom string `yaml:"get_proxy_from"`
	ProxyMedia   bool   `yaml:"proxy_media"`

	DisableXMABackfill bool `yaml:"disable_xma_backfill"`
	DisableXMAAlways   bool `yaml:"disable_xma_always"`

	MinFullReconnectIntervalSeconds int `yaml:"min_full_reconnect_interval_seconds"`
	ForceRefreshIntervalSeconds     int `yaml:"force_refresh_interval_seconds"`

	DisplaynameTemplate string             `yaml:"displayname_template"`
	displaynameTemplate *template.Template `yaml:"-"`
}

type umConfig Config

func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	err := node.Decode((*umConfig)(c))
	if err != nil {
		return err
	}
	return c.PostProcess()
}

func (c *Config) PostProcess() (err error) {
	c.Mode = types.PlatformFromString(c.RawMode)
	c.displaynameTemplate, err = template.New("displayname").Parse(c.DisplaynameTemplate)
	return err
}

func upgradeConfig(helper up.Helper) {
	helper.Copy(up.Str, "mode")
	helper.Copy(up.Bool, "ig_e2ee")
	helper.Copy(up.Str, "displayname_template")
	helper.Copy(up.Str|up.Null, "proxy")
	helper.Copy(up.Str|up.Null, "get_proxy_from")
	helper.Copy(up.Bool, "proxy_media")
	helper.Copy(up.Int, "min_full_reconnect_interval_seconds")
	helper.Copy(up.Int, "force_refresh_interval_seconds")
	helper.Copy(up.Bool, "disable_xma_backfill")
	helper.Copy(up.Bool, "disable_xma_always")
}

func (m *MetaConnector) GetConfig() (string, any, up.Upgrader) {
	return ExampleConfig, &m.Config, up.SimpleUpgrader(upgradeConfig)
}

func (m *MetaConnector) ValidateConfig() error {
	if m.Config.Mode == types.Unset && m.Config.RawMode != "" {
		return fmt.Errorf("invalid mode %q", m.Config.RawMode)
	}
	return nil
}

type DisplaynameParams struct {
	DisplayName string
	Username    string
	ID          int64
}

func (c *Config) FormatDisplayname(params DisplaynameParams) string {
	var buffer strings.Builder
	_ = c.displaynameTemplate.Execute(&buffer, params)
	return buffer.String()
}
