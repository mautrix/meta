package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

var filename = flag.String("file", "", "Bloks response to parse")

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
	flag.Parse()
	if *filename == "" {
		return fmt.Errorf("-file is mandatory")
	}
	bundle, err := readAndParse[BloksBundle](*filename)
	if err != nil {
		return err
	}
	un, err := GetUnminifier()
	if err != nil {
		return err
	}
	bundle.Unminify(un)
	loginText := bundle.FindDescendant(func(comp *BloksTreeComponent) bool {
		if comp.ComponentID != "bk.data.TextSpan" {
			return false
		}
		text, ok := comp.Attributes["text"].BloksTreeNodeContent.(*BloksTreeLiteral)
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
	var loginExtension *BloksTreeComponent
	loginText.FindAncestor(func(comp *BloksTreeComponent) bool {
		loginExtension = comp.FindDescendant(func(comp *BloksTreeComponent) bool {
			return comp.ComponentID == "bk.components.FoaTouchExtension"
		})
		return loginExtension != nil
	})
	if loginExtension == nil {
		return fmt.Errorf("couldn't find login extension")
	}
	onTouchDown, ok := loginExtension.Attributes["on_touch_down"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("login button doesn't have on_touch_down script")
	}
	onTouchUp, ok := loginExtension.Attributes["on_touch_up"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("login button doesn't have on_touch_up script")
	}
	interp := NewInterpreter(bundle)
	_, err = interp.Execute(&onTouchDown.AST)
	if err != nil {
		return fmt.Errorf("on_touch_down: %w", err)
	}
	_, err = interp.Execute(&onTouchUp.AST)
	if err != nil {
		return fmt.Errorf("on_touch_up: %w", err)
	}
	return nil
}
