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
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

// newestPostbackMessage returns the newest of the given messages that has stored postback buttons.
func newestPostbackMessage(msgs []*database.Message) *database.Message {
	var newest *database.Message
	for _, msg := range msgs {
		meta, ok := msg.Metadata.(*metaid.MessageMetadata)
		if !ok || len(meta.PostbackButtons) == 0 {
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
		if meta, ok := replyTo.Metadata.(*metaid.MessageMetadata); ok && len(meta.PostbackButtons) > 0 {
			button, err := igconv.MatchUserPostback(meta.PostbackButtons, text, true)
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
	button, err := igconv.MatchUserPostback(target.Metadata.(*metaid.MessageMetadata).PostbackButtons, text, false)
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
	clientContext, err := ic.Client.SendPostback(ctx, portalMeta.IGID, button.Title, button.Payload)
	if err != nil {
		return nil, true, wrapInstagramError(err)
	}
	zerolog.Ctx(ctx).Debug().Msg("Sent Matrix message as a postback button tap")
	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        metaid.MakeFBMessageID("postback-" + clientContext),
			SenderID:  networkid.UserID(ic.UserLogin.ID),
			Timestamp: time.UnixMilli(msg.Event.Timestamp),
		},
	}, true, nil
}
