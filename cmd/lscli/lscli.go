package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tidwall/gjson"
	"github.com/zyedidia/clipboard"
	"go.mau.fi/util/exerrors"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/messagix/table"
)

func main() {
	var data []byte
	if exerrors.Must(os.Stdin.Stat()).Mode()&os.ModeNamedPipe != 0 {
		data = exerrors.Must(io.ReadAll(os.Stdin))
	} else {
		exerrors.Must(0, clipboard.Initialize())
		data = []byte(exerrors.Must(clipboard.ReadAll("clipboard")))
	}
	fmt.Println(len(data))
	if !json.Valid(data) {
		_, _ = fmt.Fprintln(os.Stderr, "invalid json")
		return
	}

	var lsData *lightspeed.LightSpeedData
	var dependencies map[string]string
	if gjson.GetBytes(data, "payload").Exists() {
		var pbd *messagix.PublishResponseData
		exerrors.Must(0, json.Unmarshal(data, &pbd))
		dependencies = table.SPToDepMap(pbd.Sp)
		exerrors.Must(0, json.Unmarshal([]byte(pbd.Payload), &lsData))
	} else if gjson.GetBytes(data, "data.lightspeed_web_request_for_igd").Exists() {

	} else {
		_, _ = fmt.Fprintln(os.Stderr, "unsupported json")
		return
	}
	tbl := table.LSTable{}
	decoder := lightspeed.NewLightSpeedDecoder(dependencies, &tbl)
	decoder.Decode(lsData.Steps)
	exerrors.Must(0, json.NewEncoder(os.Stdout).Encode(&tbl))
}
