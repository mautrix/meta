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
	"strings"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
)

func itemWithXMA(t *testing.T, raw string) *slidetypes.ThreadItem {
	t.Helper()
	var item slidetypes.ThreadItem
	if err := json.Unmarshal([]byte(`{"item_id":"1","generic_xma":`+raw+`}`), &item); err != nil {
		t.Fatal(err)
	}
	return &item
}

const cardObject = `{"title_text":"Hello card","cta_buttons":[{"cta_type":"postback","title":"Go","postback_payload":"pb-1"}]}`

func TestCardHTMLFromThreadItem(t *testing.T) {
	t.Run("object with postback", func(t *testing.T) {
		item := itemWithXMA(t, cardObject)
		if xma := XMAFromThreadItem(item); xma == nil || xma.TitleText != "Hello card" {
			t.Fatalf("bad xma: %+v", xma)
		}
		out := CardHTMLFromThreadItem(item, CTAOptions{})
		for _, want := range []string{"<strong>Hello card</strong>", `<div data-ig-cta="">`, `[` + "\u00a0" + `Go` + "\u00a0" + `]`} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in %s", want, out)
			}
		}
	})
	t.Run("one-element array", func(t *testing.T) {
		item := itemWithXMA(t, "["+cardObject+"]")
		if xma := XMAFromThreadItem(item); xma == nil || xma.TitleText != "Hello card" {
			t.Fatalf("bad xma: %+v", xma)
		}
		if out := CardHTMLFromThreadItem(item, CTAOptions{}); !strings.Contains(out, "<strong>Hello card</strong>") || !strings.Contains(out, `[`+"\u00a0"+`Go`+"\u00a0"+`]`) {
			t.Errorf("unexpected %s", out)
		}
	})
	t.Run("URL CTAs as list", func(t *testing.T) {
		item := itemWithXMA(t, `{"title_text":"Links","cta_buttons":[`+
			`{"cta_type":"xma_web_url","action_url":"https://example.com/a","title":"A"},`+
			`{"cta_type":"xma_web_url","action_url":"https://example.com/b","title":"B"}]}`)
		out := CardHTMLFromThreadItem(item, CTAOptions{})
		want := `<a href="https://example.com/a">[` + " " + `A` + " " + `]</a><br><a href="https://example.com/b">[` + " " + `B` + " " + `]</a>`
		if !strings.Contains(out, want) || strings.Contains(out, " - ") || strings.Contains(out, "<ul>") {
			t.Errorf("unexpected %s", out)
		}
	})
	t.Run("CTA-only postback", func(t *testing.T) {
		item := itemWithXMA(t, `{"cta_buttons":[{"cta_type":"postback","title":"Go","postback_payload":"pb-1"}]}`)
		out := CardHTMLFromThreadItem(item, CTAOptions{})
		for _, want := range []string{`<div data-ig-cta="">`, `[` + "\u00a0" + `Go` + "\u00a0" + `]`} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in %s", want, out)
			}
		}
	})
	t.Run("CTA-only URLs", func(t *testing.T) {
		item := itemWithXMA(t, `{"cta_buttons":[{"cta_type":"xma_web_url","action_url":"https://example.com/a","title":"A"}]}`)
		out := CardHTMLFromThreadItem(item, CTAOptions{})
		if want := `<a href="https://example.com/a">[` + "\u00a0" + `A` + "\u00a0" + `]</a>`; !strings.Contains(out, want) || strings.Contains(out, " - ") || strings.Contains(out, "<ul>") {
			t.Errorf("unexpected %s", out)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if out := CardHTMLFromThreadItem(&slidetypes.ThreadItem{}, CTAOptions{}); out != "" {
			t.Errorf("got %q", out)
		}
		if XMAFromThreadItem(nil) != nil {
			t.Error("nil item")
		}
	})
}
