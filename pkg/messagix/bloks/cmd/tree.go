package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type BloksBundle struct {
	Layout BloksLayout `json:"layout"`
}

func (bb *BloksBundle) Print(indent string) error {
	return bb.Layout.Payload.Tree.Print(indent)
}

type BloksLayout struct {
	Payload BloksPayload `json:"bloks_payload"`
}

type BloksPayload struct {
	Scripts     map[BloksScriptID]BloksTreeScript `json:"ft"`
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

type BloksTreeNode struct {
	BloksTreeNodeContent
}

func (btn *BloksTreeNode) UnmarshalJSON(data []byte) error {
	var mapType map[BloksComponentID]json.RawMessage
	mapErr := json.Unmarshal(data, &mapType)
	if mapErr == nil {
		if len(mapType) != 1 {
			ids := []BloksComponentID{}
			for id := range mapType {
				ids = append(ids, id)
			}
			return fmt.Errorf("not exactly one component in map, ids: %q", ids)
		}
		for id, subdata := range mapType {
			comp := BloksTreeComponent{
				ComponentID: id,
			}
			err := json.Unmarshal(subdata, &comp)
			if err != nil {
				return fmt.Errorf("component %q: %w", id, err)
			}
			btn.BloksTreeNodeContent = &comp
		}
		return nil
	}
	sliceErr := json.Unmarshal(data, &[]map[BloksComponentID]json.RawMessage{})
	if sliceErr == nil {
		var comps BloksTreeComponentList
		err := json.Unmarshal(data, &comps)
		if err != nil {
			return err
		}
		btn.BloksTreeNodeContent = &comps
		return nil
	}
	var literal BloksTreeLiteral
	err := json.Unmarshal(data, &literal)
	if err != nil {
		return err
	}
	if str, ok := literal.BloksJavascriptValue.(string); ok && strings.HasPrefix(str, "\t") {
		script := BloksTreeScript{}
		err := script.Parse(str)
		if err != nil {
			return fmt.Errorf("script: %w", err)
		}
		btn.BloksTreeNodeContent = &script
	} else {
		btn.BloksTreeNodeContent = &literal
	}
	return nil
}

type BloksTreeNodeContent interface {
	Print(prefix string) error
}

type BloksTreeComponent struct {
	ComponentID BloksComponentID
	Attributes  map[BloksAttributeID]BloksTreeNode
}

// This could just unmarshal the whole map directly, but I wrote it
// explicitly to add better error messaging.
func (btc *BloksTreeComponent) UnmarshalJSON(data []byte) error {
	var rawAttrs = map[BloksAttributeID]json.RawMessage{}
	err := json.Unmarshal(data, &rawAttrs)
	if err != nil {
		return err
	}
	btc.Attributes = map[BloksAttributeID]BloksTreeNode{}
	for attr, subdata := range rawAttrs {
		var node BloksTreeNode
		err := json.Unmarshal(subdata, &node)
		if err != nil {
			return fmt.Errorf("attribute %q: %w", attr, err)
		}
		btc.Attributes[attr] = node
	}
	return nil
}

func (btc *BloksTreeComponent) Print(indent string) error {
	fmt.Printf("%s<Component id=%q>\n", indent, btc.ComponentID)
	attrs := []BloksAttributeID{}
	for attr := range btc.Attributes {
		attrs = append(attrs, attr)
	}
	sort.Slice(attrs, func(i, j int) bool { return attrs[i] < attrs[j] })
	for attr, value := range btc.Attributes {
		switch node := value.BloksTreeNodeContent.(type) {
		case *BloksTreeComponent:
			fmt.Printf("%s  <Attribute type=\"component\" id=%q>\n", indent, attr)
			err := node.Print(indent + "    ")
			if err != nil {
				return err
			}
			fmt.Printf("%s  </Attribute type=\"component\" id=%q>\n", indent, attr)
		case *BloksTreeComponentList:
			fmt.Printf("%s  <Attribute type=\"component-list\" id=%q>\n", indent, attr)
			err := node.Print(indent + "    ")
			if err != nil {
				return err
			}
			fmt.Printf("%s  </Attribute type=\"component-list\" id=%q>\n", indent, attr)
		case *BloksTreeLiteral:
			fmt.Printf("%s  <Attribute type=\"literal\" id=%q>\n", indent, attr)
			err := node.Print(indent + "    ")
			if err != nil {
				return err
			}
			fmt.Printf("%s  </Attribute type=\"literal\" id=%q>\n", indent, attr)
		case *BloksTreeScript:
			fmt.Printf("%s  <Attribute type=\"script\" id=%q>\n", indent, attr)
			err := node.Print(indent + "    ")
			if err != nil {
				return err
			}
			fmt.Printf("\n%s  </Attribute type=\"script\" id=%q>\n", indent, attr)
		}
	}
	fmt.Printf("%s</Component>\n", indent)
	return nil
}

type BloksTreeComponentList []*BloksTreeComponent

func (btcl *BloksTreeComponentList) UnmarshalJSON(data []byte) error {
	var rawComps = []json.RawMessage{}
	err := json.Unmarshal(data, &rawComps)
	if err != nil {
		return err
	}
	*btcl = BloksTreeComponentList{}
	for idx, subdata := range rawComps {
		var node BloksTreeNode
		err := json.Unmarshal(subdata, &node)
		if err != nil {
			return fmt.Errorf("item %d: %w", idx, err)
		}
		comp, ok := node.BloksTreeNodeContent.(*BloksTreeComponent)
		if !ok {
			return fmt.Errorf("item %d: unexpected type %T", idx, node.BloksTreeNodeContent)
		}
		*btcl = append(*btcl, comp)
	}
	return nil
}

func (btcl BloksTreeComponentList) Print(indent string) error {
	for _, comp := range btcl {
		err := comp.Print(indent)
		if err != nil {
			return err
		}
	}
	return nil
}

type BloksTreeLiteral struct {
	BloksJavascriptValue
}

func (btl *BloksTreeLiteral) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &btl.BloksJavascriptValue)
}

func (btl *BloksTreeLiteral) Print(indent string) error {
	str, err := json.Marshal(btl.BloksJavascriptValue)
	if err != nil {
		return err
	}
	fmt.Printf("%s%s\n", indent, str)
	return nil
}
