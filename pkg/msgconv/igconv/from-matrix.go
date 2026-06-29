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
	"errors"
	"fmt"
	"strconv"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

func (mc *MessageConverter) ToInstagram(
	ctx context.Context,
	client *instameow.Client,
	evt *event.Event,
	content *event.MessageEventContent,
	replyTo *database.Message,
	otid int64,
	relaybotFormatted bool,
	portal *bridgev2.Portal,
) (any, error) {
	if evt.Type == event.EventSticker {
		content.MsgType = event.MsgImage
	}

	threadID := metaid.ParseFBPortalID(portal.ID)
	meta := portal.Metadata.(*metaid.PortalMetadata)
	var replyToMsgID *string
	if replyTo != nil {
		msgID, ok := metaid.ParseMessageID(replyTo.ID).(metaid.ParsedFBMessageID)
		if ok {
			replyToMsgID = &msgID.ID
		} // TODO log warning in else case?
	}
	if content.MsgType == event.MsgEmote && !relaybotFormatted {
		content.Body = "/me " + content.Body
		if content.FormattedBody != "" {
			content.FormattedBody = "/me " + content.FormattedBody
		}
	}
	switch content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		text, mentions := mc.HTMLParser.Parse(ctx, content, portal)
		realMentions, mentionedUserIDs, commands := slidetypes.SocketMentionsToInput(mentions)
		textReq := slidetypes.SendTextRequest{
			OfflineThreadingID: strconv.FormatInt(otid, 10),
			ReplyToMessageID:   replyToMsgID,
			Text:               slidetypes.SensitiveString{Value: text},
			Mentions:           realMentions,
			MentionedUserIDs:   mentionedUserIDs,
			Commands:           commands,
			SendAttribution:    ptr.Ptr("igd_web_chat_tab:in_thread"),
		}
		if meta.IGID != "" {
			textReq.IGThreadIGID = &meta.IGID
		} else if portal.RoomType == database.RoomTypeDM {
			textReq.RecipientIGIDs = []string{strconv.FormatInt(threadID, 10)}
		} else {
			// TODO use recipients in groups?
			return nil, fmt.Errorf("no IGID for group thread saved")
		}
		return &textReq, nil
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		mediaReq := slidetypes.SendMediaRequest{
			OfflineThreadingID: strconv.FormatInt(otid, 10),
			ReplyToMessageID:   replyToMsgID,
		}
		if meta.IGID != "" {
			mediaReq.ThreadID = meta.IGID
		} else {
			return nil, fmt.Errorf("no IGID for thread saved")
		}
		attachmentID, err := mediadl.ReuploadFileToMeta(ctx, client.GetHTTP(), portal, content)
		if errors.Is(err, types.ErrPleaseReloadPage) {
			zerolog.Ctx(ctx).Warn().Err(err).
				Msg("Got please reload page error while reuploading media, reloading index and retrying")
			reloadErr := client.ReloadIndex(ctx)
			if reloadErr != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to reload page to retry media upload")
			} else {
				zerolog.Ctx(ctx).Debug().Msg("Successfully reloaded index, retrying media upload")
				attachmentID, err = mediadl.ReuploadFileToMeta(ctx, client.GetHTTP(), portal, content)
			}
		}
		if err != nil {
			return nil, err
		}
		mediaReq.AttachmentFBID = strconv.FormatInt(attachmentID, 10)
		return &mediaReq, nil
	default:
		return nil, fmt.Errorf("%w %s", bridgev2.ErrUnsupportedMessageType, content.MsgType)
	}
}
