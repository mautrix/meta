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
	"fmt"
	"slices"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	// ButtonPollFallbackQuestion is the poll question of a card that has no usable text.
	ButtonPollFallbackQuestion = "Responder"
	buttonPollMaxQuestionRunes = 120
	buttonPollKind             = "org.matrix.msc3381.poll.disclosed"
)

// ButtonPollOptionID is the poll answer ID of the 1-based button index. Titles can repeat, so the
// ID is the index and not the title.
func ButtonPollOptionID(idx int) string {
	return strconv.Itoa(idx)
}

// ButtonPollIndex is the inverse of ButtonPollOptionID. It only accepts the canonical form of an
// index that exists in a list of count buttons.
func ButtonPollIndex(answerID string, count int) (int, bool) {
	idx, err := strconv.Atoi(answerID)
	if err != nil || idx < 1 || idx > count || ButtonPollOptionID(idx) != answerID {
		return 0, false
	}
	return idx, true
}

// insertButtonPoll adds the Matrix-only poll that mirrors the buttons of the first part that has
// any, immediately after that part. Parts after the card, like the text of an XMA message, move
// down behind the poll, so the poll never ends up after text that came later. The card parts
// stay as they are.
func insertButtonPoll(cm *bridgev2.ConvertedMessage) {
	for i, part := range cm.Parts {
		meta, ok := part.DBMetadata.(*metaid.MessageMetadata)
		if part.DontBridge || !ok || len(meta.TapButtons()) == 0 {
			continue
		}
		poll := NewButtonPollPart(buttonPollQuestion(cm.Parts), meta.PostbackButtons, meta.QuickReplies)
		cm.Parts = slices.Insert(cm.Parts, i+1, poll)
		return
	}
}

// NewButtonPollPart builds the poll start part for a card's buttons. A vote on it is a button tap
// and is never forwarded to Instagram as a poll, which is what metaid.MessageMetadata.ButtonPoll records.
func NewButtonPollPart(question string, postbacks, quickReplies []*metaid.PostbackButton) *bridgev2.ConvertedMessagePart {
	buttons := metaid.TapButtons(postbacks, quickReplies)
	answers := make([]event.PollOption, len(buttons))
	lines := make([]string, len(buttons))
	htmlLines := make([]string, len(buttons))
	for i, button := range buttons {
		title := button.Title
		if title == "" {
			title = strconv.Itoa(i + 1)
		}
		answers[i] = event.PollOption{ID: ButtonPollOptionID(i + 1), MSC1767Message: event.MSC1767Message{Text: title}}
		lines[i] = fmt.Sprintf("%d. %s", i+1, title)
		htmlLines[i] = "<li>" + event.TextToHTML(title) + "</li>"
	}
	body := question + "\n\n" + strings.Join(lines, "\n")
	formatted := "<p>" + event.TextToHTML(question) + "</p><ol>" + strings.Join(htmlLines, "") + "</ol>"
	return &bridgev2.ConvertedMessagePart{
		ID:   metaid.ButtonPollPartID,
		Type: event.EventUnstablePollStart,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgText,
			Body:          body,
			Format:        event.FormatHTML,
			FormattedBody: formatted,
		},
		Extra: map[string]any{
			"org.matrix.msc1767.message": []event.ExtensibleText{
				{MimeType: "text/plain", Body: body},
				{MimeType: "text/html", Body: formatted},
			},
			"org.matrix.msc3381.poll.start": event.PollStart{
				Kind:          buttonPollKind,
				MaxSelections: 1,
				Question:      event.MSC1767Message{Text: question},
				Answers:       answers,
			},
		},
		DBMetadata: &metaid.MessageMetadata{
			ButtonPoll:      true,
			PostbackButtons: slices.Clone(postbacks),
			QuickReplies:    slices.Clone(quickReplies),
		},
	}
}

// ButtonPollQuestion is the poll question for a card whose plain text is body.
func ButtonPollQuestion(body string) string {
	return buttonPollQuestion([]*bridgev2.ConvertedMessagePart{{Type: event.EventMessage, Content: &event.MessageEventContent{Body: body}}})
}

// buttonPollQuestion is the first line of card text that is not a button, quote or media file name.
func buttonPollQuestion(parts []*bridgev2.ConvertedMessagePart) string {
	for _, part := range parts {
		if part.DontBridge || part.Content == nil || part.Type != event.EventMessage {
			continue
		}
		if part.Content.FileName != "" && part.Content.Body == part.Content.FileName {
			continue
		}
		for _, line := range strings.Split(part.Content.Body, "\n") {
			line = strings.TrimSpace(strings.ReplaceAll(line, "**", ""))
			if line == "" || strings.HasPrefix(line, ">") ||
				strings.Contains(line, "["+nbsp) || strings.Contains(line, appFallbackSnippet) {
				continue
			}
			if runes := []rune(line); len(runes) > buttonPollMaxQuestionRunes {
				line = strings.TrimSpace(string(runes[:buttonPollMaxQuestionRunes-1])) + "…"
			}
			return line
		}
	}
	return ButtonPollFallbackQuestion
}
