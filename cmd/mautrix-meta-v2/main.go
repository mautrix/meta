package main

import (
	_ "go.mau.fi/util/dbutil/litestream"
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

func main() {
	m := mxmain.BridgeMain{
		Name:        "mautrix-meta",
		URL:         "https://github.com/mautrix/meta",
		Description: "A Matrix-Meta puppeting bridge.",
		Version:     "0.16.0",

		Connector: connector.NewConnector(),
	}
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}