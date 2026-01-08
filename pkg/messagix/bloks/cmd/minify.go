package main

import (
	_ "embed"
)

//go:embed minify.json
var unminifierJson []byte

type Unminifier struct {
	Functions map[BloksFunctionID]BloksFunctionID `json:"functions"`
}
