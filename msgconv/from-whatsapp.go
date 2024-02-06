// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
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

package msgconv

import (
	"context"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"slices"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/binary/armadillo/waCommon"
	"go.mau.fi/whatsmeow/binary/armadillo/waConsumerApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func (mc *MessageConverter) whatsappTextToMatrix(ctx context.Context, text *waCommon.MessageText) *ConvertedMessagePart {
	content := &event.MessageEventContent{
		MsgType:  event.MsgText,
		Body:     text.GetText(),
		Mentions: &event.Mentions{},
	}
	silent := false
	if len(text.Commands) > 0 {
		for _, cmd := range text.Commands {
			switch cmd.CommandType {
			case waCommon.Command_SILENT:
				silent = true
				content.Mentions.Room = false
			case waCommon.Command_EVERYONE:
				if !silent {
					content.Mentions.Room = true
				}
			case waCommon.Command_AI:
				// TODO ???
			}
		}
	}
	if len(text.GetMentionedJID()) > 0 {
		content.Format = event.FormatHTML
		content.FormattedBody = event.TextToHTML(content.Body)
		for _, jid := range text.GetMentionedJID() {
			parsed, err := types.ParseJID(jid)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Str("jid", jid).Msg("Failed to parse mentioned JID")
				continue
			}
			mxid := mc.GetUserMXID(ctx, int64(parsed.UserInt()))
			if !silent {
				content.Mentions.UserIDs = append(content.Mentions.UserIDs, mxid)
			}
			mentionText := "@" + jid
			content.Body = strings.ReplaceAll(content.Body, mentionText, mxid.String())
			content.FormattedBody = strings.ReplaceAll(content.FormattedBody, mentionText, fmt.Sprintf(`<a href="%s">%s</a>`, mxid.URI().MatrixToURL(), mxid.String()))
		}
	}
	return &ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: content,
	}
}

func (mc *MessageConverter) WhatsAppToMatrix(ctx context.Context, evt *events.FBConsumerMessage) *ConvertedMessage {
	cm := &ConvertedMessage{
		Parts: make([]*ConvertedMessagePart, 0),
	}
	switch content := evt.Message.GetPayload().GetContent().GetContent().(type) {
	case *waConsumerApplication.ConsumerApplication_Content_MessageText:
		cm.Parts = append(cm.Parts, mc.whatsappTextToMatrix(ctx, content.MessageText))
	case *waConsumerApplication.ConsumerApplication_Content_ExtendedTextMessage:
		// TODO convert url previews
		cm.Parts = append(cm.Parts, mc.whatsappTextToMatrix(ctx, content.ExtendedTextMessage.GetText()))
	case *waConsumerApplication.ConsumerApplication_Content_ImageMessage:
	case *waConsumerApplication.ConsumerApplication_Content_StickerMessage:
	case *waConsumerApplication.ConsumerApplication_Content_ViewOnceMessage:
	case *waConsumerApplication.ConsumerApplication_Content_DocumentMessage:
	case *waConsumerApplication.ConsumerApplication_Content_AudioMessage:
	case *waConsumerApplication.ConsumerApplication_Content_VideoMessage:
	case *waConsumerApplication.ConsumerApplication_Content_LocationMessage:
	case *waConsumerApplication.ConsumerApplication_Content_LiveLocationMessage:
	case *waConsumerApplication.ConsumerApplication_Content_ContactMessage:
	case *waConsumerApplication.ConsumerApplication_Content_ContactsArrayMessage:
	}
	if len(cm.Parts) == 0 {
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Unsupported message",
			},
		})
	}
	var replyTo id.EventID
	var sender id.UserID
	if qm := evt.Application.GetMetadata().GetQuotedMessage(); qm != nil {
		pcp, _ := types.ParseJID(qm.GetParticipant())
		replyTo, sender = mc.GetMatrixReply(ctx, qm.GetStanzaID(), int64(pcp.UserInt()))
	}
	for _, part := range cm.Parts {
		if part.Content.Mentions == nil {
			part.Content.Mentions = &event.Mentions{}
		}
		if replyTo != "" {
			part.Content.RelatesTo = (&event.RelatesTo{}).SetReplyTo(replyTo)
			if !slices.Contains(part.Content.Mentions.UserIDs, sender) {
				part.Content.Mentions.UserIDs = append(part.Content.Mentions.UserIDs, sender)
			}
		}
	}
	return cm
}
