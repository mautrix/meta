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
	"errors"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/ffmpeg"
	"golang.org/x/exp/maps"
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
	mediaContent.Mentions = textContent.Mentions
	maps.Copy(cm.Parts[0].Extra, cm.Parts[1].Extra)
	cm.Parts = cm.Parts[:1]
}

type ConvertedMessagePart struct {
	Type    event.Type
	Content *event.MessageEventContent
	Extra   map[string]any
}

func isProbablyURLPreview(xma *table.WrappedXMA) bool {
	return xma.CTA != nil && xma.CTA.Type_ == "xma_web_url" && xma.CTA.TargetId == 0 && xma.CTA.NativeUrl == "" &&
		(strings.HasPrefix(xma.CTA.ActionUrl, "https://l.facebook.com") || strings.HasPrefix(xma.CTA.ActionUrl, "https://l.messenger.com"))
}

func (mc *MessageConverter) ToMatrix(ctx context.Context, msg *table.WrappedMessage) *ConvertedMessage {
	cm := &ConvertedMessage{
		Parts: make([]*ConvertedMessagePart, 0),
	}
	for _, blobAtt := range msg.BlobAttachments {
		cm.Parts = append(cm.Parts, mc.blobAttachmentToMatrix(ctx, blobAtt))
	}
	for _, xmaAtt := range msg.XMAAttachments {
		// Skip URL previews and polls for now
		if xmaAtt.CTA != nil && ((msg.Text != "" && isProbablyURLPreview(xmaAtt)) || strings.HasPrefix(xmaAtt.CTA.Type_, "xma_poll_")) {
			continue
		}
		cm.Parts = append(cm.Parts, mc.xmaAttachmentToMatrix(ctx, xmaAtt)...)
	}
	for _, sticker := range msg.Stickers {
		cm.Parts = append(cm.Parts, mc.stickerToMatrix(ctx, sticker))
	}
	if msg.Text != "" || msg.ReplySnippet != "" {
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
		extra := make(map[string]any)
		if msg.ReplySnippet != "" && len(msg.XMAAttachments) > 0 {
			extra["com.beeper.relation_snippet"] = msg.ReplySnippet
			// This is extremely hacky
			isReaction := strings.Contains(msg.ReplySnippet, "Reacted")
			if isReaction {
				extra["com.beeper.raw_reaction"] = content.Body
			} else if msg.Text != "" {
				extra["com.beeper.raw_reply_text"] = content.Body
			}
			content.Body = strings.TrimSpace(fmt.Sprintf("%s\n\n%s", msg.ReplySnippet, content.Body))
			if content.FormattedBody != "" {
				content.FormattedBody = strings.TrimSpace(fmt.Sprintf("%s\n\n%s", html.EscapeString(msg.ReplySnippet), content.FormattedBody))
			}
			switch msg.ReplySourceTypeV2 {
			case table.ReplySourceTypeIGStoryShare:
				if isReaction {
					extra["com.beeper.relation_preview_type"] = "story_reaction"
				} else if msg.Text != "" {
					extra["com.beeper.relation_preview_type"] = "story_reply"
				} else {
					extra["com.beeper.relation_preview_type"] = "story"
				}
			default:
			}
		}
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: content,
			Extra:   extra,
		})
	}
	if len(cm.Parts) == 0 {
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Unsupported message",
			},
			Extra: map[string]any{
				"fi.mau.unsupported": true,
			},
		})
	}
	replyTo, sender := mc.GetMatrixReply(ctx, msg.ReplySourceId, msg.ReplyToUserId)
	for _, part := range cm.Parts {
		_, hasExternalURL := part.Extra["external_url"]
		unsupported, _ := part.Extra["fi.mau.unsupported"].(bool)
		if unsupported && !hasExternalURL {
			_, threadURL := mc.GetThreadURL(ctx)
			if threadURL != "" {
				part.Extra["external_url"] = threadURL
			}
		}
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

func errorToNotice(err error, attachmentContainerType string) *ConvertedMessagePart {
	errMsg := "Failed to transfer attachment"
	if errors.Is(err, ErrURLNotFound) {
		errMsg = fmt.Sprintf("Unrecognized %s attachment type", attachmentContainerType)
	} else if errors.Is(err, ErrTooLargeFile) {
		errMsg = "Too large attachment"
	}
	return &ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    errMsg,
		},
		Extra: map[string]any{
			"fi.mau.unsupported": true,
		},
	}
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
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer blob media")
		return errorToNotice(err, "blob")
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
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer sticker media")
		return errorToNotice(err, "sticker")
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
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer fetched media")
		return errorToNotice(err, "fetched")
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

func trimPostTitle(title string, maxLines int) string {
	// For some reason Instagram gives maxLines 1 less than what they mean (i.e. what the official clients render)
	maxLines++
	lines := strings.SplitN(title, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] += "…"
	}
	maxCharacters := maxLines * 50
	for i, line := range lines {
		lineRunes := []rune(line)
		if len(lineRunes) > maxCharacters {
			lines[i] = string(lineRunes[:maxCharacters]) + "…"
			lines = lines[:i+1]
			break
		}
		maxCharacters -= len(lineRunes)
	}
	return strings.Join(lines, "\n")
}

func removeLPHP(addr string) string {
	parsed, _ := url.Parse(addr)
	if parsed != nil && parsed.Path == "/l.php" {
		return parsed.Query().Get("u")
	}
	return addr
}

func (mc *MessageConverter) fetchFullXMA(ctx context.Context, att *table.WrappedXMA, minimalConverted *ConvertedMessagePart) []*ConvertedMessagePart {
	ig := mc.GetClient(ctx).Instagram
	if att.CTA == nil || ig == nil {
		parts := []*ConvertedMessagePart{minimalConverted}
		// This is a hacky way to include the title and external_url for FB post shares
		// TODO deduplicate TitleText handling with IG post shares, but ensure it doesn't add titles to anything else?
		if att.TitleText != "" && att.CTA != nil && att.CTA.ActionUrl != "" {
			parts = append(parts, &ConvertedMessagePart{
				Type: event.EventMessage,
				Content: &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    trimPostTitle(att.TitleText, int(att.MaxTitleNumOfLines)),
				},
				Extra: map[string]any{
					"external_url": removeLPHP(att.CTA.ActionUrl),
				},
			})
		}
		return parts
	}
	log := zerolog.Ctx(ctx)
	switch {
	case strings.HasPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode="):
		actionURL, _ := url.Parse(removeLPHP(att.CTA.ActionUrl))
		var carouselChildMediaID string
		if actionURL != nil {
			carouselChildMediaID = actionURL.Query().Get("carousel_share_child_media_id")
		}

		externalURL := fmt.Sprintf("https://www.instagram.com/p/%s/", strings.TrimPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode="))
		minimalConverted.Extra["external_url"] = externalURL
		if !mc.ShouldFetchXMA(ctx) {
			log.Debug().Msg("Not fetching XMA media")
			return []*ConvertedMessagePart{minimalConverted}
		}

		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA media from CTA data")
		resp, err := ig.FetchMedia(strconv.FormatInt(att.CTA.TargetId, 10), att.CTA.NativeUrl)
		if err != nil {
			log.Err(err).Int64("target_id", att.CTA.TargetId).Msg("Failed to fetch XMA media")
		} else if len(resp.Items) == 0 {
			log.Warn().Int64("target_id", att.CTA.TargetId).Msg("Got empty XMA media response")
		} else {
			log.Trace().Int64("target_id", att.CTA.TargetId).Any("response", resp).Msg("Fetched XMA media")
			log.Debug().Msg("Fetched XMA media")
			targetItem := resp.Items[0]
			minimalConverted.Extra["com.beeper.instagram_item_username"] = targetItem.User.Username
			if targetItem.CarouselMedia != nil && carouselChildMediaID != "" {
				for _, subitem := range targetItem.CarouselMedia {
					if subitem.ID == carouselChildMediaID {
						targetItem = subitem
						break
					}
				}
			}
			secondConverted := mc.instagramFetchedMediaToMatrix(ctx, att, targetItem)
			secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
			secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
			secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
			}
			parts := []*ConvertedMessagePart{secondConverted}
			if att.TitleText != "" {
				parts = append(parts, &ConvertedMessagePart{
					Type: event.EventMessage,
					Content: &event.MessageEventContent{
						MsgType: event.MsgText,
						Body:    trimPostTitle(att.TitleText, int(att.MaxTitleNumOfLines)),
					},
					Extra: map[string]any{
						"com.beeper.instagram_full_post_caption": att.TitleText,
					},
				})
			}
			return parts
		}
	case strings.HasPrefix(att.CTA.ActionUrl, "/stories/direct/"):
		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA story from CTA data")
		externalURL := fmt.Sprintf("https://www.instagram.com%s", att.CTA.ActionUrl)
		minimalConverted.Extra["external_url"] = externalURL
		if !mc.ShouldFetchXMA(ctx) {
			log.Debug().Msg("Not fetching XMA media")
			return []*ConvertedMessagePart{minimalConverted}
		}

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
			minimalConverted.Extra["com.beeper.instagram_item_username"] = reel.User.Username
			var relevantItem *responses.Items
			foundIDs := make([]string, len(reel.Items))
			for i, item := range reel.Items {
				foundIDs[i] = item.Pk
				if item.Pk == match[1] {
					relevantItem = &item.Items
				}
			}
			if relevantItem == nil {
				log.Warn().
					Str("action_url", att.CTA.ActionUrl).
					Str("reel_id", match[2]).
					Str("media_id", match[1]).
					Strs("found_ids", foundIDs).
					Msg("Failed to find exact item in fetched XMA story")
				return []*ConvertedMessagePart{minimalConverted}
			}
			log.Debug().Msg("Fetched XMA story and found exact item")
			secondConverted := mc.instagramFetchedMediaToMatrix(ctx, att, relevantItem)
			secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
			secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
			secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
			}
			return []*ConvertedMessagePart{secondConverted}
		}
	default:
		log.Debug().Any("cta_data", att.CTA).Msg("Unrecognized CTA data")
	}
	return []*ConvertedMessagePart{minimalConverted}
}

var instagramProfileURLRegex = regexp.MustCompile(`^https://www.instagram.com/([a-z0-9._]{1,30})$`)

func (mc *MessageConverter) xmaProfileShareToMatrix(ctx context.Context, att *table.WrappedXMA) *ConvertedMessagePart {
	if att.CTA == nil || att.HeaderSubtitleText == "" || att.HeaderImageUrl == "" || att.PlayableUrl != "" {
		return nil
	}
	match := instagramProfileURLRegex.FindStringSubmatch(att.CTA.NativeUrl)
	if len(match) != 2 || match[1] != att.HeaderTitle {
		return nil
	}
	return &ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgText,
			Format:        event.FormatHTML,
			Body:          fmt.Sprintf("Shared %s's profile: %s", att.HeaderSubtitleText, att.CTA.NativeUrl),
			FormattedBody: fmt.Sprintf(`Shared %s's profile: <a href="%s">@%s</a>`, att.HeaderSubtitleText, att.CTA.NativeUrl, match[1]),
		},
		Extra: map[string]any{
			"external_url": att.CTA.NativeUrl,
		},
	}
}

func (mc *MessageConverter) xmaAttachmentToMatrix(ctx context.Context, att *table.WrappedXMA) []*ConvertedMessagePart {
	if att.CTA != nil && att.CTA.Type_ == "xma_live_location_sharing" {
		return []*ConvertedMessagePart{mc.xmaLocationToMatrix(ctx, att)}
	} else if profileShare := mc.xmaProfileShareToMatrix(ctx, att); profileShare != nil {
		return []*ConvertedMessagePart{profileShare}
	}
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	var width, height int64
	if url == "" {
		url = att.PreviewUrl
		mime = att.PreviewUrlMimeType
		width, height = att.PreviewWidth, att.PreviewHeight
	}
	// Slightly hacky hack to make reuploadAttachment add gif metadata if the shouldAutoplayVideo flag is set.
	// No idea why Instagram has two different ways of flagging gifs.
	if att.ShouldAutoplayVideo {
		att.AttachmentType = table.AttachmentTypeAnimatedImage
	}
	converted, err := mc.reuploadAttachment(ctx, att.AttachmentType, url, att.Filename, mime, int(width), int(height), 0)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer XMA media")
		return []*ConvertedMessagePart{errorToNotice(err, "XMA")}
	}
	return mc.fetchFullXMA(ctx, att, converted)
}

func (mc *MessageConverter) uploadAttachment(ctx context.Context, data []byte, fileName, mimeType string) (*event.MessageEventContent, error) {
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
			Size:     len(data),
		},
	}
	if file != nil {
		file.URL = mxc
		content.File = file
	} else {
		content.URL = mxc
	}
	return content, nil
}

func (mc *MessageConverter) reuploadAttachment(
	ctx context.Context, attachmentType table.AttachmentType,
	url, fileName, mimeType string,
	width, height, duration int,
) (*ConvertedMessagePart, error) {
	if url == "" {
		return nil, ErrURLNotFound
	}
	data, err := DownloadMedia(ctx, url, mc.MaxFileSize)
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
	content, err := mc.uploadAttachment(ctx, data, fileName, mimeType)
	if err != nil {
		return nil, err
	}
	content.Info.Duration = duration
	content.Info.Width = width
	content.Info.Height = height

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
	return &ConvertedMessagePart{
		Type:    eventType,
		Content: content,
		Extra:   extra,
	}, nil
}
