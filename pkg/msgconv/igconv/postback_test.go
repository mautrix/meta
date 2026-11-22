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

package igconv

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"maunium.net/go/mautrix/format"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func pb(title, payload string) *metaid.PostbackButton {
	return &metaid.PostbackButton{Title: title, Payload: payload, CTAType: "postback"}
}

func TestResolvePostbackButton(t *testing.T) {
	buttons := []*metaid.PostbackButton{pb("Yes", "p1"), pb("No", "p2"), pb("Maybe", "p3"), pb("maybe", "p4"), pb("Later", "p5")}
	tests := []struct {
		arg     string
		want    string
		wantErr error
	}{
		{"1", "p1", nil},
		{"2", "p2", nil},
		{"5", "p5", nil},
		{"0", "", ErrPostbackNotFound},
		{"6", "", ErrPostbackNotFound},
		{"No", "p2", nil},
		{"no", "p2", nil},
		{"LATER", "p5", nil},
		{"Maybe", "p3", nil}, // exact match wins over the case-insensitive duplicate
		{"MAYBE", "", ErrPostbackAmbiguous},
		{"Nope", "", ErrPostbackNotFound},
		{"", "", ErrPostbackNotFound},
		{"  Yes ", "p1", nil},
	}
	for _, tt := range tests {
		got, err := ResolvePostbackButton(buttons, tt.arg)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("%q: error %v, want %v", tt.arg, err, tt.wantErr)
			continue
		}
		if tt.wantErr == nil && got.Payload != tt.want {
			t.Errorf("%q: got payload %q, want %q", tt.arg, got.Payload, tt.want)
		}
	}
	if _, err := ResolvePostbackButton(nil, "1"); !errors.Is(err, ErrPostbackNotFound) {
		t.Errorf("no buttons: got %v", err)
	}
	dup := []*metaid.PostbackButton{pb("Same", "a"), pb("Same", "b")}
	if _, err := ResolvePostbackButton(dup, "Same"); !errors.Is(err, ErrPostbackAmbiguous) {
		t.Errorf("duplicate exact titles: got %v", err)
	}
}

func parseCTAs(t *testing.T, raw string) []*slidetypes.CTAButton {
	t.Helper()
	var ctas []*slidetypes.CTAButton
	if err := json.Unmarshal([]byte(raw), &ctas); err != nil {
		t.Fatal(err)
	}
	return ctas
}

const mixedCTAs = `[
	{"cta_type":"xma_web_url","action_url":"https://example.com/a","title":"Website"},
	{"cta_type":"postback","action_url":"","title":"Yes <b>","payload":"payload-a"},
	{"cta_type":"postback","action_url":"","title":"No","postback_payload":"payload-b"}]`

func TestXMACTAButtonsHTML(t *testing.T) {
	ctas := parseCTAs(t, mixedCTAs)

	t.Run("links and postbacks", func(t *testing.T) {
		out := xmaCTAButtonsHTML(ctas, PostbackButtonsFromCTAs(ctas), nil, CTAOptions{})
		for _, want := range []string{
			`<div data-ig-cta="">`,
			`<a href="https://example.com/a">[` + " " + `Website` + " " + `]</a>`,
			`<br>[` + " " + `Yes &lt;b&gt;` + " " + `]<br>[` + " " + `No` + " " + `]`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in %s", want, out)
			}
		}
		if strings.Contains(out, "Use the Instagram app") {
			t.Errorf("unexpected app warning: %s", out)
		}
	})

	t.Run("three links are a list", func(t *testing.T) {
		three := parseCTAs(t, `[
			{"cta_type":"xma_web_url","action_url":"https://example.com/1","title":"Guide"},
			{"cta_type":"xma_web_url","action_url":"https://example.com/2","title":"Following"},
			{"cta_type":"xma_web_url","action_url":"https://example.com/3","title":"Docs"}]`)
		out := xmaCTAButtonsHTML(three, nil, nil, CTAOptions{})
		if got := strings.Count(out, "<a href="); got != 3 || strings.Contains(out, "<ul>") || strings.Contains(out, "<li>") || strings.Count(out, "<br>") != 2 {
			t.Errorf("expected 3 pseudo-buttons joined by <br>, got %s", out)
		}
		if strings.Contains(out, "↗") || strings.Contains(out, "React with") {
			t.Errorf("bridge must not add ↗ or a footer: %s", out)
		}
		if strings.Contains(out, " - ") {
			t.Errorf("links are still joined with a dash: %s", out)
		}
	})

	t.Run("fallback warning when payload not recovered", func(t *testing.T) {
		hidden := parseCTAs(t, `[{"cta_type":"postback","action_url":"","title":"Yes"},{"cta_type":"placeholder","title":"No"}]`)
		out := xmaCTAButtonsHTML(hidden, nil, nil, CTAOptions{})
		if want := `<div data-ig-cta=""><p>Yes, No<br>Use the Instagram app to click buttons</p></div>`; out != want {
			t.Errorf("got %q, want %q", out, want)
		}
		if strings.Contains(out, "!ig tap") || strings.Contains(out, " - ") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("url only", func(t *testing.T) {
		out := xmaCTAButtonsHTML(ctas[:1], nil, nil, CTAOptions{})
		if want := `<div data-ig-cta=""><a href="https://example.com/a">[` + "\u00a0" + `Website` + "\u00a0" + `]</a></div>`; out != want {
			t.Errorf("got %q, want %q", out, want)
		}
	})

	t.Run("no buttons", func(t *testing.T) {
		if out := xmaCTAButtonsHTML(nil, nil, nil, CTAOptions{}); out != "" {
			t.Errorf("got %q", out)
		}
	})

	t.Run("partially recovered", func(t *testing.T) {
		out := xmaCTAButtonsHTML(parseCTAs(t, `[{"cta_type":"postback","title":"A"},{"cta_type":"postback","title":"B"}]`), []*metaid.PostbackButton{pb("A", "x")}, nil, CTAOptions{})
		if !strings.Contains(out, `[`+"\u00a0"+`A`+"\u00a0"+`]`) || strings.Contains(out, "Use the Instagram app") || strings.Contains(out, "<li>") {
			t.Errorf("unexpected output %s", out)
		}
	})
}

func TestReplaceOrAppendCTAButtonsHTML(t *testing.T) {
	newCTA := `<div data-ig-cta=""><ul><li><a href="https://example.com/n">New</a></li></ul></div>`

	t.Run("replaces marked block", func(t *testing.T) {
		old := `<p>Caption</p><div data-ig-cta=""><ul><li>old</li></ul></div><p>After</p>`
		want := `<p>Caption</p>` + newCTA + `<p>After</p>`
		if got := ReplaceOrAppendCTAButtonsHTML(old, newCTA); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("strips old app sentence and appends", func(t *testing.T) {
		old := `<p>Caption</p><p>Yes - No<br>Use the Instagram app to click buttons</p>`
		got := ReplaceOrAppendCTAButtonsHTML(old, newCTA)
		if want := `<p>Caption</p>` + newCTA; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if strings.Contains(got, "Instagram app") {
			t.Errorf("old sentence kept: %s", got)
		}
	})

	t.Run("strips old link run and tap list", func(t *testing.T) {
		old := `<p>Caption</p><p><a href="https://example.com/1">Guide</a> - <a href="https://example.com/2">Following</a></p>` +
			`<ol><li>Yes</li></ol><p>Tap a button with <code>!ig tap N</code> or <code>!ig tap &lt;title&gt;</code></p>`
		if got, want := ReplaceOrAppendCTAButtonsHTML(old, newCTA), `<p>Caption</p>`+newCTA; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("keeps lone target link and caption", func(t *testing.T) {
		old := `<p>Caption</p><p><a href="https://example.com/post">https://example.com/post</a></p>`
		if got, want := ReplaceOrAppendCTAButtonsHTML(old, newCTA), old+newCTA; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestMatchUserPostback(t *testing.T) {
	buttons := []*metaid.PostbackButton{pb("Guide", "payload-a"), pb("Following", "payload-b")}

	if got, err := MatchUserPostback(buttons, " 1 ", true); err != nil || got.Payload != "payload-a" {
		t.Errorf("reply+1: got %v, %v", got, err)
	}
	if _, err := MatchUserPostback(buttons, "1", false); !errors.Is(err, ErrPostbackNotFound) {
		t.Errorf("non-reply 1 must not match, got %v", err)
	}
	if got, err := MatchUserPostback(buttons, "following", false); err != nil || got.Payload != "payload-b" {
		t.Errorf("non-reply title: got %v, %v", got, err)
	}
	if _, err := MatchUserPostback(buttons, "hello there", false); !errors.Is(err, ErrPostbackNotFound) {
		t.Errorf("unknown text: got %v", err)
	}
	if _, err := MatchUserPostback(buttons, "  ", true); !errors.Is(err, ErrPostbackNotFound) {
		t.Errorf("empty text: got %v", err)
	}
	dup := []*metaid.PostbackButton{pb("Same", "payload-a"), pb("same", "payload-b")}
	if _, err := MatchUserPostback(dup, "SAME", false); !errors.Is(err, ErrPostbackAmbiguous) {
		t.Errorf("ambiguous: got %v", err)
	}
}

func TestXMANeedsPostbackFetch(t *testing.T) {
	tests := []struct {
		name string
		xma  *slidetypes.XMAContent
		want bool
	}{
		{"nil", nil, false},
		{"url only", &slidetypes.XMAContent{CTAButtons: parseCTAs(t, `[{"cta_type":"xma_web_url","action_url":"https://example.com","title":"Go"}]`)}, false},
		{"postback with payload", &slidetypes.XMAContent{CTAButtons: parseCTAs(t, `[{"cta_type":"postback","title":"Go","payload":"payload-a"}]`)}, false},
		{"postback without payload", &slidetypes.XMAContent{CTAButtons: parseCTAs(t, `[{"cta_type":"postback","title":"Go"}]`)}, true},
		{"placeholder", &slidetypes.XMAContent{CTAButtons: parseCTAs(t, `[{"cta_type":"placeholder","title":"Go"}]`)}, true},
		{"empty buttons text card", &slidetypes.XMAContent{TitleText: "Pick one"}, true},
		{"empty buttons media share", &slidetypes.XMAContent{TitleText: "Reel", TargetID: "123", TargetURL: "https://www.instagram.com/reel/x/"}, false},
		{"empty everything", &slidetypes.XMAContent{}, false},
	}
	for _, tt := range tests {
		if got := xmaNeedsPostbackFetch(tt.xma); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPickPostbackButtons(t *testing.T) {
	var resp slidetypes.ThreadDetailsResponse
	err := json.Unmarshal([]byte(`{"thread":{"items":[
		{"item_type":"generic_xma","timestamp":"1700000200000000","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"new-yes"},{"cta_type":"postback","title":"No","payload":"new-no"}]},
		{"item_type":"generic_xma","timestamp":"1700000100000000","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"old-yes"},{"cta_type":"postback","title":"No","payload":"old-no"}]},
		{"item_type":"generic_xma","timestamp":"1700000050000000","cta_buttons":[{"cta_type":"postback","title":"Other","payload":"other"}]}]}}`), &resp)
	if err != nil {
		t.Fatal(err)
	}
	items := resp.GenericXMAItems()
	if got := PickPostbackButtons(items, "", []string{"Yes", "No"}, 1700000100500); got[0].Payload != "old-yes" {
		t.Errorf("closest timestamp: got %+v", got)
	}
	if got := PickPostbackButtons(items, "", []string{"Yes"}, 0); got[0].Payload != "new-yes" {
		t.Errorf("no timestamp should pick newest: got %+v", got)
	}
	if got := PickPostbackButtons(items, "", []string{"Other"}, 1700000200000); len(got) != 1 || got[0].Payload != "other" {
		t.Errorf("title filter: got %+v", got)
	}
	if got := PickPostbackButtons(items, "", []string{"Missing"}, 0); got != nil {
		t.Errorf("expected nil for unmatched titles, got %+v", got)
	}
}

func TestPickPostbackButtonsByItemID(t *testing.T) {
	var resp slidetypes.ThreadDetailsResponse
	err := json.Unmarshal([]byte(`{"thread":{"items":[
		{"item_id":"B","item_type":"generic_xma","timestamp":"1700000200000000","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"b-yes"}]},
		{"item_id":"A","item_type":"generic_xma","timestamp":"1700000100000000","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"a-yes"}]},
		{"item_id":"C","item_type":"generic_xma","timestamp":"1700000300000000","cta_buttons":[{"cta_type":"xma_web_url","action_url":"https://example.com","title":"Go"}]}]}}`), &resp)
	if err != nil {
		t.Fatal(err)
	}
	items := resp.GenericXMAItems()
	if got := PickPostbackButtons(items, "A", nil, 1700000200000); len(got) != 1 || got[0].Payload != "a-yes" {
		t.Errorf("item ID must beat closer timestamp: got %+v", got)
	}
	if got := PickPostbackButtons(items, "A", nil, 0); len(got) != 1 || got[0].Payload != "a-yes" {
		t.Errorf("item ID without timestamp: got %+v", got)
	}
	if got := PickPostbackButtons(items, "", nil, 1700000200000); got != nil {
		t.Errorf("no ID and no titles must return nothing: got %+v", got)
	}
	if got := PickPostbackButtons(items, "plain-text-id", nil, 1700000200000); got != nil {
		t.Errorf("unrelated item ID must return nothing: got %+v", got)
	}
	if got := PickPostbackButtons(items, "C", []string{"Yes"}, 1700000200000); got != nil {
		t.Errorf("matched item without postbacks must not fall through: got %+v", got)
	}
}

func TestPickPostbackButtonsEmptyTitlesRecoverByMessageID(t *testing.T) {
	var resp slidetypes.ThreadDetailsResponse
	err := json.Unmarshal([]byte(`{"thread":{"items":[
		{"item_id":"B","item_type":"generic_xma","timestamp":"1700000200000000","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"b-yes"}]},
		{"item_id":"A","item_type":"generic_xma","timestamp":"1700000100000000","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"a-yes"}]}]}}`), &resp)
	if err != nil {
		t.Fatal(err)
	}
	items := resp.GenericXMAItems()
	// Placeholder cards have no GraphQL titles; the message's own ID must find the card.
	if got := PickPostbackButtons(items, "A", ctaTitles(nil), 1700000200000); len(got) != 1 || got[0].Payload != "a-yes" {
		t.Errorf("empty titles + message ID must recover that card: got %+v", got)
	}
	if got := PickPostbackButtons(items, "", ctaTitles(nil), 1700000200000); got != nil {
		t.Errorf("empty titles + empty ID must return nothing: got %+v", got)
	}
	// A plain text message: its ID is not a generic_xma item and there are no titles.
	if got := PickPostbackButtons(items, "plain-text-id", ctaTitles(nil), 1700000200000); got != nil {
		t.Errorf("plain text message must get no buttons: got %+v", got)
	}
	// Message ID that matches nothing still falls back to titles + timestamp.
	if got := PickPostbackButtons(items, "other-id", []string{"Yes"}, 1700000100500); len(got) != 1 || got[0].Payload != "a-yes" {
		t.Errorf("titles fallback: got %+v", got)
	}
}

func TestItemIDFromMessageID(t *testing.T) {
	if got := ItemIDFromMessageID(metaid.MakeFBMessageID("abc")); got != "abc" {
		t.Errorf("got %q", got)
	}
	if got := ItemIDFromMessageID("garbage"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestPostbackThreadID(t *testing.T) {
	for name, tc := range map[string]struct {
		meta *metaid.PortalMetadata
		want string
	}{
		"prefers ig thread id": {&metaid.PortalMetadata{IGID: "1806437297362083", IGThreadID: "340282366841710301244259981454874449059"}, "340282366841710301244259981454874449059"},
		"falls back to igid":   {&metaid.PortalMetadata{IGID: "1806437297362083"}, "1806437297362083"},
		"both empty":           {&metaid.PortalMetadata{}, ""},
		"nil":                  {nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := PostbackThreadID(tc.meta); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPickPostbackButtonsByMessageID(t *testing.T) {
	var resp slidetypes.ThreadDetailsResponse
	err := json.Unmarshal([]byte(`{"thread":{"items":[
		{"item_id":"33107782603843137058094129889673216","message_id":"mid.$AAA","item_type":"generic_xma","cta_buttons":[{"cta_type":"postback","title":"Yes","platform_token":{"postback":"a-yes"}}]},
		{"item_id":"2","message_id":"mid.$BBB","item_type":"generic_xma","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"b-yes"}]}]}}`), &resp)
	if err != nil {
		t.Fatal(err)
	}
	items := resp.GenericXMAItems()
	if got := PickPostbackButtons(items, "mid.$AAA", nil, 0); len(got) != 1 || got[0].Payload != "a-yes" {
		t.Errorf("message_id match: got %+v", got)
	}
	if got := PickPostbackButtons(items, "2", nil, 0); len(got) != 1 || got[0].Payload != "b-yes" {
		t.Errorf("item_id match: got %+v", got)
	}
	if got := PickPostbackButtons(items, "plain-text-id", nil, 0); got != nil {
		t.Errorf("unrelated ID must not match: got %+v", got)
	}
}

func TestHasKnownCTAFooter(t *testing.T) {
	for _, html := range []string{``, `<p>Just a caption - with a dash</p>`, `<p><a href="https://a.example">only one</a></p>`, `<p>He said Use the Instagram app to click buttons in a guide</p>`} {
		if HasKnownCTAFooter(html) {
			t.Errorf("%q: not a footer", html)
		}
	}
	for _, html := range []string{
		`<p>x</p><div data-ig-cta=""></div>`,
		`<p>x<br>Use the Instagram app to click buttons</p>`,
		`<p><a href="https://a.example">A</a> - <a href="https://b.example">B</a></p>`,
		`<ol><li>A</li></ol><p>Tap a button with <code>!ig tap N</code>.</p>`,
	} {
		if !HasKnownCTAFooter(html) {
			t.Errorf("%q: footer not detected", html)
		}
	}
}

func TestPseudoButtonLayout(t *testing.T) {
	const nbsp = "\u00a0"
	urlOnly := xmaCTAButtonsHTML(parseCTAs(t, `[{"cta_type":"xma_web_url","action_url":"https://example.com/a","title":"grab the skill"}]`), nil, nil, CTAOptions{})
	if strings.Contains(urlOnly, "<ul>") || strings.Contains(urlOnly, "<ol>") || strings.Contains(urlOnly, "<li>") || strings.Contains(urlOnly, "React with") || strings.Contains(urlOnly, "↗") {
		t.Fatalf("url-only must be a single pseudo-button with no footer: %s", urlOnly)
	}
	wantURL := `<a href="https://example.com/a">[` + nbsp + `grab the skill` + nbsp + `]</a>`
	if !strings.Contains(urlOnly, wantURL) {
		t.Fatalf("missing glued external label in %s", urlOnly)
	}
	plain := format.HTMLToContent(urlOnly).Body
	if !strings.Contains(plain, "["+nbsp+"grab the skill"+nbsp+"]") || strings.Contains(plain, "↗") {
		t.Fatalf("plain body must keep the bracketed label, got %q", plain)
	}

	taps := xmaCTAButtonsHTML(nil, []*metaid.PostbackButton{pb("Send me the link", "x"), pb("Book a call", "y")}, []*metaid.PostbackButton{{Title: "Yes", Payload: "z", CTAType: "quick_reply"}}, CTAOptions{})
	if strings.Contains(taps, "↗") || strings.Contains(taps, "React with") || strings.Contains(taps, "<p>") {
		t.Fatalf("tappable buttons must not get ↗ or a footer: %s", taps)
	}
	for _, title := range []string{"Send me the link", "Book a call", "Yes"} {
		if !strings.Contains(taps, "["+nbsp+title+nbsp+"]") {
			t.Fatalf("missing [%s] in %s", title, taps)
		}
	}
	if strings.Count(taps, "<br>") != 2 || strings.Contains(taps, "<ul>") || strings.Contains(taps, "<li>") {
		t.Fatalf("taps must be one pseudo-button per line: %s", taps)
	}
	if strings.Contains(taps, " — postback") || strings.Contains(taps, " — quick reply") || strings.Contains(taps, "react with 1️⃣, or reply") {
		t.Fatalf("per-button instructions must not appear: %s", taps)
	}
	plainTaps := format.HTMLToContent(taps).Body
	if strings.Count(plainTaps, "\n") < 2 || !strings.Contains(plainTaps, "["+nbsp+"Yes"+nbsp+"]") || strings.Contains(plainTaps, "React with") {
		t.Fatalf("plain body must list one label per line, got %q", plainTaps)
	}
}
