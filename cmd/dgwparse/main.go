package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"slices"

	"go.mau.fi/util/exerrors"

	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
)

func main() {
	input := exerrors.Must(io.ReadAll(os.Stdin))
	input = exerrors.Must(base64.StdEncoding.AppendDecode(nil, input))
	for len(input) > 0 {
		frame := dgw.CheckFrameType(input)
		input = exerrors.Must(frame.Unmarshal(input))
		if slices.Contains(os.Args, "-r") {
			df, ok := frame.(*dgw.DataFrame)
			if ok {
				_, _ = os.Stdout.Write(df.Payload)
			}
		} else {
			fmt.Printf("%s\n", frame)
		}
	}
}
