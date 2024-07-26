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
	"html"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"net/url"
	"regexp"
	"slices"

	"strings"

	"github.com/rs/zerolog"
	//"github.com/rs/zerolog/log"

	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/messagix/data/responses"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/connector/ids"
)

// func (cm *bridgev2.ConvertedMessage) MergeCaption() {
// 	if len(cm.Parts) != 2 || cm.Parts[1].Content.MsgType != event.MsgText {
// 		return
// 	}
// 	switch cm.Parts[0].Content.MsgType {
// 	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
// 	default:
// 		return
// 	}
// 	mediaContent := cm.Parts[0].Content
// 	textContent := cm.Parts[1].Content
// 	if mediaContent.FileName != "" && mediaContent.FileName != mediaContent.Body {
// 		if textContent.FormattedBody != "" || mediaContent.FormattedBody != "" {
// 			textContent.EnsureHasHTML()
// 			mediaContent.EnsureHasHTML()
// 			mediaContent.FormattedBody = fmt.Sprintf("%s<br><br>%s", mediaContent.FormattedBody, textContent.FormattedBody)
// 		}
// 		mediaContent.Body = fmt.Sprintf("%s\n\n%s", mediaContent.Body, textContent.Body)
// 	} else {
// 		mediaContent.FileName = mediaContent.Body
// 		mediaContent.Body = textContent.Body
// 		mediaContent.Format = textContent.Format
// 		mediaContent.FormattedBody = textContent.FormattedBody
// 	}
// 	mediaContent.Mentions = textContent.Mentions
// 	maps.Copy(cm.Parts[0].Extra, cm.Parts[1].Extra)
// 	cm.Parts = cm.Parts[:1]
// }

func isProbablyURLPreview(xma *table.WrappedXMA) bool {
	return xma.CTA != nil &&
		xma.CTA.Type_ == "xma_web_url" &&
		xma.CTA.TargetId == 0 &&
		xma.CTA.NativeUrl == "" &&
		strings.Contains(xma.CTA.ActionUrl, "/l.php?")
}

type basicUserInfo struct {
	MXID        id.UserID
	DisplayName string
}

func (mc *MessageConverter) getBasicUserInfo(ctx context.Context, portal *bridgev2.Portal, user networkid.UserID) (*basicUserInfo, error) {
	log := zerolog.Ctx(ctx).With().
		Any("user", user).
		Logger()

	ghost, err := portal.Bridge.GetGhostByID(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to get ghost by ID: %w", err)
	}

	login := portal.Bridge.GetCachedUserLoginByID(networkid.UserLoginID(user))

	if login != nil {
		return &basicUserInfo{login.UserMXID, ghost.Name}, nil
	}

	log.Debug().Msg("User login not found for reply target message, using ghost")

	return &basicUserInfo{ghost.Intent.GetMXID(), ghost.Name}, nil
}

type matrixReplyInfo struct {
	ReplyTo           id.EventID
	ReplyTargetSender id.UserID
}

func (mc *MessageConverter) getMatrixReply(ctx context.Context, portal *bridgev2.Portal, replyToUser networkid.UserID, replyToMessage networkid.MessageID) (*matrixReplyInfo, error) {
	log := zerolog.Ctx(ctx).With().
		Any("reply_to_user", replyToUser).
		Any("reply_to_msg", replyToMessage).
		Logger()

	if replyToUser == networkid.UserID("") || replyToMessage == networkid.MessageID("") {
		log.Debug().Msg("Empty reply user or message ID, returning nil")
		return nil, nil
	}

	message, err := portal.Bridge.DB.Message.GetFirstPartByID(ctx, portal.Receiver, replyToMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to get reply target message from database: %w", err)
	} else if message == nil {
		return nil, fmt.Errorf("reply target message not found")
	}

	uinfo, err := mc.getBasicUserInfo(ctx, portal, replyToUser)
	if err != nil {
		return nil, fmt.Errorf("failed to get basic user info: %w", err)
	}

	mxid := uinfo.MXID

	return &matrixReplyInfo{message.MXID, mxid}, nil
}

func (mc *MessageConverter) ToMatrix(ctx context.Context, msg *table.WrappedMessage, portal *bridgev2.Portal) *bridgev2.ConvertedMessage {
	cm := &bridgev2.ConvertedMessage{
		Parts: make([]*bridgev2.ConvertedMessagePart, 0),
	}
	if msg.IsUnsent {
		return cm
	}
	for _, blobAtt := range msg.BlobAttachments {
		cm.Parts = append(cm.Parts, mc.blobAttachmentToMatrix(ctx, blobAtt))
	}
	for _, legacyAtt := range msg.Attachments {
		cm.Parts = append(cm.Parts, mc.legacyAttachmentToMatrix(ctx, legacyAtt))
	}
	var urlPreviews []*table.WrappedXMA
	for _, xmaAtt := range msg.XMAAttachments {
		if isProbablyURLPreview(xmaAtt) {
			// URL previews are handled in the text section
			urlPreviews = append(urlPreviews, xmaAtt)
			continue
		} else if xmaAtt.CTA != nil && strings.HasPrefix(xmaAtt.CTA.Type_, "xma_poll_") {
			// Skip poll metadata entirely for now
			continue
		}
		cm.Parts = append(cm.Parts, mc.xmaAttachmentToMatrix(ctx, xmaAtt)...)
	}
	for _, sticker := range msg.Stickers {
		cm.Parts = append(cm.Parts, mc.stickerToMatrix(ctx, sticker))
	}
	if msg.Text != "" || msg.ReplySnippet != "" || len(urlPreviews) > 0 {
		mentions := &socket.MentionData{
			MentionIDs:     msg.MentionIds,
			MentionOffsets: msg.MentionOffsets,
			MentionLengths: msg.MentionLengths,
			MentionTypes:   msg.MentionTypes,
		}
		content := mc.metaToMatrixText(ctx, msg.Text, mentions, portal)
		if msg.IsAdminMessage {
			content.MsgType = event.MsgNotice
		}
		if len(urlPreviews) > 0 {
			content.BeeperLinkPreviews = make([]*event.BeeperLinkPreview, len(urlPreviews))
			previewLinks := make([]string, len(urlPreviews))
			for i, preview := range urlPreviews {
				content.BeeperLinkPreviews[i] = mc.urlPreviewToBeeper(ctx, preview)
				previewLinks[i] = content.BeeperLinkPreviews[i].CanonicalURL
			}
			// TODO do more fancy detection of whether the link is in the body?
			if len(content.Body) == 0 {
				content.Body = strings.Join(previewLinks, "\n")
			}
		}
		extra := make(map[string]any)
		if msg.ReplySnippet != "" && len(msg.XMAAttachments) > 0 && len(msg.XMAAttachments) != len(urlPreviews) {
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

		cm.Parts = append(cm.Parts, &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: content,
			Extra:   extra,
		})
	}
	if len(cm.Parts) == 0 {
		cm.Parts = append(cm.Parts, &bridgev2.ConvertedMessagePart{
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

	rinfo, err := mc.getMatrixReply(ctx, portal, ids.MakeUserID(msg.ReplyToUserId), networkid.MessageID(msg.ReplySourceId))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get reply target message")
	}

	for _, part := range cm.Parts {
		_, hasExternalURL := part.Extra["external_url"]
		unsupported, _ := part.Extra["fi.mau.unsupported"].(bool)
		if unsupported && !hasExternalURL {
			//_, threadURL := mc.GetThreadURL(ctx)
			panic("GetThreadURL not implemented")
			// if threadURL != "" {
			// 	part.Extra["external_url"] = threadURL
			// 	part.Content.EnsureHasHTML()
			// 	var protocolName string
			// 	switch {
			// 	case strings.HasPrefix(threadURL, "https://www.instagram.com"):
			// 		protocolName = "Instagram"
			// 	case strings.HasPrefix(threadURL, "https://www.facebook.com"):
			// 		protocolName = "Facebook"
			// 	case strings.HasPrefix(threadURL, "https://www.messenger.com"):
			// 		protocolName = "Messenger"
			// 	default:
			// 		protocolName = "native app"
			// 	}
			// 	part.Content.Body = fmt.Sprintf("%s\n\nOpen in %s: %s", part.Content.Body, protocolName, threadURL)
			// 	part.Content.FormattedBody = fmt.Sprintf("%s<br><br><a href=\"%s\">Click here to open in %s</a>", part.Content.FormattedBody, threadURL, protocolName)
			// }
		}
		if part.Content.Mentions == nil {
			part.Content.Mentions = &event.Mentions{}
		}
		if rinfo != nil {
			part.Content.RelatesTo = (&event.RelatesTo{}).SetReplyTo(rinfo.ReplyTo)
			if !slices.Contains(part.Content.Mentions.UserIDs, rinfo.ReplyTargetSender) {
				part.Content.Mentions.UserIDs = append(part.Content.Mentions.UserIDs, rinfo.ReplyTargetSender)
			}
		}
	}
	return cm
}

func errorToNotice(err error, attachmentContainerType string) *bridgev2.ConvertedMessagePart {
	errMsg := "Failed to transfer attachment"
	if errors.Is(err, ErrURLNotFound) {
		errMsg = fmt.Sprintf("Unrecognized %s attachment type", attachmentContainerType)
	} else if errors.Is(err, ErrTooLargeFile) {
		errMsg = "Too large attachment"
	}
	return &bridgev2.ConvertedMessagePart{
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

func (mc *MessageConverter) blobAttachmentToMatrix(ctx context.Context, att *table.LSInsertBlobAttachment) *bridgev2.ConvertedMessagePart {
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	if mime == "" {
		mime = att.AttachmentMimeType
	}
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

func (mc *MessageConverter) legacyAttachmentToMatrix(ctx context.Context, att *table.LSInsertAttachment) *bridgev2.ConvertedMessagePart {
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	if mime == "" {
		mime = att.AttachmentMimeType
	}
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
		return errorToNotice(err, "generic")
	}
	return converted
}

func (mc *MessageConverter) stickerToMatrix(ctx context.Context, att *table.LSInsertStickerAttachment) *bridgev2.ConvertedMessagePart {
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

func (mc *MessageConverter) instagramFetchedMediaToMatrix(ctx context.Context, att *table.WrappedXMA, resp *responses.Items) (*bridgev2.ConvertedMessagePart, error) {
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
	return mc.reuploadAttachment(ctx, att.AttachmentType, url, att.Filename, mime, width, height, int(resp.VideoDuration*1000))
}

func (mc *MessageConverter) xmaLocationToMatrix(ctx context.Context, att *table.WrappedXMA) *bridgev2.ConvertedMessagePart {
	if att.CTA.NativeUrl == "" {
		// This happens for live locations
		// TODO figure out how to support them properly
		return &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    fmt.Sprintf("%s\n%s", att.TitleText, att.SubtitleText),
			},
		}
	}
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgLocation,
			GeoURI:  fmt.Sprintf("geo:%s", att.CTA.NativeUrl),
			Body:    fmt.Sprintf("%s\n%s", att.TitleText, att.SubtitleText),
		},
	}
}

var reelActionURLRegex = regexp.MustCompile(`^/stories/direct/(\d+)_(\d+)$`)
var reelActionURLRegex2 = regexp.MustCompile(`^https://instagram\.com/stories/([a-z0-9.-_]{3,32})/(\d+)$`)
var usernameRegex = regexp.MustCompile(`^[a-z0-9.-_]{3,32}$`)

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

func addExternalURLCaption(content *event.MessageEventContent, externalURL string) {
	if content.FileName == "" {
		content.FileName = content.Body
		content.Body = externalURL
		content.Format = event.FormatHTML
		content.FormattedBody = fmt.Sprintf(`<a href="%s">%s</a>`, externalURL, externalURL)
	} else {
		content.EnsureHasHTML()
		content.Body = fmt.Sprintf("%s\n\n%s", content.Body, externalURL)
		content.FormattedBody = fmt.Sprintf(`%s<br><br><a href="%s">%s</a>`, content.FormattedBody, externalURL, externalURL)
	}
}

// func (mc *MessageConverter) fetchFullXMA(ctx context.Context, att *table.WrappedXMA, minimalConverted *bridgev2.ConvertedMessagePart) *bridgev2.ConvertedMessagePart {
// 	ig := mc.GetClient(ctx).Instagram
// 	if att.CTA == nil || ig == nil {
// 		minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "unsupported"
// 		return minimalConverted
// 	}
// 	log := zerolog.Ctx(ctx)
// 	switch {
// 	case strings.HasPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode="), strings.HasPrefix(att.CTA.NativeUrl, "instagram://reels_share/?shortcode="):
// 		actionURL, _ := url.Parse(removeLPHP(att.CTA.ActionUrl))
// 		var carouselChildMediaID string
// 		if actionURL != nil {
// 			carouselChildMediaID = actionURL.Query().Get("carousel_share_child_media_id")
// 		}

// 		mediaShortcode := strings.TrimPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode=")
// 		mediaShortcode = strings.TrimPrefix(mediaShortcode, "instagram://reels_share/?shortcode=")
// 		externalURL := fmt.Sprintf("https://www.instagram.com/p/%s/", mediaShortcode)
// 		minimalConverted.Extra["external_url"] = externalURL
// 		addExternalURLCaption(minimalConverted.Content, externalURL)
// 		if !mc.ShouldFetchXMA(ctx) {
// 			log.Debug().Msg("Not fetching XMA media")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "skip"
// 			return minimalConverted
// 		}

// 		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA media from CTA data")
// 		resp, err := ig.FetchMedia(strconv.FormatInt(att.CTA.TargetId, 10), mediaShortcode)
// 		if err != nil {
// 			log.Err(err).Int64("target_id", att.CTA.TargetId).Msg("Failed to fetch XMA media")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "fetch fail"
// 			return minimalConverted
// 		} else if len(resp.Items) == 0 {
// 			log.Warn().Int64("target_id", att.CTA.TargetId).Msg("Got empty XMA media response")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "empty response"
// 			return minimalConverted
// 		} else {
// 			log.Trace().Int64("target_id", att.CTA.TargetId).Any("response", resp).Msg("Fetched XMA media")
// 			log.Debug().Msg("Fetched XMA media")
// 			targetItem := resp.Items[0]
// 			if targetItem.CarouselMedia != nil && carouselChildMediaID != "" {
// 				for _, subitem := range targetItem.CarouselMedia {
// 					if subitem.ID == carouselChildMediaID {
// 						targetItem = subitem
// 						break
// 					}
// 				}
// 			}
// 			secondConverted, err := mc.instagramFetchedMediaToMatrix(ctx, att, targetItem)
// 			if err != nil {
// 				zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer fetched media")
// 				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "reupload fail"
// 				return minimalConverted
// 			}
// 			secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
// 			secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
// 			secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
// 			secondConverted.Extra["com.beeper.instagram_item_username"] = targetItem.User.Username
// 			if externalURL != "" {
// 				secondConverted.Extra["external_url"] = externalURL
// 			}
// 			secondConverted.Extra["fi.mau.meta.xma_fetch_status"] = "success"
// 			return secondConverted
// 		}
// 	case strings.HasPrefix(att.CTA.ActionUrl, "/stories/direct/"):
// 		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA story from CTA data")
// 		externalURL := fmt.Sprintf("https://www.instagram.com%s", att.CTA.ActionUrl)
// 		match := reelActionURLRegex.FindStringSubmatch(att.CTA.ActionUrl)
// 		if usernameRegex.MatchString(att.HeaderTitle) && len(match) == 3 {
// 			// Very hacky way to hopefully fix the URL to work on mobile.
// 			// When fetching the XMA data, this is done again later in a safer way.
// 			externalURL = fmt.Sprintf("https://www.instagram.com/stories/%s/%s/", att.HeaderTitle, match[1])
// 		}
// 		minimalConverted.Extra["external_url"] = externalURL
// 		addExternalURLCaption(minimalConverted.Content, externalURL)
// 		if !mc.ShouldFetchXMA(ctx) {
// 			log.Debug().Msg("Not fetching XMA media")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "skip"
// 			return minimalConverted
// 		}

// 		if len(match) != 3 {
// 			log.Warn().Str("action_url", att.CTA.ActionUrl).Msg("Failed to parse story action URL")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "parse fail"
// 			return minimalConverted
// 		} else if resp, err := ig.FetchReel([]string{match[2]}, match[1]); err != nil {
// 			log.Err(err).Str("action_url", att.CTA.ActionUrl).Msg("Failed to fetch XMA story")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "fetch fail"
// 			return minimalConverted
// 		} else if reel, ok := resp.Reels[match[2]]; !ok {
// 			log.Trace().
// 				Str("action_url", att.CTA.ActionUrl).
// 				Any("response", resp).
// 				Msg("XMA story fetch data")
// 			log.Warn().
// 				Str("action_url", att.CTA.ActionUrl).
// 				Str("reel_id", match[2]).
// 				Str("media_id", match[1]).
// 				Str("response_status", resp.Status).
// 				Msg("Got empty XMA story response")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "empty response"
// 			return minimalConverted
// 		} else {
// 			log.Trace().
// 				Str("action_url", att.CTA.ActionUrl).
// 				Str("reel_id", match[2]).
// 				Str("media_id", match[1]).
// 				Any("response", resp).
// 				Msg("Fetched XMA story")
// 			minimalConverted.Extra["com.beeper.instagram_item_username"] = reel.User.Username
// 			// Update external URL to use username so it works on mobile
// 			externalURL = fmt.Sprintf("https://www.instagram.com/stories/%s/%s/", reel.User.Username, match[1])
// 			minimalConverted.Extra["external_url"] = externalURL
// 			var relevantItem *responses.Items
// 			foundIDs := make([]string, len(reel.Items))
// 			for i, item := range reel.Items {
// 				foundIDs[i] = item.Pk
// 				if item.Pk == match[1] {
// 					relevantItem = &item.Items
// 				}
// 			}
// 			if relevantItem == nil {
// 				log.Warn().
// 					Str("action_url", att.CTA.ActionUrl).
// 					Str("reel_id", match[2]).
// 					Str("media_id", match[1]).
// 					Strs("found_ids", foundIDs).
// 					Msg("Failed to find exact item in fetched XMA story")
// 				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "item not found in response"
// 				return minimalConverted
// 			}
// 			log.Debug().Msg("Fetched XMA story and found exact item")
// 			secondConverted, err := mc.instagramFetchedMediaToMatrix(ctx, att, relevantItem)
// 			if err != nil {
// 				zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer fetched media")
// 				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "reupload fail"
// 				return minimalConverted
// 			}
// 			secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
// 			secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
// 			secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
// 			secondConverted.Extra["com.beeper.instagram_item_username"] = reel.User.Username
// 			if externalURL != "" {
// 				secondConverted.Extra["external_url"] = externalURL
// 			}
// 			secondConverted.Extra["fi.mau.meta.xma_fetch_status"] = "success"
// 			return secondConverted
// 		}
// 	//case strings.HasPrefix(att.CTA.ActionUrl, "/stories/archive/"):
// 	//		TODO can these be handled?
// 	case strings.HasPrefix(att.CTA.ActionUrl, "https://instagram.com/stories/"):
// 		log.Trace().Any("cta_data", att.CTA).Msg("Fetching second type of XMA story from CTA data")
// 		externalURL := att.CTA.ActionUrl
// 		minimalConverted.Extra["external_url"] = externalURL
// 		addExternalURLCaption(minimalConverted.Content, externalURL)
// 		if !mc.ShouldFetchXMA(ctx) {
// 			log.Debug().Msg("Not fetching XMA media")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "skip"
// 			return minimalConverted
// 		}

// 		if match := reelActionURLRegex2.FindStringSubmatch(att.CTA.ActionUrl); len(match) != 3 {
// 			log.Warn().Str("action_url", att.CTA.ActionUrl).Msg("Failed to parse story action URL (type 2)")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "parse fail"
// 			return minimalConverted
// 		} else if resp, err := ig.FetchMedia(match[2], ""); err != nil {
// 			log.Err(err).Str("action_url", att.CTA.ActionUrl).Msg("Failed to fetch XMA story (type 2)")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "fetch fail"
// 			return minimalConverted
// 		} else if len(resp.Items) == 0 {
// 			log.Trace().
// 				Str("action_url", att.CTA.ActionUrl).
// 				Any("response", resp).
// 				Msg("XMA story fetch data")
// 			log.Warn().
// 				Str("action_url", att.CTA.ActionUrl).
// 				Str("reel_id", match[2]).
// 				Str("media_id", match[1]).
// 				Str("response_status", resp.Status).
// 				Msg("Got empty XMA story response (type 2)")
// 			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "empty response"
// 			return minimalConverted
// 		} else {
// 			relevantItem := resp.Items[0]
// 			log.Trace().
// 				Str("action_url", att.CTA.ActionUrl).
// 				Str("reel_id", match[2]).
// 				Str("media_id", match[1]).
// 				Any("response", resp).
// 				Msg("Fetched XMA story (type 2)")
// 			minimalConverted.Extra["com.beeper.instagram_item_username"] = relevantItem.User.Username
// 			log.Debug().Int("item_count", len(resp.Items)).Msg("Fetched XMA story (type 2)")
// 			secondConverted, err := mc.instagramFetchedMediaToMatrix(ctx, att, relevantItem)
// 			if err != nil {
// 				zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer fetched media")
// 				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "reupload fail"
// 				return minimalConverted
// 			}
// 			secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
// 			secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
// 			secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
// 			secondConverted.Extra["com.beeper.instagram_item_username"] = relevantItem.User.Username
// 			if externalURL != "" {
// 				secondConverted.Extra["external_url"] = externalURL
// 			}
// 			secondConverted.Extra["fi.mau.meta.xma_fetch_status"] = "success"
// 			return secondConverted
// 		}
// 	default:
// 		log.Debug().
// 			Any("cta_data", att.CTA).
// 			Any("xma_data", att.LSInsertXmaAttachment).
// 			Msg("Unrecognized CTA data")
// 		minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "unrecognized"
// 		return minimalConverted
// 	}
// }

var instagramProfileURLRegex = regexp.MustCompile(`^https://www.instagram.com/([a-z0-9._]{1,30})$`)

func (mc *MessageConverter) xmaProfileShareToMatrix(ctx context.Context, att *table.WrappedXMA) *bridgev2.ConvertedMessagePart {
	if att.CTA == nil || att.HeaderSubtitleText == "" || att.HeaderImageUrl == "" || att.PlayableUrl != "" {
		return nil
	}
	match := instagramProfileURLRegex.FindStringSubmatch(att.CTA.NativeUrl)
	if len(match) != 2 || match[1] != att.HeaderTitle {
		return nil
	}
	return &bridgev2.ConvertedMessagePart{
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

func (mc *MessageConverter) urlPreviewToBeeper(ctx context.Context, att *table.WrappedXMA) *event.BeeperLinkPreview {
	preview := &event.BeeperLinkPreview{
		LinkPreview: event.LinkPreview{
			CanonicalURL: removeLPHP(att.CTA.ActionUrl),
			Title:        att.TitleText,
			Description:  att.SubtitleText,
		},
	}
	if att.PreviewUrl != "" {
		converted, err := mc.reuploadAttachment(ctx, att.AttachmentType, att.PreviewUrl, "preview", att.PreviewUrlMimeType, int(att.PreviewWidth), int(att.PreviewHeight), 0)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to reupload URL preview image")
		} else {
			preview.ImageEncryption = converted.Content.File
			preview.ImageURL = converted.Content.URL
			preview.ImageWidth = converted.Content.Info.Width
			preview.ImageHeight = converted.Content.Info.Height
			preview.ImageSize = converted.Content.Info.Size
		}
	}
	return preview
}

func (mc *MessageConverter) xmaAttachmentToMatrix(ctx context.Context, att *table.WrappedXMA) []*bridgev2.ConvertedMessagePart {
	if att.CTA != nil && att.CTA.Type_ == "xma_live_location_sharing" {
		return []*bridgev2.ConvertedMessagePart{mc.xmaLocationToMatrix(ctx, att)}
	} else if profileShare := mc.xmaProfileShareToMatrix(ctx, att); profileShare != nil {
		return []*bridgev2.ConvertedMessagePart{profileShare}
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
		converted = errorToNotice(err, "XMA")
	} else {
		//converted = mc.fetchFullXMA(ctx, att, converted)
	}
	_, hasExternalURL := converted.Extra["external_url"]
	if !hasExternalURL && att.CTA != nil && att.CTA.ActionUrl != "" {
		externalURL := removeLPHP(att.CTA.ActionUrl)
		converted.Extra["external_url"] = externalURL
		addExternalURLCaption(converted.Content, externalURL)
	}
	parts := []*bridgev2.ConvertedMessagePart{converted}
	if att.TitleText != "" || att.CaptionBodyText != "" {
		captionContent := &event.MessageEventContent{
			MsgType: event.MsgText,
		}
		if att.TitleText != "" {
			captionContent.Body = trimPostTitle(att.TitleText, int(att.MaxTitleNumOfLines))
		}
		if att.CaptionBodyText != "" {
			if captionContent.Body != "" {
				captionContent.Body += "\n\n"
			}
			captionContent.Body += att.CaptionBodyText
		}
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: captionContent,
			Extra: map[string]any{
				"com.beeper.meta.full_post_title": att.TitleText,
			},
		})
	}
	return parts
}

// func (mc *MessageConverter) uploadAttachment(ctx context.Context, data []byte, fileName, mimeType string) (*event.MessageEventContent, error) {
// 	var file *event.EncryptedFileInfo
// 	uploadMime := mimeType
// 	uploadFileName := fileName
// 	if mc.GetData(ctx).Encrypted {
// 		file = &event.EncryptedFileInfo{
// 			EncryptedFile: *attachment.NewEncryptedFile(),
// 			URL:           "",
// 		}
// 		file.EncryptInPlace(data)
// 		uploadMime = "application/octet-stream"
// 		uploadFileName = ""
// 	}
// 	mxc, err := mc.UploadMatrixMedia(ctx, data, uploadFileName, uploadMime)
// 	if err != nil {
// 		return nil, err
// 	}
// 	content := &event.MessageEventContent{
// 		Body: fileName,
// 		Info: &event.FileInfo{
// 			MimeType: mimeType,
// 			Size:     len(data),
// 		},
// 	}
// 	if file != nil {
// 		file.URL = mxc
// 		content.File = file
// 	} else {
// 		content.URL = mxc
// 	}
// 	return content, nil
// }

func (mc *MessageConverter) reuploadAttachment(
	ctx context.Context, attachmentType table.AttachmentType,
	url, fileName, mimeType string,
	width, height, duration int,
) (*bridgev2.ConvertedMessagePart, error) {
	panic("not implemented")
}

// 	if url == "" {
// 		return nil, ErrURLNotFound
// 	}
// 	data, err := DownloadMedia(ctx, mimeType, url, mc.MaxFileSize)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to download attachment: %w", err)
// 	}
// 	if mimeType == "" {
// 		mimeType = http.DetectContentType(data)
// 	}
// 	extra := map[string]any{}
// 	if attachmentType == table.AttachmentTypeAudio && mc.ConvertVoiceMessages && ffmpeg.Supported() {
// 		data, err = ffmpeg.ConvertBytes(ctx, data, ".ogg", []string{}, []string{"-c:a", "libopus"}, mimeType)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to convert audio to ogg/opus: %w", err)
// 		}
// 		fileName += ".ogg"
// 		mimeType = "audio/ogg"
// 		extra["org.matrix.msc3245.voice"] = map[string]any{}
// 		extra["org.matrix.msc1767.audio"] = map[string]any{
// 			"duration": duration,
// 		}
// 	}
// 	if (attachmentType == table.AttachmentTypeImage || attachmentType == table.AttachmentTypeEphemeralImage) && (width == 0 || height == 0) {
// 		config, _, err := image.DecodeConfig(bytes.NewReader(data))
// 		if err == nil {
// 			width, height = config.Width, config.Height
// 		}
// 	}
// 	//content, err := mc.uploadAttachment(ctx, data, fileName, mimeType)
// 	if err != nil {
// 		return nil, err
// 	}
// 	//content.Info.Duration = duration
// 	//content.Info.Width = width
// 	//content.Info.Height = height

// 	if attachmentType == table.AttachmentTypeAnimatedImage && mimeType == "video/mp4" {
// 		extra["info"] = map[string]any{
// 			"fi.mau.gif":           true,
// 			"fi.mau.loop":          true,
// 			"fi.mau.autoplay":      true,
// 			"fi.mau.hide_controls": true,
// 			"fi.mau.no_audio":      true,
// 		}
// 	}
// 	eventType := event.EventMessage
// 	switch attachmentType {
// 	case table.AttachmentTypeSticker:
// 		eventType = event.EventSticker
// 	case table.AttachmentTypeImage, table.AttachmentTypeEphemeralImage:
// 		content.MsgType = event.MsgImage
// 	case table.AttachmentTypeVideo, table.AttachmentTypeEphemeralVideo:
// 		content.MsgType = event.MsgVideo
// 	case table.AttachmentTypeFile:
// 		content.MsgType = event.MsgFile
// 	case table.AttachmentTypeAudio:
// 		content.MsgType = event.MsgAudio
// 	default:
// 		switch strings.Split(mimeType, "/")[0] {
// 		case "image":
// 			content.MsgType = event.MsgImage
// 		case "video":
// 			content.MsgType = event.MsgVideo
// 		case "audio":
// 			content.MsgType = event.MsgAudio
// 		default:
// 			content.MsgType = event.MsgFile
// 		}
// 	}
// 	if content.Body == "" {
// 		content.Body = strings.TrimPrefix(string(content.MsgType), "m.") + exmime.ExtensionFromMimetype(mimeType)
// 	}
// 	return &bridgev2.ConvertedMessagePart{
// 		Type:    eventType,
// 		Content: content,
// 		Extra:   extra,
// 	}, nil
// }
