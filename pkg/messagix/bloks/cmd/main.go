package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

var filename = flag.String("file", "", "Bloks response to parse")
var referenceFilename = flag.String("reference", "", "Bloks reference file")

func main() {
	err := mainE()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err.Error())
		os.Exit(1)
	}
}

func readBloks(filename string) (*BloksBundle, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileB, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	var bundle BloksBundle
	err = json.Unmarshal(fileB, &bundle)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &bundle, nil
}

func mainE() error {
	flag.Parse()
	if *filename == "" {
		return fmt.Errorf("-file is mandatory")
	}
	bundle, err := readBloks(*filename)
	if err != nil {
		return err
	}
	if *referenceFilename != "" {
		reference, err := readBloks(*referenceFilename)
		if err != nil {
			return err
		}
		minify, err := ReverseMinify(bundle, reference)
		if err != nil {
			return err
		}
		bundle.Unminify(minify)
	}
	return bundle.Print("")
}
