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
	"regexp"
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
	"go.mau.fi/mautrix-meta/messagix/socket"
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
		// Skip URL previews for now
		if xmaAtt.CTA != nil && xmaAtt.CTA.Type_ == "xma_web_url" {
			continue
		}
		cm.Parts = append(cm.Parts, mc.xmaAttachmentToMatrix(ctx, xmaAtt))
	}
	for _, sticker := range msg.Stickers {
		cm.Parts = append(cm.Parts, mc.stickerToMatrix(ctx, sticker))
	}
	if msg.Text != "" {
		mentions := &socket.MentionData{
			MentionIDs:     msg.MentionIds,
			MentionOffsets: msg.MentionOffsets,
			MentionLengths: msg.MentionLengths,
			MentionTypes:   msg.MentionTypes,
		}
		content := mc.metaToMatrixText(ctx, msg.Text, mentions)
		if msg.IsAdminMessage {
			content.MsgType = event.MsgNotice
		}
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: content,
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

func (mc *MessageConverter) xmaLocationToMatrix(ctx context.Context, att *table.WrappedXMA) *ConvertedMessagePart {
	if att.CTA.NativeUrl == "" {
		// This happens for live locations
		// TODO figure out how to support them properly
		return &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    fmt.Sprintf("%s\n%s", att.TitleText, att.SubtitleText),
			},
		}
	}
	return &ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgLocation,
			GeoURI:  fmt.Sprintf("geo:%s", att.CTA.NativeUrl),
			Body:    fmt.Sprintf("%s\n%s", att.TitleText, att.SubtitleText),
		},
	}
}

var reelActionURLRegex = regexp.MustCompile(`^/stories/direct/(\d+)_(\d+)$`)

func (mc *MessageConverter) fetchFullXMA(ctx context.Context, att *table.WrappedXMA, minimalConverted *ConvertedMessagePart) *ConvertedMessagePart {
	ig := mc.GetClient(ctx).Instagram
	if att.CTA == nil || ig == nil {
		return nil
	}
	log := zerolog.Ctx(ctx)
	switch {
	case strings.HasPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode="):
		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA media from CTA data")
		externalURL := fmt.Sprintf("https://www.instagram.com/p/%s/", strings.TrimPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode="))
		minimalConverted.Extra["external_url"] = externalURL

		resp, err := ig.FetchMedia(strconv.FormatInt(att.CTA.TargetId, 10), att.CTA.NativeUrl)
		if err != nil {
			log.Err(err).Int64("target_id", att.CTA.TargetId).Msg("Failed to fetch XMA media")
		} else if len(resp.Items) == 0 {
			log.Warn().Int64("target_id", att.CTA.TargetId).Msg("Got empty XMA media response")
		} else {
			log.Trace().Int64("target_id", att.CTA.TargetId).Any("response", resp).Msg("Fetched XMA media")
			secondConverted := mc.instagramFetchedMediaToMatrix(ctx, att, resp.Items[0])
			secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
			secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
			secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
			}
			return secondConverted
		}
	case strings.HasPrefix(att.CTA.ActionUrl, "/stories/direct/"):
		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA story from CTA data")
		externalURL := fmt.Sprintf("https://www.instagram.com%s", att.CTA.ActionUrl)
		minimalConverted.Extra["external_url"] = externalURL
		if match := reelActionURLRegex.FindStringSubmatch(att.CTA.ActionUrl); len(match) != 3 {
			log.Warn().Str("action_url", att.CTA.ActionUrl).Msg("Failed to parse story action URL")
		} else if resp, err := ig.FetchReel([]string{match[2]}, match[1]); err != nil {
			log.Err(err).Str("action_url", att.CTA.ActionUrl).Msg("Failed to fetch XMA story")
		} else if reel, ok := resp.Reels[match[2]]; !ok {
			log.Trace().
				Str("action_url", att.CTA.ActionUrl).
				Any("response", resp).
				Msg("XMA story fetch data")
			log.Warn().
				Str("action_url", att.CTA.ActionUrl).
				Str("reel_id", match[2]).
				Str("media_id", match[1]).
				Str("response_status", resp.Status).
				Msg("Got empty XMA story response")
		} else {
			log.Trace().
				Str("action_url", att.CTA.ActionUrl).
				Str("reel_id", match[2]).
				Str("media_id", match[1]).
				Any("response", resp).
				Msg("Fetched XMA story")
			secondConverted := mc.instagramFetchedMediaToMatrix(ctx, att, &reel.Items[0].Items)
			secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
			secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
			secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
			}
			return secondConverted
		}
	default:
		log.Debug().Any("cta_data", att.CTA).Msg("Unrecognized CTA data")
	}
	return minimalConverted
}

func (mc *MessageConverter) xmaAttachmentToMatrix(ctx context.Context, att *table.WrappedXMA) *ConvertedMessagePart {
	if att.CTA != nil && att.CTA.Type_ == "xma_live_location_sharing" {
		return mc.xmaLocationToMatrix(ctx, att)
	}
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
	return mc.fetchFullXMA(ctx, att, converted)
}

func (mc *MessageConverter) reuploadAttachment(
	ctx context.Context, attachmentType table.AttachmentType,
	url, fileName, mimeType string,
	width, height, duration int,
) (*ConvertedMessagePart, error) {
	if url == "" {
		return nil, fmt.Errorf("url not found")
	}
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
