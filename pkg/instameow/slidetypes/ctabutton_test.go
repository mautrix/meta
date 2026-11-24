// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package slidetypes_test

import (
	"encoding/json"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
)

// All fixtures are synthetic.
func TestCTAButtonPostback(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"url cta", `{"cta_type":"xma_web_url","action_url":"https://example.com/x","title":"Open"}`, ""},
		{"payload key", `{"cta_type":"postback","action_url":"","title":"Yes","payload":"1:GT-PB:ACT::aa11"}`, "1:GT-PB:ACT::aa11"},
		{"postback_payload key", `{"cta_type":"postback","action_url":"","title":"Yes","postback_payload":"1:GT-PB:ACT::bb22"}`, "1:GT-PB:ACT::bb22"},
		{"missing payload", `{"cta_type":"postback","action_url":"","title":"Yes"}`, ""},
		{"GT-PB in unrecognized", `{"cta_type":"postback","title":"Yes","some_other_key":"1:GT-PB:ACT::cc33","unrelated":"abc"}`, "1:GT-PB:ACT::cc33"},
		{"payload without marker still accepted", `{"cta_type":"postback","title":"Yes","payload":"opaque"}`, "opaque"},
		{"platform_token postback marker string", `{"cta_type":"postback","title":"Yes","platform_token":{"postback":"1:GT-PB:ACT::dd44"}}`, "1:GT-PB:ACT::dd44"},
		{"platform_token postback opaque string", `{"cta_type":"postback","title":"Yes","platform_token":{"postback":"opaque"}}`, "opaque"},
		{"platform_token postback object", `{"cta_type":"postback","title":"Yes","platform_token":{"postback":{"postback_payload":"1:GT-PB:ACT::ee55"}}}`, "1:GT-PB:ACT::ee55"},
		{"platform_token postback object token", `{"cta_type":"postback","title":"Yes","platform_token":{"postback":{"token":"tok"}}}`, "tok"},
		{"platform_token postback object marker field", `{"cta_type":"postback","title":"Yes","platform_token":{"postback":{"zzz":"1:GT-PB:ACT::ff66","a":"nope"}}}`, "1:GT-PB:ACT::ff66"},
		{"platform_token direct payload", `{"cta_type":"postback","title":"Yes","platform_token":{"payload":"direct"}}`, "direct"},
		{"platform_token empty postback", `{"cta_type":"postback","title":"Yes","platform_token":{"postback":""}}`, ""},
		{"platform_token not object", `{"cta_type":"postback","title":"Yes","platform_token":"str"}`, ""},
		{"top-level payload beats platform_token", `{"cta_type":"postback","payload":"top","platform_token":{"postback":"tok"}}`, "top"},
		{"marker one level deep in unrecognized", `{"cta_type":"postback","title":"Yes","wrapper":{"b":"x","a":"1:GT-PB:ACT::77"}}`, "1:GT-PB:ACT::77"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var btn slidetypes.CTAButton
			if err := json.Unmarshal([]byte(tt.json), &btn); err != nil {
				t.Fatal(err)
			}
			if got := btn.Postback(); got != tt.want {
				t.Fatalf("Postback() = %q, want %q (unrecognized: %v)", got, tt.want, btn.Unrecognized)
			}
		})
	}
}

func TestThreadDetailsGenericXMAItems(t *testing.T) {
	const body = `{"status":"ok","thread":{"items":[
		{"item_id":"1","item_type":"text","text":"hi"},
		{"item_id":"2","item_type":"generic_xma","timestamp":"1700000000123456","cta_buttons":[{"cta_type":"postback","title":"A","payload":"1:GT-PB:ACT::01"}]},
		{"item_id":"3","item_type":"generic_xma","timestamp":1700000001123456,"generic_xma":{"cta_buttons":[{"cta_type":"postback","title":"B","postback_payload":"1:GT-PB:ACT::02"}]}},
		{"item_id":"4","item_type":"generic_xma","generic_xma":[{"title_text":"x"},{"cta_buttons":[{"cta_type":"postback","title":"C","payload":"1:GT-PB:ACT::03"}]}]}
	]}}`
	var resp slidetypes.ThreadDetailsResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	items := resp.GenericXMAItems()
	if len(items) != 3 {
		t.Fatalf("expected 3 generic_xma items, got %d", len(items))
	}
	for i, want := range []string{"1:GT-PB:ACT::01", "1:GT-PB:ACT::02", "1:GT-PB:ACT::03"} {
		ctas := items[i].AllCTAButtons()
		if len(ctas) != 1 || ctas[0].Postback() != want {
			t.Errorf("item %d: unexpected buttons %+v", i, ctas)
		}
	}
	if got := items[0].TimestampMS(); got != 1700000000123 {
		t.Errorf("string timestamp: got %d", got)
	}
	if got := items[1].TimestampMS(); got != 1700000001123 {
		t.Errorf("numeric timestamp: got %d", got)
	}
	if got := items[2].TimestampMS(); got != 0 {
		t.Errorf("missing timestamp: got %d", got)
	}
}

func TestCTAButtonReuseNoStaleState(t *testing.T) {
	var b slidetypes.CTAButton
	if err := json.Unmarshal([]byte(`{"cta_type":"postback","title":"A","stale_key":"1:GT-PB:ACT::old1"}`), &b); err != nil {
		t.Fatal(err)
	}
	if got := b.Postback(); got != "1:GT-PB:ACT::old1" {
		t.Fatalf("first Postback() = %q", got)
	}
	if err := json.Unmarshal([]byte(`{"cta_type":"postback","title":"B"}`), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Unrecognized) != 0 {
		t.Errorf("stale Unrecognized kept: %v", b.Unrecognized)
	}
	if got := b.Postback(); got != "" {
		t.Errorf("stale Postback() = %q", got)
	}

	var tok slidetypes.CTAButton
	if err := json.Unmarshal([]byte(`{"cta_type":"postback","title":"A","platform_token":"tok"}`), &tok); err != nil {
		t.Fatal(err)
	}
	if len(tok.PlatformToken) == 0 {
		t.Fatal("first unmarshal missing platform_token")
	}
	if err := json.Unmarshal([]byte(`{"cta_type":"postback","title":"B"}`), &tok); err != nil {
		t.Fatal(err)
	}
	if len(tok.PlatformToken) != 0 {
		t.Errorf("stale PlatformToken kept: %s", tok.PlatformToken)
	}
	if err := json.Unmarshal([]byte(`null`), &tok); err != nil {
		t.Fatal(err)
	}
	if tok.Title != "" {
		t.Errorf("null left Title = %q", tok.Title)
	}
}
