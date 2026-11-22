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
	"slices"
	"strings"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func textMessageWithQuickReplies(text string, titles ...string) *slidetypes.Message {
	qrs := make([]any, len(titles))
	for i, title := range titles {
		qrs[i] = map[string]any{"title": title, "payload": "payload-" + title}
	}
	msg := &slidetypes.Message{MessageID: "mid.$T"}
	msg.Content.Content = &slidetypes.MessageContentText{TextBody: text}
	if len(qrs) > 0 {
		msg.Unrecognized = map[string]any{"quick_replies": qrs}
	}
	return msg
}

func convertForPollTest(msg *slidetypes.Message) *bridgev2.ConvertedMessage {
	return (&MessageConverter{}).ToMatrix(context.Background(), nil, nil, nil, nil, "mid:T", msg, true)
}

func pollStartOf(t *testing.T, part *bridgev2.ConvertedMessagePart) event.PollStart {
	t.Helper()
	start, ok := part.Extra["org.matrix.msc3381.poll.start"].(event.PollStart)
	if !ok {
		t.Fatalf("poll part has no poll.start: %+v", part.Extra)
	}
	return start
}

func TestButtonPollAfterCard(t *testing.T) {
	cm := convertForPollTest(textMessageWithQuickReplies("Quer o guia?", "Sim", "Não"))
	if len(cm.Parts) != 2 {
		t.Fatalf("expected card and poll, got %d parts", len(cm.Parts))
	}
	card, poll := cm.Parts[0], cm.Parts[1]
	wantCard := "Quer o guia?" + xmaCTAButtonsHTML(nil, nil, []*metaid.PostbackButton{
		{Title: "Sim", Payload: "payload-Sim", CTAType: metaid.CTATypeQuickReply},
		{Title: "Não", Payload: "payload-Não", CTAType: metaid.CTATypeQuickReply},
	}, CTAOptions{})
	if card.Type != event.EventMessage || card.ID != "" || card.Content.FormattedBody != wantCard {
		t.Errorf("card changed: type=%v id=%q html=%q want html %q", card.Type, card.ID, card.Content.FormattedBody, wantCard)
	}
	if card.DBMetadata.(*metaid.MessageMetadata).ButtonPoll {
		t.Error("card must not be flagged as a button poll")
	}

	if poll.ID != metaid.ButtonPollPartID || poll.Type != event.EventUnstablePollStart {
		t.Errorf("poll part id=%q type=%v", poll.ID, poll.Type)
	}
	start := pollStartOf(t, poll)
	if start.Question.Text != "Quer o guia?" || start.MaxSelections != 1 || start.Kind != "org.matrix.msc3381.poll.disclosed" {
		t.Errorf("unexpected poll start: %+v", start)
	}
	if len(start.Answers) != 2 || start.Answers[0].Text != "Sim" || start.Answers[1].Text != "Não" ||
		start.Answers[0].ID != "1" || start.Answers[1].ID != "2" {
		t.Errorf("unexpected answers: %+v", start.Answers)
	}
	if !strings.Contains(poll.Content.Body, "Quer o guia?") || !strings.Contains(poll.Content.FormattedBody, "<li>Sim</li>") {
		t.Errorf("unexpected fallback content: %+v", poll.Content)
	}
	meta := poll.DBMetadata.(*metaid.MessageMetadata)
	if !meta.ButtonPoll || meta.PollID != 0 || len(meta.PostbackButtons) != 0 || len(meta.QuickReplies) != 2 ||
		meta.QuickReplies[1].Payload != "payload-Não" {
		t.Errorf("unexpected poll metadata: %+v", meta)
	}
}

func TestButtonPollSitsRightAfterCardBeforeTrailingText(t *testing.T) {
	msg := &slidetypes.Message{MessageID: "mid.$T"}
	msg.Content.Content = &slidetypes.MessageContentXMA{
		XMATextBody: "Texto depois do cartão",
		XMA: &slidetypes.XMAContent{
			TitleText: "Quer o guia?", TargetID: "123", TargetURL: "https://www.instagram.com/reel/x/",
			CTAButtons: parseCTAs(t, `[{"cta_type":"postback","title":"Sim","payload":"payload-a"}]`),
		},
	}
	cm := convertForPollTest(msg)
	if len(cm.Parts) != 3 {
		t.Fatalf("expected card, poll and trailing text, got %d parts", len(cm.Parts))
	}
	card, poll, text := cm.Parts[0], cm.Parts[1], cm.Parts[2]
	if len(card.DBMetadata.(*metaid.MessageMetadata).TapButtons()) != 1 || card.Type != event.EventMessage {
		t.Errorf("part 0 must be the card with the buttons: %+v", card)
	}
	if poll.ID != metaid.ButtonPollPartID || poll.Type != event.EventUnstablePollStart {
		t.Errorf("the poll must sit immediately after the card, got part 1 id=%q type=%v", poll.ID, poll.Type)
	}
	if text.ID != "xma-text-body" || !strings.Contains(text.Content.Body, "Texto depois do cartão") {
		t.Errorf("the trailing text must stay after the poll: id=%q body=%q", text.ID, text.Content.Body)
	}
	if start := pollStartOf(t, poll); start.Question.Text != "Quer o guia?" || len(start.Answers) != 1 {
		t.Errorf("unexpected poll start: %+v", start)
	}
}

func TestInsertButtonPollKeepsPartsBeforeTheCardInPlace(t *testing.T) {
	button := []*metaid.PostbackButton{{Title: "Sim", Payload: "payload-a", CTAType: metaid.CTATypeQuickReply}}
	text := func(id, body string) *bridgev2.ConvertedMessagePart {
		return &bridgev2.ConvertedMessagePart{ID: networkid.PartID(id), Type: event.EventMessage, Content: &event.MessageEventContent{Body: body}}
	}
	cm := &bridgev2.ConvertedMessage{Parts: []*bridgev2.ConvertedMessagePart{
		text("before", "Antes"),
		{ID: "card", Type: event.EventMessage, Content: &event.MessageEventContent{Body: "Cartão"}, DBMetadata: &metaid.MessageMetadata{QuickReplies: button}},
		text("after", "Depois"),
	}}
	insertButtonPoll(cm)
	var ids []networkid.PartID
	for _, part := range cm.Parts {
		ids = append(ids, part.ID)
	}
	if want := []networkid.PartID{"before", "card", metaid.ButtonPollPartID, "after"}; !slices.Equal(ids, want) {
		t.Errorf("part order %v, want %v", ids, want)
	}
}

func TestButtonPollOnlyWithButtons(t *testing.T) {
	cm := convertForPollTest(textMessageWithQuickReplies("Olá"))
	if len(cm.Parts) != 1 || cm.Parts[0].Type != event.EventMessage {
		t.Errorf("a message without buttons must stay a single part, got %d", len(cm.Parts))
	}
	cm = convertForPollTest(textMessageWithQuickReplies("Só uma", "Única"))
	if len(cm.Parts) != 2 || len(pollStartOf(t, cm.Parts[1]).Answers) != 1 {
		t.Errorf("a single button must still get a poll, got %d parts", len(cm.Parts))
	}
}

func TestButtonPollQuestionFallbackAndOptionIDs(t *testing.T) {
	cm := convertForPollTest(textMessageWithQuickReplies("", "Igual", "Igual"))
	start := pollStartOf(t, cm.Parts[len(cm.Parts)-1])
	if start.Question.Text != ButtonPollFallbackQuestion || ButtonPollFallbackQuestion != "Responder" {
		t.Errorf("empty card text must ask %q, got %q", ButtonPollFallbackQuestion, start.Question.Text)
	}
	if len(start.Answers) != 2 || start.Answers[0].ID == start.Answers[1].ID || start.Answers[0].Text != start.Answers[1].Text {
		t.Errorf("equal titles need distinct option IDs: %+v", start.Answers)
	}
}

func TestButtonPollQuestionPicksCardText(t *testing.T) {
	long := strings.Repeat("palavra ", 40)
	parts := func(body string) []*bridgev2.ConvertedMessagePart {
		return []*bridgev2.ConvertedMessagePart{{Type: event.EventMessage, Content: &event.MessageEventContent{Body: body}}}
	}
	for name, tt := range map[string]struct {
		body string
		want string
	}{
		"skips blank, quote and button lines": {"\n> Reply\n**Título**\n[ Ir ]", "Título"},
		"skips the app fallback":              {appFallbackSnippet + "\nTexto", "Texto"},
		"only buttons":                        {"[ Ir ]", ButtonPollFallbackQuestion},
	} {
		if got := buttonPollQuestion(parts(tt.body)); got != tt.want {
			t.Errorf("%s: got %q, want %q", name, got, tt.want)
		}
	}
	if got := []rune(buttonPollQuestion(parts(long))); len(got) != buttonPollMaxQuestionRunes || got[len(got)-1] != '…' {
		t.Errorf("long question must be cut to %d runes, got %d", buttonPollMaxQuestionRunes, len(got))
	}
	media := []*bridgev2.ConvertedMessagePart{{Type: event.EventMessage, Content: &event.MessageEventContent{Body: "photo.jpg", FileName: "photo.jpg"}}}
	if got := buttonPollQuestion(media); got != ButtonPollFallbackQuestion {
		t.Errorf("a file name is not card text, got %q", got)
	}
}

func TestButtonPollIndex(t *testing.T) {
	for id, want := range map[string]int{"1": 1, "3": 3} {
		if got, ok := ButtonPollIndex(id, 3); !ok || got != want {
			t.Errorf("%q = %d, %v", id, got, ok)
		}
	}
	for _, id := range []string{"", "0", "4", "01", "+1", "-1", "1 ", "a"} {
		if got, ok := ButtonPollIndex(id, 3); ok {
			t.Errorf("%q must be rejected, got %d", id, got)
		}
	}
}
