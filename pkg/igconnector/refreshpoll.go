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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

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

// buttonPollInsertAttempts is how many times the row of a poll that was already sent is inserted
// before the poll is parked in the pending marker.
const buttonPollInsertAttempts = 3

// waitBeforeButtonPollInsertRetry pauses between two insert attempts. It is a variable so tests
// can retry without waiting.
var waitBeforeButtonPollInsertRetry = func(ctx context.Context, attempt int) {
	timer := time.NewTimer(time.Duration(attempt) * 200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// pendingButtonPoll is the durable record of a button poll that reached Matrix but whose database
// row couldn't be saved. It holds everything the row needs, so a later refresh, even after a
// restart, finishes the insert and never sends the poll a second time.
type pendingButtonPoll struct {
	EventID          id.EventID              `json:"event_id"`
	PartID           networkid.PartID        `json:"part_id"`
	PortalID         networkid.PortalID      `json:"portal_id"`
	Receiver         networkid.UserLoginID   `json:"receiver"`
	MessageID        networkid.MessageID     `json:"message_id"`
	SenderID         networkid.UserID        `json:"sender_id"`
	SenderMXID       id.UserID               `json:"sender_mxid,omitempty"`
	Timestamp        time.Time               `json:"timestamp"`
	IsDoublePuppeted bool                    `json:"is_double_puppeted,omitempty"`
	Metadata         *metaid.MessageMetadata `json:"metadata"`
}

func newPendingButtonPoll(card *database.Message, partID networkid.PartID, pollMeta *metaid.MessageMetadata, eventID id.EventID) *pendingButtonPoll {
	return &pendingButtonPoll{
		EventID:          eventID,
		PartID:           partID,
		PortalID:         card.Room.ID,
		Receiver:         card.Room.Receiver,
		MessageID:        card.ID,
		SenderID:         card.SenderID,
		SenderMXID:       card.SenderMXID,
		Timestamp:        card.Timestamp,
		IsDoublePuppeted: card.IsDoublePuppeted,
		Metadata:         pollMeta,
	}
}

// message is the database row of the poll.
func (p *pendingButtonPoll) message() *database.Message {
	return &database.Message{
		ID:               p.MessageID,
		PartID:           p.PartID,
		MXID:             p.EventID,
		Room:             networkid.PortalKey{ID: p.PortalID, Receiver: p.Receiver},
		SenderID:         p.SenderID,
		SenderMXID:       p.SenderMXID,
		Timestamp:        p.Timestamp,
		IsDoublePuppeted: p.IsDoublePuppeted,
		Metadata:         p.Metadata,
	}
}

// pendingButtonPollKey is the bridge KV key of the pending record of a card's poll. It is a hash
// because portal and message IDs contain separators of their own.
func pendingButtonPollKey(card *database.Message) database.Key {
	sum := sha256.Sum256([]byte(tapKey(card.Room, card.ID, 0, "")))
	return database.Key("ig_button_poll_pending:" + hex.EncodeToString(sum[:]))
}

// loadPendingButtonPoll returns the pending record of a card's poll, or nil if it has none. A
// record that can't be read is an error, not "none": sending anyway could duplicate the poll.
func (ic *IGConnector) loadPendingButtonPoll(ctx context.Context, card *database.Message) (*pendingButtonPoll, error) {
	key := pendingButtonPollKey(card)
	raw, err := ic.Bridge.DB.KV.GetWithError(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read the pending button poll: %w", err)
	}
	var pending pendingButtonPoll
	if err = json.Unmarshal([]byte(raw), &pending); err != nil {
		return nil, fmt.Errorf("pending button poll %q is unreadable: %w", key, err)
	} else if pending.EventID == "" || pending.PartID != metaid.ButtonPollPartID || pending.MessageID != card.ID || pending.Metadata == nil {
		return nil, fmt.Errorf("pending button poll %q is incomplete", key)
	}
	return &pending, nil
}

func (ic *IGConnector) savePendingButtonPoll(ctx context.Context, card *database.Message, pending *pendingButtonPoll) error {
	raw, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return ic.Bridge.DB.KV.SetWithError(ctx, pendingButtonPollKey(card), string(raw))
}

// insertButtonPollRow saves the row of a poll that was already sent. A few quick retries ride out
// a busy database, because a failure here is what leaves a sent poll without a row.
func (ic *IGConnector) insertButtonPollRow(ctx context.Context, row *database.Message) error {
	var err error
	for attempt := 1; attempt <= buttonPollInsertAttempts; attempt++ {
		if err = ic.Bridge.DB.Message.Insert(ctx, row); err == nil {
			return nil
		}
		if attempt < buttonPollInsertAttempts {
			waitBeforeButtonPollInsertRetry(ctx, attempt)
		}
	}
	return err
}

// attachButtonPoll sends the button poll of an old card that has none, as the sender of the card,
// and saves it next to the card. It does nothing if the card has no stored buttons or already
// has a poll, so refreshing again never adds a second one. It only sends the poll start to
// Matrix: it never votes, ends a poll or sends anything to Instagram. The bool is true if a poll was added.
//
// A poll that was sent is remembered durably. If its row can't be saved, a pending record in the
// bridge KV store keeps the event ID and metadata, and the next attach, even in a new process,
// finishes the insert without sending again. pollAttached is only a same-process shortcut.
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
	pending, err := ic.loadPendingButtonPoll(ctx, card)
	if err != nil {
		return false, err
	}
	sentNow := pending == nil
	if sentNow {
		if pending, err = ic.sendButtonPoll(ctx, portal, login, items, card, meta); err != nil {
			return false, err
		}
	}
	if err = ic.insertButtonPollRow(ctx, pending.message()); err != nil {
		if !sentNow {
			return false, fmt.Errorf("failed to save the button poll that was already sent: %w", err)
		}
		// Park the sent poll even if the request was cancelled, so it is never sent again.
		if markErr := ic.savePendingButtonPoll(context.WithoutCancel(ctx), card, pending); markErr != nil {
			// Nothing durable is left. At least this process won't send another one.
			if ic.pollAttached == nil {
				ic.pollAttached = make(map[string]struct{})
			}
			ic.pollAttached[key] = struct{}{}
			return false, fmt.Errorf("the button poll was sent, but neither its row (%w) nor a pending record (%v) could be saved", err, markErr)
		}
		return false, fmt.Errorf("the button poll was sent, but its row couldn't be saved; the next refresh will save it: %w", err)
	}
	if ic.pollAttached == nil {
		ic.pollAttached = make(map[string]struct{})
	}
	ic.pollAttached[key] = struct{}{}
	if !sentNow {
		if err = ic.Bridge.DB.KV.DeleteWithError(ctx, pendingButtonPollKey(card)); err != nil {
			// The row exists now, so the stale record is never read again.
			zerolog.Ctx(ctx).Warn().Err(err).Str("message_id", string(card.ID)).Msg("Failed to delete the pending button poll record")
		}
	}
	return true, nil
}

// sendButtonPoll sends the poll start of a card to Matrix as the sender of the card. It saves nothing.
func (ic *IGConnector) sendButtonPoll(ctx context.Context, portal *bridgev2.Portal, login *bridgev2.UserLogin, items []*slidetypes.ThreadItem, card *database.Message, meta *metaid.MessageMetadata) (*pendingButtonPoll, error) {
	intent, err := messageSenderIntent(ctx, ic.Bridge, portal, login, card)
	if err != nil || intent == nil {
		return nil, fmt.Errorf("failed to get the card sender's intent: %w", err)
	}
	part := igconv.NewButtonPollPart(refreshPollQuestion(items, card), meta.PostbackButtons, meta.QuickReplies)
	pollMeta, ok := part.DBMetadata.(*metaid.MessageMetadata)
	if !ok || pollMeta == nil {
		// Checked before sending: after a send, the event ID must never be lost.
		return nil, fmt.Errorf("button poll part has metadata of type %T", part.DBMetadata)
	}
	resp, err := intent.SendMessage(ctx, portal.MXID, part.Type, &event.Content{Parsed: part.Content, Raw: part.Extra}, &bridgev2.MatrixSendExtra{
		// Place the poll right after the card instead of at the end of an old chat.
		Timestamp: card.Timestamp.Add(time.Millisecond),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send the button poll: %w", err)
	}
	return newPendingButtonPoll(card, part.ID, pollMeta, resp.EventID), nil
}
