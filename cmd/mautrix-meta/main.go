package main

import (
	"strconv"
	"strings"

	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"go.mau.fi/mautrix-meta/pkg/connector"
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
	Version:     "0.4.0",
	Connector:   c,
}

func main() {
	m.PostInit = func() {
		copyData := strings.ReplaceAll(
			legacyMigrateCopyData,
			"'hacky platform placeholder'",
			strconv.Itoa(int(c.Config.Mode.ToPlatform())),
		)
		if c.Config.Mode == "" {
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
