#!/bin/sh
set -e

for f in mqttbypass requeststream; do
	thrift --gen go -out . $f.thrift
	echo '//lint:file-ignore ST1005 The thrift generator is bad' >> ./$f/$f.go
	goimports -w ./$f/*.go
done
