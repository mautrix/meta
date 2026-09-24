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
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
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
	var buttons strings.Builder
	for _, cta := range xma.CTAButtons {
		if cta == nil || cta.Title == "" || cta.ActionURL == "" {
			continue
		}
		fmt.Fprintf(&buttons, `<li><a href="%s">%s</a></li>`,
			html.EscapeString(cta.ActionURL), html.EscapeString(cta.Title))
	}
	if buttons.Len() > 0 {
		captionHTML += "<ul>" + buttons.String() + "</ul>"
	}
	if captionHTML == "" {
		return nil
	}
	return ptr.Ptr(format.HTMLToContent(captionHTML))
}

func xmaLooksLikeWhatsAppButton(xma *slidetypes.XMAContent) bool {
	return xma != nil && len(xma.CTAButtons) == 1 && strings.HasPrefix(xma.CTAButtons[0].ActionURL, "https://api.whatsapp.com/send")
}

func (mc *MessageConverter) wrapXMA(ctx context.Context, xma *slidetypes.XMAContent) *bridgev2.ConvertedMessagePart {
	if xma == nil {
		return nil
	}
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
	ctx = context.WithValue(ctx, mediadl.ContextKeyPartID, networkid.PartID(""))
	return mc.fetchXMA(ctx, xma, previewPart)
}

func isNumeric(str string) bool {
	for i := 0; i < len(str); i++ {
		if str[i] < '0' || str[i] > '9' {
			return false
		}
	}
	return len(str) > 0
}

type UnresolvedMediaContent struct {
	Kind string                     `json:"kind"`
	Base *event.MessageEventContent `json:"fi.mau.instagram.base_part,omitempty"`
}

func filterUnresolvedMediaContent(content *event.MessageEventContent) *event.MessageEventContent {
	if content == nil {
		return nil
	}
	return &event.MessageEventContent{
		MsgType:       content.MsgType,
		Body:          content.Body,
		FormattedBody: content.FormattedBody,
		URL:           content.URL,
		Info:          content.Info,
		File:          content.File,
	}
}

func (mc *MessageConverter) fetchXMA(ctx context.Context, xma *slidetypes.XMAContent, basePart *bridgev2.ConvertedMessagePart) *bridgev2.ConvertedMessagePart {
	if basePart.Extra == nil {
		basePart.Extra = make(map[string]any)
	}
	basePart.Extra["external_url"] = xma.TargetURL
	targetID, _ := strconv.ParseInt(xma.TargetID, 10, 64)
	targetURL, _ := url.Parse(xma.TargetURL)
	if targetURL == nil {
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "unrecognized url"
		return basePart
	}
	isStory := strings.HasPrefix(targetURL.Path, "/stories/") && isNumeric(path.Base(targetURL.Path))
	isMedia := targetID != 0
	if !isStory && !isMedia {
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "unrecognized type"
		return basePart
	}
	if basePart.DBMetadata == nil {
		basePart.DBMetadata = &metaid.MessageMetadata{}
	}
	if !mediadl.ShouldFetchXMA(ctx) {
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "skip"
		basePart.Extra["com.beeper.unresolved_media"] = &UnresolvedMediaContent{
			Kind: "permanent",
			Base: filterUnresolvedMediaContent(basePart.Content),
		}
		basePart.DBMetadata.(*metaid.MessageMetadata).XMAFetchMeta = &metaid.XMAFetchMeta{
			TargetURL: xma.TargetURL,
			TargetID:  targetID,
			IsStory:   isStory,
		}
		return basePart
	}
	baseDMM := basePart.DBMetadata.(*metaid.MessageMetadata).DirectMediaMeta
	var resultPart *bridgev2.ConvertedMessagePart
	var fetchStatus string
	var fetchErr error
	if isStory {
		resultPart, fetchStatus, fetchErr = mc.fetchXMAStory(ctx, targetURL, basePart.Content, baseDMM)
	} else {
		resultPart, fetchStatus, fetchErr = mc.fetchXMAMedia(ctx, targetURL, targetID, basePart.Content, baseDMM)
	}
	if fetchErr != nil {
		zerolog.Ctx(ctx).Warn().Err(fetchErr).
			Str("target_url", xma.TargetURL).
			Int64("target_id", targetID).
			Msg("Failed to fetch XMA item")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = fetchStatus
		return basePart
	}
	return resultPart
}

func (mc *MessageConverter) FetchUnresolvedXMA(
	ctx context.Context,
	client *instameow.Client,
	userLogin *bridgev2.UserLogin,
	portal *bridgev2.Portal,
	msg *database.Message,
	unresolvedMedia json.RawMessage,
) (*bridgev2.ConvertedEditPart, error) {
	ctx = context.WithValue(ctx, mediadl.ContextKeyIGClient, client)
	ctx = context.WithValue(ctx, mediadl.ContextKeyIntent, portal.Bridge.Bot)
	ctx = context.WithValue(ctx, mediadl.ContextKeyUserLogin, userLogin)
	ctx = context.WithValue(ctx, mediadl.ContextKeyPortal, portal)
	ctx = context.WithValue(ctx, mediadl.ContextKeyMsgID, msg.ID)
	var unresolvedMediaContent UnresolvedMediaContent
	err := json.Unmarshal(unresolvedMedia, &unresolvedMediaContent)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal unresolved media: %w", err)
	} else if unresolvedMediaContent.Base == nil {
		return nil, fmt.Errorf("unrecognized unresolved media content")
	}
	msgMeta := msg.Metadata.(*metaid.MessageMetadata)
	if msgMeta.XMAFetched {
		zerolog.Ctx(ctx).Debug().Msg("Received fetch request for already resolved XMA item, returning no-op response")
		return nil, nil
	}
	if msgMeta.XMAFetchMeta == nil {
		return nil, mautrix.MNotFound.WithMessage("Message isn't a fetchable reel or story")
	}
	targetURL, err := url.Parse(msgMeta.XMAFetchMeta.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target URL: %w", err)
	}
	baseContent := unresolvedMediaContent.Base
	baseDMM := msgMeta.DirectMediaMeta
	var resultPart *bridgev2.ConvertedMessagePart
	if msgMeta.XMAFetchMeta.IsStory {
		resultPart, _, err = mc.fetchXMAStory(ctx, targetURL, baseContent, baseDMM)
	} else {
		resultPart, _, err = mc.fetchXMAMedia(ctx, targetURL, msgMeta.XMAFetchMeta.TargetID, baseContent, baseDMM)
	}
	// TODO for some errors, there should be an edit part to remove the unresolved status
	if err != nil {
		return nil, err
	}
	editPart := resultPart.ToEditPart(msg)
	if editPart.TopLevelExtra == nil {
		editPart.TopLevelExtra = make(map[string]any)
	}
	editPart.TopLevelExtra["com.beeper.dont_render_edited"] = true
	return editPart, nil
}

func (mc *MessageConverter) fetchXMAStory(
	ctx context.Context,
	targetURL *url.URL,
	baseContent *event.MessageEventContent,
	baseDirectMediaMeta json.RawMessage,
) (*bridgev2.ConvertedMessagePart, string, error) {
	cli := ctx.Value(mediadl.ContextKeyIGClient).(*instameow.Client)
	mediaID := path.Base(targetURL.Path)
	reelID := targetURL.Query().Get("reel_id")
	resp, err := cli.FetchReel(ctx, []string{reelID}, mediaID)
	if err != nil {
		return nil, "fetch fail", err
	}
	reel, ok := resp.Reels[reelID]
	if !ok {
		return nil, "empty response", fmt.Errorf("got empty XMA story response")
	}
	var targetItem *responses.ReelItem
	for _, item := range reel.Items {
		if item.Pk == mediaID {
			targetItem = item
			break
		}
	}
	if targetItem == nil {
		return nil, "item not found in response", fmt.Errorf("no matching reel item found in XMA response")
	}
	return mc.wrapXMAItem(ctx, &targetItem.Items, targetURL.String(), baseContent, baseDirectMediaMeta, &mediadl.MediaRefreshMeta{
		StoryMediaID: mediaID,
		StoryReelID:  reelID,
	})
}

var mediaShortcodeRegex = regexp.MustCompile(`/(?:reel|p)/([a-zA-Z0-9_-]+)/?`)

func (mc *MessageConverter) fetchXMAMedia(
	ctx context.Context,
	targetURL *url.URL,
	targetID int64,
	baseContent *event.MessageEventContent,
	baseDirectMediaMeta json.RawMessage,
) (*bridgev2.ConvertedMessagePart, string, error) {
	cli := ctx.Value(mediadl.ContextKeyIGClient).(*instameow.Client)
	carouselMediaID := targetURL.Query().Get("carousel_share_child_media_id")
	var mediaShortcode string
	if match := mediaShortcodeRegex.FindStringSubmatch(targetURL.Path); len(match) > 1 {
		mediaShortcode = match[1]
	}
	resp, err := cli.FetchMedia(ctx, strconv.FormatInt(targetID, 10), mediaShortcode)
	if err != nil {
		return nil, "fetch fail", err
	} else if len(resp.Items) == 0 {
		return nil, "empty response", fmt.Errorf("no items found in XMA media fetch response")
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
	return mc.wrapXMAItem(ctx, targetItem, targetURL.String(), baseContent, baseDirectMediaMeta, &mediadl.MediaRefreshMeta{
		XMATargetID:     targetID,
		XMAShortcode:    mediaShortcode,
		CarouselMediaID: carouselMediaID,
	})
}

func (mc *MessageConverter) wrapXMAItem(
	ctx context.Context,
	targetItem *responses.Items,
	externalURL string,
	baseContent *event.MessageEventContent,
	baseDirectMediaMeta json.RawMessage,
	refreshMeta *mediadl.MediaRefreshMeta,
) (*bridgev2.ConvertedMessagePart, string, error) {
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
		return nil, "reupload fail", fmt.Errorf("failed to reupload media: %w", err)
	}
	part.Content.EnsureHasHTML()
	part.Extra["external_url"] = externalURL
	part.Content.Body = baseContent.Body
	part.Content.FormattedBody = baseContent.FormattedBody
	if part.Content.FormattedBody != "" {
		part.Content.Format = event.FormatHTML
	}
	info := part.Content.Info
	// TODO support thumbnails with direct media
	if info != nil && baseContent.MsgType == event.MsgImage {
		part.DBMetadata.(*metaid.MessageMetadata).DirectMediaMeta = baseDirectMediaMeta
		info.ThumbnailURL = baseContent.URL
		info.ThumbnailFile = baseContent.File
		info.ThumbnailInfo = baseContent.Info
	}
	part.DBMetadata.(*metaid.MessageMetadata).XMAFetched = true
	return part, "", nil
}
