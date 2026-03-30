package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"go.mau.fi/util/exhttp"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

var filename = flag.String("file", "", "Bloks response to parse")
var doPrint = flag.Bool("print", false, "Pretty-print the bundle")
var doHTML = flag.Bool("html", false, "Print as HTML")
var doLogin = flag.Bool("login", false, "Click the login button")
var do2FA = flag.String("2fa", "", "Submit a two-factor code")
var doMethods = flag.Bool("methods", false, "Print the available 2FA methods")
var selectedMethod = flag.String("method", "", "Select one of the 2FA methods")
var afad = flag.Bool("afad", false, "Run the AFAD handlers")
var doAction = flag.Bool("action", false, "Run the action script")
var logLevel = flag.String("log-level", "debug", "How much logging (zerolog)")
var doRPC = flag.String("rpc", "", "Make a Bloks RPC network request")
var rpcParams = flag.String("rpc-params", "", "JSON body for Bloks RPC")
var captcha = flag.Bool("captcha", false, "Extract information from the captcha page")
var doWeirdAction = flag.Bool("weird-action", false, "Do the weird page-embedded action")
var anotherWay = flag.Bool("another-way", false, "Click the alternate MFA method button")
var doSMS = flag.Bool("sms", false, "Trigger sending SMS code")
var doSMSCode = flag.String("sms-code", "", "Submit SMS code")
var doBackupCode = flag.String("backup-code", "", "Submit backup code")
var doEncrypt = flag.String("encrypt", "", "Encrypt a password")
var deviceID = flag.String("device-id", "", "Device ID for password encryption")

func main() {
	err := mainE()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err.Error())
		os.Exit(1)
	}
}

func readAndParse[T any](filename string) (*T, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileB, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	var data T
	err = json.Unmarshal(fileB, &data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &data, nil
}

// This code is liable to be out of date and crappy, because it is
// just for manual testing. See the actual login flow integrating with
// bridgev2 for the latest best practices.
func mainE() error {
	ctx := context.Background()

	flag.Parse()

	logLevel, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		return err
	}
	log := zerolog.New(zerolog.NewConsoleWriter()).Level(logLevel)
	ctx = log.WithContext(ctx)

	if *doEncrypt != "" {
		if *deviceID == "" {
			return fmt.Errorf("must give -device-id to use -encrypt")
		}
		cook := &cookies.Cookies{
			Platform: types.MessengerLite,
		}
		cl := messagix.NewClient(cook, log, &messagix.Config{
			ClientSettings: exhttp.SensibleClientSettings,
		})
		cl.MessengerLite.SetDeviceIdentifiers(uuid.MustParse(*deviceID))
		key, err := cl.FetchLightspeedKey(ctx)
		if err != nil {
			return err
		}
		enc, err := crypto.EncryptPassword(int(types.MessengerLite), key.KeyID, key.PublicKey, *doEncrypt)
		if err != nil {
			return err
		}

		fmt.Println(enc)
		return nil
	}
	if *doRPC != "" {
		if *rpcParams == "" {
			return fmt.Errorf("-rpc-params is mandatory when using -rpc")
		}

		var paramsOuter map[string]string
		err := json.Unmarshal([]byte(*rpcParams), &paramsOuter)
		if err != nil {
			return fmt.Errorf("parsing outer params: %w", err)
		}

		var paramsInner bloks.BloksParamsInner
		err = json.Unmarshal([]byte(paramsOuter["params"]), &paramsInner)
		if err != nil {
			return fmt.Errorf("parsing inner params: %w", err)
		}

		mcl := messagix.NewClient(&cookies.Cookies{
			Platform: types.MessengerLite,
		}, log, &messagix.Config{})
		mcl.MakeBloksRequest(ctx, &bloks.BloksAppDoc, bloks.NewBloksRequest(*doRPC, paramsInner))

		return nil
	}

	if *filename == "" {
		return fmt.Errorf("-file is mandatory")
	}

	bundle, err := readAndParse[bloks.BloksBundle](*filename)
	if err != nil {
		return err
	}
	if *doPrint {
		return bundle.Print("")
	}
	if *doHTML {
		return bundle.PrintHTML("")
	}
	lastURL := ""
	bridge := bloks.InterpBridge{
		DoRPC: func(ctx context.Context, name string, params map[string]string, isPage bool, callback func(result *bloks.BloksScriptLiteral) error) error {
			fmt.Printf("%s isPage=%v\n", name, isPage)
			payload, err := json.Marshal(params)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", string(payload))
			return nil
		},
		HandleLoginResponse: func(ctx context.Context, data string) error {
			fmt.Printf("%s\n", data)
			return nil
		},
		StartTimer: func(name string, interval time.Duration, callback func() error) error {
			for i := 0; i < 3; i++ {
				err := callback()
				if err != nil {
					return err
				}
			}
			return nil
		},
		OpenURL: func(url string) error {
			if lastURL != "" {
				return fmt.Errorf("already opened a url this session")
			}
			lastURL = url
			return nil
		},
	}
	interp, err := bloks.NewInterpreter(ctx, bundle, &bridge, nil)
	if err != nil {
		return err
	}
	if *doWeirdAction {
		action := bundle.FindDescendant(bloks.FilterByComponent("action")).GetScript("on_load")
		if action == nil {
			return fmt.Errorf("page-embedded action not found")
		}
		_, err := interp.Evaluate(ctx, &action.AST)
		if err != nil {
			return err
		}
		return nil
	}
	if *anotherWay {
		err := bundle.
			FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "Try another way")).
			FindContainingButton().
			TapButton(ctx, interp)
		if err != nil {
			return fmt.Errorf("tapping method selection button: %w", err)
		}
		return nil
	}
	if *doSMS {
		for _, mount := range bundle.FindDescendants(bloks.FilterByComponent("bk.components.OnMount")) {
			script := mount.GetScript("on_first_mount")
			if script == nil {
				continue
			}
			_, err := interp.Evaluate(ctx, &script.AST)
			if err != nil {
				return err
			}
		}
	}
	if *doAction {
		gotNewScreen := false
		if *doLogin {
			interp.Bridge.DisplayNewScreen = func(ctx context.Context, name string, newBundle *bloks.BloksBundle) error {
				bundle = newBundle
				interp, err = bloks.NewInterpreter(ctx, bundle, &bridge, interp)
				if err != nil {
					return err
				}
				gotNewScreen = true
				return nil
			}
		}
		_, err := interp.Evaluate(ctx, &bundle.Layout.Payload.Action.AST)
		if err != nil {
			return err
		}
		if !*doLogin {
			return nil
		}
		if !gotNewScreen {
			return fmt.Errorf("didn't get new screen from action")
		}
	}
	fillTextInput := func(fieldName string, fillText string) error {
		input := bundle.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			if comp.ComponentID != "bk.components.TextInput" {
				return false
			}
			name, ok := comp.Attributes["html_name"].BloksTreeNodeContent.(*bloks.BloksTreeLiteral)
			if !ok {
				return false
			}
			str, ok := name.BloksJavascriptValue.(string)
			if !ok {
				return false
			}
			return str == fieldName
		})
		if input == nil {
			return fmt.Errorf("couldn't find %s field", fieldName)
		}
		err := input.SetTextContent(fillText)
		if err != nil {
			return err
		}
		onChanged, ok := input.Attributes["on_text_change"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return fmt.Errorf("%s field doesn't have on_text_change script", fieldName)
		}
		_, err = interp.Evaluate(bloks.InterpBindThis(ctx, input), &onChanged.AST)
		if err != nil {
			return fmt.Errorf("%s on_text_changed: %w", fieldName, err)
		}
		return nil
	}
	tapButton := func(buttonText string) error {
		textComp := bundle.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			if comp.Attributes["text"] == nil {
				return false
			}
			text, ok := comp.Attributes["text"].BloksTreeNodeContent.(*bloks.BloksTreeLiteral)
			if !ok {
				return false
			}
			str, ok := text.BloksJavascriptValue.(string)
			if !ok {
				return false
			}
			return str == buttonText
		})
		if textComp == nil {
			return fmt.Errorf("couldn't find %s button", buttonText)
		}
		var buttonExtension *bloks.BloksTreeComponent
		textComp.FindAncestor(func(comp *bloks.BloksTreeComponent) bool {
			buttonExtension = comp.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
				return comp.ComponentID == "bk.components.FoaTouchExtension"
			})
			return buttonExtension != nil
		})
		if buttonExtension == nil {
			return fmt.Errorf("couldn't find %s button extension", buttonText)
		}
		onTouchDown, ok := buttonExtension.Attributes["on_touch_down"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return fmt.Errorf("%s button doesn't have on_touch_down script", buttonText)
		}
		onTouchUp, ok := buttonExtension.Attributes["on_touch_up"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return fmt.Errorf("%s button doesn't have on_touch_up script", buttonText)
		}
		_, err = interp.Evaluate(bloks.InterpBindThis(ctx, buttonExtension), &onTouchDown.AST)
		if err != nil {
			return fmt.Errorf("%s on_touch_down: %w", buttonText, err)
		}
		_, err = interp.Evaluate(bloks.InterpBindThis(ctx, buttonExtension), &onTouchUp.AST)
		if err != nil {
			return fmt.Errorf("%s on_touch_up: %w", buttonText, err)
		}
		return nil
	}
	if *doLogin {
		err = fillTextInput("email", "hello@example.com")
		if err != nil {
			return err
		}
		err = fillTextInput("password", "correct horse battery staple")
		if err != nil {
			return err
		}
		err = tapButton("Log in")
		if err != nil {
			return err
		}
	} else if *do2FA != "" {
		codeInput := bundle.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			if comp.ComponentID != "bk.components.TextInput" {
				return false
			}
			return comp.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.AccessibilityExtension" {
					return false
				}
				label, ok := comp.Attributes["label"].BloksTreeNodeContent.(*bloks.BloksTreeLiteral)
				if !ok {
					return false
				}
				str, ok := label.BloksJavascriptValue.(string)
				if !ok {
					return false
				}
				return str == "Code" || str == "Enter code"
			}) != nil
		})
		err := codeInput.SetTextContent(*do2FA)
		if err != nil {
			return err
		}
		onChanged, ok := codeInput.Attributes["on_text_change"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return fmt.Errorf("code field doesn't have on_text_change script")
		}
		_, err = interp.Evaluate(bloks.InterpBindThis(ctx, codeInput), &onChanged.AST)
		if err != nil {
			return fmt.Errorf("code on_text_changed: %w", err)
		}
		err = tapButton("Continue")
		if err != nil {
			return err
		}
	} else if *doMethods {
		foundMethods := map[string]*bloks.BloksTreeComponent{}
		methodNames := []string{}

		items := bundle.FindDescendant(bloks.FilterByAttribute(
			"bk.data.TextSpan", "text", "Choose a way to confirm it’s you",
		)).
			FindAncestor(bloks.FilterByComponent("bk.components.Collection")).
			FindDescendant(bloks.FilterByAttribute("bk.components.BoxDecoration", "border_width", "1dp")).
			FindAncestor(bloks.FilterByComponent("bk.components.Flexbox")).
			GetChildren("children")

		for _, item := range items {
			span := item.
				FindDescendant(bloks.FilterByComponent("bk.components.RichText")).
				GetChildren("spans")[0].
				FindDescendant(bloks.FilterByComponent("bk.data.TextSpan"))
			method := span.GetAttribute("text")
			foundMethods[method] = span
			methodNames = append(methodNames, method)
		}

		fmt.Printf("Found %d MFA method(s):\n", len(foundMethods))
		for _, methodName := range methodNames {
			fmt.Printf("- %s\n", methodName)
		}
		if len(foundMethods) == 0 {
			fmt.Printf("  (none)\n")
			return nil
		}
		if *selectedMethod == "" {
			return nil
		}
		err := foundMethods[*selectedMethod].FindContainingButton().TapButton(ctx, interp)
		if err != nil {
			return fmt.Errorf("tap selected method: %w", err)
		}
		err = bundle.
			FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, interp)
		if err != nil {
			return fmt.Errorf("tap continue: %w", err)
		}
	} else if *afad {
		for _, comp := range bundle.FindDescendants(func(comp *bloks.BloksTreeComponent) bool {
			if comp.ComponentID != "bk.components.VisibilityExtension" {
				return false
			}
			return comp.GetScript("on_appear") != nil
		}) {
			script := comp.GetScript("on_appear")
			_, err := interp.Evaluate(ctx, &script.AST)
			if err != nil {
				return fmt.Errorf("on_appear: %w", err)
			}
		}
	} else if *captcha {
		img := bundle.FindDescendant(bloks.FilterByAttribute("bk.components.Image", "unique_id", "i:com.bloks.www.two_step_verification.enter_text_captcha_code/p:captcha_image"))
		if img == nil {
			return fmt.Errorf("can't find captcha image")
		}
		imageURL := img.GetDynamicAttribute(ctx, interp, "url")
		if imageURL == "" {
			return fmt.Errorf("captcha image has no url")
		}
		fmt.Println("Image:", imageURL)
		audio := bundle.FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "play audio"))
		if audio == nil {
			return fmt.Errorf("can't find audio text")
		}
		clickable := audio.FindDescendant(bloks.FilterByComponent("bk.style.textspan.ClickableStyle"))
		if clickable == nil {
			return fmt.Errorf("audio text is not clickable")
		}
		onClick := clickable.GetScript("on_click")
		if onClick == nil {
			return fmt.Errorf("no on_click on audio text")
		}
		_, err := interp.Evaluate(ctx, &onClick.AST)
		if err != nil {
			return fmt.Errorf("clicking on audio text: %w", err)
		}
		if lastURL == "" {
			return fmt.Errorf("clicking on audio text failed to open url")
		}
		audioURL := strings.Replace(lastURL, "/player/", "/", 1)
		fmt.Println("Audio:", audioURL)
	} else if *doSMSCode != "" {
		err := bundle.
			FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(bloks.FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, interp, *doSMSCode)
		if err != nil {
			return fmt.Errorf("filling sms code input: %w", err)
		}

		err = bundle.
			FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, interp)
		if err != nil {
			return fmt.Errorf("tapping continue: %w", err)
		}
	} else if *doBackupCode != "" {
		err := bundle.
			FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(bloks.FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, interp, *doBackupCode)
		if err != nil {
			return fmt.Errorf("filling backup code input: %w", err)
		}

		err = bundle.
			FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, interp)
		if err != nil {
			return fmt.Errorf("tapping continue: %w", err)
		}
	}
	return nil
}
