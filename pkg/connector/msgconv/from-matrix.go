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
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/ffmpeg"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
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

func (mc *MessageConverter) ToMeta(ctx context.Context, evt *event.Event, content *event.MessageEventContent, relaybotFormatted bool) ([]socket.Task, int64, error) {
	if evt.Type == event.EventSticker {
		content.MsgType = event.MsgImage
	}

	task := &socket.SendMessageTask{
		ThreadId:         mc.GetData(ctx).ThreadID,
		Otid:             methods.GenerateEpochId(),
		Source:           table.MESSENGER_INBOX_IN_THREAD,
		InitiatingSource: table.FACEBOOK_INBOX,
		SendType:         table.TEXT,
		SyncGroup:        1,

		ReplyMetaData: mc.GetMetaReply(ctx, content),
	}
	if content.MsgType == event.MsgEmote && !relaybotFormatted {
		content.Body = "/me " + content.Body
		if content.FormattedBody != "" {
			content.FormattedBody = "/me " + content.FormattedBody
		}
	}
	switch content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		task.Text = content.Body
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		resp, err := mc.reuploadFileToMeta(ctx, evt, content)
		if err != nil {
			return nil, 0, err
		}
		attachmentID := resp.Payload.RealMetadata.GetFbId()
		if attachmentID == 0 {
			zerolog.Ctx(ctx).Warn().RawJSON("response", resp.Raw).Msg("No fbid received for upload")
			return nil, 0, fmt.Errorf("failed to upload attachment: fbid not received")
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
		return nil, 0, fmt.Errorf("%w %s", ErrUnsupportedMsgType, content.MsgType)
	}
	readTask := &socket.ThreadMarkReadTask{
		ThreadId:  task.ThreadId,
		SyncGroup: 1,

		LastReadWatermarkTs: time.Now().UnixMilli(),
	}
	return []socket.Task{task, readTask}, task.Otid, nil
}

func (mc *MessageConverter) downloadMatrixMedia(ctx context.Context, content *event.MessageEventContent) (data []byte, mimeType, fileName string, err error) {
	mxc := content.URL
	if content.File != nil {
		mxc = content.File.URL
	}
	data, err = mc.DownloadMatrixMedia(ctx, mxc)
	if err != nil {
		err = exerrors.NewDualError(ErrMediaDownloadFailed, err)
		return
	}
	if content.File != nil {
		err = content.File.DecryptInPlace(data)
		if err != nil {
			err = exerrors.NewDualError(ErrMediaDecryptFailed, err)
			return
		}
	}
	mimeType = content.GetInfo().MimeType
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	fileName = content.FileName
	if fileName == "" {
		fileName = content.Body
		if fileName == "" {
			fileName = string(content.MsgType)[2:] + exmime.ExtensionFromMimetype(mimeType)
		}
	}
	return
}

func (mc *MessageConverter) reuploadFileToMeta(ctx context.Context, evt *event.Event, content *event.MessageEventContent) (*types.MercuryUploadResponse, error) {
	threadID := mc.GetData(ctx).ThreadID
	data, mimeType, fileName, err := mc.downloadMatrixMedia(ctx, content)
	if err != nil {
		return nil, err
	}
	_, isVoice := evt.Content.Raw["org.matrix.msc3245.voice"]
	if isVoice {
		data, err = ffmpeg.ConvertBytes(ctx, data, ".m4a", []string{}, []string{"-c:a", "aac"}, mimeType)
		if err != nil {
			return nil, err
		}
		mimeType = "audio/mp4"
		fileName += ".m4a"
	}
	resp, err := mc.GetClient(ctx).SendMercuryUploadRequest(ctx, threadID, &messagix.MercuryUploadMedia{
		Filename:    fileName,
		MimeType:    mimeType,
		MediaData:   data,
		IsVoiceClip: isVoice,
	})
	if err != nil {
		zerolog.Ctx(ctx).Debug().
			Str("file_name", fileName).
			Str("mime_type", mimeType).
			Bool("is_voice_clip", isVoice).
			Msg("Failed upload metadata")
		return nil, fmt.Errorf("%w: %w", ErrMediaUploadFailed, err)
	}
	return resp, nil
}
