package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

var filename = flag.String("file", "", "Bloks response to parse")
var minifyFilename = flag.String("minify", "", "Minification map")

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
	if *minifyFilename != "" {
		minify, err := readAndParse[Minification](*minifyFilename)
		if err != nil {
			return err
		}
		bundle.Unminify(minify)
	}
	return bundle.Print("")
}
