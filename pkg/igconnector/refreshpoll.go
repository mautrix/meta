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

package igconnector

import (
	"context"
	"fmt"
	"slices"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

// refreshPollQuestion is the poll question for an old card: the first line of its text in the
// fetched thread history, or the fallback question if the history has no such text.
func refreshPollQuestion(items []*slidetypes.ThreadItem, card *database.Message) string {
	itemID := igconv.ItemIDFromMessageID(card.ID)
	for _, item := range items {
		if !item.MatchesID(itemID) {
			continue
		}
		bodies := []string{item.Text}
		if cardHTML := igconv.CardHTMLFromThreadItem(item, igconv.CTAOptions{}); cardHTML != "" {
			bodies = []string{format.HTMLToContent(cardHTML).Body, item.Text}
		}
		for _, body := range bodies {
			if question := igconv.ButtonPollQuestion(body); question != igconv.ButtonPollFallbackQuestion {
				return question
			}
		}
	}
	return igconv.ButtonPollFallbackQuestion
}

// attachButtonPoll sends the button poll of an old card that has none, as the sender of the card,
// and saves it next to the card. It does nothing if the card has no stored buttons or already
// has a poll, so refreshing again never adds a second one. It only sends the poll start to
// Matrix: it never votes, ends a poll or sends anything to Instagram. The bool is true if a poll was added.
func (ic *IGConnector) attachButtonPoll(ctx context.Context, portal *bridgev2.Portal, login *bridgev2.UserLogin, items []*slidetypes.ThreadItem, card *database.Message) (bool, error) {
	meta, ok := card.Metadata.(*metaid.MessageMetadata)
	if !ok || meta == nil || len(meta.TapButtons()) == 0 || isMatrixOnlyButtonPart(card) || card.MXID == "" || portal.MXID == "" {
		return false, nil
	}
	ic.pollAttachLock.Lock()
	defer ic.pollAttachLock.Unlock()
	key := tapKey(card.Room, card.ID, 0, "")
	if _, done := ic.pollAttached[key]; done {
		return false, nil
	}
	parts, err := ic.Bridge.DB.Message.GetAllPartsByID(ctx, portal.Receiver, card.ID)
	if err != nil {
		return false, fmt.Errorf("failed to look up message parts: %w", err)
	}
	if slices.ContainsFunc(parts, func(part *database.Message) bool { return part.PartID == metaid.ButtonPollPartID }) {
		return false, nil
	}
	intent, err := messageSenderIntent(ctx, ic.Bridge, portal, login, card)
	if err != nil || intent == nil {
		return false, fmt.Errorf("failed to get the card sender's intent: %w", err)
	}
	part := igconv.NewButtonPollPart(refreshPollQuestion(items, card), meta.PostbackButtons, meta.QuickReplies)
	resp, err := intent.SendMessage(ctx, portal.MXID, part.Type, &event.Content{Parsed: part.Content, Raw: part.Extra}, &bridgev2.MatrixSendExtra{
		// Place the poll right after the card instead of at the end of an old chat.
		Timestamp: card.Timestamp.Add(time.Millisecond),
	})
	if err != nil {
		return false, fmt.Errorf("failed to send the button poll: %w", err)
	}
	if ic.pollAttached == nil {
		ic.pollAttached = make(map[string]struct{})
	}
	ic.pollAttached[key] = struct{}{}
	err = ic.Bridge.DB.Message.Insert(ctx, &database.Message{
		ID:               card.ID,
		PartID:           part.ID,
		MXID:             resp.EventID,
		Room:             card.Room,
		SenderID:         card.SenderID,
		SenderMXID:       card.SenderMXID,
		Timestamp:        card.Timestamp,
		IsDoublePuppeted: card.IsDoublePuppeted,
		Metadata:         part.DBMetadata,
	})
	if err != nil {
		return false, fmt.Errorf("failed to save the button poll: %w", err)
	}
	return true, nil
}
