package bloks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactBundleKeepsScriptsParseable(t *testing.T) {
	raw := []byte(`{"layout":{"bloks_payload":{"ft":{"a":"(e2f (eud 12345678901234) (djj 0.25 0.1 0.25 1) (fh0 1 2 3 4 5 6 7) (dkc \"call +44 7700 900123\" \"me@example.com\"))"}}}}`)
	redacted, err := redactBundle(raw)
	if err != nil {
		t.Fatalf("redacting bundle returned error: %v", err)
	}
	var bundle struct {
		Layout struct {
			Payload struct {
				Scripts map[string]string `json:"ft"`
			} `json:"bloks_payload"`
		} `json:"layout"`
	}
	if err = json.Unmarshal(redacted, &bundle); err != nil {
		t.Fatalf("redacted bundle is not JSON: %v", err)
	}
	script := bundle.Layout.Payload.Scripts["a"]
	var node BloksScriptNode
	if _, err = node.ParseAny(script, 0); err != nil {
		t.Fatalf("redacted script %q does not parse: %v", script, err)
	}
	for _, kept := range []string{"(djj 0.25 0.1 0.25 1)", "(fh0 1 2 3 4 5 6 7)"} {
		if !strings.Contains(script, kept) {
			t.Errorf("redacted script %q lost %q", script, kept)
		}
	}
	for _, leaked := range []string{"12345678901234", "7700 900123", "me@example.com"} {
		if strings.Contains(script, leaked) {
			t.Errorf("redacted script %q still contains %q", script, leaked)
		}
	}
}
