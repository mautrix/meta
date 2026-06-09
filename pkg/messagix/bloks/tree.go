package bloks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type BloksBundle struct {
	Layout      BloksLayout  `json:"layout"`
	Interpreter *Interpreter `json:"-"`
}

// same as BloksBundle but doesn't have json methods defined, so that we can do
// pre/post-processing and then call back to the normal json methods without
// entering an infinite loop
type fakeBloksBundle struct {
	Layout      BloksLayout  `json:"layout"`
	Interpreter *Interpreter `json:"-"`
}

func (bb *BloksBundle) GetInterpreter() *Interpreter {
	if bb == nil {
		return nil
	}
	return bb.Interpreter
}

func (bb *BloksBundle) Action() *BloksScriptNode {
	return &bb.Layout.Payload.Action.AST
}

func (bb *BloksBundle) SetupInterpreter(ctx context.Context, br *InterpBridge, prev *Interpreter, clearLocals bool) error {
	interp, err := NewInterpreter(ctx, bb, br, prev, clearLocals)
	if err != nil {
		return err
	}
	bb.Interpreter = interp
	return nil
}

func (bb *BloksBundle) UnmarshalJSON(data []byte) error {
	var raw fakeBloksBundle
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}
	*bb = BloksBundle(raw)
	m, err := GetUnminifier(bb)
	if err != nil {
		return err
	}
	bb.Unminify(m)
	return nil
}

func (bb *BloksBundle) MarshalJSON() ([]byte, error) {
	copy := *bb
	pay := &copy.Layout.Payload
	if pay.VariablesOwner != &bb.Layout.Payload {
		pay.Variables = nil
	}
	return json.Marshal((*fakeBloksBundle)(&copy))
}

func (bb *BloksBundle) Unminify(m *Unminifier) {
	p := &bb.Layout.Payload
	p.VariablesOwner = p
	for _, d := range p.Variables {
		if real, ok := m.Variables[d.ID]; ok && len(real) > 0 {
			d.ID = real
		}
		if d.Info.InitialScript != nil {
			d.Info.InitialScript.Unminify(m, nil)
		}
	}
	for _, s := range p.Scripts {
		s.Unminify(m, nil)
	}
	for _, e := range p.Embedded {
		pp := &e.Contents.Layout.Payload
		pp.Variables = p.Variables
		e.Contents.Unminify(m)
	}
	for _, t := range p.Templates {
		t.Unminify(m, nil)
	}
	if p.Action != nil {
		p.Action.Unminify(m, nil)
	}
	if p.Tree != nil {
		p.Tree.Unminify(m, nil)
	}
}

func (bb *BloksBundle) Print(w io.Writer, indent string) error {
	p := &bb.Layout.Payload
	fmt.Fprintf(w, "%s<Bundle>\n", indent)
	if p.VariablesOwner == p {
		for _, datum := range p.Variables {
			fmt.Fprintf(w, "%s  <Datum id=%q>\n", indent, datum.ID)
			if datum.Info.InitialScript != nil {
				datum.Info.InitialScript.Print(w, indent+"    ")
			} else {
				BloksLiteralFromJavaScript(datum.Info.Initial).Print(w, indent+"    ")
			}
			fmt.Fprintf(w, "\n%s  </Datum id=%q>\n", indent, datum.ID)
		}
	}
	scriptIDs := []BloksScriptID{}
	for id := range p.Scripts {
		scriptIDs = append(scriptIDs, id)
	}
	slices.Sort(scriptIDs)
	for _, id := range scriptIDs {
		script := p.Scripts[id]
		fmt.Fprintf(w, "%s  <Script id=%q>\n", indent, id)
		script.Print(w, indent+"    ")
		fmt.Fprintf(w, "\n%s  </Script id=%q>\n", indent, id)
	}
	for _, emb := range p.Embedded {
		fmt.Fprintf(w, "%s  <EmbeddedPayload id=%q>\n", indent, emb.ID)
		emb.Contents.Print(w, indent+"    ")
		fmt.Fprintf(w, "%s  </EmbeddedPayload id=%q>\n", indent, emb.ID)
	}
	templateIDs := []BloksTemplateID{}
	for id := range p.Templates {
		templateIDs = append(templateIDs, id)
	}
	slices.Sort(templateIDs)
	for _, id := range templateIDs {
		template := p.Templates[id]
		fmt.Fprintf(w, "%s  <Template id=%q>\n", indent, id)
		template.Print(w, indent+"    ")
		fmt.Fprintf(w, "%s  </Template id=%q>\n", indent, id)
	}
	if p.Tree != nil {
		fmt.Fprintf(w, "%s  <Tree>\n", indent)
		err := p.Tree.Print(w, indent+"    ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s  </Tree>\n", indent)
	}
	if p.Action != nil {
		fmt.Fprintf(w, "%s  <Action>\n", indent)
		err := p.Action.Print(w, indent+"    ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "\n%s  </Action>\n", indent)
	}
	fmt.Fprintf(w, "%s</Bundle>\n", indent)
	return nil
}

func (bb *BloksBundle) PrintHTML(w io.Writer, indent string) error {
	fmt.Fprintf(w, "%s<!DOCTYPE html>\n", indent)
	fmt.Fprintf(w, "%s<html>\n", indent)
	fmt.Fprintf(w, "%s  <head>\n", indent)
	fmt.Fprintf(w, "%s    <meta charset=\"utf-8\">\n", indent)
	fmt.Fprintf(w, "%s    <title>Bloks Page</title>\n", indent)
	fmt.Fprintf(w, "%s  </head>\n", indent)
	fmt.Fprintf(w, "%s  <body>\n", indent)
	err := bb.Layout.Payload.Tree.PrintHTML(w, indent+"    ")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s  </body>\n", indent)
	fmt.Fprintf(w, "%s</html>\n", indent)
	return nil
}

func (bb *BloksBundle) Redact() {
	p := &bb.Layout.Payload
	if p.VariablesOwner == p {
		for _, datum := range p.Variables {
			redactJavaScriptValue((*any)(&datum.Info.Initial))
			datum.Info.InitialScript.Redact()
		}
	}
	for _, scr := range p.Scripts {
		scr.Redact()
	}
	for _, emb := range p.Embedded {
		emb.Contents.Redact()
	}
	for _, tmp := range p.Templates {
		tmp.Redact()
	}
	p.Tree.Redact()
	p.Action.Redact()
}

type BloksLayout struct {
	Payload         BloksPayload `json:"bloks_payload"`
	PreparsePayload bool         `json:"preparse_payload"`
}

type BloksPayload struct {
	Scripts             map[BloksScriptID]BloksTreeScript `json:"ft"`
	ReferencedVariables []BloksVariableID                 `json:"referenced"`
	ReferencedPayloads  []BloksPayloadID                  `json:"referenced_embedded_payload"`
	Variables           []*BloksVariable                  `json:"data"`
	VariablesOwner      *BloksPayload                     `json:"-"`
	Embedded            []*BloksEmbeddedPayload           `json:"embedded_payloads"`
	Props               []BloksProp                       `json:"props"`
	Templates           map[BloksTemplateID]BloksTreeNode `json:"templates"`
	Attribution         BloksErrorAttribution             `json:"error_attribution"`
	Tree                *BloksTreeNode                    `json:"tree"`
	Action              *BloksTreeScript                  `json:"action"`
}

type BloksVariable struct {
	ID   BloksVariableID `json:"id"`
	Type string          `json:"type"`
	Info BloksDatumInfo  `json:"data"`
}

type BloksDatumInfo struct {
	Name          string               `json:"key"`
	Mode          string               `json:"mode"`
	Initial       BloksJavaScriptValue `json:"initial"`
	InitialScript *BloksTreeScript     `json:"initial_lispy"`
}

type BloksJavaScriptValue any

var likelyNonSensitive = regexp.MustCompile(strings.Join([]string{
	// clearly internal identifiers
	`^com.bloks.`,
	`^(CAA|caa|INTERNAL)_`,
	`^i:(caa|com\.bloks)\.`,
	// floating-point number
	`^[0-9]{1,3}\.[0-9]{1,16}dp$`,
}, `|`))

var likelyNonSensitiveURLPath = regexp.MustCompile(strings.Join([]string{
	// cdn url, not user specific
	`^rsrc.php/`,
}, `|`))

var englishWord = regexp.MustCompile(`([A-Za-z0-9'-]+)[,.:;]? *`)
var digitRegexp = regexp.MustCompile(`[0-9]`)
var lowercaseLetterRegexp = regexp.MustCompile(`[a-z]`)
var uppercaseLetterRegexp = regexp.MustCompile(`[A-Z]`)
var alnumsRegexp = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func likelyEnglishSentence(s string) bool {
	words := englishWord.FindAllStringSubmatch(s, -1)
	numReasonableChars := 0
	for _, wordMatch := range words {
		wordAndExtra := wordMatch[0]
		word := wordMatch[1]
		if len(word) > 24 {
			continue
		}
		if strings.Count(word, "-") > 1 {
			continue
		}
		if strings.Count(word, "'") > 1 {
			continue
		}
		if len(digitRegexp.FindAllString(word, -1)) > 6 {
			continue
		}
		if len(lowercaseLetterRegexp.FindAllString(word, -1)) > 2 && len(uppercaseLetterRegexp.FindAllString(word, -1)) > 2 {
			continue
		}
		numReasonableChars += len(wordAndExtra)
	}
	return float64(numReasonableChars)/float64(len(s)) > 0.8 || len(s)-numReasonableChars < 10
}

func likelyAttributeName(s string) bool {
	for _, delimiter := range []string{"-", "_"} {
		bad := false
		for part := range strings.SplitSeq(s, delimiter) {
			if len(part) > 24 {
				bad = true
			}
			if !alnumsRegexp.MatchString(part) {
				bad = true
			}
			if len(digitRegexp.FindAllString(part, -1)) > 2 {
				bad = true
			}
			if len(lowercaseLetterRegexp.FindAllString(part, -1)) > 2 && len(uppercaseLetterRegexp.FindAllString(part, -1)) > 2 {
				bad = true
			}
		}
		if !bad {
			return true
		}
	}
	return false
}

var stringTypes = map[string]*regexp.Regexp{
	"uuid_lowercase": regexp.MustCompile(`^[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}`),
	"uuid_uppercase": regexp.MustCompile(`^[0-9A-Z]{8}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{12}`),
	"hex":            regexp.MustCompile(`^[0-9a-f]*[a-f][0-9a-f]*$`),
	"number":         regexp.MustCompile(`^[0-9]+$`),
}

var likelyURL = regexp.MustCompile(`^([a-z]{2,10}://[a-z0-9.-]+/)(.+)$`)

func redactString(s string) string {
	if strings.Contains(s, "redacted_") {
		return s
	}
	var obj any
	err := json.Unmarshal([]byte(s), &obj)
	if err == nil {
		redactJavaScriptValue(&obj)
		out, err := json.Marshal(obj)
		if err == nil {
			return string(out)
		}
	}
	if len(s) < 16 {
		return s
	}
	if likelyNonSensitive.MatchString(s) {
		return s
	}
	if likelyAttributeName(s) {
		return s
	}
	if likelyEnglishSentence(s) {
		return s
	}
	prefix := ""
	if match := likelyURL.FindStringSubmatch(s); match != nil {
		prefix = match[1]
		s = match[2]
	}
	if likelyNonSensitiveURLPath.MatchString(s) {
		return prefix + s
	}
	stringType := "str"
	for guess, regex := range stringTypes {
		if regex.MatchString(s) {
			stringType = guess
			break
		}
	}
	h := sha256.New()
	h.Write([]byte(s))
	digest := h.Sum(nil)
	return fmt.Sprintf("%sredacted_%dchar_%s_%s", prefix, len(s), stringType, hex.EncodeToString(digest[:4]))
}

// Integers can only have 10 significant figures before it might start
// being the case that they are communicating sensitive data. Allowing
// for 10 to accommodate seconds-since-epoch timestamps.
const maxRedactedSigFigs = 10

func redactInteger(num int64) int64 {
	mag := math.Log10(math.Abs(float64(num)))
	if mag <= maxRedactedSigFigs {
		return num
	}
	factor := int64(math.Pow10(int(math.Ceil(mag) - maxRedactedSigFigs)))
	num /= factor
	num *= factor
	return num
}

func redactFloat(num float64) float64 {
	mag := math.Log10(math.Abs(num))
	if mag <= maxRedactedSigFigs {
		return num
	}
	factor := math.Pow10(int(math.Ceil(mag) - maxRedactedSigFigs))
	num /= factor
	num = math.Round(num)
	num *= factor
	return num
}

func redactJavaScriptValue(ptr *any) {
	switch val := (*ptr).(type) {
	case string:
		*ptr = redactString(val)
	case int64:
		*ptr = redactInteger(val)
	case float64:
		*ptr = redactFloat(val)
	case []any:
		for idx := range val {
			redactJavaScriptValue(&val[idx])
		}
	case map[string]any:
		for key := range val {
			obj := val[key]
			redactJavaScriptValue(&obj)
			val[key] = obj
		}
	}
}

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
	if str, ok := literal.BloksJavaScriptValue.(string); ok && (strings.HasPrefix(str, "\t") || strings.HasPrefix(str, "(")) {
		script := BloksTreeScript{}
		err := script.Parse(str)
		if err != nil {
			return fmt.Errorf("script: %w", err)
		}
		btn.BloksTreeNodeContent = &script
		return nil
	}
	if arr, ok := literal.BloksJavaScriptValue.([]any); ok {
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

func (btn BloksTreeNode) MarshalJSON() ([]byte, error) {
	switch node := btn.BloksTreeNodeContent.(type) {
	case *BloksTreeComponent:
		return json.Marshal(map[BloksComponentID]any{
			node.ComponentID: node.Attributes,
		})
	case *BloksTreeComponentList:
		comps := []any{}
		for _, comp := range *node {
			comps = append(comps, map[BloksComponentID]any{
				comp.ComponentID: comp.Attributes,
			})
		}
		return json.Marshal(comps)
	case *BloksTreeLiteral:
		return json.Marshal(node.BloksJavaScriptValue)
	case *BloksTreeScript:
		return json.Marshal(node)
	case *BloksTreeScriptSet:
		data := []any{}
		for prop, script := range node.Scripts {
			data = append(data, prop, script)
		}
		return json.Marshal(data)
	default:
		return nil, fmt.Errorf("unexpected node type during marshal %T", node)
	}
}

func (btn *BloksTreeNode) Redact() {
	if btn == nil {
		return
	}
	btn.BloksTreeNodeContent.Redact()
}

type BloksTreeNodeContent interface {
	Unminify(m *Unminifier, parent *BloksTreeComponent)
	Print(w io.Writer, prefix string) error
	PrintHTML(w io.Writer, prefix string) error
	Redact()
}

type BloksTreeComponent struct {
	ComponentID BloksComponentID
	Attributes  map[BloksAttributeID]*BloksTreeNode

	parent      *BloksTreeComponent
	textContent *string
}

func (btc *BloksTreeComponent) SetTextContent(text string) error {
	if btc.ComponentID != "bk.components.TextInput" {
		return fmt.Errorf("can't set text content of %s", btc.ComponentID)
	}
	btc.textContent = &text
	return nil
}

// This could just unmarshal the whole map directly, but I wrote it
// explicitly to add better error messaging.
func (btc *BloksTreeComponent) UnmarshalJSON(data []byte) error {
	var rawAttrs = map[BloksAttributeID]json.RawMessage{}
	err := json.Unmarshal(data, &rawAttrs)
	if err != nil {
		return err
	}
	btc.Attributes = map[BloksAttributeID]*BloksTreeNode{}
	for attr, subdata := range rawAttrs {
		var node BloksTreeNode
		err := json.Unmarshal(subdata, &node)
		if err != nil {
			return fmt.Errorf("attribute %q: %w", attr, err)
		}
		btc.Attributes[attr] = &node
	}
	return nil
}

func (btc *BloksTreeComponent) Unminify(m *Unminifier, parent *BloksTreeComponent) {
	btc.parent = parent
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
		value.Unminify(m, btc)
	}
}

func (btc *BloksTreeComponent) Print(w io.Writer, indent string) error {
	fmt.Fprintf(w, "%s<Component name=%s>\n", indent, btc.ComponentID)
	attrs := []BloksAttributeID{}
	for attr := range btc.Attributes {
		attrs = append(attrs, attr)
	}
	slices.Sort(attrs)
	for _, id := range attrs {
		value := btc.Attributes[id]
		attrtype := ""
		trailer := ""
		extra := ""
		switch content := value.BloksTreeNodeContent.(type) {
		case *BloksTreeComponent:
			attrtype = "component"
		case *BloksTreeComponentList:
			attrtype = "component-list"
			extra = fmt.Sprintf(" length=%d", len(*content))
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
		fmt.Fprintf(w, "%s  <Property %s type=%s%s>\n", indent, id.ToTag(), attrtype, extra)
		err := value.BloksTreeNodeContent.Print(w, indent+"    ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s%s  </Property %s type=%s%s>\n", trailer, indent, id.ToTag(), attrtype, extra)
	}
	fmt.Fprintf(w, "%s</Component name=%s>\n", indent, btc.ComponentID)
	return nil
}

func cssToString(css map[string]string) string {
	attrs := []string{}
	for attr := range css {
		attrs = append(attrs, attr)
	}
	slices.Sort(attrs)
	parts := []string{}
	for _, attr := range attrs {
		if len(attr) <= 1 {
			continue
		}
		if css[attr] == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", attr, css[attr]))
	}
	return strings.Join(parts, "; ")
}

func (btc *BloksTreeComponent) getCSS() map[string]string {
	css := map[string]string{}
	if style, ok := btc.Attributes["_style"]; ok {
		style := style.BloksTreeNodeContent.(*BloksTreeComponent)
		if style.ComponentID == "flex" {
			for prop := range style.Attributes {
				css[strings.ReplaceAll(string(prop), "_", "-")] = style.GetAttribute(prop)
			}
		}
	}
	return css
}

func (btc *BloksTreeComponent) PrintHTML(w io.Writer, indent string) error {
	switch btc.ComponentID {
	case "bk.cds.bottomsheet.Wrapper":
		fmt.Fprintf(w, "%s<div class=\"%s\">\n", indent, btc.ComponentID)
		fmt.Fprintf(w, "%s  <div class=\"header\">\n", indent)
		err := btc.Attributes["header"].PrintHTML(w, indent+"    ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s  </div>\n", indent)
		fmt.Fprintf(w, "%s  <div class=\"content\">\n", indent)
		err = btc.Attributes["content"].PrintHTML(w, indent+"    ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s  </div>\n", indent)
		fmt.Fprintf(w, "%s</div>\n", indent)
	case "bk.components.Flexbox":
		css := btc.getCSS()
		css["align-items"] = btc.GetAttribute("align_items")
		css["flex-direction"] = btc.GetAttribute("flex_direction")
		css["justify-content"] = btc.GetAttribute("justify_content")
		fmt.Fprintf(w, "%s<div class=\"%s\" style=\"%s\">\n", indent, btc.ComponentID, cssToString(css))
		err := btc.Attributes["children"].PrintHTML(w, indent+"  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s</div>\n", indent)
	case "bk.components.Collection":
		css := btc.getCSS()
		fmt.Fprintf(w, "%s<div class=\"%s\" style=\"%s\">\n", indent, btc.ComponentID, cssToString(css))
		err := btc.Attributes["children"].PrintHTML(w, indent+"  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s</div>\n", indent)
	case "bk.components.RichText", "bk.data.ComposableTextSpan":
		fmt.Fprintf(w, "%s<div class=\"%s\">\n", indent, btc.ComponentID)
		err := btc.Attributes["spans"].PrintHTML(w, indent+"  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s</div>\n", indent)
	case "bk.data.TextSpan":
		fmt.Fprintf(w, "%s<span>%s</span>\n", indent, btc.GetAttribute("text"))
	case "bk.components.TextInput":
		fmt.Fprintf(w, "%s<input type=\"%s\" placeholder=\"%s\">\n", indent, btc.GetAttribute("type"), btc.GetAttribute("placeholder"))
	case "bk.components.Image":
		fmt.Fprintf(w, "%s<img id=\"%s\" src=\"%s\">\n", indent, btc.GetAttribute("unique_id"), btc.GetAttribute("url"))
	default:
		attrs := []BloksAttributeID{}
		for attr := range btc.Attributes {
			attrs = append(attrs, attr)
		}
		fmt.Fprintf(w, "%s<!-- component %s omitted (props %+v) -->\n", indent, btc.ComponentID, attrs)
	}
	return nil
}

func (btc *BloksTreeComponent) Redact() {
	for _, child := range btc.Attributes {
		child.Redact()
	}
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

func (btcl *BloksTreeComponentList) Unminify(m *Unminifier, parent *BloksTreeComponent) {
	for _, value := range *btcl {
		value.Unminify(m, parent)
	}
}

func (btcl BloksTreeComponentList) Print(w io.Writer, indent string) error {
	for _, comp := range btcl {
		err := comp.Print(w, indent)
		if err != nil {
			return err
		}
	}
	return nil
}

func (btcl *BloksTreeComponentList) PrintHTML(w io.Writer, indent string) error {
	for _, comp := range *btcl {
		err := comp.PrintHTML(w, indent)
		if err != nil {
			return err
		}
	}
	return nil
}

func (btcl *BloksTreeComponentList) Redact() {
	for _, comp := range *btcl {
		comp.Redact()
	}
}

type BloksTreeLiteral struct {
	BloksJavaScriptValue
}

func (btl *BloksTreeLiteral) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &btl.BloksJavaScriptValue)
}

func (btl *BloksTreeLiteral) Unminify(m *Unminifier, parent *BloksTreeComponent) {
	//
}

func (btl *BloksTreeLiteral) Print(w io.Writer, indent string) error {
	str, err := json.Marshal(btl.BloksJavaScriptValue)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s%s\n", indent, str)
	return nil
}

func (btl *BloksTreeLiteral) PrintHTML(w io.Writer, indent string) error {
	fmt.Fprintf(w, "%s<!-- literal omitted -->\n", indent)
	return nil
}

func (btl *BloksTreeLiteral) Redact() {
	redactJavaScriptValue((*any)(&btl.BloksJavaScriptValue))
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

var indentRegexp = regexp.MustCompile(`\n?BLOKS_INDENT *`)

func (bs BloksTreeScript) MarshalJSON() ([]byte, error) {
	out := bytes.Buffer{}
	err := bs.Print(&out, "BLOKS_INDENT")
	if err != nil {
		return nil, err
	}
	return json.Marshal("\t" + indentRegexp.ReplaceAllString(out.String(), " "))
}

func (bs *BloksTreeScript) Unminify(m *Unminifier, parent *BloksTreeComponent) {
	bs.AST.Content.Unminify(m)
}

func (bst *BloksTreeScript) Parse(code string) error {
	_, err := bst.AST.ParseAny(code, 0)
	return err
}

func (bst *BloksTreeScript) Print(w io.Writer, indent string) error {
	return bst.AST.Content.Print(w, indent)
}

func (bst *BloksTreeScript) PrintHTML(w io.Writer, indent string) error {
	fmt.Fprintf(w, "%s<!-- script omitted -->\n", indent)
	return nil
}

func (bst *BloksTreeScript) Redact() {
	if bst == nil {
		return
	}
	bst.AST.Content.Redact()
}

type BloksTreeScriptSet struct {
	Scripts map[BloksAttributeID]BloksTreeScript
}

func (bst *BloksTreeScriptSet) Unminify(m *Unminifier, parent *BloksTreeComponent) {
	for id, script := range bst.Scripts {
		if idx, ok := id.ToInt(); ok {
			attr := BloksAttributeID(strconv.Itoa(idx))
			if real, ok := m.Properties[parent.ComponentID][attr]; ok && len(real) > 0 {
				bst.Scripts[real] = script
				delete(bst.Scripts, id)
			}
		}
	}
	for _, script := range bst.Scripts {
		script.Unminify(m, parent)
	}
}

func (bst *BloksTreeScriptSet) Print(w io.Writer, indent string) error {
	ids := []BloksAttributeID{}
	for id := range bst.Scripts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		script := bst.Scripts[id]
		fmt.Fprintf(w, "%s<Script %s>\n", indent, id.ToTag())
		err := script.Print(w, indent+"  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "\n%s</Script %s>\n", indent, id.ToTag())
	}
	return nil
}

func (bst *BloksTreeScriptSet) PrintHTML(w io.Writer, indent string) error {
	fmt.Fprintf(w, "%s<!-- script set omitted -->\n", indent)
	return nil
}

func (bst *BloksTreeScriptSet) Redact() {
	for key := range bst.Scripts {
		scr := bst.Scripts[key]
		scr.Redact()
		bst.Scripts[key] = scr
	}
}
