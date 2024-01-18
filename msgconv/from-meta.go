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
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/ffmpeg"
	"golang.org/x/exp/slices"
	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/crypto/attachment"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/messagix/data/responses"
	"go.mau.fi/mautrix-meta/messagix/table"
)

type ConvertedMessage struct {
	Parts []*ConvertedMessagePart
}

func (cm *ConvertedMessage) MergeCaption() {
	if len(cm.Parts) != 2 || cm.Parts[1].Content.MsgType != event.MsgText {
		return
	}
	switch cm.Parts[0].Content.MsgType {
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
	default:
		return
	}
	mediaContent := cm.Parts[0].Content
	textContent := cm.Parts[1].Content
	mediaContent.FileName = mediaContent.Body
	mediaContent.Body = textContent.Body
	mediaContent.Format = textContent.Format
	mediaContent.FormattedBody = textContent.FormattedBody
	cm.Parts = cm.Parts[:1]
}

type ConvertedMessagePart struct {
	Type    event.Type
	Content *event.MessageEventContent
	Extra   map[string]any
}

func (mc *MessageConverter) ToMatrix(ctx context.Context, msg *table.WrappedMessage) *ConvertedMessage {
	cm := &ConvertedMessage{
		Parts: make([]*ConvertedMessagePart, 0),
	}
	for _, blobAtt := range msg.BlobAttachments {
		cm.Parts = append(cm.Parts, mc.blobAttachmentToMatrix(ctx, blobAtt))
	}
	for _, xmaAtt := range msg.XMAAttachments {
		cm.Parts = append(cm.Parts, mc.xmaAttachmentToMatrix(ctx, xmaAtt))
	}
	for _, sticker := range msg.Stickers {
		cm.Parts = append(cm.Parts, mc.stickerToMatrix(ctx, sticker))
	}
	if msg.Text != "" {
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    msg.Text,
			},
		})
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
	replyTo, sender := mc.GetMatrixReply(ctx, msg.ReplySourceId)
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

func (mc *MessageConverter) blobAttachmentToMatrix(ctx context.Context, att *table.LSInsertBlobAttachment) *ConvertedMessagePart {
	url := att.PlayableUrl
	mime := att.AttachmentMimeType
	duration := att.PlayableDurationMs
	var width, height int64
	if url == "" {
		url = att.PreviewUrl
		mime = att.PreviewUrlMimeType
		width, height = att.PreviewWidth, att.PreviewHeight
	}
	converted, err := mc.reuploadAttachment(ctx, att.AttachmentType, url, att.Filename, mime, int(width), int(height), int(duration))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer media")
		return &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Failed to transfer attachment",
			},
		}
	}
	return converted
}

func (mc *MessageConverter) stickerToMatrix(ctx context.Context, att *table.LSInsertStickerAttachment) *ConvertedMessagePart {
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	var width, height int64
	if url == "" {
		url = att.PreviewUrl
		mime = att.PreviewUrlMimeType
		width, height = att.PreviewWidth, att.PreviewHeight
	}
	converted, err := mc.reuploadAttachment(ctx, table.AttachmentTypeSticker, url, att.AccessibilitySummaryText, mime, int(width), int(height), 0)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer media")
		return &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Failed to transfer attachment",
			},
		}
	}
	return converted
}

func (mc *MessageConverter) instagramFetchedMediaToMatrix(ctx context.Context, att *table.WrappedXMA, resp *responses.Items) *ConvertedMessagePart {
	var url, mime string
	var width, height int
	var found bool
	for _, ver := range resp.VideoVersions {
		if ver.Width*ver.Height > width*height {
			url = ver.URL
			width, height = ver.Width, ver.Height
			found = true
		}
	}
	if !found {
		for _, ver := range resp.ImageVersions2.Candidates {
			if ver.Width*ver.Height > width*height {
				url = ver.URL
				width, height = ver.Width, ver.Height
				found = true
			}
		}
	}
	converted, err := mc.reuploadAttachment(ctx, att.AttachmentType, url, att.Filename, mime, width, height, int(resp.VideoDuration*1000))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer media")
		return &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Failed to transfer attachment",
			},
		}
	}
	return converted
}

func (mc *MessageConverter) xmaAttachmentToMatrix(ctx context.Context, att *table.WrappedXMA) *ConvertedMessagePart {
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	var width, height int64
	if url == "" {
		url = att.PreviewUrl
		mime = att.PreviewUrlMimeType
		width, height = att.PreviewWidth, att.PreviewHeight
	}
	converted, err := mc.reuploadAttachment(ctx, att.AttachmentType, url, att.Filename, mime, int(width), int(height), 0)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer media")
		return &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Failed to transfer attachment",
			},
		}
	}
	var externalURL string
	if att.CTA != nil && att.CTA.NativeUrl != "" {
		if strings.HasPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode=") {
			externalURL = fmt.Sprintf("https://www.instagram.com/p/%s/", strings.TrimPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode="))
		}
	}
	if ig := mc.GetClient(ctx).Instagram; ig != nil && att.CTA != nil && att.CTA.TargetId != 0 {
		zerolog.Ctx(ctx).Debug().Int64("target_id", att.CTA.TargetId).Msg("Fetching XMA media")
		resp, err := ig.FetchMedia(strconv.FormatInt(att.CTA.TargetId, 10), att.CTA.NativeUrl)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Int64("target_id", att.CTA.TargetId).Msg("Failed to fetch XMA media")
		} else if len(resp.Items) == 0 {
			zerolog.Ctx(ctx).Warn().Int64("target_id", att.CTA.TargetId).Msg("Got empty XMA media response")
		} else {
			zerolog.Ctx(ctx).Trace().Int64("target_id", att.CTA.TargetId).Any("response", resp).Msg("Fetched XMA media")
			secondConverted := mc.instagramFetchedMediaToMatrix(ctx, att, &resp.Items[0])
			secondConverted.Content.Info.ThumbnailInfo = converted.Content.Info
			secondConverted.Content.Info.ThumbnailURL = converted.Content.URL
			secondConverted.Content.Info.ThumbnailFile = converted.Content.File
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
			}
			return secondConverted
		}
	}
	if externalURL != "" {
		converted.Extra["external_url"] = externalURL
	}
	return converted
}

func (mc *MessageConverter) reuploadAttachment(
	ctx context.Context, attachmentType table.AttachmentType,
	url, fileName, mimeType string,
	width, height, duration int,
) (*ConvertedMessagePart, error) {
	data, err := DownloadMedia(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to download attachment: %w", err)
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	extra := map[string]any{}
	if attachmentType == table.AttachmentTypeAudio && mc.ConvertVoiceMessages && ffmpeg.Supported() {
		data, err = ffmpeg.ConvertBytes(ctx, data, ".ogg", []string{}, []string{"-c:a", "libopus"}, mimeType)
		if err != nil {
			return nil, fmt.Errorf("failed to convert audio to ogg/opus: %w", err)
		}
		fileName += ".ogg"
		mimeType = "audio/ogg"
		extra["org.matrix.msc3245.voice"] = map[string]any{}
		extra["org.matrix.msc1767.audio"] = map[string]any{}
	}
	if (attachmentType == table.AttachmentTypeImage || attachmentType == table.AttachmentTypeEphemeralImage) && (width == 0 || height == 0) {
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err == nil {
			width, height = config.Width, config.Height
		}
	}
	var file *event.EncryptedFileInfo
	uploadMime := mimeType
	uploadFileName := fileName
	if mc.GetData(ctx).Encrypted {
		file = &event.EncryptedFileInfo{
			EncryptedFile: *attachment.NewEncryptedFile(),
			URL:           "",
		}
		file.EncryptInPlace(data)
		uploadMime = "application/octet-stream"
		uploadFileName = ""
	}
	mxc, err := mc.UploadMatrixMedia(ctx, data, uploadFileName, uploadMime)
	if err != nil {
		return nil, err
	}
	content := &event.MessageEventContent{
		Body: fileName,
		Info: &event.FileInfo{
			MimeType: mimeType,
			Duration: duration,
			Width:    width,
			Height:   height,
			Size:     len(data),
		},
	}
	if attachmentType == table.AttachmentTypeAnimatedImage && mimeType == "video/mp4" {
		extra["info"] = map[string]any{
			"fi.mau.gif":           true,
			"fi.mau.loop":          true,
			"fi.mau.autoplay":      true,
			"fi.mau.hide_controls": true,
			"fi.mau.no_audio":      true,
		}
	}
	eventType := event.EventMessage
	switch attachmentType {
	case table.AttachmentTypeSticker:
		eventType = event.EventSticker
	case table.AttachmentTypeImage, table.AttachmentTypeEphemeralImage:
		content.MsgType = event.MsgImage
	case table.AttachmentTypeVideo, table.AttachmentTypeEphemeralVideo:
		content.MsgType = event.MsgVideo
	case table.AttachmentTypeFile:
		content.MsgType = event.MsgFile
	case table.AttachmentTypeAudio:
		content.MsgType = event.MsgAudio
	default:
		switch strings.Split(mimeType, "/")[0] {
		case "image":
			content.MsgType = event.MsgImage
		case "video":
			content.MsgType = event.MsgVideo
		case "audio":
			content.MsgType = event.MsgAudio
		default:
			content.MsgType = event.MsgFile
		}
	}
	if content.Body == "" {
		content.Body = strings.TrimPrefix(string(content.MsgType), "m.") + exmime.ExtensionFromMimetype(mimeType)
	}
	if file != nil {
		file.URL = mxc
		content.File = file
	} else {
		content.URL = mxc
	}
	return &ConvertedMessagePart{
		Type:    eventType,
		Content: content,
		Extra:   extra,
	}, nil
}
