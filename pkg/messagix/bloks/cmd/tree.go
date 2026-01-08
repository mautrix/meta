package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type BloksBundle struct {
	Layout BloksLayout `json:"layout"`
}

func (bb *BloksBundle) Unminify(m *Unminifier) {
	bb.Layout.Payload.Tree.Unminify(m)
}

func (bb *BloksBundle) Print(indent string) error {
	p := bb.Layout.Payload
	fmt.Printf("%s<Bundle>\n", indent)
	scriptIDs := []BloksScriptID{}
	for id := range p.Scripts {
		scriptIDs = append(scriptIDs, id)
	}
	sort.Slice(scriptIDs, func(i, j int) bool { return scriptIDs[i] < scriptIDs[j] })
	for _, id := range scriptIDs {
		script := p.Scripts[id]
		fmt.Printf("%s  <Script id=%q>\n", indent, id)
		script.Print(indent + "    ")
		fmt.Printf("\n%s  </Script id=%q>\n", indent, id)
	}
	templateIDs := []BloksTemplateID{}
	for id := range p.Templates {
		templateIDs = append(templateIDs, id)
	}
	sort.Slice(templateIDs, func(i, j int) bool { return templateIDs[i] < templateIDs[j] })
	for _, id := range templateIDs {
		template := p.Templates[id]
		fmt.Printf("%s  <Template id=%q>\n", indent, id)
		template.Print(indent + "    ")
		fmt.Printf("%s  </Template id=%q>\n", indent, id)
	}
	fmt.Printf("%s  <Tree>\n", indent)
	err := bb.Layout.Payload.Tree.Print(indent + "    ")
	if err != nil {
		return err
	}
	fmt.Printf("%s  </Tree>\n", indent)
	fmt.Printf("%s</Bundle>\n", indent)
	return nil
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
	if str, ok := literal.BloksJavascriptValue.(string); ok && (strings.HasPrefix(str, "\t") || strings.HasPrefix(str, "(")) {
		script := BloksTreeScript{}
		err := script.Parse(str)
		if err != nil {
			return fmt.Errorf("script: %w", err)
		}
		btn.BloksTreeNodeContent = &script
		return nil
	}
	if arr, ok := literal.BloksJavascriptValue.([]any); ok {
		set := BloksTreeScriptSet{
			Scripts: map[BloksAttributeID]BloksTreeScript{},
		}
		good := len(arr) > 0 && len(arr)%2 == 0
		if good {
			for idx := 0; idx < len(arr); idx += 2 {
				key, ok := arr[idx].(string)
				if !ok {
					good = false
					break
				}
				str, ok := arr[idx+1].(string)
				if !ok {
					good = false
					break
				}
				if !strings.HasPrefix(str, "\t") {
					good = false
					break
				}
				script := BloksTreeScript{}
				err := script.Parse(str)
				if err != nil {
					return fmt.Errorf("script: %w", err)
				}
				set.Scripts[BloksAttributeID(key)] = script
			}
		}
		if len(arr) > 0 && good {
			btn.BloksTreeNodeContent = &set
			return nil
		}
	}
	btn.BloksTreeNodeContent = &literal
	return nil
}

type BloksTreeNodeContent interface {
	Unminify(m *Unminifier)
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

func (btc *BloksTreeComponent) Unminify(m *Unminifier) {
	if real, ok := m.Components[btc.ComponentID]; ok && len(real) > 0 {
		btc.ComponentID = real
	}
	for id, value := range btc.Attributes {
		if idx, ok := id.ToInt(); ok {
			attr := BloksAttributeID(strconv.Itoa(idx))
			if real, ok := m.Properties[btc.ComponentID][attr]; ok && len(real) > 0 {
				btc.Attributes[real] = value
				delete(btc.Attributes, id)
			}
		}
	}
	for _, value := range btc.Attributes {
		value.Unminify(m)
	}
}

func (btc *BloksTreeComponent) Print(indent string) error {
	fmt.Printf("%s<Component name=%s>\n", indent, btc.ComponentID)
	attrs := []BloksAttributeID{}
	for attr := range btc.Attributes {
		attrs = append(attrs, attr)
	}
	sort.Slice(attrs, func(i, j int) bool { return attrs[i] < attrs[j] })
	for _, id := range attrs {
		value := btc.Attributes[id]
		attrtype := ""
		trailer := ""
		switch value.BloksTreeNodeContent.(type) {
		case *BloksTreeComponent:
			attrtype = "component"
		case *BloksTreeComponentList:
			attrtype = "component-list"
		case *BloksTreeLiteral:
			attrtype = "literal"
		case *BloksTreeScript:
			attrtype = "script"
			trailer = "\n"
		case *BloksTreeScriptSet:
			attrtype = "script-set"
		default:
			panic("missing case in bloks tree switch")
		}
		fmt.Printf("%s  <Property %s type=%s>\n", indent, id.ToTag(), attrtype)
		err := value.BloksTreeNodeContent.Print(indent + "    ")
		if err != nil {
			return err
		}
		fmt.Printf("%s%s  </Property %s type=%s>\n", trailer, indent, id.ToTag(), attrtype)
	}
	fmt.Printf("%s</Component name=%s>\n", indent, btc.ComponentID)
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

func (btcl *BloksTreeComponentList) Unminify(m *Unminifier) {
	for _, value := range *btcl {
		value.Unminify(m)
	}
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

func (btl *BloksTreeLiteral) Unminify(m *Unminifier) {
	//
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

func (bs *BloksTreeScript) UnmarshalJSON(data []byte) error {
	var code string
	err := json.Unmarshal(data, &code)
	if err != nil {
		return err
	}
	err = bs.Parse(code)
	if err != nil {
		return fmt.Errorf("script: %w", err)
	}
	return nil
}

func (bs *BloksTreeScript) Unminify(m *Unminifier) {
	bs.AST.Unminify(m)
}

func (bst *BloksTreeScript) Parse(code string) error {
	_, err := bst.AST.ParseAny(code, 0)
	return err
}

func (bst *BloksTreeScript) Print(indent string) error {
	return bst.AST.Print(indent)
}

type BloksTreeScriptSet struct {
	Scripts map[BloksAttributeID]BloksTreeScript
}

func (bst *BloksTreeScriptSet) Unminify(m *Unminifier) {
	for _, script := range bst.Scripts {
		script.Unminify(m)
	}
}

func (bst *BloksTreeScriptSet) Print(indent string) error {
	ids := []BloksAttributeID{}
	for id := range bst.Scripts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		script := bst.Scripts[id]
		fmt.Printf("%s<Script %s>\n", indent, id.ToTag())
		err := script.Print(indent + "  ")
		if err != nil {
			return err
		}
		fmt.Printf("\n%s</Script %s>\n", indent, id.ToTag())
	}
	return nil
}
