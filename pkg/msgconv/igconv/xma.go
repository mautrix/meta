// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
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

package igconv

import (
	"cmp"
	"context"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

func (mc *MessageConverter) wrapXMAPreviewImage(ctx context.Context, xma *slidetypes.XMAContent) *bridgev2.ConvertedMessagePart {
	part := cmp.Or(xma.PreviewImage, xma.XMAPreviewImage)
	if part == nil || part.URL == "" {
		return nil
	}
	return mc.wrapMedia(ctx, "xma preview image", 0, mediadl.ReuploadParams{
		AttachmentType: table.AttachmentTypeImage,
		URL:            part.URL,
		PreviewWidth:   part.Width,
		PreviewHeight:  part.Height,
	})
}

func (mc *MessageConverter) wrapXMACaption(ctx context.Context, xma *slidetypes.XMAContent) *event.MessageEventContent {
	var captionHTML string
	if xma.EyebrowText != "" {
		captionHTML += fmt.Sprintf("<blockquote>%s</blockquote>", html.EscapeString(xma.EyebrowText))
	}
	if xma.TitleText != "" {
		if len(xma.TitleText) > 110 {
			xma.TitleText = xma.TitleText[:100] + "…"
		}
		captionHTML += fmt.Sprintf("<strong>%s</strong>", html.EscapeString(xma.TitleText))
	}
	if xma.SubtitleText != "" {
		captionHTML += fmt.Sprintf("<br>%s", html.EscapeString(xma.SubtitleText))
	}
	if xma.CaptionBodyText != "" {
		captionHTML += fmt.Sprintf("<p>%s</p>", event.TextToHTML(xma.CaptionBodyText))
	}

	targetURL, err := url.Parse(xma.TargetURL)
	if err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Str("url", xma.TargetURL).Msg("Failed to parse XMA target URL")
	} else if targetURL.String() != "" {
		fullTargetURL := targetURL.String()
		targetURL.RawQuery = ""
		captionHTML += fmt.Sprintf(`<p><a href="%s">%s</a></p>`, html.EscapeString(fullTargetURL), html.EscapeString(targetURL.String()))
	}
	if captionHTML == "" {
		return nil
	}
	return ptr.Ptr(format.HTMLToContent(captionHTML))
}

const xmaPartID networkid.PartID = "xma"

func (mc *MessageConverter) wrapXMA(ctx context.Context, xma *slidetypes.XMAContent) *bridgev2.ConvertedMessagePart {
	ctx = context.WithValue(ctx, mediadl.ContextKeyPartID, xmaPartID)
	previewPart := mc.wrapXMAPreviewImage(ctx, xma)
	captionPart := mc.wrapXMACaption(ctx, xma)
	if previewPart == nil {
		if captionPart == nil {
			return nil
		}
		return &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: captionPart,
		}
	} else if captionPart != nil {
		previewPart.Content.EnsureHasHTML()
		previewPart.Content.Body = captionPart.Body
		previewPart.Content.Format = captionPart.Format
		previewPart.Content.FormattedBody = captionPart.FormattedBody
	}
	fetchedPart := mc.fetchXMA(ctx, xma, previewPart)
	fetchedPart.ID = xmaPartID
	return fetchedPart
}

func isNumeric(str string) bool {
	for i := 0; i < len(str); i++ {
		if str[i] < '0' || str[i] > '9' {
			return false
		}
	}
	return len(str) > 0
}

func (mc *MessageConverter) fetchXMA(ctx context.Context, xma *slidetypes.XMAContent, basePart *bridgev2.ConvertedMessagePart) *bridgev2.ConvertedMessagePart {
	if basePart.Extra == nil {
		basePart.Extra = make(map[string]any)
	}
	basePart.Extra["external_url"] = xma.TargetURL
	if !mediadl.ShouldFetchXMA(ctx) {
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "skip"
		return basePart
	}
	targetURL, _ := url.Parse(xma.TargetURL)
	if targetURL == nil {
		return basePart
	}
	if strings.HasPrefix(targetURL.Path, "/stories/") && isNumeric(path.Base(targetURL.Path)) {
		return mc.fetchXMAStory(ctx, targetURL, basePart)
	} else if targetIDInt, err := strconv.ParseInt(xma.TargetID, 10, 64); err == nil {
		return mc.fetchXMAMedia(ctx, targetURL, targetIDInt, basePart)
	}
	basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "unrecognized type"
	return basePart
}

func (mc *MessageConverter) fetchXMAStory(
	ctx context.Context,
	targetURL *url.URL,
	basePart *bridgev2.ConvertedMessagePart,
) *bridgev2.ConvertedMessagePart {
	cli := ctx.Value(mediadl.ContextKeyIGClient).(*instameow.Client)
	mediaID := path.Base(targetURL.Path)
	reelID := targetURL.Query().Get("reel_id")
	log := zerolog.Ctx(ctx).With().
		Str("action", "fetch xma story").
		Str("reel_id", reelID).
		Str("media_id", mediaID).
		Logger()
	resp, err := cli.FetchReel(ctx, []string{reelID}, mediaID)
	if err != nil {
		log.Err(err).Msg("Failed to fetch XMA story")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "fetch fail"
		return basePart
	}
	reel, ok := resp.Reels[reelID]
	if !ok {
		log.Warn().Msg("Got empty XMA story response")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "empty response"
		return basePart
	}
	var targetItem *responses.ReelItem
	for _, item := range reel.Items {
		if item.Pk == mediaID {
			targetItem = item
			break
		}
	}
	if targetItem == nil {
		log.Warn().Msg("No matching reel item found in XMA response")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "item not found in response"
		return basePart
	}
	return mc.wrapXMAItem(ctx, &targetItem.Items, basePart, &mediadl.MediaRefreshMeta{
		StoryMediaID: mediaID,
		StoryReelID:  reelID,
	})
}

var mediaShortcodeRegex = regexp.MustCompile(`/(?:reel|p)/([a-zA-Z0-9_-]+)/?`)

func (mc *MessageConverter) fetchXMAMedia(
	ctx context.Context,
	targetURL *url.URL,
	targetID int64,
	basePart *bridgev2.ConvertedMessagePart,
) *bridgev2.ConvertedMessagePart {
	log := zerolog.Ctx(ctx).With().Str("action", "fetch xma media").Int64("target_id", targetID).Logger()
	cli := ctx.Value(mediadl.ContextKeyIGClient).(*instameow.Client)
	carouselMediaID := targetURL.Query().Get("carousel_share_child_media_id")
	var mediaShortcode string
	if match := mediaShortcodeRegex.FindStringSubmatch(targetURL.Path); len(match) > 1 {
		mediaShortcode = match[1]
	}
	resp, err := cli.FetchMedia(ctx, strconv.FormatInt(targetID, 10), mediaShortcode)
	if err != nil {
		log.Err(err).Msg("Failed to fetch XMA media")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "fetch fail"
		return basePart
	} else if len(resp.Items) == 0 {
		log.Warn().Msg("No items found in XMA media fetch response")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "empty response"
		return basePart
	}
	targetItem := resp.Items[0]
	if carouselMediaID != "" {
		for _, subitem := range targetItem.CarouselMedia {
			if strings.Contains(subitem.ID, carouselMediaID) {
				targetItem = subitem
				break
			}
		}
	}
	return mc.wrapXMAItem(ctx, targetItem, basePart, &mediadl.MediaRefreshMeta{
		XMATargetID:     targetID,
		XMAShortcode:    mediaShortcode,
		CarouselMediaID: carouselMediaID,
	})
}

func (mc *MessageConverter) wrapXMAItem(
	ctx context.Context,
	targetItem *responses.Items,
	basePart *bridgev2.ConvertedMessagePart,
	refreshMeta *mediadl.MediaRefreshMeta,
) *bridgev2.ConvertedMessagePart {
	var width, height int
	var bestURL string
	attachmentType := table.AttachmentTypeVideo
	for _, video := range targetItem.VideoVersions {
		if video.Width*video.Height > width*height {
			bestURL = video.URL
			width = video.Width
			height = video.Height
		}
	}
	if bestURL == "" {
		attachmentType = table.AttachmentTypeImage
		for _, image := range targetItem.ImageVersions2.Candidates {
			if image.Width*image.Height > width*height {
				bestURL = image.URL
				width = image.Width
				height = image.Height
			}
		}
	}
	part, err := mediadl.ReuploadFileToMatrix(ctx, mediadl.ReuploadParams{
		AttachmentType: attachmentType,
		URL:            bestURL,
		Width:          width,
		Height:         height,
		RefreshMeta:    refreshMeta,
		DirectMedia:    mc.DirectMedia,
		MaxFileSize:    mc.MaxFileSize,
	})
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to reupload XMA media")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "reupload fail"
		return basePart
	}
	part.Content.EnsureHasHTML()
	part.Extra["external_url"] = basePart.Extra["external_url"]
	part.Content.Body = basePart.Content.Body
	part.Content.Format = basePart.Content.Format
	part.Content.FormattedBody = basePart.Content.FormattedBody
	info := part.Content.Info
	// TODO support thumbnails with direct media
	if info != nil && part.Content.MsgType == event.MsgVideo && basePart.Content.MsgType == event.MsgImage && !mc.DirectMedia {
		info.ThumbnailURL = basePart.Content.URL
		info.ThumbnailFile = basePart.Content.File
		info.ThumbnailInfo = basePart.Content.Info
	}
	return part
}
