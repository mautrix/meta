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

type BloksScriptID string
type BloksDatumID string
type BloksPayloadID string
type BloksTemplateID string
type BloksComponentID string
type BloksClassID string
type BloksAttributeID string

type BloksScript struct {
	Raw string
}

func (bs *BloksScript) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &bs.Raw)
	if err != nil {
		return fmt.Errorf("parse bloks script: %w", err)
	}
	return nil
}

type BloksBundle struct {
	Layout BloksLayout `json:"layout"`
}

func (bb *BloksBundle) Print() {
	bb.Layout.Payload.Tree.Print()
}

type BloksLayout struct {
	Payload BloksPayload `json:"bloks_payload"`
}

type BloksPayload struct {
	Scripts     map[BloksScriptID]BloksScript     `json:"ft"`
	Data        []BloksDatum                      `json:"data"`
	Embedded    []BloksEmbeddedPayload            `json:"embedded_payloads"`
	Props       []BloksProp                       `json:"props"`
	Templates   map[BloksTemplateID]BloksTreeNode `json:"templates"`
	Attribution BloksErrorAttribution             `json:"error_attribution"`
	Tree        BloksTreeNode                     `json:"tree"`
}

type BloksDatum struct {
	ID   BloksDatumID   `json:"id"`
	Type string         `json:"type"`
	Info BloksDatumInfo `json:"data"`
}

type BloksDatumInfo struct {
	Name    string               `json:"key"`
	Mode    string               `json:"mode"`
	Initial BloksJavascriptValue `json:"initial"`
}

type BloksJavascriptValue any

type BloksEmbeddedPayload struct {
	ID       BloksPayloadID `json:"id"`
	Contents BloksBundle    `json:"payload"`
}

type BloksProp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BloksErrorAttribution struct {
	LoggingID   string `json:"logging_id"`
	SourceMapID string `json:"source_map_id"`
}

type BloksTreeNode interface {
	Print()
}

type BloksTreeComponent struct {
	ComponentID BloksComponentID
	Attributes  map[BloksAttributeID]BloksTreeNode
}

func (btc *BloksTreeComponent) Print() {
	//
}

type BloksTreeComponentList []BloksTreeComponent

func (btcl BloksTreeComponentList) Print() {
	//
}

type BloksTreeLiteral struct {
	BloksJavascriptValue
}

func (btl BloksTreeLiteral) Print() {
	//
}

func (btc *BloksTreeComponent) UnmarshalJSON(data []byte) error {
	btc.Attributes = map[BloksAttributeID]BloksTreeNode{}
	var raw map[string]json.RawMessage
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}
	for key, val := range raw {
		attr := BloksAttributeID(key)
		var mapType map[BloksComponentID]json.RawMessage
		mapErr := json.Unmarshal(val, &mapType)
		if mapErr == nil {
			if len(mapType) != 1 {
				return fmt.Errorf("bare components, length %d", len(mapType))
			}
			for id, subval := range mapType {
				comp := BloksTreeComponent{
					ComponentID: id,
				}
				btc.Attributes[attr] = &comp
				err := json.Unmarshal(subval, &comp)
				if err != nil {
					return err
				}
			}
			continue
		}
		var sliceType []map[BloksComponentID]json.RawMessage
		sliceErr := json.Unmarshal(val, &sliceType)
		if sliceErr == nil {
			children := []BloksTreeComponent{}
			for _, child := range sliceType {
				if len(child) != 1 {
					return fmt.Errorf("bare components, length %d", len(child))
				}
				for id, subval := range child {
					comp := BloksTreeComponent{
						ComponentID: id,
					}
					err := json.Unmarshal(subval, &comp)
					if err != nil {
						return err
					}
					children = append(children, comp)
				}
			}
			btc.Attributes[attr] = BloksTreeComponentList(children)
		}
		var literal BloksTreeLiteral
		err := json.Unmarshal(val, &literal)
		if err != nil {
			return err
		}
		btc.Attributes[attr] = literal
	}
	return nil
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
	bundle.Print()
	return nil
}
