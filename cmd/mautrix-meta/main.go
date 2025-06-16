package main

import (
	"strings"

	"maunium.net/go/mautrix/bridgev2/bridgeconfig"
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"go.mau.fi/mautrix-meta/pkg/connector"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// Information to find out exactly which commit the bridge was built from.
// These are filled at build time with the -X linker flag.
var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var c = &connector.MetaConnector{}
var m = mxmain.BridgeMain{
	Name:        "mautrix-meta",
	URL:         "https://github.com/mautrix/meta",
	Description: "A Matrix-Meta puppeting bridge.",
	Version:     "0.5.1",
	Connector:   c,
}

func main() {
	bridgeconfig.HackyMigrateLegacyNetworkConfig = migrateLegacyConfig
	m.PostInit = func() {
		copyData := strings.ReplaceAll(
			legacyMigrateCopyData,
			"hacky platform placeholder",
			c.Config.Mode.String(),
		)
		if c.Config.Mode == types.Unset {
			copyData = "can't migrate;"
		}
		m.CheckLegacyDB(
			6,
			"v0.1.0",
			"v0.4.0",
			m.LegacyMigrateSimple(legacyMigrateRenameTables, copyData, 16),
			true,
		)
	}
	m.PostStart = func() {
		if m.Matrix.Provisioning != nil {
			m.Matrix.Provisioning.Router.HandleFunc("/v1/login", legacyProvLogin)
			m.Matrix.Provisioning.Router.HandleFunc("/v1/logout", legacyProvLogout)
		}
	}
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
