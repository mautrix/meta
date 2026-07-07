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
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
	"go.mau.fi/mautrix-meta/pkg/msgconv/textfmt"
)

func (mc *MessageConverter) ShouldFetchXMA(ctx context.Context) bool {
	return mediadl.ShouldFetchXMA(ctx)
}

func isProbablyURLPreview(xma *table.WrappedXMA) bool {
	return xma.CTA != nil &&
		xma.CTA.Type_ == "xma_web_url" &&
		xma.CTA.TargetId == 0 &&
		xma.CTA.NativeUrl == "" &&
		strings.Contains(xma.CTA.ActionUrl, "/l.php?")
}

func (mc *MessageConverter) getBasicUserInfo(ctx context.Context, user networkid.UserID) (id.UserID, string, error) {
	ghost, err := mc.Bridge.GetGhostByID(ctx, user)
	if err != nil {
		return "", "", fmt.Errorf("failed to get ghost by ID: %w", err)
	}
	login := mc.Bridge.GetCachedUserLoginByID(networkid.UserLoginID(user))
	if login != nil {
		return login.UserMXID, ghost.Name, nil
	}
	return ghost.Intent.GetMXID(), ghost.Name, nil
}

// The fake stickers that are sent when someone presses the thumbs-up
// button in Messenger. They are handled specially by the Messenger
// web client instead of being displayed as normal stickers. There are
// three variants depending on how long the sending user held down the
// send button.
const (
	facebookThumbsUpSmallStickerID  = 369239263222822
	facebookThumbsUpMediumStickerID = 369239343222814
	facebookThumbsUpLargeStickerID  = 369239383222810
)

func (mc *MessageConverter) MetaToMatrixText(ctx context.Context, text string, mentions []socket.Mention) *event.MessageEventContent {
	return textfmt.MetaToMatrixText(ctx, text, mentions, mc.getBasicUserInfo)
}

func (mc *MessageConverter) ToMatrix(
	ctx context.Context,
	portal *bridgev2.Portal,
	client *messagix.Client,
	intent bridgev2.MatrixAPI,
	messageID networkid.MessageID,
	msg *table.WrappedMessage,
	disableXMA bool,
) *bridgev2.ConvertedMessage {
	ctx = context.WithValue(ctx, mediadl.ContextKeyFBClient, client)
	ctx = context.WithValue(ctx, mediadl.ContextKeyIGClient, client.Instagram)
	ctx = context.WithValue(ctx, mediadl.ContextKeyIntent, intent)
	ctx = context.WithValue(ctx, mediadl.ContextKeyPortal, portal)
	ctx = context.WithValue(ctx, mediadl.ContextKeyFetchXMA, !disableXMA)
	ctx = context.WithValue(ctx, mediadl.ContextKeyMsgID, messageID)
	cm := &bridgev2.ConvertedMessage{
		Parts: make([]*bridgev2.ConvertedMessagePart, 0),
	}
	if msg.IsUnsent {
		return cm
	}
	// Display the thumbs-up sticker as a simple emoji message,
	// which is the same way that it is displayed for encrypted
	// chats, to be consistent between the two types of chats.
	switch msg.StickerId {
	case facebookThumbsUpLargeStickerID, facebookThumbsUpMediumStickerID, facebookThumbsUpSmallStickerID:
		if len(msg.Stickers) == 1 {
			msg.Text = "👍"
			msg.Stickers = nil
		}
	}
	// Keep track of any part IDs that are important and must be
	// kept the same in order for the message to render correctly.
	// We have to ensure that these part IDs are not overwritten
	// by later code.
	importantPartIDs := []networkid.PartID{}
	seenBlobFBIDs := map[string]bool{}
	for i, blobAtt := range msg.BlobAttachments {
		// Sometimes Facebook literally sends two exact copies
		// of LSInsertBlobAttachment, byte for byte identical,
		// for the same media, one right after the other. This
		// in particular seems to happen with the initial
		// Lightspeed table if the latest message in a thread
		// in your inbox happens to be a media. Detect this
		// and don't include two copies of the media. Note: if
		// you literally attach the exact same image to your
		// message twice, it shows up as different fbids in
		// LSInsertBlobAttachment, so that use case won't be
		// affected by this check.
		if blobAtt.AttachmentFbid != "" && seenBlobFBIDs[blobAtt.AttachmentFbid] {
			continue
		} else {
			seenBlobFBIDs[blobAtt.AttachmentFbid] = true
		}
		partID := networkid.PartID(fmt.Sprintf("blob_attachment_%d", i))
		ctx := context.WithValue(ctx, mediadl.ContextKeyPartID, partID)
		cm.Parts = append(cm.Parts, mc.blobAttachmentToMatrix(ctx, blobAtt, i))
		importantPartIDs = append(importantPartIDs, partID)
	}
	for i, legacyAtt := range msg.Attachments {
		partID := networkid.PartID(fmt.Sprintf("attachment_%d", i))
		ctx := context.WithValue(ctx, mediadl.ContextKeyPartID, partID)
		cm.Parts = append(cm.Parts, mc.legacyAttachmentToMatrix(ctx, legacyAtt, i))
		importantPartIDs = append(importantPartIDs, partID)
	}
	var urlPreviews []*table.WrappedXMA
	for i, xmaAtt := range msg.XMAAttachments {
		partID := networkid.PartID(fmt.Sprintf("xma_attachment_%d", i))
		ctx := context.WithValue(ctx, mediadl.ContextKeyPartID, partID)
		if isProbablyURLPreview(xmaAtt) {
			// URL previews are handled in the text section
			urlPreviews = append(urlPreviews, xmaAtt)
			continue
		} else if xmaAtt.CTA != nil && strings.HasPrefix(xmaAtt.CTA.Type_, "xma_poll_") {
			// Skip poll metadata entirely for now
			continue
		}
		cm.Parts = append(cm.Parts, mc.xmaAttachmentToMatrix(ctx, xmaAtt)...)
		importantPartIDs = append(importantPartIDs, partID)
	}
	for i, sticker := range msg.Stickers {
		partID := networkid.PartID(fmt.Sprintf("sticker_%d", i))
		ctx := context.WithValue(ctx, mediadl.ContextKeyPartID, partID)
		cm.Parts = append(cm.Parts, mc.stickerToMatrix(ctx, sticker))
		importantPartIDs = append(importantPartIDs, partID)
	}
	hasRelationSnippet := msg.ReplySnippet != "" && len(msg.XMAAttachments) > 0 && len(msg.XMAAttachments) != len(urlPreviews)
	if msg.Text != "" || hasRelationSnippet || len(urlPreviews) > 0 {
		mentions, _ := (&socket.MentionData{
			MentionIDs:     msg.MentionIds,
			MentionOffsets: msg.MentionOffsets,
			MentionLengths: msg.MentionLengths,
			MentionTypes:   msg.MentionTypes,
		}).Parse()
		content := mc.MetaToMatrixText(ctx, msg.Text, mentions)
		if msg.IsAdminMessage {
			content.MsgType = event.MsgNotice
		}
		if len(urlPreviews) > 0 {
			content.BeeperLinkPreviews = make([]*event.BeeperLinkPreview, len(urlPreviews))
			previewLinks := make([]string, len(urlPreviews))
			for i, preview := range urlPreviews {
				partID := networkid.PartID(fmt.Sprintf("beeper_link_preview_%d", i))
				ctx := context.WithValue(ctx, mediadl.ContextKeyPartID, partID)
				content.BeeperLinkPreviews[i] = mc.urlPreviewToBeeper(ctx, preview)
				previewLinks[i] = content.BeeperLinkPreviews[i].CanonicalURL
				importantPartIDs = append(importantPartIDs, partID)
			}
			// TODO do more fancy detection of whether the link is in the body?
			if len(content.Body) == 0 {
				content.Body = strings.Join(previewLinks, "\n")
			}
		}
		extra := make(map[string]any)
		if hasRelationSnippet {
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
			// These duplicate information about people joining and leaving a room,
			// which would generally be rendered already by a Matrix client. There may
			// be other "admin" messages that are important, but there is no structured
			// data from Meta about what kind of message it is, so we can't really tell.
			// Drop them all for now.
			DontBridge: msg.IsAdminMessage,
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
	if msg.ReplySourceId != "" {
		cm.ReplyTo = &networkid.MessageOptionalPartID{MessageID: metaid.MakeFBMessageID(msg.ReplySourceId)}
		if msg.IsSubthreadStart {
			cm.ThreadRoot = &cm.ReplyTo.MessageID
			cm.ReplyTo = nil
		}
	}
	if msg.ThreadID != "" {
		cm.ThreadRoot = ptr.Ptr(metaid.MakeFBMessageID(msg.ThreadID))
	}

	for i, part := range cm.Parts {
		// If part ID is already assigned, use the one that is given, as some code paths
		// above generate message fragments with specific part IDs so that media can refer
		// to those part IDs. Otherwise, assign a sequential integer. It is okay to my
		// knowledge for two different schemas to co-exist in the same message for part IDs,
		// as long as they are all mutually unique.
		if part.ID == "" {
			part.ID = metaid.MakeMessagePartID(i)
		}
		_, hasExternalURL := part.Extra["external_url"]
		unsupported, _ := part.Extra["fi.mau.unsupported"].(bool)
		if unsupported && !hasExternalURL {
			var threadURL, protocolName string
			if client.GetPlatform().IsInstagram() {
				threadURL = fmt.Sprintf("https://www.instagram.com/direct/t/%s/", portal.ID)
				protocolName = "Instagram"
			} else if client.GetPlatform().IsViaFacebook() {
				threadURL = fmt.Sprintf("https://www.facebook.com/messages/t/%s/", portal.ID)
				protocolName = "Facebook"
			} else if client.GetPlatform().IsViaMessenger() {
				threadURL = fmt.Sprintf("https://www.messenger.com/t/%s/", portal.ID)
				protocolName = "Messenger"
			}
			if threadURL != "" {
				part.Content.Body = fmt.Sprintf("%s\n\nOpen in %s: %s", part.Content.Body, protocolName, threadURL)
				part.Content.FormattedBody = fmt.Sprintf("%s<br><br><a href=\"%s\">Click here to open in %s</a>", part.Content.FormattedBody, threadURL, protocolName)
			}
		}
		if part.Content.Mentions == nil {
			part.Content.Mentions = &event.Mentions{}
		}
	}

	if cm.MergeCaption() {
		// The MergeCaption method only does something if there are exactly two
		// parts in the message, and we don't add text parts to the "important"
		// slice, so we are safe to assume that if it returns true, then there is
		// exactly one item in the slice and it is the media part ID.
		cm.Parts[0].ID = importantPartIDs[0]
	}
	return cm
}

func errorToNotice(err error, attachmentContainerType string) *bridgev2.ConvertedMessagePart {
	errMsg := "Failed to transfer attachment"
	if errors.Is(err, mediadl.ErrURLNotFound) {
		errMsg = fmt.Sprintf("Unrecognized %s attachment type", attachmentContainerType)
	} else if errors.Is(err, mediadl.ErrTooLargeFile) {
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

func (mc *MessageConverter) blobAttachmentToMatrix(ctx context.Context, att *table.LSInsertBlobAttachment, partIndex int) *bridgev2.ConvertedMessagePart {
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	if mime == "" {
		mime = att.AttachmentMimeType
	}
	duration := att.PlayableDurationMs
	var width, height int64
	expiresAt := att.PlayableUrlExpirationTimestampMs
	if url == "" {
		url = att.PreviewUrl
		mime = att.PreviewUrlMimeType
		width, height = att.PreviewWidth, att.PreviewHeight
		expiresAt = att.PreviewUrlExpirationTimestampMs
	}

	refreshMeta := &mediadl.MediaRefreshMeta{
		ExpiresAt:      expiresAt,
		AttachmentFBID: att.AttachmentFbid,
		PartIndex:      partIndex,
	}

	converted, err := mc.reuploadAttachment(
		ctx, att.AttachmentType, url, att.Filename, mime, int(att.Filesize), int(width), int(height), int(duration),
		refreshMeta,
	)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer blob media")
		return errorToNotice(err, "blob")
	}
	return converted
}

func appName(ctx context.Context) string {
	if ctx.Value(mediadl.ContextKeyFBClient).(*messagix.Client).GetPlatform().IsInstagram() {
		return "Instagram app"
	}
	return "Messenger app"
}

func (mc *MessageConverter) makeViewOnceError(ctx context.Context, mediaType, viewed string) *bridgev2.ConvertedMessagePart {
	body := fmt.Sprintf("This %s can only be %s once. Use the %s to view.", mediaType, viewed, appName(ctx))
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    body,
		},
	}
}

func (mc *MessageConverter) legacyAttachmentToMatrix(ctx context.Context, att *table.LSInsertAttachment, partIndex int) *bridgev2.ConvertedMessagePart {
	if mc.DisableViewOnce && (att.EphemeralMediaViewMode == table.EphemeralMediaViewOnce || att.EphemeralMediaViewMode == table.EphemeralMediaReplayable) {
		mediaType := "photo"
		viewed := "viewed"
		if att.EphemeralMediaViewMode == table.EphemeralMediaReplayable {
			viewed = "replayed"
		}
		if att.AttachmentType == table.AttachmentTypeEphemeralVideo {
			mediaType = "video"
		}
		return mc.makeViewOnceError(ctx, mediaType, viewed)
	}
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	if mime == "" {
		mime = att.AttachmentMimeType
	}
	duration := att.PlayableDurationMs
	var width, height int64
	expiresAt := att.PlayableUrlExpirationTimestampMs
	if url == "" {
		url = att.PreviewUrl
		mime = att.PreviewUrlMimeType
		width, height = att.PreviewWidth, att.PreviewHeight
		expiresAt = att.PreviewUrlExpirationTimestampMs
	}

	refreshMeta := &mediadl.MediaRefreshMeta{
		ExpiresAt:      expiresAt,
		AttachmentFBID: att.AttachmentFbid,
		PartIndex:      partIndex,
	}

	converted, err := mc.reuploadAttachment(
		ctx, att.AttachmentType, url, att.Filename, mime, int(att.Filesize), int(width), int(height), int(duration),
		refreshMeta,
	)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer media")
		return errorToNotice(err, "generic")
	}
	return converted
}

// All stickers are rendered in the Messenger web client as 96x96
// pixels no matter what. For example you'll have a 240x240 pixel
// sticker image, and the PreviewWidth attribute says it's 128x128,
// nonetheless it's displayed at 96x96 like everything else.
const stickerSize = 96

func (mc *MessageConverter) stickerToMatrix(ctx context.Context, att *table.LSInsertStickerAttachment) *bridgev2.ConvertedMessagePart {
	url := att.PlayableUrl
	mime := att.PlayableUrlMimeType
	if url == "" {
		url = att.PreviewUrl
		mime = att.PreviewUrlMimeType
	}
	// Stickers don't typically expire, so no refresh metadata needed
	converted, err := mc.reuploadAttachment(
		ctx, table.AttachmentTypeSticker, url, att.AccessibilitySummaryText, mime, 0, stickerSize, stickerSize, 0,
		nil,
	)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer sticker media")
		return errorToNotice(err, "sticker")
	}
	return converted
}

func (mc *MessageConverter) instagramFetchedMediaToMatrix(ctx context.Context, att *table.WrappedXMA, resp *responses.Items, xmaRefresh *mediadl.MediaRefreshMeta) (*bridgev2.ConvertedMessagePart, error) {
	var url, mime string
	var width, height int
	var found bool
	mime = att.PlayableUrlMimeType
	if mime == "" {
		mime = att.PreviewUrlMimeType
	}
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

	return mc.reuploadAttachment(
		ctx, att.AttachmentType, url, att.Filename, mime, int(att.Filesize), width, height, int(resp.VideoDuration*1000),
		xmaRefresh,
	)
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

func (mc *MessageConverter) fetchFullXMA(ctx context.Context, att *table.WrappedXMA, minimalConverted *bridgev2.ConvertedMessagePart) *bridgev2.ConvertedMessagePart {
	ig := ctx.Value(mediadl.ContextKeyFBClient).(*messagix.Client).Instagram
	if att.CTA == nil || ig == nil {
		minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "unsupported"
		return minimalConverted
	}
	log := zerolog.Ctx(ctx)
	switch {
	case strings.HasPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode="), strings.HasPrefix(att.CTA.NativeUrl, "instagram://reels_share/?shortcode="):
		actionURL, _ := url.Parse(removeLPHP(att.CTA.ActionUrl))
		var carouselChildMediaID string
		if actionURL != nil {
			carouselChildMediaID = actionURL.Query().Get("carousel_share_child_media_id")
		}

		mediaShortcode := strings.TrimPrefix(att.CTA.NativeUrl, "instagram://media/?shortcode=")
		mediaShortcode = strings.TrimPrefix(mediaShortcode, "instagram://reels_share/?shortcode=")
		externalURL := fmt.Sprintf("https://www.instagram.com/p/%s/", mediaShortcode)
		minimalConverted.Extra["external_url"] = externalURL
		addExternalURLCaption(minimalConverted.Content, externalURL)
		if !mc.ShouldFetchXMA(ctx) {
			log.Debug().Msg("Not fetching XMA media")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "skip"
			return minimalConverted
		}

		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA media from CTA data")
		resp, err := ig.FetchMedia(ctx, strconv.FormatInt(att.CTA.TargetId, 10), mediaShortcode)
		if err != nil {
			log.Err(err).Int64("target_id", att.CTA.TargetId).Msg("Failed to fetch XMA media")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "fetch fail"
			return minimalConverted
		} else if len(resp.Items) == 0 {
			log.Warn().Int64("target_id", att.CTA.TargetId).Msg("Got empty XMA media response")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "empty response"
			return minimalConverted
		} else {
			log.Trace().Int64("target_id", att.CTA.TargetId).Any("response", resp).Msg("Fetched XMA media")
			log.Debug().Msg("Fetched XMA media")
			targetItem := resp.Items[0]
			if targetItem.CarouselMedia != nil && carouselChildMediaID != "" {
				for _, subitem := range targetItem.CarouselMedia {
					if subitem.ID == carouselChildMediaID {
						targetItem = subitem
						break
					}
				}
			}
			xmaRefresh := &mediadl.MediaRefreshMeta{
				XMATargetID:  att.CTA.TargetId,
				XMAShortcode: mediaShortcode,
			}
			secondConverted, err := mc.instagramFetchedMediaToMatrix(ctx, att, targetItem, xmaRefresh)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer fetched media")
				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "reupload fail"
				return minimalConverted
			}
			if !mc.DirectMedia {
				secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
				secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
				secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
			}
			secondConverted.Extra["com.beeper.instagram_item_username"] = targetItem.User.Username
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
				addExternalURLCaption(secondConverted.Content, externalURL)
			}
			secondConverted.Extra["fi.mau.meta.xma_fetch_status"] = "success"
			return secondConverted
		}
	case strings.HasPrefix(att.CTA.ActionUrl, "/stories/direct/"):
		log.Trace().Any("cta_data", att.CTA).Msg("Fetching XMA story from CTA data")
		externalURL := fmt.Sprintf("https://www.instagram.com%s", att.CTA.ActionUrl)
		match := reelActionURLRegex.FindStringSubmatch(att.CTA.ActionUrl)
		if usernameRegex.MatchString(att.HeaderTitle) && len(match) == 3 {
			// Very hacky way to hopefully fix the URL to work on mobile.
			// When fetching the XMA data, this is done again later in a safer way.
			externalURL = fmt.Sprintf("https://www.instagram.com/stories/%s/%s/", att.HeaderTitle, match[1])
		}
		minimalConverted.Extra["external_url"] = externalURL
		if !mc.ShouldFetchXMA(ctx) {
			log.Debug().Msg("Not fetching XMA media")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "skip"
			addExternalURLCaption(minimalConverted.Content, externalURL)
			return minimalConverted
		}

		if len(match) != 3 {
			log.Warn().Str("action_url", att.CTA.ActionUrl).Msg("Failed to parse story action URL")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "parse fail"
			addExternalURLCaption(minimalConverted.Content, externalURL)
			return minimalConverted
		} else if resp, err := ig.FetchReel(ctx, []string{match[2]}, match[1]); err != nil {
			log.Err(err).Str("action_url", att.CTA.ActionUrl).Msg("Failed to fetch XMA story")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "fetch fail"
			addExternalURLCaption(minimalConverted.Content, externalURL)
			return minimalConverted
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
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "empty response"
			addExternalURLCaption(minimalConverted.Content, externalURL)
			return minimalConverted
		} else {
			log.Trace().
				Str("action_url", att.CTA.ActionUrl).
				Str("reel_id", match[2]).
				Str("media_id", match[1]).
				Any("response", resp).
				Msg("Fetched XMA story")
			minimalConverted.Extra["com.beeper.instagram_item_username"] = reel.User.Username
			// Update external URL to use username so it works on mobile
			externalURL = fmt.Sprintf("https://www.instagram.com/stories/%s/%s/", reel.User.Username, match[1])
			minimalConverted.Extra["external_url"] = externalURL
			addExternalURLCaption(minimalConverted.Content, externalURL)
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
				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "item not found in response"
				return minimalConverted
			}
			log.Debug().Msg("Fetched XMA story and found exact item")
			xmaRefresh := &mediadl.MediaRefreshMeta{
				StoryMediaID: match[1],
				StoryReelID:  match[2],
			}
			secondConverted, err := mc.instagramFetchedMediaToMatrix(ctx, att, relevantItem, xmaRefresh)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer fetched media")
				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "reupload fail"
				return minimalConverted
			}
			if !mc.DirectMedia {
				secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
				secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
				secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
			}
			secondConverted.Extra["com.beeper.instagram_item_username"] = reel.User.Username
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
				addExternalURLCaption(secondConverted.Content, externalURL)
			}
			secondConverted.Extra["fi.mau.meta.xma_fetch_status"] = "success"
			return secondConverted
		}
	//case strings.HasPrefix(att.CTA.ActionUrl, "/stories/archive/"):
	//		TODO can these be handled?
	case strings.HasPrefix(att.CTA.ActionUrl, "https://instagram.com/stories/"):
		log.Trace().Any("cta_data", att.CTA).Msg("Fetching second type of XMA story from CTA data")
		externalURL := att.CTA.ActionUrl
		minimalConverted.Extra["external_url"] = externalURL
		addExternalURLCaption(minimalConverted.Content, externalURL)
		if !mc.ShouldFetchXMA(ctx) {
			log.Debug().Msg("Not fetching XMA media")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "skip"
			return minimalConverted
		}

		if match := reelActionURLRegex2.FindStringSubmatch(att.CTA.ActionUrl); len(match) != 3 {
			log.Warn().Str("action_url", att.CTA.ActionUrl).Msg("Failed to parse story action URL (type 2)")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "parse fail"
			return minimalConverted
		} else if resp, err := ig.FetchMedia(ctx, match[2], ""); err != nil {
			log.Err(err).Str("action_url", att.CTA.ActionUrl).Msg("Failed to fetch XMA story (type 2)")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "fetch fail"
			return minimalConverted
		} else if len(resp.Items) == 0 {
			log.Trace().
				Str("action_url", att.CTA.ActionUrl).
				Any("response", resp).
				Msg("XMA story fetch data")
			log.Warn().
				Str("action_url", att.CTA.ActionUrl).
				Str("reel_id", match[2]).
				Str("media_id", match[1]).
				Str("response_status", resp.Status).
				Msg("Got empty XMA story response (type 2)")
			minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "empty response"
			return minimalConverted
		} else {
			relevantItem := resp.Items[0]
			log.Trace().
				Str("action_url", att.CTA.ActionUrl).
				Str("reel_id", match[2]).
				Str("media_id", match[1]).
				Any("response", resp).
				Msg("Fetched XMA story (type 2)")
			minimalConverted.Extra["com.beeper.instagram_item_username"] = relevantItem.User.Username
			log.Debug().Int("item_count", len(resp.Items)).Msg("Fetched XMA story (type 2)")
			xmaRefresh := &mediadl.MediaRefreshMeta{
				StoryMediaID: match[2],
			}
			secondConverted, err := mc.instagramFetchedMediaToMatrix(ctx, att, relevantItem, xmaRefresh)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer fetched media")
				minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "reupload fail"
				return minimalConverted
			}
			if !mc.DirectMedia {
				secondConverted.Content.Info.ThumbnailInfo = minimalConverted.Content.Info
				secondConverted.Content.Info.ThumbnailURL = minimalConverted.Content.URL
				secondConverted.Content.Info.ThumbnailFile = minimalConverted.Content.File
			}
			secondConverted.Extra["com.beeper.instagram_item_username"] = relevantItem.User.Username
			if externalURL != "" {
				secondConverted.Extra["external_url"] = externalURL
				addExternalURLCaption(secondConverted.Content, externalURL)
			}
			secondConverted.Extra["fi.mau.meta.xma_fetch_status"] = "success"
			return secondConverted
		}
	default:
		log.Debug().
			Any("cta_data", att.CTA).
			Any("xma_data", att.LSInsertXmaAttachment).
			Msg("Unrecognized CTA data")
		minimalConverted.Extra["fi.mau.meta.xma_fetch_status"] = "unrecognized"
		return minimalConverted
	}
}

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
		// URL previews don't typically need refresh metadata
		converted, err := mc.reuploadAttachment(
			ctx, att.AttachmentType, att.PreviewUrl, "preview", att.PreviewUrlMimeType, 0, int(att.PreviewWidth), int(att.PreviewHeight), 0,
			nil,
		)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to reupload URL preview image")
		} else {
			preview.ImageEncryption = converted.Content.File
			preview.ImageURL = converted.Content.URL
			preview.ImageWidth = event.IntOrString(converted.Content.Info.Width)
			preview.ImageHeight = event.IntOrString(converted.Content.Info.Height)
			preview.ImageSize = event.IntOrString(converted.Content.Info.Size)
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
	// This is minimal conversion; fetchFullXMA will enhance with proper refresh metadata
	converted, err := mc.reuploadAttachment(
		ctx, att.AttachmentType, url, att.Filename, mime, int(att.Filesize), int(width), int(height), 0,
		nil,
	)
	if errors.Is(err, mediadl.ErrURLNotFound) && att.TitleText != "" {
		return []*bridgev2.ConvertedMessagePart{{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    att.TitleText,
			},
		}}
	} else if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to transfer XMA media")
		converted = errorToNotice(err, "XMA")
	} else {
		converted = mc.fetchFullXMA(ctx, att, converted)
	}
	_, hasExternalURL := converted.Extra["external_url"]
	if !hasExternalURL && att.CTA != nil && att.CTA.ActionUrl != "" {
		externalURL := removeLPHP(att.CTA.ActionUrl)
		converted.Extra["external_url"] = externalURL
		addExternalURLCaption(converted.Content, externalURL)
	}
	parts := []*bridgev2.ConvertedMessagePart{converted}
	if att.TitleText != "" || att.CaptionBodyText != "" {
		converted.Extra["com.beeper.meta.full_post_title"] = att.TitleText
		converted.Extra["com.beeper.meta.caption_body_text"] = att.CaptionBodyText
	}
	return parts
}

func (mc *MessageConverter) reuploadAttachment(
	ctx context.Context, attachmentType table.AttachmentType,
	url, fileName, mimeType string,
	fileSize, width, height, duration int,
	refreshMeta *mediadl.MediaRefreshMeta,
) (*bridgev2.ConvertedMessagePart, error) {
	return mediadl.ReuploadFileToMatrix(ctx, mediadl.ReuploadParams{
		AttachmentType: attachmentType,
		URL:            url,
		FileName:       fileName,
		MimeType:       mimeType,
		FileSize:       fileSize,
		Width:          width,
		Height:         height,
		Duration:       duration,
		RefreshMeta:    refreshMeta,
		DirectMedia:    mc.DirectMedia,
		MaxFileSize:    mc.MaxFileSize,
	})
}
