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
	"fmt"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/msgconv/textfmt"
)

func (mc *MessageConverter) getBasicUserInfo(ctx context.Context, user networkid.UserID) (id.UserID, string, error) {
	ghost, err := mc.Bridge.GetGhostByID(ctx, user)
	if err != nil {
		return "", "", fmt.Errorf("failed to get ghost by ID: %w", err)
	}
	login := mc.Bridge.GetCachedUserLoginByID(networkid.UserLoginID(user))
	if login != nil {
		return login.UserMXID, ghost.Name, nil
	}
	return ghost.Intent.GetMXID(), ghost.Name, nil
}

func (mc *MessageConverter) MetaToMatrixText(ctx context.Context, text string, mentions slidetypes.MentionList) *event.MessageEventContent {
	return textfmt.MetaToMatrixText(ctx, text, mentions.ToSocket(), mc.getBasicUserInfo)
}

func (mc *MessageConverter) ToMatrix(
	ctx context.Context,
	portal *bridgev2.Portal,
	client *instameow.Client,
	intent bridgev2.MatrixAPI,
	messageID networkid.MessageID,
	msg *slidetypes.Message,
	disableXMA bool,
) (*bridgev2.ConvertedMessage, error) {
	ctx = context.WithValue(ctx, contextKeyIGClient, client)
	ctx = context.WithValue(ctx, contextKeyIntent, intent)
	ctx = context.WithValue(ctx, contextKeyPortal, portal)
	ctx = context.WithValue(ctx, contextKeyFetchXMA, !disableXMA)
	ctx = context.WithValue(ctx, contextKeyMsgID, messageID)
	cm := &bridgev2.ConvertedMessage{
		Parts: make([]*bridgev2.ConvertedMessagePart, 0),
	}
	if msg.TextBody != "" {
		cm.Parts = append(cm.Parts, &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: mc.MetaToMatrixText(ctx, msg.TextBody, msg.Mentions),
		})
	}
	return cm, nil
}
