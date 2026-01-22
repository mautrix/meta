package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
)

var filename = flag.String("file", "", "Bloks response to parse")
var doPrint = flag.Bool("print", false, "Pretty-print the bundle")
var doLogin = flag.Bool("login", false, "Click the login button")
var do2FA = flag.String("2fa", "", "Submit a two-factor code")
var doMethods = flag.Bool("methods", false, "Print the available 2FA methods")
var selectedMethod = flag.String("method", "", "Select one of the 2FA methods")
var afad = flag.Bool("afad", false, "Run the AFAD handlers")
var doAction = flag.Bool("action", false, "Run the action script")
var logLevel = flag.String("log-level", "debug", "How much logging (zerolog)")

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
	if *filename == "" {
		return fmt.Errorf("-file is mandatory")
	}

	logLevel, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		return err
	}
	log := zerolog.New(zerolog.NewConsoleWriter()).Level(logLevel)
	ctx = log.WithContext(ctx)

	bundle, err := readAndParse[bloks.BloksBundle](*filename)
	if err != nil {
		return err
	}
	if *doPrint {
		return bundle.Print("")
	}
	bridge := bloks.InterpBridge{
		DoRPC: func(name string, params map[string]string, isPage bool, callback func(result *bloks.BloksScriptLiteral) error) error {
			fmt.Printf("%s isPage=%v\n", name, isPage)
			payload, err := json.Marshal(params)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", string(payload))
			return nil
		},
		HandleLoginResponse: func(data string) error {
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
	}
	interp, err := bloks.NewInterpreter(ctx, bundle, &bridge, nil)
	if err != nil {
		return err
	}
	if *doAction {
		gotNewScreen := false
		if *doLogin {
			interp.Bridge.DisplayNewScreen = func(name string, newBundle *bloks.BloksBundle) error {
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
				return str == "Code"
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
		possibleMethods := []string{
			"Notification on another device",
			"Authentication app",
		}
		foundMethods := map[string]*bloks.BloksTreeComponent{}
		for _, methodName := range possibleMethods {
			elem := bundle.FindDescendant(bloks.FilterByAttribute(
				"bk.data.TextSpan", "text", methodName,
			))
			if elem != nil {
				foundMethods[methodName] = elem
			}
		}
		fmt.Printf("Found %d MFA method(s):\n", len(foundMethods))
		for _, methodName := range possibleMethods {
			elem := foundMethods[methodName]
			if elem != nil {
				fmt.Printf("- %s\n", methodName)
			}
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
	}
	return nil
}
