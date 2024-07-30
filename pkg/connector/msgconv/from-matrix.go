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
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"maunium.net/go/mautrix/format"

	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/connector/ids"
)

var (
	ErrUnsupportedMsgType  = errors.New("unsupported msgtype")
	ErrMediaDownloadFailed = errors.New("failed to download media")
	ErrMediaDecryptFailed  = errors.New("failed to decrypt media")
	ErrMediaConvertFailed  = errors.New("failed to convert")
	ErrMediaUploadFailed   = errors.New("failed to upload media")
	ErrInvalidGeoURI       = errors.New("invalid `geo:` URI in message")
	ErrURLNotFound         = errors.New("url not found")
)

func (mc *MessageConverter) GetMetaReply(ctx context.Context, content *event.MessageEventContent, portal *bridgev2.Portal) *socket.ReplyMetaData {
	log := zerolog.Ctx(ctx)

	replyToID := content.RelatesTo.GetReplyTo()
	if len(replyToID) == 0 {
		return nil
	}

	message, err := portal.Bridge.DB.Message.GetPartByMXID(ctx, replyToID)
	if err != nil {
		log.Warn().Err(err).Stringer("reply_to_mxid", replyToID).Msg("Failed to get reply target message from database")
		return nil
	} else if message == nil {
		log.Warn().Stringer("reply_to_mxid", replyToID).Msg("Reply target message not found")
		return nil
	}

	return &socket.ReplyMetaData{
		ReplyMessageId:  string(message.ID),
		ReplySourceType: 1,
		ReplyType:       0,
		ReplySender:     ids.ParseUserID(message.SenderID),
	}
}

const MENTION_LOCATOR = "meta_mention_"

type MetaMention struct {
	Locator string
	Name    string
	UserID  int64
}

func NewMetaMention(userID int64, name string) *MetaMention {
	// Come up with a random string to use as the locator
	// This is a bit of a hack, but it should work

	length := 16
	b := make([]byte, length+2)
	rand.Read(b)
	locator := fmt.Sprintf("%x", b)[2 : length+2]

	return &MetaMention{
		Locator: MENTION_LOCATOR + locator,
		Name:    name,
		UserID:  userID,
	}
}

func (mc *MessageConverter) parseFormattedBody(ctx context.Context, content *event.MessageEventContent, portal *bridgev2.Portal, task *socket.SendMessageTask) {
	log := zerolog.Ctx(ctx)
	if content.FormattedBody == "" || content.FormattedBody == content.Body || string(content.Format) != "org.matrix.custom.html" {
		log.Debug().Any("formatted_body", content.FormattedBody).Any("format", content.Format).Msg("No formatted body to parse")
		return
	}

	mentions := make([]*MetaMention, 0)

	parsed := (&format.HTMLParser{
		PillConverter: func(displayname, mxid, eventID string, ctx format.Context) string {
			ghost, err := portal.Bridge.GetGhostByMXID(ctx.Ctx, id.UserID(mxid))
			if err != nil {
				log.Err(err).Str("mxid", mxid).Msg("Failed to get user for mention")
				return displayname
			}
			mention := NewMetaMention(ids.ParseUserID(ghost.ID), ghost.Name)
			mentions = append(mentions, mention)
			return mention.Locator
		},
		BoldConverter: func(text string, ctx format.Context) string {
			return "*" + text + "*"
		},
		ItalicConverter: func(text string, ctx format.Context) string {
			return "_" + text + "_"
		},
		StrikethroughConverter: func(text string, ctx format.Context) string {
			return "~" + text + "~"
		},
		MonospaceConverter: func(text string, ctx format.Context) string {
			return "`" + text + "`"
		},
		MonospaceBlockConverter: func(code, language string, ctx format.Context) string {
			return "```\n" + code + "\n```"
		},
	}).Parse(content.FormattedBody, format.NewContext(ctx))

	var socketMentions socket.Mentions

	for _, mention := range mentions {
		// Find the mention in the parsed body
		mentionIndex := strings.Index(parsed, mention.Locator)
		if mentionIndex == -1 {
			log.Warn().Any("mention", mention).Msg("Mention not found in parsed body")
			continue
		}

		parsed = parsed[:mentionIndex] + "@" + mention.Name + parsed[mentionIndex+len(mention.Locator):]

		socketMentions = append(socketMentions, socket.Mention{
			ID:     mention.UserID,
			Offset: mentionIndex,
			Length: len("@" + mention.Name),
			Type:   socket.MentionTypePerson,
		})
	}

	data := socketMentions.ToData()
	task.MentionData = &data

	task.Text = parsed
}

func (mc *MessageConverter) ToMeta(ctx context.Context, evt *event.Event, content *event.MessageEventContent, relaybotFormatted bool, threadID int64, portal *bridgev2.Portal) ([]socket.Task, int64, error) {
	if evt.Type == event.EventSticker {
		content.MsgType = event.MsgImage
	}

	task := &socket.SendMessageTask{
		ThreadId:         threadID,
		Otid:             methods.GenerateEpochId(),
		Source:           table.MESSENGER_INBOX_IN_THREAD,
		InitiatingSource: table.FACEBOOK_INBOX,
		SendType:         table.TEXT,
		SyncGroup:        1,

		ReplyMetaData: mc.GetMetaReply(ctx, content, portal),
	}
	if content.MsgType == event.MsgEmote && !relaybotFormatted {
		content.Body = "/me " + content.Body
		if content.FormattedBody != "" {
			content.FormattedBody = "/me " + content.FormattedBody
		}
	}
	switch content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		if content.FormattedBody != "" {
			mc.parseFormattedBody(ctx, content, portal, task)
		} else {
			task.Text = content.Body
		}

	// case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
	// 	resp, err := mc.reuploadFileToMeta(ctx, evt, content)
	// 	if err != nil {
	// 		return nil, 0, err
	// 	}
	// 	attachmentID := resp.Payload.RealMetadata.GetFbId()
	// 	if attachmentID == 0 {
	// 		zerolog.Ctx(ctx).Warn().RawJSON("response", resp.Raw).Msg("No fbid received for upload")
	// 		return nil, 0, fmt.Errorf("failed to upload attachment: fbid not received")
	// 	}
	// 	task.SendType = table.MEDIA
	// 	task.AttachmentFBIds = []int64{attachmentID}
	// 	if content.FileName != "" && content.Body != content.FileName {
	// 		// This might not actually be allowed
	// 		task.Text = content.Body
	// 	}
	case event.MsgLocation:
		// TODO implement
		fallthrough
	default:
		return nil, 0, fmt.Errorf("%w %s", ErrUnsupportedMsgType, content.MsgType)
	}
	readTask := &socket.ThreadMarkReadTask{
		ThreadId:  task.ThreadId,
		SyncGroup: 1,

		LastReadWatermarkTs: time.Now().UnixMilli(),
	}
	return []socket.Task{task, readTask}, task.Otid, nil
}

// func (mc *MessageConverter) downloadMatrixMedia(ctx context.Context, content *event.MessageEventContent) (data []byte, mimeType, fileName string, err error) {
// 	mxc := content.URL
// 	if content.File != nil {
// 		mxc = content.File.URL
// 	}
// 	data, err = mc.DownloadMatrixMedia(ctx, mxc)
// 	if err != nil {
// 		err = exerrors.NewDualError(ErrMediaDownloadFailed, err)
// 		return
// 	}
// 	if content.File != nil {
// 		err = content.File.DecryptInPlace(data)
// 		if err != nil {
// 			err = exerrors.NewDualError(ErrMediaDecryptFailed, err)
// 			return
// 		}
// 	}
// 	mimeType = content.GetInfo().MimeType
// 	if mimeType == "" {
// 		mimeType = http.DetectContentType(data)
// 	}
// 	fileName = content.FileName
// 	if fileName == "" {
// 		fileName = content.Body
// 		if fileName == "" {
// 			fileName = string(content.MsgType)[2:] + exmime.ExtensionFromMimetype(mimeType)
// 		}
// 	}
// 	return
// }

// func (mc *MessageConverter) reuploadFileToMeta(ctx context.Context, evt *event.Event, content *event.MessageEventContent) (*types.MercuryUploadResponse, error) {
// 	threadID := mc.GetData(ctx).ThreadID
// 	data, mimeType, fileName, err := mc.downloadMatrixMedia(ctx, content)
// 	if err != nil {
// 		return nil, err
// 	}
// 	_, isVoice := evt.Content.Raw["org.matrix.msc3245.voice"]
// 	if isVoice {
// 		data, err = ffmpeg.ConvertBytes(ctx, data, ".m4a", []string{}, []string{"-c:a", "aac"}, mimeType)
// 		if err != nil {
// 			return nil, err
// 		}
// 		mimeType = "audio/mp4"
// 		fileName += ".m4a"
// 	}
// 	resp, err := mc.GetClient(ctx).SendMercuryUploadRequest(ctx, threadID, &messagix.MercuryUploadMedia{
// 		Filename:    fileName,
// 		MimeType:    mimeType,
// 		MediaData:   data,
// 		IsVoiceClip: isVoice,
// 	})
// 	if err != nil {
// 		zerolog.Ctx(ctx).Debug().
// 			Str("file_name", fileName).
// 			Str("mime_type", mimeType).
// 			Bool("is_voice_clip", isVoice).
// 			Msg("Failed upload metadata")
// 		return nil, fmt.Errorf("%w: %w", ErrMediaUploadFailed, err)
// 	}
// 	return resp, nil
// }
