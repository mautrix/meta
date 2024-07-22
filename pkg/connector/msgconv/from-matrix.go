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
	"errors"
	"fmt"
	"strings"

	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

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

func (mc *MessageConverter) parseFormattedBody(ctx context.Context, content *event.MessageEventContent, portal *bridgev2.Portal, task *socket.SendMessageTask) {
	log := zerolog.Ctx(ctx)
	if content.FormattedBody == "" || content.FormattedBody == content.Body || string(content.Format) != "org.matrix.custom.html" {
		log.Debug().Any("formatted_body", content.FormattedBody).Any("format", content.Format).Msg("No formatted body to parse")
		return
	}

	// By necessity, we have to parse the formatted body to find mentions, rather than the unformatted body
	// This means that we have to do all the parsing ourselves, rather than relying on whatever the unformatted body has
	// This HTML parsing code is shit, but it works for now.
	// TODO: Fix this

	formattedBody := content.FormattedBody

	var mentions socket.Mentions

	for {
		start := strings.Index(formattedBody, "<a href=\"https://matrix.to/#/@")
		if start == -1 {
			log.Debug().Any("formatted_body", content.FormattedBody).Msg("No more mentions in formatted body")
			break
		}

		end := strings.Index(formattedBody[start:], ":beeper.local\">")
		if end == -1 {
			log.Debug().Any("formatted_body", content.FormattedBody).Msg("No mentions in formatted body")
			break
		}
		end += start + len(":beeper.local\">")

		url := formattedBody[start+9 : end-2]

		uri, err := id.ParseMatrixURIOrMatrixToURL(url)
		if err != nil {
			log.Err(err).Str("url", url).Msg("Failed to parse mention URI")
			break
		}

		log.Debug().Str("url", url).Any("uri", uri).Msg("Found mention in formatted body")

		ghost, err := portal.Bridge.GetGhostByMXID(ctx, uri.UserID())
		if err != nil {
			log.Err(err).Stringer("uri", uri).Msg("Failed to get user for mention")
			break
		}

		endTag := strings.Index(formattedBody[end:], "</a>")
		if endTag == -1 {
			log.Debug().Any("formatted_body", content.FormattedBody).Msg("No end tag for mention in formatted body")
			break
		}
		endTag += end + len("</a>")

		newContent := "@" + ghost.Name

		// Add the mention to the list
		mentions = append(mentions, socket.Mention{
			ID:     ids.ParseUserID(ghost.ID),
			Offset: start,
			Length: len(newContent),
			Type:   socket.MentionTypePerson,
		})

		// Replace the mention with @user's name
		formattedBody = formattedBody[:start] + newContent + formattedBody[endTag:]
	}

	data := mentions.ToData()
	task.MentionData = &data
	content.Body = formattedBody
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
		}
		task.Text = content.Body

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
