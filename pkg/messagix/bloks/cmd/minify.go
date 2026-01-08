package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed minify.json
var unminifierJson []byte

type Unminifier struct {
	Functions  map[BloksFunctionID]BloksFunctionID                        `json:"functions"`
	Components map[BloksComponentID]BloksComponentID                      `json:"components"`
	Properties map[BloksComponentID]map[BloksAttributeID]BloksAttributeID `json:"properties"`
}

var cachedUnminifier *Unminifier

func GetUnminifier() (*Unminifier, error) {
	if cachedUnminifier != nil {
		return cachedUnminifier, nil
	}
	var u Unminifier
	err := json.Unmarshal(unminifierJson, &u)
	if err != nil {
		return nil, err
	}
	cachedUnminifier = &u
	return &u, nil
}
