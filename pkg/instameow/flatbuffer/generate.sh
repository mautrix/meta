#!/bin/sh
set -e

flatc --go --gen-onefile --go-namespace lightspeed --require-explicit-ids -o . lightspeed.fbs
gofmt -w lightspeed_generated.go
