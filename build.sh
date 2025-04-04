#!/bin/bash
if [[ -z "$LIBRARY_PATH" && -d /opt/homebrew ]]; then
	echo "Using /opt/homebrew for LIBRARY_PATH and CPATH"
	export LIBRARY_PATH=/opt/homebrew/lib
	export CPATH=/opt/homebrew/include
fi
export MAUTRIX_VERSION=$(cat go.mod | grep 'maunium.net/go/mautrix ' | awk '{ print $2 }')
export GO_LDFLAGS="-s -w -X main.Tag=$(git describe --exact-match --tags 2>/dev/null) -X main.Commit=$(git rev-parse HEAD) -X 'main.BuildTime=`date '+%b %_d %Y, %H:%M:%S'`' -X 'maunium.net/go/mautrix.GoModVersion=$MAUTRIX_VERSION'"
go build -ldflags "$GO_LDFLAGS" "$@" ./cmd/mautrix-meta
