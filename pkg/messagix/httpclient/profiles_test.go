package httpclient

import (
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestParseSwitchableProfiles(t *testing.T) {
	log := zerolog.Nop()
	parser := &ModuleParser{log: &log}
	payload := json.RawMessage(`{
		"data": {"viewer": {"actor": {
			"profiles": {"edges": [
				{"node":{"profile":{"__typename":"User","id":"200","name":"TMT Muscle Cars","username":"TMTMC1","profile_picture":{"uri":"https://example.test/page.jpg"}}}},
				{"node":{"profile":{"__typename":"User","id":"200","name":"TMT Muscle Cars"}}}
			]}
		}}}}
	`)

	parser.parseSwitchableProfiles(payload)

	if len(parser.SwitchableProfiles) != 1 {
		t.Fatalf("expected one unique Page, got %#v", parser.SwitchableProfiles)
	}
	profile := parser.SwitchableProfiles[0]
	if profile.ID != "200" || profile.Name != "TMT Muscle Cars" || profile.Username != "TMTMC1" {
		t.Fatalf("unexpected Page: %#v", profile)
	}
}
