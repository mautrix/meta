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
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func TestClassifyButtons(t *testing.T) {
	ctas := parseCTAs(t, `[
		{"cta_type":"xma_web_url","action_url":"https://example.com/a","title":"Website"},
		{"cta_type":"POSTBACK","title":"Following","payload":"payload-pb-1"},
		{"cta_type":"text","title":"Hidden GT","payload":"1:GT-PB:ACT::ff"},
		{"cta_type":"quick_reply","title":"Yes","payload":"payload-qr-1"},
		{"cta_type":"quick_replies","title":"Maybe later"},
		{"cta_type":"placeholder","title":"Title only"},
		{"cta_type":"postback","title":"No payload"}]`)
	got := ButtonsFromCTAs(ctas)
	var pbs, qrs []string
	for _, b := range got.Postbacks {
		pbs = append(pbs, b.Title)
	}
	for _, b := range got.QuickReplies {
		qrs = append(qrs, b.Title)
		if !b.IsQuickReply() {
			t.Errorf("%q isn't marked as quick reply", b.Title)
		}
	}
	if strings.Join(pbs, "|") != "Following|Hidden GT" {
		t.Errorf("postbacks = %v", pbs)
	}
	if strings.Join(qrs, "|") != "Yes|Maybe later|Title only" {
		t.Errorf("quick replies = %v", qrs)
	}
	if got.QuickReplies[0].Payload != "payload-qr-1" || got.QuickReplies[1].Payload != "" {
		t.Errorf("unexpected quick reply payloads: %+v", got.QuickReplies)
	}
	platform := parseCTAs(t, `[{"cta_type":"text","title":"Tok","platform_token":{"postback":"tok-1"}}]`)
	if b := ButtonsFromCTAs(platform); len(b.Postbacks) != 1 || len(b.QuickReplies) != 0 {
		t.Errorf("platform_token.postback must stay a postback: %+v", b)
	}
}

func TestButtonsFromQuickReplies(t *testing.T) {
	got := ButtonsFromQuickReplies([]*slidetypes.QuickReply{
		{Title: "Yes", Payload: "payload-qr-1", ContentType: "text"},
		{Title: "Maybe later", QuickReplyPayload: "payload-qr-2"},
		{Title: "Plain"},
		{Title: "GT", Payload: "1:GT-PB:ACT::ab"},
		{Title: ""},
	})
	if len(got.QuickReplies) != 3 || got.QuickReplies[1].Payload != "payload-qr-2" || len(got.Postbacks) != 1 {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestQuickReplyHTML(t *testing.T) {
	buttons := Buttons{
		Postbacks:    []*metaid.PostbackButton{pb("Following", "payload-a")},
		QuickReplies: []*metaid.PostbackButton{{Title: "Maybe later", Payload: "payload-qr-1", CTAType: "quick_reply"}},
	}
	out := xmaCTAButtonsHTML(nil, buttons.Postbacks, buttons.QuickReplies, CTAOptions{})
	for _, want := range []string{
		`<div data-ig-cta="">`,
		`[` + " " + `Following` + " " + `]`,
		`[` + " " + `Maybe later` + " " + `]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, " - ") || strings.Contains(out, "Use the Instagram app") || strings.Contains(out, "↗") || strings.Contains(out, "React with") {
		t.Errorf("unexpected output: %s", out)
	}
	many := make([]*metaid.PostbackButton, 11)
	for i := range many {
		many[i] = &metaid.PostbackButton{Title: "Q", CTAType: "quick_reply"}
	}
	out = xmaCTAButtonsHTML(nil, nil, many, CTAOptions{})
	if strings.Count(out, `[`+" "+`Q`+" "+`]`) != 11 || strings.Count(out, "<br>") != 10 || strings.Contains(out, "<li>") {
		t.Errorf("expected 11 pseudo-buttons, got %s", out)
	}
	links := parseCTAs(t, `[{"cta_type":"xma_web_url","action_url":"https://example.com/a","title":"Website"}]`)
	out = xmaCTAButtonsHTML(links, buttons.Postbacks, buttons.QuickReplies, CTAOptions{})
	if !strings.Contains(out, `<a href="https://example.com/a">[`+" "+`Website`+" "+`]</a>`) || !strings.Contains(out, `<br>[`+" "+`Following`) || strings.Contains(out, "<ul>") || strings.Contains(out, "<ol>") {
		t.Errorf("links and taps must be pseudo-buttons, got %s", out)
	}
	if !HasKnownCTAFooter(out) {
		t.Errorf("footer not detected")
	}
	if got := ReplaceOrAppendCTAButtonsHTML("<p>Hi</p>"+out, "NEW"); got != "<p>Hi</p>NEW" {
		t.Errorf("block not replaced: %s", got)
	}
}

func TestKeycapReactionIndex(t *testing.T) {
	for emoji, want := range map[string]int{"1️⃣": 1, "1⃣": 1, "9️⃣": 9, "🔟": 10, "5⃣": 5} {
		if got, ok := KeycapReactionIndex(emoji); !ok || got != want {
			t.Errorf("%q = %d, %v; want %d", emoji, got, ok, want)
		}
	}
	for _, emoji := range []string{"❤", "❤️", "0️⃣", "👍", "", "1", "12⃣", "a⃣"} {
		if got, ok := KeycapReactionIndex(emoji); ok {
			t.Errorf("%q must not be a button, got %d", emoji, got)
		}
	}
	if KeycapEmoji(3) != "3️⃣" || KeycapEmoji(10) != "🔟" || KeycapEmoji(11) != "" || KeycapEmoji(0) != "" {
		t.Error("unexpected KeycapEmoji")
	}
}

func TestCombinedListMatching(t *testing.T) {
	all := Buttons{
		Postbacks:    []*metaid.PostbackButton{pb("Following", "payload-a")},
		QuickReplies: []*metaid.PostbackButton{{Title: "Maybe later", Payload: "payload-qr-1", CTAType: "quick_reply"}},
	}.All()
	if got, err := MatchUserPostback(all, "2", true); err != nil || !got.IsQuickReply() {
		t.Errorf("reply 2 = %+v, %v", got, err)
	}
	if got, err := MatchUserPostback(all, "maybe LATER", false); err != nil || got.Payload != "payload-qr-1" {
		t.Errorf("title = %+v, %v", got, err)
	}
	if got, err := MatchUserPostback(all, "following", false); err != nil || got.IsQuickReply() {
		t.Errorf("postback title = %+v, %v", got, err)
	}
	if _, err := MatchUserPostback(all, "2", false); !errors.Is(err, ErrPostbackNotFound) {
		t.Errorf("bare number outside reply must not match: %v", err)
	}
	if got := ButtonAtIndex(all, 2); got == nil || got.Title != "Maybe later" || ButtonAtIndex(all, 3) != nil || ButtonAtIndex(all, 0) != nil {
		t.Errorf("unexpected ButtonAtIndex")
	}
}

func TestPickButtonsTextQuickReplies(t *testing.T) {
	var resp slidetypes.ThreadDetailsResponse
	raw := `{"thread":{"items":[
		{"item_id":"1","message_id":"mid.$T","item_type":"text","quick_replies":[
			{"title":"Yes","payload":"payload-qr-1","content_type":"text"},{"title":"Maybe later"}]},
		{"item_id":"2","item_type":"text"}]}}`
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	items := resp.ButtonItems()
	got := PickButtons(items, "mid.$T", nil, 0)
	if len(got.QuickReplies) != 2 || len(got.Postbacks) != 0 || got.QuickReplies[0].Payload != "payload-qr-1" {
		t.Errorf("unexpected buttons: %+v", got)
	}
	if got := PickButtons(items, "", []string{"Yes", "Maybe later"}, 0); len(got.QuickReplies) != 2 {
		t.Errorf("title match failed: %+v", got)
	}
	if got := PickPostbackButtons(items, "mid.$T", nil, 0); len(got) != 0 {
		t.Errorf("quick replies leaked into postbacks: %+v", got)
	}
}

func TestAppendButtonsToText(t *testing.T) {
	part := &bridgev2.ConvertedMessagePart{Type: event.EventMessage, Content: &event.MessageEventContent{MsgType: event.MsgText, Body: "Pick one"}}
	appendButtonsToText(part, Buttons{QuickReplies: []*metaid.PostbackButton{{Title: "Yes", Payload: "payload-qr-1", CTAType: "quick_reply"}}}, CTAOptions{})
	if !strings.Contains(part.Content.FormattedBody, `[`+" "+`Yes`+" "+`]`) || !strings.Contains(part.Content.Body, "Pick one") || !strings.Contains(part.Content.Body, "[") {
		t.Errorf("unexpected content: %+v", part.Content)
	}
	meta, ok := part.DBMetadata.(*metaid.MessageMetadata)
	if !ok || len(meta.QuickReplies) != 1 || len(meta.PostbackButtons) != 0 {
		t.Errorf("unexpected metadata: %+v", part.DBMetadata)
	}
	plain := &bridgev2.ConvertedMessagePart{Content: &event.MessageEventContent{Body: "Hi"}}
	appendButtonsToText(plain, Buttons{}, CTAOptions{})
	if plain.Content.FormattedBody != "" || plain.DBMetadata != nil {
		t.Errorf("text without buttons must stay untouched")
	}
}

func TestRecoverTextQuickRepliesFromUnrecognized(t *testing.T) {
	mc := &MessageConverter{}
	msg := &slidetypes.Message{MessageID: "mid.$T", Unrecognized: map[string]any{"quick_replies": []any{
		map[string]any{"title": "Yes", "payload": "payload-qr-1"}, map[string]any{"title": "Maybe later"}}}}
	got := mc.recoverTextQuickReplies(context.Background(), msg, nil)
	if len(got.QuickReplies) != 2 {
		t.Errorf("unexpected: %+v", got)
	}
	content := map[string]any{"quick_replies": []any{map[string]any{"title": "Nested"}}}
	if got := mc.recoverTextQuickReplies(context.Background(), &slidetypes.Message{}, content); len(got.QuickReplies) != 1 {
		t.Errorf("content unrecognized: %+v", got)
	}
	if got := mc.recoverTextQuickReplies(context.Background(), &slidetypes.Message{}, nil); !got.Empty() {
		t.Errorf("plain text: %+v", got)
	}
}
