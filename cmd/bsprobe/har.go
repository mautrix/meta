package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The HAR analyzer is the one part of bsprobe that needs no live session: it
// runs offline against a DevTools recording. It answers the decisive question
// — does Business Suite speak Lightspeed (reuse pkg/messagix) or something
// else (build a separate transport) — and emits the operation registry that
// every later command reads.

type harFile struct {
	Log struct {
		Entries []harEntry `json:"entries"`
	} `json:"log"`
}

type harNV struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harEntry struct {
	StartedDateTime string `json:"startedDateTime"`
	Request         struct {
		Method   string  `json:"method"`
		URL      string  `json:"url"`
		Headers  []harNV `json:"headers"`
		PostData struct {
			MimeType string  `json:"mimeType"`
			Text     string  `json:"text"`
			Params   []harNV `json:"params"`
		} `json:"postData"`
	} `json:"request"`
	Response struct {
		Status  int `json:"status"`
		Content struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"content"`
	} `json:"response"`
	WebSocketMessages []harWSMessage `json:"_webSocketMessages"`
}

type harWSMessage struct {
	Type   string  `json:"type"` // "send" | "receive"
	Time   float64 `json:"time"`
	Opcode int     `json:"opcode"` // 1 = text, 2 = binary
	Data   string  `json:"data"`
}

// Operation is one persisted GraphQL call observed in the capture. This is the
// registry entry format the spec calls for — never hard-coded, always sourced
// from a real recording.
type Operation struct {
	FriendlyName string   `json:"friendly_name"`
	DocID        string   `json:"doc_id"`
	Method       string   `json:"method"`
	Endpoint     string   `json:"endpoint"`
	VariableKeys []string `json:"variable_keys"`
	Seen         int      `json:"seen"`
	ResponseFile string   `json:"response_shape_file,omitempty"`
}

type realtimeFinding struct {
	URL          string
	TextFrames   int
	BinaryFrames int
	Samples      []string
}

type harReport struct {
	Operations   map[string]*Operation
	GraphQLHosts map[string]int
	Realtime     []*realtimeFinding
	Identifiers  map[string]map[string]bool
	Verdict      string
	VerdictWhy   []string
}

func cmdHAR(args []string) error {
	fs := flag.NewFlagSet("har", flag.ExitOnError)
	out := fs.String("out", "fixtures", "directory to write the registry and response shapes into")
	shapes := fs.Bool("shapes", true, "write a redacted response shape file per operation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: bsprobe har [--out dir] <capture.har>")
	}

	raw, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var h harFile
	if err := json.Unmarshal(raw, &h); err != nil {
		return fmt.Errorf("parse har: %w", err)
	}

	rep := analyzeHAR(&h)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if *shapes {
		if err := writeShapes(&h, rep, *out); err != nil {
			return err
		}
	}
	if err := writeRegistry(rep, *out); err != nil {
		return err
	}

	fmt.Print(renderReport(rep, fs.Arg(0)))
	return nil
}

func analyzeHAR(h *harFile) *harReport {
	rep := &harReport{
		Operations:   map[string]*Operation{},
		GraphQLHosts: map[string]int{},
		Identifiers:  map[string]map[string]bool{},
	}

	for i := range h.Log.Entries {
		e := &h.Log.Entries[i]
		if isGraphQL(e.Request.URL) {
			collectGraphQL(rep, e)
		}
		if len(e.WebSocketMessages) > 0 || isRealtimeURL(e.Request.URL) {
			collectRealtime(rep, e)
		}
		// Identifiers can appear in request bodies and responses alike.
		harvestIdentifiers(rep, e.Request.PostData.Text)
		if len(e.Response.Content.Text) < 2_000_000 {
			harvestIdentifiers(rep, e.Response.Content.Text)
		}
	}

	decideVerdict(rep)
	return rep
}

func isGraphQL(raw string) bool {
	return strings.Contains(raw, "/api/graphql") || strings.HasSuffix(rawPath(raw), "/graphql/") ||
		strings.HasSuffix(rawPath(raw), "/graphql")
}

func isRealtimeURL(raw string) bool {
	l := strings.ToLower(raw)
	return strings.HasPrefix(l, "ws://") || strings.HasPrefix(l, "wss://") ||
		strings.Contains(l, "edge-chat") || strings.Contains(l, "mqtt") ||
		strings.Contains(l, "/ws/realtime")
}

func rawPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func collectGraphQL(rep *harReport, e *harEntry) {
	if u, err := url.Parse(e.Request.URL); err == nil {
		rep.GraphQLHosts[u.Host+u.Path]++
	}

	form := parseForm(e)
	if form == nil {
		return
	}
	name := form.Get("fb_api_req_friendly_name")
	docID := form.Get("doc_id")
	if name == "" && docID == "" {
		return
	}
	if name == "" {
		name = "unnamed_doc_" + docID
	}

	op, ok := rep.Operations[name]
	if !ok {
		u, _ := url.Parse(e.Request.URL)
		endpoint := e.Request.URL
		if u != nil {
			endpoint = u.Scheme + "://" + u.Host + u.Path
		}
		op = &Operation{
			FriendlyName: name,
			DocID:        docID,
			Method:       e.Request.Method,
			Endpoint:     endpoint,
		}
		rep.Operations[name] = op
	}
	op.Seen++
	if op.DocID == "" {
		op.DocID = docID
	}

	// Variable *keys* are protocol facts worth keeping; the values are customer
	// data and are dropped here.
	if vars := form.Get("variables"); vars != "" {
		var decoded any
		if err := json.Unmarshal([]byte(vars), &decoded); err == nil {
			op.VariableKeys = mergeKeys(op.VariableKeys, topLevelKeys(decoded))
			harvestIdentifiersJSON(rep, decoded)
		}
	}
}

func parseForm(e *harEntry) url.Values {
	pd := e.Request.PostData
	if len(pd.Params) > 0 {
		v := url.Values{}
		for _, p := range pd.Params {
			// DevTools percent-encodes param values in the params array.
			if dec, err := url.QueryUnescape(p.Value); err == nil {
				v.Set(p.Name, dec)
			} else {
				v.Set(p.Name, p.Value)
			}
		}
		return v
	}
	if pd.Text == "" {
		return nil
	}
	if strings.Contains(pd.MimeType, "json") {
		// Some surfaces post JSON rather than form-encoded bodies.
		var m map[string]any
		if err := json.Unmarshal([]byte(pd.Text), &m); err != nil {
			return nil
		}
		v := url.Values{}
		for k, val := range m {
			switch t := val.(type) {
			case string:
				v.Set(k, t)
			default:
				b, _ := json.Marshal(t)
				v.Set(k, string(b))
			}
		}
		return v
	}
	v, err := url.ParseQuery(pd.Text)
	if err != nil {
		return nil
	}
	return v
}

func collectRealtime(rep *harReport, e *harEntry) {
	var f *realtimeFinding
	for _, existing := range rep.Realtime {
		if existing.URL == e.Request.URL {
			f = existing
			break
		}
	}
	if f == nil {
		f = &realtimeFinding{URL: e.Request.URL}
		rep.Realtime = append(rep.Realtime, f)
	}
	for _, m := range e.WebSocketMessages {
		if m.Opcode == 2 {
			f.BinaryFrames++
		} else {
			f.TextFrames++
		}
		if len(f.Samples) < 12 {
			f.Samples = append(f.Samples, decodeFrame(m))
		}
	}
}

func decodeFrame(m harWSMessage) string {
	data := m.Data
	if m.Opcode == 2 {
		if b, err := base64.StdEncoding.DecodeString(data); err == nil {
			data = string(b)
		}
	}
	if len(data) > 600 {
		data = data[:600]
	}
	return RedactString(data)
}

// lightspeedMarkers are the field names messagix's socket decoder already
// expects. Finding them in a Business Suite frame is the Case A signal.
var lightspeedMarkers = []string{
	`"request_id"`, `"payload"`, `"sp"`, `"target"`,
	"ls_req", "lightspeed", "LSPlatform", "deltaNewMessage",
}

func decideVerdict(rep *harReport) {
	var textFrames, binaryFrames int
	markerHits := map[string]int{}
	for _, f := range rep.Realtime {
		textFrames += f.TextFrames
		binaryFrames += f.BinaryFrames
		for _, s := range f.Samples {
			for _, marker := range lightspeedMarkers {
				if strings.Contains(s, marker) {
					markerHits[marker]++
				}
			}
		}
	}

	switch {
	case len(rep.Realtime) == 0:
		rep.Verdict = "INCONCLUSIVE - no realtime traffic in capture"
		rep.VerdictWhy = append(rep.VerdictWhy,
			"No WebSocket entries found. Chrome only records frames if the WS connection is opened while DevTools is already recording.",
			"Re-record: open DevTools first, then load the inbox, and keep 'Preserve log' on.")
	case len(markerHits) >= 2:
		rep.Verdict = "CASE A - Lightspeed markers present, reuse pkg/messagix"
		for m, n := range markerHits {
			rep.VerdictWhy = append(rep.VerdictWhy, fmt.Sprintf("marker %s seen %d times in realtime frames", m, n))
		}
		rep.VerdictWhy = append(rep.VerdictWhy,
			"Next: confirm pkg/messagix/lightspeed decodes a captured payload before committing to this path.")
	case binaryFrames > 0 && textFrames == 0:
		rep.Verdict = "CASE A LIKELY - binary realtime frames (MQTT-shaped), same transport family as messagix"
		rep.VerdictWhy = append(rep.VerdictWhy,
			fmt.Sprintf("%d binary frames, 0 text frames", binaryFrames),
			"Binary frames are consistent with the MQTT transport pkg/messagix/socket already speaks.",
			"HAR cannot decode these. Confirm with: bsprobe watch-events --dump-frames")
	case textFrames > 0:
		rep.Verdict = "CASE B LIKELY - text realtime frames without Lightspeed markers"
		rep.VerdictWhy = append(rep.VerdictWhy,
			fmt.Sprintf("%d text frames, no Lightspeed field names matched", textFrames),
			"Plan for a separate decoder under pkg/bizsuite/realtime/ rather than bending messagix around it.")
	default:
		rep.Verdict = "INCONCLUSIVE"
		rep.VerdictWhy = append(rep.VerdictWhy, "Realtime endpoints seen but no frames captured.")
	}
}

// identifierKeys are the routing IDs the connector will need to address a
// Page mailbox. Collecting the observed values proves which ones actually vary
// per Page versus per business.
var identifierKeys = []string{
	"business_id", "asset_id", "mailbox_id", "page_id", "thread_id",
	"selected_item_id", "folder", "sync_group", "database_id", "i_user", "av",
}

func harvestIdentifiers(rep *harReport, body string) {
	if body == "" {
		return
	}
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err == nil {
		harvestIdentifiersJSON(rep, decoded)
		return
	}
	if v, err := url.ParseQuery(body); err == nil {
		for _, k := range identifierKeys {
			if got := v.Get(k); got != "" {
				addIdentifier(rep, k, got)
			}
		}
	}
}

func harvestIdentifiersJSON(rep *harReport, v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			for _, want := range identifierKeys {
				if strings.EqualFold(k, want) {
					switch s := val.(type) {
					case string:
						addIdentifier(rep, want, s)
					case float64:
						addIdentifier(rep, want, fmt.Sprintf("%.0f", s))
					}
				}
			}
			harvestIdentifiersJSON(rep, val)
		}
	case []any:
		for _, val := range t {
			harvestIdentifiersJSON(rep, val)
		}
	}
}

func addIdentifier(rep *harReport, key, val string) {
	if val == "" || val == "0" || len(val) > 64 {
		return
	}
	if rep.Identifiers[key] == nil {
		rep.Identifiers[key] = map[string]bool{}
	}
	rep.Identifiers[key][val] = true
}

func topLevelKeys(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func mergeKeys(a, b []string) []string {
	seen := map[string]bool{}
	for _, k := range a {
		seen[k] = true
	}
	for _, k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func writeShapes(h *harFile, rep *harReport, dir string) error {
	shapeDir := filepath.Join(dir, "shapes")
	if err := os.MkdirAll(shapeDir, 0o755); err != nil {
		return err
	}
	written := map[string]bool{}
	for i := range h.Log.Entries {
		e := &h.Log.Entries[i]
		if !isGraphQL(e.Request.URL) {
			continue
		}
		form := parseForm(e)
		if form == nil {
			continue
		}
		name := form.Get("fb_api_req_friendly_name")
		if name == "" || written[name] {
			continue
		}
		body := strings.TrimPrefix(e.Response.Content.Text, "for (;;);")
		var decoded any
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			continue
		}
		file := filepath.Join(shapeDir, sanitize(name)+".json")
		payload := map[string]any{
			"friendly_name": name,
			"doc_id":        form.Get("doc_id"),
			"shape":         Shape(decoded),
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			continue
		}
		if err := os.WriteFile(file, b, 0o644); err != nil {
			return err
		}
		written[name] = true
		if op := rep.Operations[name]; op != nil {
			op.ResponseFile = filepath.ToSlash(filepath.Join("shapes", sanitize(name)+".json"))
		}
	}
	return nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, s)
}

func writeRegistry(rep *harReport, dir string) error {
	ops := make([]*Operation, 0, len(rep.Operations))
	for _, op := range rep.Operations {
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].FriendlyName < ops[j].FriendlyName })
	b, err := json.MarshalIndent(map[string]any{
		"note":       "Generated by bsprobe har. Meta rotates doc_id values; re-capture when calls start failing.",
		"operations": ops,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "operations.json"), b, 0o644)
}

func renderReport(rep *harReport, src string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nbsprobe har - %s\n\n", src)

	fmt.Fprintf(&b, "VERDICT: %s\n", rep.Verdict)
	for _, w := range rep.VerdictWhy {
		fmt.Fprintf(&b, "  - %s\n", w)
	}

	fmt.Fprintf(&b, "\nGraphQL endpoints (%d)\n", len(rep.GraphQLHosts))
	hosts := make([]string, 0, len(rep.GraphQLHosts))
	for h := range rep.GraphQLHosts {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		fmt.Fprintf(&b, "  %-60s %d calls\n", h, rep.GraphQLHosts[h])
	}

	fmt.Fprintf(&b, "\nOperations (%d)\n", len(rep.Operations))
	ops := make([]*Operation, 0, len(rep.Operations))
	for _, op := range rep.Operations {
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Seen > ops[j].Seen })
	for _, op := range ops {
		fmt.Fprintf(&b, "  %-45s doc_id=%-18s x%d\n", op.FriendlyName, op.DocID, op.Seen)
		if len(op.VariableKeys) > 0 {
			fmt.Fprintf(&b, "      vars: %s\n", strings.Join(op.VariableKeys, ", "))
		}
	}

	fmt.Fprintf(&b, "\nRealtime endpoints (%d)\n", len(rep.Realtime))
	for _, f := range rep.Realtime {
		fmt.Fprintf(&b, "  %s\n      text=%d binary=%d\n", f.URL, f.TextFrames, f.BinaryFrames)
	}

	fmt.Fprintf(&b, "\nIdentifiers observed\n")
	keys := make([]string, 0, len(rep.Identifiers))
	for k := range rep.Identifiers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		fmt.Fprintf(&b, "  (none - check that the capture includes inbox bootstrap requests)\n")
	}
	for _, k := range keys {
		vals := make([]string, 0, len(rep.Identifiers[k]))
		for v := range rep.Identifiers[k] {
			vals = append(vals, v)
		}
		sort.Strings(vals)
		if len(vals) > 6 {
			vals = append(vals[:6], fmt.Sprintf("... +%d more", len(rep.Identifiers[k])-6))
		}
		fmt.Fprintf(&b, "  %-18s %s\n", k, strings.Join(vals, ", "))
	}

	fmt.Fprintf(&b, "\nWrote operations.json and shapes/ - review before committing.\n\n")
	return b.String()
}
