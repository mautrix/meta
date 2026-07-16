package main

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"go.mau.fi/mautrix-meta/pkg/igconnector"
)

// Information to find out exactly which commit the bridge was built from.
// These are filled at build time with the -X linker flag.
var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var m = mxmain.BridgeMain{
	Name:        "mautrix-meta",
	URL:         "https://github.com/mautrix/meta",
	Description: "A Matrix-Instagram puppeting bridge.",
	Version:     "26.07",
	SemCalVer:   true,
	Connector:   &igconnector.IGConnector{},
}

func main() {
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
