package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// Tool configuration
var flagLogLevel = flag.String("log-level", "debug", "How much zerolog logging")

// Client configuration
var flagIsAndroid = flag.Bool("android", false, "Use Android client instead of iOS")

// Main data inputs
var flagUserInput = flag.String("user-input", "", "Login step user input key/value json")
var flagReplayLog = flag.String("replay", "", "Log file to replay Bloks responses from")

func main() {
	err := mainE()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err.Error())
		os.Exit(1)
	}
}

func readFile(filename string) ([]byte, error) {
	if filename == "-" {
		filename = "/dev/stdin"
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

var logsRespGzRegexp = regexp.MustCompile(`"resp_gz":"([^"]+)"`)
var logsBloksAppRegexp = regexp.MustCompile(`"bloks_app":"([^"]+)"`)

func readReplayLogFile(filename string) (map[string][][]byte, error) {
	contents, err := readFile(filename)
	if err != nil {
		return nil, err
	}
	history := map[string][][]byte{}
	for _, line := range bytes.Split(contents, []byte("\n")) {
		bloksAppMatch := logsBloksAppRegexp.FindSubmatch(line)
		respGzMatch := logsRespGzRegexp.FindSubmatch(line)
		if bloksAppMatch == nil || respGzMatch == nil {
			continue
		}
		bloksApp := string(bloksAppMatch[1])
		respGzBase64 := respGzMatch[1]
		respGz, err := base64.StdEncoding.AppendDecode(nil, respGzBase64)
		if err != nil {
			return nil, err
		}
		gzipReader, err := gzip.NewReader(bytes.NewReader(respGz))
		if err != nil {
			return nil, err
		}
		resp, err := io.ReadAll(gzipReader)
		if err != nil {
			return nil, err
		}
		history[bloksApp] = append(history[bloksApp], resp)
	}
	return history, nil
}

func mainE() error {
	flag.Parse()

	ctx := context.Background()
	logLevel, err := zerolog.ParseLevel(*flagLogLevel)
	if err != nil {
		return err
	}
	log := zerolog.New(zerolog.NewConsoleWriter()).Level(logLevel)
	ctx = log.WithContext(ctx)

	plat := types.MessengerLiteIOS
	if *flagIsAndroid {
		plat = types.MessengerLiteAndroid
	}

	userInput := map[string]string{}
	if *flagUserInput != "" {
		err = json.Unmarshal([]byte(*flagUserInput), &userInput)
		if err != nil {
			return err
		}
	}

	replayLog := map[string][][]byte{}
	if *flagReplayLog != "" {
		replayLog, err = readReplayLogFile(*flagReplayLog)
		if err != nil {
			return err
		}
	}

	b, err := bloks.NewBrowser(&bloks.BrowserConfig{
		Platform: plat,
		EncryptPassword: func(ctx context.Context, password string) (string, error) {
			return fmt.Sprintf(`#PWD_TEST_UNENCRYPTED:` + password), nil
		},
		MakeBloksRequest: func(ctx context.Context, doc *bloks.BloksDoc, appID string, inner bloks.BloksParamsInner, deviceID string, familyDeviceID string) (*bloks.BloksBundle, error) {
			log.Debug().Str("bloks_app", appID).Msg("Making Bloks request")
			if prepared := replayLog[appID]; prepared != nil {
				replayLog[appID] = prepared[1:]
				var respInner bloks.BloksBundle
				err := json.Unmarshal(prepared[0], &respInner)
				if err != nil {
					return nil, err
				}
				return &respInner, nil
			}
			return nil, fmt.Errorf("missing bloks response: %s", appID)
		},
	})
	if err != nil {
		return err
	}

	availableUserInput := map[string]string{}
	for b.State != bloks.StateSuccess {
		step, err := b.DoLoginStep(ctx, availableUserInput)
		if err != nil {
			return err
		}
		if step != nil && step.UserInputParams != nil {
			for _, input := range step.UserInputParams.Fields {
				if provided := userInput[input.ID]; provided != "" {
					availableUserInput[input.ID] = provided
				} else {
					return fmt.Errorf("missing user input: %s", input.ID)
				}
			}
		}
	}

	return nil
}
