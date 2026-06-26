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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/util/random"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
	"go.mau.fi/mautrix-meta/pkg/metaid"
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
		attachmentID, err := mc.reuploadFileToMeta(ctx, client, portal, content)
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

func (mc *MessageConverter) reuploadFileToMeta(ctx context.Context, client *messagix.Client, portal *bridgev2.Portal, content *event.MessageEventContent) (int64, error) {
	threadID := metaid.ParseFBPortalID(portal.ID)
	mime := content.Info.MimeType
	fileName := content.Body
	if content.FileName != "" {
		fileName = content.FileName
	}
	data, err := mc.Bridge.Bot.DownloadMedia(ctx, content.URL, content.File)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", bridgev2.ErrMediaDownloadFailed, err)
	}
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	isVoice := content.MSC3245Voice != nil
	if isVoice && ffmpeg.Supported() {
		data, err = ffmpeg.ConvertBytes(ctx, data, ".m4a", []string{}, []string{"-c:a", "aac"}, mime)
		if err != nil {
			return 0, fmt.Errorf("%w (ogg to m4a): %w", bridgev2.ErrMediaConvertFailed, err)
		}
		mime = "audio/mp4"
		fileName += ".m4a"
	}
	resp, err := client.SendMercuryUploadRequest(ctx, threadID, &messagix.MercuryUploadMedia{
		Filename:    fileName,
		MimeType:    mime,
		MediaData:   data,
		IsVoiceClip: isVoice,
	})
	if err != nil {
		zerolog.Ctx(ctx).Debug().
			Str("file_name", fileName).
			Str("mime_type", mime).
			Bool("is_voice_clip", isVoice).
			Msg("Failed upload metadata")
		return 0, fmt.Errorf("%w: %w", bridgev2.ErrMediaReuploadFailed, err)
	}
	attachmentID := resp.Payload.RealMetadata.GetFbId()
	if attachmentID == 0 {
		zerolog.Ctx(ctx).Warn().RawJSON("response", resp.Raw).Msg("No fbid received for upload")
	}
	if attachmentID == 0 && content.MsgType == event.MsgVideo && client.Platform == types.Instagram {
		attachmentID, err = mc.reuploadVideoToMetaFallback(ctx, client, data, mime)
		if err != nil {
			zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to upload attachment via Instagram Android fallback")
			return 0, fmt.Errorf("%w: fallback upload failed: %w", bridgev2.ErrMediaReuploadFailed, err)
		} else {
			zerolog.Ctx(ctx).Info().Msg("Uploaded attachment via Instagram Android fallback")
		}
	}
	if attachmentID == 0 {
		return 0, fmt.Errorf("%w: fbid not received", bridgev2.ErrMediaReuploadFailed)
	}
	return attachmentID, nil
}

// There is a subset of Instagram accounts that are for some reason unable to upload
// videos through the Instagram web API (even using the official Instagram website).
// All other uploads and messages work fine (image, audio, file), it is specifically
// videos. For these accounts, the Instagram Android API still works and we use it as
// a fallback.
func (mc *MessageConverter) reuploadVideoToMetaFallback(ctx context.Context, client *messagix.Client, data []byte, mime string) (int64, error) {
	uploadID := fmt.Sprintf(
		"%s-%d-%d-%d-%d",
		hex.EncodeToString(random.Bytes(16)),
		0, // maybe this will change some day
		len(data),
		time.Now().Unix()*1000,
		time.Now().UnixMilli(),
	)
	h := http.Header{}
	h.Add("accept-language", "en-US")
	h.Add("authorization", client.GetRUploadToken())
	h.Add("ig-intended-user-id", client.GetCookies().Get("ds_user_id"))
	h.Add("ig-u-ds-user-id", client.GetCookies().Get("ds_user_id"))
	h.Add("ig-u-rur", client.GetCookies().Get("rur"))
	h.Add("offset", "0")
	h.Add("segment-start-offset", "0")
	h.Add("segment-type", "3")
	h.Add("user-agent", useragent.AndroidUserAgent)
	h.Add("video_type", "FILE_ATTACHMENT")
	h.Add("x-entity-length", fmt.Sprintf("%d", len(data)))
	h.Add("x-entity-name", uploadID)
	h.Add("x-entity-type", mime)
	h.Add("x-fb-client-ip", "True")
	h.Add("x-fb-friendly-name", "undefined:media-upload")
	h.Add("x-fb-http-engine", "Tigon/MNS/TCP")
	h.Add("x-fb-rmd", "state=URL_ELIGIBLE")
	h.Add("x-fb-server-cluster", "True")
	h.Add("x-zero-balance", "INIT")
	h.Add("x-zero-eh", "")
	resp, body, err := client.GetHTTP().MakeRequest(
		ctx,
		fmt.Sprintf("https://rupload.facebook.com/messenger_video/%s", uploadID),
		"POST",
		h,
		data,
		"application/octet-stream",
	)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	var respData messagix.RUploadResponse
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return 0, err
	}
	return respData.MediaID, nil
}
