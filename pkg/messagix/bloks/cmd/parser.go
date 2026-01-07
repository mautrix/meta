package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
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
type BloksFunctionID string

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

func (bb *BloksBundle) Print(indent string) error {
	return bb.Layout.Payload.Tree.Print(indent)
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
	}
	btn.BloksTreeNodeContent = &literal
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

type BloksTreeScript struct {
	AST BloksScriptNode
}

func (bst *BloksTreeScript) Parse(code string) error {
	_, err := bst.AST.ParseAny(code, 0)
	return err
}

func (bst *BloksTreeScript) Print(indent string) error {
	return bst.AST.Print(indent)
}

type BloksScriptNode struct {
	BloksScriptNodeContent
}

var ParseEndOfFuncall = errors.New("end of funcall")

func (node *BloksScriptNode) ParseAny(code string, start int) (int, error) {
	for idx := start; idx < len(code); idx++ {
		switch code[idx] {
		case '\t', ' ':
			continue
		case '(':
			funcall := BloksScriptFuncall{}
			node.BloksScriptNodeContent = &funcall
			return funcall.Parse(code, idx)
		case ')':
			return idx, ParseEndOfFuncall
		default:
			var literal BloksScriptLiteral
			node.BloksScriptNodeContent = &literal
			return literal.Parse(code, idx)
		}
	}
	return len(code), fmt.Errorf("eof at toplevel")
}

type BloksScriptNodeContent interface {
	Parse(code string, start int) (int, error)
	Print(indent string) error
}

type BloksScriptFuncall struct {
	Function BloksFunctionID
	Args     []BloksScriptNode
}

func (call *BloksScriptFuncall) Parse(code string, start int) (int, error) {
	start += 1
	for idx := start; idx < len(code); idx++ {
		switch code[idx] {
		case '\t', ' ':
			continue
		}
		start = idx
		break
	}
	end := start
	for idx := start; idx < len(code); idx++ {
		if code[idx] >= 'a' && code[idx] <= 'z' {
			continue
		}
		if code[idx] >= 'A' && code[idx] <= 'Z' {
			continue
		}
		if code[idx] >= '0' && code[idx] <= '9' {
			continue
		}
		if code[idx] == ' ' || code[idx] == '(' {
			if idx == start {
				return idx, fmt.Errorf("open paren in func name")
			}
			end = idx
			break
		}
		return idx, fmt.Errorf("unexpected char %q in func name %q", code[idx], code[start:idx])
	}
	if start == end {
		return len(code), fmt.Errorf("eof during func name")
	}
	call.Function = BloksFunctionID(code[start:end])
	call.Args = []BloksScriptNode{}
	next := end
	for {
		arg := BloksScriptNode{}
		var err error
		next, err = arg.ParseAny(code, next)
		if errors.Is(err, ParseEndOfFuncall) {
			for idx := next; idx < len(code); idx++ {
				switch code[idx] {
				case '\t', ' ':
					continue
				case ')':
					return idx + 1, nil
				}
				return idx, fmt.Errorf("eof during close paren")
			}
		}
		if err != nil {
			return next, err
		}
		call.Args = append(call.Args, arg)
	}
}

func (call *BloksScriptFuncall) Print(indent string) error {
	return nil
}

type BloksScriptLiteral struct {
	BloksJavascriptValue
}

func (lit *BloksScriptLiteral) Parse(code string, start int) (int, error) {
	for idx := start; idx < len(code); idx++ {
		switch code[idx] {
		case '\t', ' ':
			continue
		}
		start = idx
		break
	}
	decimal := false
	for idx := start; idx < len(code); idx++ {
		if code[idx] >= '0' && code[idx] <= '9' {
			continue
		}
		if code[idx] == '.' {
			decimal = true
			continue
		}
		if idx == start {
			break
		}
		switch code[idx] {
		case ' ', '(', ')':
			if decimal {
				val, err := strconv.ParseFloat(code[start:idx], 64)
				if err != nil {
					return idx, err
				}
				lit.BloksJavascriptValue = val
			} else {
				val, err := strconv.ParseInt(code[start:idx], 10, 64)
				if err != nil {
					return idx, err
				}
				lit.BloksJavascriptValue = val
			}
			return idx, nil
		}
		return idx, fmt.Errorf("unexpected char %q in numeric literal", code[idx])
	}
	if code[start] == '"' {
		idx := start + 1
		chars := []byte{}
		for idx < len(code) {
			switch code[idx] {
			case '\\':
				if idx+1 >= len(code) {
					return len(code), fmt.Errorf("backslash at eof")
				}
				chars = append(chars, code[idx+1])
				idx += 2
				continue
			case '"':
				lit.BloksJavascriptValue = string(chars)
				return idx + 1, nil
			}
			chars = append(chars, code[idx])
			idx += 1
		}
		return idx, fmt.Errorf("unterminated string literal")
	}
	if start+4 < len(code) && code[start:start+4] == "null" {
		return start + 4, nil
	}
	if start+4 < len(code) && code[start:start+4] == "true" {
		lit.BloksJavascriptValue = true
		return start + 4, nil
	}
	if start+5 < len(code) && code[start:start+5] == "false" {
		lit.BloksJavascriptValue = false
		return start + 5, nil
	}
	return start, fmt.Errorf("unknown char %q", code[start])
}

func (lit *BloksScriptLiteral) Print(indent string) error {
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
	return bundle.Print("")
}
