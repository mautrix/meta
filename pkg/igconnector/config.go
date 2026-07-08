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
	_ "embed"
	"strings"
	"text/template"
	"time"

	up "go.mau.fi/util/configupgrade"
	"gopkg.in/yaml.v3"
)

//go:embed example-config.yaml
var ExampleConfig string

type Config struct {
	Proxy        string `yaml:"proxy"`
	GetProxyFrom string `yaml:"get_proxy_from"`
	ProxyMedia   bool   `yaml:"proxy_media"`
	ProxyOther   bool   `yaml:"proxy_other"`

	DisableXMABackfill bool `yaml:"disable_xma_backfill"`
	DisableXMAAlways   bool `yaml:"disable_xma_always"`

	MinFullReconnectIntervalSeconds int  `yaml:"min_full_reconnect_interval_seconds"`
	ForceRefreshIntervalSeconds     int  `yaml:"force_refresh_interval_seconds"`
	CacheConnectionState            bool `yaml:"cache_connection_state"`

	DisplaynameTemplate string             `yaml:"displayname_template"`
	displaynameTemplate *template.Template `yaml:"-"`

	DisableViewOnce bool `yaml:"disable_view_once"`
	DisableTyping   bool `yaml:"disable_typing"`

	ThreadBackfill ThreadBackfillConfig `yaml:"thread_backfill"`
}

type ThreadBackfillConfig struct {
	BatchCount int           `yaml:"batch_count"`
	BatchDelay time.Duration `yaml:"batch_delay"`
}

func (tbc ThreadBackfillConfig) Enabled() bool {
	return tbc.BatchCount != 0 && tbc.BatchCount != 1
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
	c.displaynameTemplate, err = template.New("displayname").Parse(c.DisplaynameTemplate)
	return err
}

func upgradeConfig(helper up.Helper) {
	helper.Copy(up.Str, "displayname_template")
	helper.Copy(up.Str|up.Null, "proxy")
	helper.Copy(up.Str|up.Null, "get_proxy_from")
	helper.Copy(up.Bool, "proxy_media")
	helper.Copy(up.Bool, "proxy_other")
	helper.Copy(up.Int, "min_full_reconnect_interval_seconds")
	helper.Copy(up.Int, "force_refresh_interval_seconds")
	helper.Copy(up.Bool, "cache_connection_state")
	helper.Copy(up.Bool, "disable_xma_backfill")
	helper.Copy(up.Bool, "disable_xma_always")
	helper.Copy(up.Bool, "disable_typing")
	helper.Copy(up.Bool, "disable_view_once")
	helper.Copy(up.Int, "thread_backfill", "batch_count")
	helper.Copy(up.Str|up.Int, "thread_backfill", "batch_delay")
}

func (ic *IGConnector) GetConfig() (string, any, up.Upgrader) {
	return ExampleConfig, &ic.Config, up.SimpleUpgrader(upgradeConfig)
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
