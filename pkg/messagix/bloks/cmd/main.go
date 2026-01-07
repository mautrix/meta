package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

var filename = flag.String("file", "", "Bloks response to parse")
var reference = flag.String("reference", "", "Bloks reference file")

func main() {
	err := mainE()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err.Error())
		os.Exit(1)
	}
}

func mainE() error {
	flag.Parse()
	if *filename == "" {
		return fmt.Errorf("-file is mandatory")
	}
	file, err := os.Open(*filename)
	if err != nil {
		return err
	}
	defer file.Close()
	fileB, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	var bundle BloksBundle
	err = json.Unmarshal(fileB, &bundle)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	return bundle.Print("")
}
