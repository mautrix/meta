package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Redaction is the security path here: bsprobe records real customer
// conversations, so a regression that lets a token or a message body reach a
// fixture is the worst failure this tool can have.
func TestRedactRemovesSecretsAndContent(t *testing.T) {
	input := map[string]any{
		"fb_dtsg":                  "AQHxxxxLIVE_TOKEN",
		"lsd":                      "AVqLIVE",
		"fb_api_req_friendly_name": "BizInboxThreadListQuery",
		"doc_id":                   "987654321",
		"mailbox_id":               "100200300",
		"sender": map[string]any{
			"name":  "Jane Customer",
			"email": "jane@example.com",
		},
		"message": map[string]any{
			"text": "hi, is this still available?",
		},
		"free_text": "contact me at jane@example.com or +1 415 555 0134",
	}

	out, ok := Redact(input).(map[string]any)
	if !ok {
		t.Fatalf("Redact did not return a map, got %T", Redact(input))
	}

	if out["fb_dtsg"] != "<redacted:secret>" {
		t.Errorf("fb_dtsg leaked: %v", out["fb_dtsg"])
	}
	if out["lsd"] != "<redacted:secret>" {
		t.Errorf("lsd leaked: %v", out["lsd"])
	}

	// Protocol facts must survive, otherwise the capture is useless.
	if out["fb_api_req_friendly_name"] != "BizInboxThreadListQuery" {
		t.Errorf("friendly_name was destroyed: %v", out["fb_api_req_friendly_name"])
	}
	if out["doc_id"] != "987654321" {
		t.Errorf("doc_id was destroyed: %v", out["doc_id"])
	}
	if out["mailbox_id"] != "100200300" {
		t.Errorf("mailbox_id was destroyed: %v", out["mailbox_id"])
	}

	sender := out["sender"].(map[string]any)
	if s, _ := sender["name"].(string); !strings.HasPrefix(s, "<redacted:content") {
		t.Errorf("customer name leaked: %v", sender["name"])
	}
	msg := out["message"].(map[string]any)
	if s, _ := msg["text"].(string); !strings.HasPrefix(s, "<redacted:content") {
		t.Errorf("message text leaked: %v", msg["text"])
	}

	free, _ := out["free_text"].(string)
	if strings.Contains(free, "jane@example.com") {
		t.Errorf("email leaked in free text: %v", free)
	}
	if strings.Contains(free, "555 0134") {
		t.Errorf("phone leaked in free text: %v", free)
	}

	// Belt and braces: no serialisation of the result may contain the token.
	blob, _ := json.Marshal(out)
	if strings.Contains(string(blob), "LIVE_TOKEN") {
		t.Fatalf("live token present in serialised output: %s", blob)
	}
}

func TestShapeDropsAllValues(t *testing.T) {
	var decoded any
	raw := `{"data":{"threads":[{"id":"123","snippet":"secret text","unread":true}]}}`
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(Shape(decoded))
	if strings.Contains(string(blob), "secret text") {
		t.Fatalf("Shape leaked a value: %s", blob)
	}
	if strings.Contains(string(blob), "123") {
		t.Fatalf("Shape leaked an id value: %s", blob)
	}
	if !strings.Contains(string(blob), "string") {
		t.Fatalf("Shape did not record types: %s", blob)
	}
}

func TestComputeJazoest(t *testing.T) {
	// "abc" -> 97+98+99 = 294, prefixed with "2".
	if got := ComputeJazoest("abc"); got != "2294" {
		t.Errorf("ComputeJazoest(abc) = %q, want 2294", got)
	}
}

func TestLoadSessionParsesCurlBlob(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.curl")
	blob := `curl 'https://business.facebook.com/latest/inbox/all' ` +
		`-H 'user-agent: TestAgent/1.0' ` +
		`-H 'cookie: datr=AAA; c_user=61550000000; xs=42%3Aabc; sb=BBB'`
	if err := os.WriteFile(path, []byte(blob), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Cookies) != 4 {
		t.Errorf("got %d cookies, want 4", len(s.Cookies))
	}
	if s.UserID() != "61550000000" {
		t.Errorf("UserID = %q, want 61550000000", s.UserID())
	}
	if !s.Has("xs") || !s.Has("datr") {
		t.Error("required cookies not parsed")
	}
	if s.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %q", s.UserAgent)
	}
}

func TestLoadSessionParsesRawCookieString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.txt")
	if err := os.WriteFile(path, []byte("c_user=123; xs=abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.UserID() != "123" {
		t.Errorf("UserID = %q, want 123", s.UserID())
	}
}

const lightspeedHAR = `{"log":{"entries":[
{"request":{"method":"POST","url":"https://business.facebook.com/api/graphql/","headers":[],
 "postData":{"mimeType":"application/x-www-form-urlencoded",
 "text":"fb_dtsg=LIVE&doc_id=987654321&fb_api_req_friendly_name=BizInboxThreadListQuery&variables=%7B%22mailbox_id%22%3A%22100200300%22%2C%22count%22%3A20%7D"}},
 "response":{"status":200,"content":{"mimeType":"application/json","text":"{\"data\":{\"threads\":[]}}"}}},
{"request":{"method":"GET","url":"wss://edge-chat.facebook.com/chat","headers":[],"postData":{"mimeType":"","text":""}},
 "response":{"status":101,"content":{"mimeType":"","text":""}},
 "_webSocketMessages":[
   {"type":"receive","time":1,"opcode":1,"data":"{\"request_id\":7,\"payload\":\"x\",\"sp\":[],\"target\":1}"}
 ]}
]}}`

func TestAnalyzeHARExtractsOperationsAndCallsCaseA(t *testing.T) {
	var h harFile
	if err := json.Unmarshal([]byte(lightspeedHAR), &h); err != nil {
		t.Fatal(err)
	}
	rep := analyzeHAR(&h)

	op, ok := rep.Operations["BizInboxThreadListQuery"]
	if !ok {
		t.Fatalf("operation not extracted, got %v", rep.Operations)
	}
	if op.DocID != "987654321" {
		t.Errorf("doc_id = %q, want 987654321", op.DocID)
	}
	if strings.Join(op.VariableKeys, ",") != "count,mailbox_id" {
		t.Errorf("variable keys = %v, want [count mailbox_id]", op.VariableKeys)
	}
	if !strings.HasPrefix(rep.Verdict, "CASE A") {
		t.Errorf("verdict = %q, want CASE A", rep.Verdict)
	}
	if _, ok := rep.Identifiers["mailbox_id"]["100200300"]; !ok {
		t.Errorf("mailbox_id not harvested, got %v", rep.Identifiers)
	}
}

const plainTextHAR = `{"log":{"entries":[
{"request":{"method":"GET","url":"wss://business.facebook.com/rt","headers":[],"postData":{"mimeType":"","text":""}},
 "response":{"status":101,"content":{"mimeType":"","text":""}},
 "_webSocketMessages":[
   {"type":"receive","time":1,"opcode":1,"data":"{\"topic\":\"x\",\"subscription_id\":\"y\"}"}
 ]}
]}}`

func TestAnalyzeHARCallsCaseBOnPlainFrames(t *testing.T) {
	var h harFile
	if err := json.Unmarshal([]byte(plainTextHAR), &h); err != nil {
		t.Fatal(err)
	}
	rep := analyzeHAR(&h)
	if !strings.HasPrefix(rep.Verdict, "CASE B") {
		t.Errorf("verdict = %q, want CASE B", rep.Verdict)
	}
}

func TestAnalyzeHARInconclusiveWithoutRealtime(t *testing.T) {
	var h harFile
	if err := json.Unmarshal([]byte(`{"log":{"entries":[]}}`), &h); err != nil {
		t.Fatal(err)
	}
	rep := analyzeHAR(&h)
	if !strings.HasPrefix(rep.Verdict, "INCONCLUSIVE") {
		t.Errorf("verdict = %q, want INCONCLUSIVE", rep.Verdict)
	}
}

func TestMQTTPacketName(t *testing.T) {
	cases := map[byte]string{
		0x10: "CONNECT",
		0x20: "CONNACK",
		0x30: "PUBLISH",
		0xC0: "PINGREQ",
		0x70: "",
	}
	for b, want := range cases {
		if got := mqttPacketName(b); got != want {
			t.Errorf("mqttPacketName(%#x) = %q, want %q", b, got, want)
		}
	}
}

func TestRegistryRoleErrorIsActionable(t *testing.T) {
	dir := t.TempDir()
	ops := `{"operations":[{"friendly_name":"SomeQuery","doc_id":"1"}]}`
	if err := os.WriteFile(filepath.Join(dir, "operations.json"), []byte(ops), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reg.ForRole("list_threads")
	if err == nil {
		t.Fatal("expected an error for an unmapped role")
	}
	if !strings.Contains(err.Error(), "roles.json") {
		t.Errorf("error should point at roles.json, got: %v", err)
	}
}
