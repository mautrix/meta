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
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
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

func (mc *MessageConverter) wrapXMACaption(ctx context.Context, xma *slidetypes.XMAContent, buttons Buttons) *event.MessageEventContent {
	if _, err := url.Parse(xma.TargetURL); err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Str("url", xma.TargetURL).Msg("Failed to parse XMA target URL")
	}
	captionHTML := CaptionHTML(xma, buttons, mc.ctaOptions(ctx))
	if captionHTML == "" {
		return nil
	}
	return ptr.Ptr(format.HTMLToContent(captionHTML))
}

// CaptionHTML builds the formatted caption of an XMA card: eyebrow, title, subtitle, body, target
// link and then the CTA buttons block. It returns "" if the card has no caption text or link.
func CaptionHTML(xma *slidetypes.XMAContent, buttons Buttons, opts CTAOptions) string {
	if xma == nil {
		return ""
	}
	var captionHTML string
	if xma.EyebrowText != "" {
		captionHTML += fmt.Sprintf("<blockquote>%s</blockquote>", html.EscapeString(xma.EyebrowText))
	}
	if xma.TitleText != "" {
		title := xma.TitleText
		if titleRunes := []rune(title); len(titleRunes) > 110 {
			title = string(titleRunes[:100]) + "…"
		}
		captionHTML += fmt.Sprintf("<strong>%s</strong>", html.EscapeString(title))
	}
	if xma.SubtitleText != "" {
		captionHTML += fmt.Sprintf("<br>%s", html.EscapeString(xma.SubtitleText))
	}
	if xma.CaptionBodyText != "" {
		captionHTML += fmt.Sprintf("<p>%s</p>", event.TextToHTML(xma.CaptionBodyText))
	}
	if targetURL, err := url.Parse(xma.TargetURL); err == nil {
		targetURL = unwrapLinkShim(targetURL)
		if targetURL.String() != "" {
			fullTargetURL := targetURL.String()
			targetURL.RawQuery = ""
			captionHTML += fmt.Sprintf(`<p><a href="%s">%s</a></p>`, html.EscapeString(fullTargetURL), html.EscapeString(targetURL.String()))
		}
	}
	if captionHTML == "" {
		return ""
	}
	return captionHTML + xmaCTAButtonsHTML(xma.CTAButtons, buttons.Postbacks, buttons.QuickReplies, opts)
}

// XMAFromThreadItem decodes the generic_xma of a REST thread item, which is either an object or
// an array whose first element is the card. It returns nil if there is none.
func XMAFromThreadItem(item *slidetypes.ThreadItem) *slidetypes.XMAContent {
	if item == nil || len(item.GenericXMA) == 0 {
		return nil
	}
	var xma slidetypes.XMAContent
	if err := json.Unmarshal(item.GenericXMA, &xma); err == nil {
		return &xma
	}
	var list []*slidetypes.XMAContent
	if err := json.Unmarshal(item.GenericXMA, &list); err == nil && len(list) > 0 && list[0] != nil {
		return list[0]
	}
	return nil
}

// CardHTMLFromThreadItem rebuilds the formatted body of a card (caption plus CTA buttons) from a
// REST thread item alone, without the original Matrix event. It returns "" if there is nothing to render.
func CardHTMLFromThreadItem(item *slidetypes.ThreadItem, opts CTAOptions) string {
	if item == nil {
		return ""
	}
	xma := XMAFromThreadItem(item)
	if xma == nil {
		buttons := ButtonsFromItem(item)
		ctaHTML := xmaCTAButtonsHTML(item.AllCTAButtons(), buttons.Postbacks, buttons.QuickReplies, opts)
		if ctaHTML != "" && item.Text != "" {
			return "<p>" + event.TextToHTML(item.Text) + "</p>" + ctaHTML
		}
		return ctaHTML
	}
	if ctas := item.AllCTAButtons(); len(ctas) > 0 {
		xma.CTAButtons = ctas
	}
	buttons := ButtonsFromItem(item)
	if cardHTML := CaptionHTML(xma, buttons, opts); cardHTML != "" {
		return cardHTML
	}
	// CTA-only card: no caption text, but the buttons still need refreshing.
	return xmaCTAButtonsHTML(xma.CTAButtons, buttons.Postbacks, buttons.QuickReplies, opts)
}

func unwrapLinkShim(u *url.URL) *url.URL {
	if u.Host == "l.facebook.com" {
		if realURL, err := url.Parse(u.Query().Get("u")); err == nil && realURL.String() != "" {
			return realURL
		}
	}
	return u
}

// PostbackTapCommand is the bridge command that taps a postback or quick reply button.
const PostbackTapCommand = "!ig tap"

const (
	ctaBlockOpen       = `<div data-ig-cta="">`
	ctaBlockClose      = "</div>"
	appFallbackSnippet = "Use the Instagram app to click buttons"
)

var (
	oldAppFallbackRegex = regexp.MustCompile(`<p>[^<]*(?:<br\s*/?>)?` + appFallbackSnippet + `</p>`)
	oldTapListRegex     = regexp.MustCompile(`<ol>(?:<li>[^<]*</li>)*</ol><p>Tap a button with <code>` + PostbackTapCommand + `[^<]*</code>[^<]*(?:<code>[^<]*</code>)?</p>`)
	oldLinkRunRegex     = regexp.MustCompile(`<p><a [^>]*>[^<]*</a>(?: - <a [^>]*>[^<]*</a>)+</p>`)
)

// keycapDigits are the reaction emojis that activate buttons 1-9 (digit + VS16 + keycap).
const keycapSuffix = "\ufe0f\u20e3"
const keycapTen = "\U0001F51F"

// KeycapEmoji returns the keycap emoji for a 1-based button index, or "" above 10.
func KeycapEmoji(n int) string {
	switch {
	case n >= 1 && n <= 9:
		return strconv.Itoa(n) + keycapSuffix
	case n == 10:
		return keycapTen
	}
	return ""
}

// KeycapReactionIndex parses a keycap reaction (1️⃣-9️⃣, with or without VS16, or 🔟) into a
// 1-based button index. Other emojis, including 0️⃣ and ❤, aren't button triggers.
func KeycapReactionIndex(emoji string) (int, bool) {
	emoji = strings.TrimSpace(emoji)
	if emoji == keycapTen {
		return 10, true
	}
	digits, ok := strings.CutSuffix(emoji, "\u20e3")
	if !ok {
		return 0, false
	}
	digits = strings.TrimSuffix(digits, "\ufe0f")
	if len(digits) != 1 || digits[0] < '1' || digits[0] > '9' {
		return 0, false
	}
	return int(digits[0] - '0'), true
}

// ButtonAtIndex returns the 1-based button of the list, or nil if the index is out of range.
func ButtonAtIndex(buttons []*metaid.PostbackButton, idx int) *metaid.PostbackButton {
	if idx < 1 || idx > len(buttons) {
		return nil
	}
	return buttons[idx-1]
}

const nbsp = "\u00a0"

// pseudoButtonLabel is the visible bracketed control ("[ Título ]"). NBSPs keep
// the brackets from wrapping onto their own line. The bridge never adds a ↗;
// Beeper already appends one after external links.
func pseudoButtonLabel(title string) string {
	return "[" + nbsp + title + nbsp + "]"
}

func pseudoButtonHTML(href, title string) string {
	label := html.EscapeString(pseudoButtonLabel(title))
	if href == "" {
		return label
	}
	return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(href), label)
}

// buttonLineHTML renders one tappable button as a bracketed pseudo-button.
func buttonLineHTML(idx int, button *metaid.PostbackButton, opts CTAOptions) string {
	return pseudoButtonHTML(opts.ButtonURL(idx, button), button.Title)
}

// xmaCTAButtonsHTML renders each CTA as a bracketed pseudo-button, one per line,
// joined by <br>, with no lists and no footer. The block is wrapped in a marked
// div so it can be replaced later (see ReplaceOrAppendCTAButtonsHTML).
func xmaCTAButtonsHTML(ctas []*slidetypes.CTAButton, postbacks, quickReplies []*metaid.PostbackButton, opts CTAOptions) string {
	tap := metaid.TapButtons(postbacks, quickReplies)
	var lines, unrecovered []string
	for _, cta := range ctas {
		if cta == nil || cta.Title == "" {
			continue
		}
		if cta.IsLink() {
			actionURL, err := url.Parse(cta.ActionURL)
			if err == nil {
				actionURL = unwrapLinkShim(actionURL)
				if actionURL.Scheme == "https" || actionURL.Scheme == "http" {
					lines = append(lines, pseudoButtonHTML(actionURL.String(), cta.Title))
				}
			}
		} else if !slices.ContainsFunc(tap, func(b *metaid.PostbackButton) bool { return b.Title == cta.Title }) {
			unrecovered = append(unrecovered, html.EscapeString(cta.Title))
		}
	}
	for i, button := range tap {
		lines = append(lines, buttonLineHTML(i+1, button, opts))
	}
	var out strings.Builder
	if len(lines) > 0 {
		out.WriteString(strings.Join(lines, "<br>"))
	}
	if len(unrecovered) > 0 && len(tap) == 0 {
		fmt.Fprintf(&out, "<p>%s<br>%s</p>", strings.Join(unrecovered, ", "), appFallbackSnippet)
	}
	if out.Len() == 0 {
		return ""
	}
	return ctaBlockOpen + out.String() + ctaBlockClose
}

// BuildCTAButtonsHTML is the exported form of xmaCTAButtonsHTML, used when refreshing old cards.
func BuildCTAButtonsHTML(ctas []*slidetypes.CTAButton, postbacks, quickReplies []*metaid.PostbackButton, opts CTAOptions) string {
	return xmaCTAButtonsHTML(ctas, postbacks, quickReplies, opts)
}

// HasKnownCTAFooter reports whether a formatted body already contains a CTA footer that
// ReplaceOrAppendCTAButtonsHTML would replace or strip.
func HasKnownCTAFooter(formattedBody string) bool {
	return strings.Contains(formattedBody, ctaBlockOpen) ||
		oldAppFallbackRegex.MatchString(formattedBody) ||
		oldTapListRegex.MatchString(formattedBody) ||
		oldLinkRunRegex.MatchString(formattedBody)
}

// ReplaceOrAppendCTAButtonsHTML puts ctaHTML into a formatted body: it replaces an existing
// marked CTA block, or otherwise strips the known old CTA footers and appends the new block.
// Everything else (caption, media description) is kept.
func ReplaceOrAppendCTAButtonsHTML(formattedBody, ctaHTML string) string {
	if start := strings.Index(formattedBody, ctaBlockOpen); start >= 0 {
		if end := strings.Index(formattedBody[start:], ctaBlockClose); end >= 0 {
			end += start + len(ctaBlockClose)
			return formattedBody[:start] + ctaHTML + formattedBody[end:]
		}
	}
	formattedBody = oldAppFallbackRegex.ReplaceAllString(formattedBody, "")
	formattedBody = strings.ReplaceAll(formattedBody, appFallbackSnippet, "")
	formattedBody = oldTapListRegex.ReplaceAllString(formattedBody, "")
	formattedBody = oldLinkRunRegex.ReplaceAllString(formattedBody, "")
	return formattedBody + ctaHTML
}

func xmaLooksLikeWhatsAppButton(xma *slidetypes.XMAContent) bool {
	return xma != nil && len(xma.CTAButtons) == 1 && strings.HasPrefix(xma.CTAButtons[0].ActionURL, "https://api.whatsapp.com/send")
}

func (mc *MessageConverter) wrapXMA(ctx context.Context, xma *slidetypes.XMAContent, msg *slidetypes.Message) *bridgev2.ConvertedMessagePart {
	if xma == nil {
		return nil
	}
	buttons := mc.recoverButtons(ctx, xma, msg)
	part := mc.wrapXMAWithButtons(ctx, xma, buttons)
	attachButtons(part, buttons)
	return part
}

func (mc *MessageConverter) wrapXMAWithButtons(ctx context.Context, xma *slidetypes.XMAContent, buttons Buttons) *bridgev2.ConvertedMessagePart {
	previewPart := mc.wrapXMAPreviewImage(ctx, xma)
	captionPart := mc.wrapXMACaption(ctx, xma, buttons)
	opts := mc.ctaOptions(ctx)
	if captionPart == nil {
		if buttonsHTML := xmaCTAButtonsHTML(xma.CTAButtons, buttons.Postbacks, buttons.QuickReplies, opts); buttonsHTML != "" {
			captionPart = ptr.Ptr(format.HTMLToContent(buttonsHTML))
		}
	}
	if previewPart == nil {
		if captionPart == nil {
			return nil
		}
		part := &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: captionPart,
		}
		if xma.TitleText != "" {
			part.Extra = map[string]any{"com.beeper.meta.full_post_title": xma.TitleText}
		}
		return part
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
	Kind      string                     `json:"kind"`
	MediaType string                     `json:"media_type,omitempty"`
	Base      *event.MessageEventContent `json:"fi.mau.instagram.base_part,omitempty"`
	FullTitle string                     `json:"fi.mau.instagram.full_title,omitempty"`
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
	if xma.TitleText != "" {
		basePart.Extra["com.beeper.meta.full_post_title"] = xma.TitleText
	}
	targetID, _ := strconv.ParseInt(xma.TargetID, 10, 64)
	targetURL, _ := url.Parse(xma.TargetURL)
	if targetURL == nil {
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "unrecognized url"
		return basePart
	}
	isStory := strings.HasPrefix(targetURL.Path, "/stories/") && isNumeric(path.Base(targetURL.Path))
	isMedia := targetID != 0
	if !isStory && !isMedia {
		zerolog.Ctx(ctx).Debug().
			Str("xma_typename", xma.Typename).
			Str("target_id", xma.TargetID).
			Str("target_url", xma.TargetURL).
			Str("preview_layout_type", xma.PreviewLayoutType).
			Strs("unrecognized_fields", slices.Sorted(maps.Keys(xma.Unrecognized))).
			Msg("XMA target isn't a fetchable post or story")
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "unrecognized type"
		return basePart
	}
	if basePart.DBMetadata == nil {
		basePart.DBMetadata = &metaid.MessageMetadata{}
	}
	if !mediadl.ShouldFetchXMA(ctx) {
		var mediaType string
		if (xma.PreviewImage != nil && xma.PreviewImage.PreviewImageDecorationType == "REEL") ||
			(xma.XMAPreviewImage != nil && xma.XMAPreviewImage.PreviewImageDecorationType == "REEL") {
			mediaType = "video"
		}
		basePart.Extra["fi.mau.instagram.xma_fetch_status"] = "skip"
		basePart.Extra["com.beeper.unresolved_media"] = &UnresolvedMediaContent{
			Kind:      "permanent",
			MediaType: mediaType,
			Base:      filterUnresolvedMediaContent(basePart.Content),
			FullTitle: xma.TitleText,
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
	if xma.TitleText != "" {
		resultPart.Extra["com.beeper.meta.full_post_title"] = xma.TitleText
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
	if unresolvedMediaContent.FullTitle != "" {
		resultPart.Extra["com.beeper.meta.full_post_title"] = unresolvedMediaContent.FullTitle
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
		if item.Pk.String() == mediaID {
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
