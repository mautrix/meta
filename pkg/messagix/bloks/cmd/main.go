package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
)

var filename = flag.String("file", "", "Bloks response to parse")
var doInterp = flag.Bool("interp", false, "Run the interpreter")

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

func mainE() error {
	ctx := context.Background()
	flag.Parse()
	if *filename == "" {
		return fmt.Errorf("-file is mandatory")
	}
	bundle, err := readAndParse[bloks.BloksBundle](*filename)
	if err != nil {
		return err
	}
	un, err := bloks.GetUnminifier(bundle)
	if err != nil {
		return err
	}
	bundle.Unminify(un)
	if !*doInterp {
		return bundle.Print("")
	}
	interp := bloks.NewInterpreter(bundle, &bloks.InterpBridge{
		DoRPC: func(name string, params map[string]string) error {
			fmt.Printf("%s\n", name)
			payload, err := json.Marshal(params)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", string(payload))
			return nil
		},
	})
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
	err = fillTextInput("email", "hello@example.com")
	if err != nil {
		return err
	}
	err = fillTextInput("password", "correct horse battery staple")
	if err != nil {
		return err
	}
	loginText := bundle.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
		if comp.ComponentID != "bk.data.TextSpan" {
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
		return str == "Log in"
	})
	if loginText == nil {
		return fmt.Errorf("couldn't find login button")
	}
	var loginExtension *bloks.BloksTreeComponent
	loginText.FindAncestor(func(comp *bloks.BloksTreeComponent) bool {
		loginExtension = comp.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			return comp.ComponentID == "bk.components.FoaTouchExtension"
		})
		return loginExtension != nil
	})
	if loginExtension == nil {
		return fmt.Errorf("couldn't find login extension")
	}
	onTouchDown, ok := loginExtension.Attributes["on_touch_down"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
	if !ok {
		return fmt.Errorf("login button doesn't have on_touch_down script")
	}
	onTouchUp, ok := loginExtension.Attributes["on_touch_up"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
	if !ok {
		return fmt.Errorf("login button doesn't have on_touch_up script")
	}
	_, err = interp.Evaluate(bloks.InterpBindThis(ctx, loginExtension), &onTouchDown.AST)
	if err != nil {
		return fmt.Errorf("on_touch_down: %w", err)
	}
	_, err = interp.Evaluate(bloks.InterpBindThis(ctx, loginExtension), &onTouchUp.AST)
	if err != nil {
		return fmt.Errorf("on_touch_up: %w", err)
	}
	return nil
}
