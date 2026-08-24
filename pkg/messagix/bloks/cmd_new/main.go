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

// Starting state
var flagPayload = flag.String("payload", "", "Bloks payload filename (page or action)")
var flagDoAction = flag.Bool("action", false, "Execute the action (e.g. to display an initial page)")
var flagStartingState = flag.String("state", "", "Browser state name (also needs -payload)")

// What to do
var flagDoPerform = flag.Bool("perform", false, "Perform login actions")
var flagDoPrint = flag.Bool("print", false, "Pretty-print the Bloks payload")
var flagDoHTML = flag.Bool("html", false, "Print an HTML version of the Bloks payload")

// Q: I have a minified bloks payload from logs, and want it
// pretty-printed.
//
// A: Use -payload <file> -print or -payload <file> -html.
//
// Q: I have the logs of an entire bridge login session, which fails
// at some point. I want to reproduce the error and then test my fix.
//
// A: Use -replay <file> -perform -user-input <json>. You can add the
// json incrementally as the tool will report which field(s) are
// missing.
//
// Q: I have a minified bloks payload from logs, the bridge fails to
// handle its contents correctly. I want to reproduce the error and
// then test my fix. I only want to run code starting from that
// specific screen.
//
// A: If the bloks payload is type=page, then use -payload <file>
// -state <val> -perform. If it's type=action, then add -action too.
// You need to specify the state enum string from the selenium module,
// that corresponds to processing your page. This may also be printed
// in bridge logs.

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

func readBloksPayloadFile(filename string) (*bloks.BloksBundle, error) {
	fileB, err := readFile(filename)
	if err != nil {
		return nil, err
	}
	var data bloks.BloksBundle
	err = json.Unmarshal(fileB, &data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &data, nil
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

	var b *bloks.Browser
	b, err = bloks.NewBrowser(&bloks.BrowserConfig{
		Platform: plat,
		EncryptPassword: func(ctx context.Context, password string) (string, error) {
			return fmt.Sprintf(`#PWD_TEST_UNENCRYPTED:%s`, password), nil
		},
		MakeBloksRequest: func(ctx context.Context, doc *bloks.BloksDoc, appID string, inner bloks.BloksParamsInner, deviceID string, familyDeviceID string) (*bloks.BloksBundle, error) {
			if appID == "initial_action" {
				return b.CurrentPage, nil
			}
			log.Debug().Str("bloks_app", appID).Any("params", inner).Msg("Making Bloks request")
			if prepared := replayLog[appID]; prepared != nil {
				replayLog[appID] = prepared[1:]
				var respInner bloks.BloksBundle
				err := json.Unmarshal(prepared[0], &respInner)
				if err != nil {
					return nil, err
				}
				return &respInner, nil
			}
			if *flagReplayLog == "" {
				return nil, fmt.Errorf("missing bloks response (provide with -replay): %s", appID)
			}
			return nil, fmt.Errorf("missing bloks response: %s", appID)
		},
	})
	if err != nil {
		return err
	}

	if *flagPayload != "" {
		data, err := readBloksPayloadFile(*flagPayload)
		if err != nil {
			return err
		}
		b.CurrentPage = data

		err = b.CurrentPage.SetupInterpreter(ctx, b.Bridge, nil, true)
		if err != nil {
			return err
		}
	}
	if *flagStartingState != "" {
		b.State = bloks.BrowserState(*flagStartingState)
	}

	if *flagDoAction {
		b.Bridge.DoActionRPC(ctx, "initial_action", nil)
	}

	doneSomething := false

	if *flagDoPrint {
		doneSomething = true
		if b.CurrentPage == nil {
			return fmt.Errorf("no payload to print (provide with -payload)")
		}
		b.CurrentPage.Print(os.Stdout, "")
	}
	if *flagDoHTML {
		doneSomething = true
		if b.CurrentPage == nil {
			return fmt.Errorf("no payload to print (provide with -payload)")
		}
		b.CurrentPage.PrintHTML(os.Stdout, "")
	}
	if *flagDoPerform {
		doneSomething = true
		if *flagPayload != "" && *flagStartingState == "" {
			return fmt.Errorf("-perform with -payload needs -state too")
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
			if step != nil && step.CookiesParams != nil {
				for _, input := range step.CookiesParams.Fields {
					if provided := userInput[input.ID]; provided != "" {
						availableUserInput[input.ID] = provided
					} else {
						return fmt.Errorf("missing user input: %s", input.ID)
					}
				}
			}
		}
	}

	if !doneSomething {
		fmt.Println("no errors, but you may want -perform or -print or -html")
	}

	return nil
}
