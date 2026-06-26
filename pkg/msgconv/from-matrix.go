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
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

func (mc *MessageConverter) ToMeta(
	ctx context.Context,
	client *messagix.Client,
	evt *event.Event,
	content *event.MessageEventContent,
	replyTo *database.Message,
	threadRoot *database.Message,
	otid int64,
	relaybotFormatted bool,
	portal *bridgev2.Portal,
) ([]socket.Task, error) {
	if evt.Type == event.EventSticker {
		content.MsgType = event.MsgImage
	}

	threadID := metaid.ParseFBPortalID(portal.ID)
	task := &socket.SendMessageTask{
		ThreadId:         threadID,
		Otid:             otid,
		Source:           table.MESSENGER_INBOX_IN_THREAD,
		InitiatingSource: table.FACEBOOK_INBOX,
		SendType:         table.TEXT,
		SyncGroup:        1,
	}
	// On Messenger, replying from the pending inbox accepts the message request.
	if portal.MessageRequest && client.Platform.IsMessenger() {
		task.Source = table.MESSENGER_INBOX_PENDING_REQUESTS
	}
	if portal.Metadata.(*metaid.PortalMetadata).ThreadType == table.COMMUNITY_GROUP {
		task.SyncGroup = 104
		if threadRoot != nil {
			parsed, _ := metaid.ParseMessageID(threadRoot.ID).(metaid.ParsedFBMessageID)
			thread, err := mc.DB.GetThreadByMessage(ctx, parsed.ID)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).
					Stringer("thread_root_mxid", threadRoot.MXID).
					Str("thread_root_parsed_id", parsed.ID).
					Msg("Failed to get thread by message")
			} else if thread == 0 {
				zerolog.Ctx(ctx).Warn().
					Stringer("thread_root_mxid", threadRoot.MXID).
					Str("thread_root_parsed_id", parsed.ID).
					Msg("Thread not found")
			} else {
				zerolog.Ctx(ctx).Trace().
					Stringer("thread_root_mxid", threadRoot.MXID).
					Str("thread_root_parsed_id", parsed.ID).
					Int64("subthread_key", thread).
					Msg("Thread found")
				task.ThreadId = thread
			}
		}
	}
	if replyTo != nil {
		msgID, ok := metaid.ParseMessageID(replyTo.ID).(metaid.ParsedFBMessageID)
		if ok {
			task.ReplyMetaData = &socket.ReplyMetaData{
				ReplyMessageId:  msgID.ID,
				ReplySourceType: 1,
				ReplyType:       0,
				ReplySender:     metaid.ParseUserID(replyTo.SenderID),
			}
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
		task.MentionData = mentions.ToData()
		task.Text = text
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		attachmentID, err := mediadl.ReuploadFileToMeta(ctx, client.GetHTTP(), portal, content)
		if err != nil {
			return nil, err
		}
		task.SendType = table.MEDIA
		task.AttachmentFBIds = []int64{attachmentID}
		if content.FileName != "" && content.Body != content.FileName {
			// This might not actually be allowed
			task.Text = content.Body
		}
	case event.MsgLocation:
		// TODO implement
		fallthrough
	default:
		return nil, fmt.Errorf("%w %s", bridgev2.ErrUnsupportedMessageType, content.MsgType)
	}
	readTask := &socket.ThreadMarkReadTask{
		ThreadId:  task.ThreadId,
		SyncGroup: 1,

		LastReadWatermarkTs: time.Now().UnixMilli(),
	}
	return []socket.Task{task, readTask}, nil
}
