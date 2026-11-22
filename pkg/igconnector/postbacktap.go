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
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

// pendingPostbackTTL is how long a tap OTID is remembered if no echo arrives.
const pendingPostbackTTL = 2 * time.Minute

// isButtonPoll reports whether the message is the Matrix-only poll that mirrors the buttons of
// a card. It stores the same buttons as the card but isn't the card.
func isButtonPoll(msg *database.Message) bool {
	meta, ok := msg.Metadata.(*metaid.MessageMetadata)
	return ok && meta != nil && meta.ButtonPoll
}

// newestPostbackMessage returns the newest of the given messages that has stored postback or
// quick reply buttons. Button polls are skipped: their card is the message to tap.
func newestPostbackMessage(msgs []*database.Message) *database.Message {
	var newest *database.Message
	for _, msg := range msgs {
		meta, ok := msg.Metadata.(*metaid.MessageMetadata)
		if !ok || len(meta.TapButtons()) == 0 || meta.ButtonPoll {
			continue
		}
		if newest == nil || msg.Timestamp.After(newest.Timestamp) {
			newest = msg
		}
	}
	return newest
}

// selectPostbackForText decides whether a text message from the user is a button tap.
// If it replies to a message with buttons, number or title match; otherwise only a unique
// title of the newest recent card with buttons matches. A nil result means "send as text".
func selectPostbackForText(
	replyTo *database.Message,
	loadRecent func() ([]*database.Message, error),
	text string,
) *metaid.PostbackButton {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if replyTo != nil {
		if meta, ok := replyTo.Metadata.(*metaid.MessageMetadata); ok && len(meta.TapButtons()) > 0 {
			button, err := igconv.MatchUserPostback(meta.TapButtons(), text, true)
			if err != nil {
				return nil
			}
			return button
		}
	}
	recent, err := loadRecent()
	if err != nil {
		return nil
	}
	target := newestPostbackMessage(recent)
	if target == nil {
		return nil
	}
	button, err := igconv.MatchUserPostback(target.Metadata.(*metaid.MessageMetadata).TapButtons(), text, false)
	if err != nil {
		return nil
	}
	return button
}

// handleMatrixPostbackTap sends a postback instead of a text message if the user's message
// is a reply to, or the title of, a card button. The handled return value is true if the
// message must not be sent to Instagram as text.
func (ic *IGClient) handleMatrixPostbackTap(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, bool, error) {
	if msg.OrigSender != nil || msg.Content == nil {
		return nil, false, nil
	}
	switch msg.Content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
	default:
		return nil, false, nil
	}
	button := selectPostbackForText(msg.ReplyTo, func() ([]*database.Message, error) {
		return ic.Main.Bridge.DB.Message.GetLastNInPortal(ctx, msg.Portal.PortalKey, tapSearchMessageCount)
	}, msg.Content.Body)
	if button == nil {
		return nil, false, nil
	}
	portalMeta, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		return nil, true, err
	}
	otid := strconv.FormatInt(getOTID(msg.InputTransactionID), 10)
	txnID := networkid.TransactionID(otid)
	ic.addPendingTapOTID(otid)
	// Instagram's message ID isn't known until the echo arrives, so save nothing now and let
	// the framework save the Matrix event under the real ID when the echo with this transaction ID shows up.
	msg.AddPendingToSave(nil, txnID, nil)
	if _, err = ic.sendButtonTap(ctx, portalMeta.IGID, button, otid); err != nil {
		msg.RemovePending(txnID)
		ic.pendingPostbackOTIDs.Remove(otid)
		return nil, true, wrapInstagramError(err)
	}
	zerolog.Ctx(ctx).Debug().Msg("Sent Matrix message as a button tap")
	return &bridgev2.MatrixMessageResponse{Pending: true}, true, nil
}

// postbackTransactionID returns the transaction ID to attach to the echo of a postback tap
// sent from Matrix, or an empty ID for every other message.
func (ic *IGClient) postbackTransactionID(msg *slidetypes.Message) networkid.TransactionID {
	if msg == nil || !ic.takePendingPostbackOTID(msg.OfflineThreadingID) {
		return ""
	}
	return networkid.TransactionID(msg.OfflineThreadingID)
}

// takePendingPostbackOTID reports whether the OTID belongs to a pending postback tap, and forgets it.
func (ic *IGClient) takePendingPostbackOTID(otid string) bool {
	if otid == "" || !ic.pendingPostbackOTIDs.Has(otid) {
		return false
	}
	ic.pendingPostbackOTIDs.Remove(otid)
	return true
}

// addPendingTapOTID remembers the OTID of a tap sent from Matrix so its echo can be correlated.
func (ic *IGClient) addPendingTapOTID(otid string) {
	ic.pendingPostbackOTIDs.Add(otid)
	time.AfterFunc(pendingPostbackTTL, func() { ic.pendingPostbackOTIDs.Remove(otid) })
}

// sendButtonTap activates a button: postbacks are sent as postbacks, quick replies as text
// with the title. An empty clientContext generates one.
func (ic *IGClient) sendButtonTap(ctx context.Context, threadID string, button *metaid.PostbackButton, clientContext string) (string, error) {
	if ic.sendTap != nil {
		return ic.sendTap(ctx, threadID, button, clientContext)
	}
	if button.IsQuickReply() {
		return ic.Client.SendQuickReplyWithContext(ctx, threadID, button.Title, button.Payload, clientContext)
	}
	return ic.Client.SendPostbackWithContext(ctx, threadID, button.Title, button.Payload, clientContext)
}
