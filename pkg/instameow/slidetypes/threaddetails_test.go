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

package slidetypes

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestThreadDetailsUnrecognizedAndDebugSummary(t *testing.T) {
	const body = `{"status":"ok","extra_top":1,"thread":{"extra_thread":true,"items":[
		{"item_id":"1","item_type":"generic_xma","timestamp":"1700000000000000","extra_item":null,
		 "generic_xma":{"b":1,"a":2,"cta_buttons":[
			{"title":"SECRET-TITLE","cta_type":"postback","action_url":"https://secret.example/path?q=1","pb":"1:GT-PB:ACT::SECRETPAYLOAD","obj":{"y":1,"x":2},"arr":[1,2]}]}}]}}`
	var resp ThreadDetailsResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	sum := resp.DebugSummary()
	if !reflect.DeepEqual(sum.TopKeys, []string{"extra_top"}) || !reflect.DeepEqual(sum.ThreadKeys, []string{"extra_thread"}) {
		t.Fatalf("unexpected keys: %+v", sum)
	}
	if len(sum.GenericXMAInfo) != 1 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
	item := sum.GenericXMAInfo[0]
	if !reflect.DeepEqual(item.ItemKeys, []string{"extra_item"}) || item.TimestampMS != 1700000000000 {
		t.Errorf("unexpected item: %+v", item)
	}
	if item.GenericXMAShape != "object" || !reflect.DeepEqual(item.GenericXMAKeys, []string{"a", "b", "cta_buttons"}) {
		t.Errorf("unexpected generic_xma shape: %+v", item)
	}
	if len(item.CTAs) != 1 {
		t.Fatalf("unexpected CTAs: %+v", item.CTAs)
	}
	cta := item.CTAs[0]
	wantShapes := map[string]string{"pb": "string(len=26)", "obj": "object(x=number,y=number)", "arr": "array(len=2)"}
	if !cta.HasTitle || cta.CTAType != "postback" || cta.ActionURLScheme != "https" || !cta.HasPostback ||
		cta.HasPostbackPayload || cta.HasPayload || !reflect.DeepEqual(cta.UnrecognizedShapes, wantShapes) {
		t.Errorf("unexpected CTA summary: %+v", cta)
	}
	dump, _ := json.Marshal(sum)
	for _, secret := range []string{"SECRET", "secret.example"} {
		if strings.Contains(string(dump), secret) {
			t.Errorf("summary leaks %q: %s", secret, dump)
		}
	}
}

func TestGenericXMAShapeArray(t *testing.T) {
	shape, keys := genericXMAShape(json.RawMessage(`[{"z":1,"a":2},{"q":1}]`))
	if shape != "array(len=2)" || !reflect.DeepEqual(keys, []string{"a", "z"}) {
		t.Errorf("got %q %v", shape, keys)
	}
	if shape, _ := genericXMAShape(json.RawMessage(`"x"`)); shape != "string(len=1)" {
		t.Errorf("got %q", shape)
	}
}

func TestThreadItemMatchesID(t *testing.T) {
	item := &ThreadItem{ItemID: "331077", MessageID: "mid.$abc"}
	for id, want := range map[string]bool{"331077": true, "mid.$abc": true, "": false, "other": false} {
		if got := item.MatchesID(id); got != want {
			t.Errorf("MatchesID(%q) = %v, want %v", id, got, want)
		}
	}
	if (&ThreadItem{}).MatchesID("") {
		t.Error("empty ID must never match")
	}
}

func TestDepth2ShapeAndMessageIDNoLeak(t *testing.T) {
	const body = `{"thread":{"items":[{"item_id":"1","message_id":"mid.$SECRETMID","item_type":"generic_xma","cta_buttons":[
		{"cta_type":"postback","title":"T","platform_token":{"postback":"1:GT-PB:ACT::SECRETTOKEN"},"tok2":{"postback":{"inner":"SECRETINNER","n":1}},"tok3":{"postback":"x"}}]}]}}`
	var resp ThreadDetailsResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	item := resp.DebugSummary().GenericXMAInfo[0]
	if !item.HasMessageID || len(item.ItemKeys) != 0 {
		t.Errorf("unexpected item: %+v", item)
	}
	cta := item.CTAs[0]
	if !cta.HasPostback {
		t.Errorf("platform_token postback not detected: %+v", cta)
	}
	want := map[string]string{
		"tok2": "object(postback=object(keys=inner,n))",
		"tok3": "object(postback=string(len=1))",
	}
	if !reflect.DeepEqual(cta.UnrecognizedShapes, want) {
		t.Errorf("shapes = %v, want %v", cta.UnrecognizedShapes, want)
	}
	dump, _ := json.Marshal(resp.DebugSummary())
	for _, secret := range []string{"SECRET", "mid.$"} {
		if strings.Contains(string(dump), secret) {
			t.Errorf("summary leaks %q: %s", secret, dump)
		}
	}
}

func TestThreadItemReuseNoStaleState(t *testing.T) {
	var item ThreadItem
	first := `{"item_id":"1","message_id":"mid.$old","item_type":"generic_xma","old_key":1,"cta_buttons":[{"cta_type":"postback","stale_key":"1:GT-PB:ACT::old1"}]}`
	if err := json.Unmarshal([]byte(first), &item); err != nil {
		t.Fatal(err)
	}
	if _, ok := item.Unrecognized["old_key"]; !ok {
		t.Fatalf("first unmarshal missing old_key: %v", item.Unrecognized)
	}
	second := `{"item_id":"2","item_type":"generic_xma","new_key":2,"cta_buttons":[{"cta_type":"postback","title":"B"}]}`
	if err := json.Unmarshal([]byte(second), &item); err != nil {
		t.Fatal(err)
	}
	if _, ok := item.Unrecognized["old_key"]; ok {
		t.Errorf("stale old_key kept: %v", item.Unrecognized)
	}
	if item.MessageID != "" {
		t.Errorf("stale MessageID kept: %q", item.MessageID)
	}
	if item.MatchesID("mid.$old") {
		t.Error("MatchesID(stale message ID) = true, want false")
	}
	if _, ok := item.Unrecognized["new_key"]; !ok {
		t.Errorf("new_key missing: %v", item.Unrecognized)
	}
	for _, b := range item.AllCTAButtons() {
		if got := b.Postback(); got != "" {
			t.Errorf("stale Postback() = %q", got)
		}
	}
}

func TestThreadDetailsResponseReuseNoStaleState(t *testing.T) {
	var r ThreadDetailsResponse
	if err := json.Unmarshal([]byte(`{"status":"ok","old_top":1,"thread":{"old_thread":1,"items":[]}}`), &r); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"status":"ok","thread":{"items":[]}}`), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Unrecognized) != 0 || len(r.Thread.Unrecognized) != 0 {
		t.Errorf("stale keys: top=%v thread=%v", r.Unrecognized, r.Thread.Unrecognized)
	}
}
