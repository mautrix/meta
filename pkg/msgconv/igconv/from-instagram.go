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
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
	"go.mau.fi/mautrix-meta/pkg/msgconv/textfmt"
)

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

func (mc *MessageConverter) ToMatrix(
	ctx context.Context,
	portal *bridgev2.Portal,
	client *instameow.Client,
	intent bridgev2.MatrixAPI,
	messageID networkid.MessageID,
	msg *slidetypes.Message,
	disableXMA bool,
) *bridgev2.ConvertedMessage {
	ctx = context.WithValue(ctx, mediadl.ContextKeyIGClient, client)
	ctx = context.WithValue(ctx, mediadl.ContextKeyIntent, intent)
	ctx = context.WithValue(ctx, mediadl.ContextKeyPortal, portal)
	ctx = context.WithValue(ctx, mediadl.ContextKeyFetchXMA, !disableXMA)
	ctx = context.WithValue(ctx, mediadl.ContextKeyMsgID, messageID)
	cm := &bridgev2.ConvertedMessage{
		Parts: make([]*bridgev2.ConvertedMessagePart, 0),
	}
	if msg.RepliedToMessageID != "" {
		cm.ReplyTo = &networkid.MessageOptionalPartID{
			MessageID: metaid.MakeFBMessageID(msg.RepliedToMessageID),
		}
		cm.ReplyToUser = metaid.MakeUserID(msg.RepliedToMessage.SenderFBID)
		cm.ReplyToLogin = metaid.MakeUserLoginID(msg.RepliedToMessage.SenderFBID)
	}
	switch content := msg.Content.Content.(type) {
	case *slidetypes.MessageContentText:
		cm.Parts = append(cm.Parts, mc.wrapText(ctx, content.TextBody, msg.Mentions))
	case *slidetypes.MessageContentAdminText:
		cm.Parts = append(cm.Parts, mc.wrapAdminText(content.TextFragments))
	case *slidetypes.MessageContentImage:
		for i, att := range content.Attachments {
			cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "image", i, mc.attachmentReuploadParams(att, table.AttachmentTypeImage)))
		}
	case *slidetypes.MessageContentVideo:
		for i, att := range content.Videos {
			cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "video", i, mc.attachmentReuploadParams(att, table.AttachmentTypeVideo)))
		}
	case *slidetypes.MessageContentAudio:
		for i, att := range content.AudioAttachments {
			cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "audio", i, mc.audioReuploadParams(att)))
		}
	case *slidetypes.MessageContentMultiMedia:
		for i, att := range content.Attachments {
			cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "multimedia", i, mc.attachmentReuploadParams(att, table.AttachmentTypeNone)))
		}
	case *slidetypes.MessageContentAnimatedMedia:
		for i, att := range content.AnimatedMedia {
			cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "animated media", i, mc.animatedMediaReuploadParams(att)))
		}
	case *slidetypes.MessageContentRavenImage:
		if content.Attachment == nil && content.ViewMode.ViewType() != "" {
			cm.Parts = append(cm.Parts, mc.makeViewOnceError("image", content.ViewMode.ViewType()))
		} else {
			cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "raven image", 0, mc.attachmentReuploadParams(content.Attachment, table.AttachmentTypeImage)))
		}
	case *slidetypes.MessageContentRavenVideo:
		if content.Attachment == nil && content.ViewMode.ViewType() != "" {
			cm.Parts = append(cm.Parts, mc.makeViewOnceError("video", content.ViewMode.ViewType()))
		} else {
			cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "raven video", 0, mc.attachmentReuploadParams(content.Attachment, table.AttachmentTypeVideo)))
		}
	case *slidetypes.MessageContentSticker:
		cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "sticker", 0, mc.stickerReuploadParams(content)))
	case *slidetypes.MessageContentMusicSticker:
		cm.Parts = append(cm.Parts, mc.wrapMedia(ctx, "music sticker", 0, mc.musicStickerReuploadParams(content)))
	case *slidetypes.MessageContentXMA:
		if content.XMATextBody != "" && content.XMA.TargetID == "" {
			part := mc.wrapText(ctx, content.XMATextBody, msg.Mentions)
			preview, err := mc.wrapLinkPreview(ctx, content.XMA)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to wrap XMA link preview")
			} else {
				part.Content.BeeperLinkPreviews = []*event.BeeperLinkPreview{preview}
			}
			// Replies to instants don't have any media on web, but do have the quote text
			if content.XMA.EyebrowText != "" {
				part.Content.FormattedBody = fmt.Sprintf(
					"<blockquote>%s</blockquote>%s",
					html.EscapeString(content.XMA.EyebrowText),
					part.Content.FormattedBody,
				)
			}
			cm.Parts = append(cm.Parts, part)
		} else if xmaPart := mc.wrapXMA(ctx, content.XMA); xmaPart != nil {
			cm.Parts = append(cm.Parts, xmaPart)
			if content.XMATextBody != "" {
				textPart := mc.wrapText(ctx, content.XMATextBody, msg.Mentions)
				textPart.ID = "xma-text-body"
				cm.Parts = append(cm.Parts, textPart)
			}
		} else {
			cm.Parts = append(cm.Parts, mc.wrapUnsupportedContent(content))
		}
	case *slidetypes.MessageContentAIRichResponse:
		// TODO the AI types haven't been observed in the wild to confirm the schema
		cm.Parts = append(cm.Parts, mc.wrapText(ctx, content.UnifiedResponse, nil))
	case *slidetypes.MessageContentAISearchResponse:
		cm.Parts = append(cm.Parts, mc.wrapText(ctx, content.MessageTextBody, nil))
	case slidetypes.MessageContentUnknown:
		cm.Parts = append(cm.Parts, mc.wrapUnsupportedContent(content))
	default:
		// This shouldn't happen since all the structs are defined in our code
		zerolog.Ctx(ctx).Warn().Type("content_struct", content).Msg("Unrecognized content struct in message")
		cm.Parts = append(cm.Parts, mc.wrapUnsupportedContent(content))
	}
	return cm
}

func (mc *MessageConverter) MetaToMatrixText(ctx context.Context, text string, mentions slidetypes.MentionList) *event.MessageEventContent {
	return textfmt.MetaToMatrixText(ctx, text, mentions.ToSocket(), mc.getBasicUserInfo)
}

func (mc *MessageConverter) makeViewOnceError(mediaType, viewed string) *bridgev2.ConvertedMessagePart {
	body := fmt.Sprintf("This %s can only be %s once. Use the Instagram app to view.", mediaType, viewed)
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    body,
		},
	}
}

func (mc *MessageConverter) wrapText(ctx context.Context, text string, mentions slidetypes.MentionList) *bridgev2.ConvertedMessagePart {
	return &bridgev2.ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: mc.MetaToMatrixText(ctx, text, mentions),
	}
}

func (mc *MessageConverter) wrapAdminText(fragments []slidetypes.TextFragment) *bridgev2.ConvertedMessagePart {
	var buf strings.Builder
	for _, f := range fragments {
		htmlText := event.TextToHTML(f.Plaintext)
		if f.LinkFragment != nil {
			_, _ = buf.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(f.LinkFragment.URI), htmlText))
		} else {
			buf.WriteString(htmlText)
		}
	}
	content := format.HTMLToContent(buf.String())
	content.MsgType = event.MsgNotice
	return &bridgev2.ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: &content,
	}
}

func (mc *MessageConverter) wrapLinkPreview(ctx context.Context, xma *slidetypes.XMAContent) (*event.BeeperLinkPreview, error) {
	realURL := xma.TargetURL
	parsedURL, err := url.Parse(xma.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XMA target URL: %w", err)
	}
	if parsedURL.Host == "l.facebook.com" {
		realURL = parsedURL.Query().Get("u")
	}
	img := cmp.Or(xma.PreviewImage, xma.XMAPreviewImage)
	if img == nil {
		return &event.BeeperLinkPreview{
			LinkPreview: event.LinkPreview{
				CanonicalURL: realURL,
				Title:        xma.TitleText,
				Description:  xma.SubtitleText,
			},
			MatchedURL: realURL,
		}, nil
	}
	res, err := mediadl.ReuploadFileToMatrix(ctx, mediadl.ReuploadParams{
		AttachmentType: table.AttachmentTypeImage,
		URL:            img.URL,
		PreviewWidth:   img.Width,
		PreviewHeight:  img.Height,
		RefreshMeta:    &mediadl.MediaRefreshMeta{},
		DirectMedia:    mc.DirectMedia,
		MaxFileSize:    mc.MaxFileSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to reupload XMA preview image: %w", err)
	}
	return &event.BeeperLinkPreview{
		LinkPreview: event.LinkPreview{
			CanonicalURL: realURL,
			Title:        xma.TitleText,
			Description:  xma.SubtitleText,
			ImageURL:     res.Content.URL,
			ImageSize:    event.IntOrString(res.Content.GetInfo().Size),
			ImageWidth:   event.IntOrString(res.Content.GetInfo().Width),
			ImageHeight:  event.IntOrString(res.Content.GetInfo().Height),
			ImageType:    res.Content.GetInfo().MimeType,
		},
		MatchedURL:      realURL,
		ImageEncryption: res.Content.File,
		ImageBlurhash:   res.Content.GetInfo().Blurhash,
	}, nil
}

func (mc *MessageConverter) wrapMedia(
	ctx context.Context,
	typeName string,
	index int,
	params mediadl.ReuploadParams,
) *bridgev2.ConvertedMessagePart {
	if params.URL == "" {
		return &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    fmt.Sprintf("Unrecognized %s attachment message. Use the Instagram app to view.", typeName),
			},
		}
	}
	params.DirectMedia = mc.DirectMedia
	params.MaxFileSize = mc.MaxFileSize
	if params.RefreshMeta == nil {
		params.RefreshMeta = &mediadl.MediaRefreshMeta{}
	}
	params.RefreshMeta.PartIndex = index

	partID := networkid.PartID(fmt.Sprintf("%s-%d", strings.ReplaceAll(typeName, " ", ""), index))
	ctx = context.WithValue(ctx, mediadl.ContextKeyPartID, partID)

	res, err := mediadl.ReuploadFileToMatrix(ctx, params)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to reupload attachment")
		return errorToNotice(err, typeName, params)
	}
	res.ID = partID
	return res
}

func (mc *MessageConverter) attachmentReuploadParams(att *slidetypes.Attachment, typ table.AttachmentType) mediadl.ReuploadParams {
	if att == nil {
		return mediadl.ReuploadParams{}
	}
	return mediadl.ReuploadParams{
		AttachmentType: typ,
		URL:            att.AttachmentCDNURL,
		PreviewWidth:   att.PreviewWidth,
		PreviewHeight:  att.PreviewHeight,
		RefreshMeta:    &mediadl.MediaRefreshMeta{AttachmentFBID: att.AttachmentFBID},
	}
}

func (mc *MessageConverter) audioReuploadParams(att *slidetypes.AudioAttachment) mediadl.ReuploadParams {
	if att == nil {
		return mediadl.ReuploadParams{}
	}
	return mediadl.ReuploadParams{
		AttachmentType: table.AttachmentTypeAudio,
		URL:            att.AttachmentCDNURL,
		Duration:       att.PlayableDurationMS,
		RefreshMeta:    &mediadl.MediaRefreshMeta{AttachmentFBID: att.AttachmentFBID},
	}
}

func (mc *MessageConverter) animatedMediaReuploadParams(att *slidetypes.AnimatedAttachment) mediadl.ReuploadParams {
	if att == nil {
		return mediadl.ReuploadParams{}
	}
	var typ table.AttachmentType
	var filename, url, mimeType string
	if att.AttachmentWebpURL != "" && (att.IsSticker || att.AttachmentMP4URL == "") {
		typ = table.AttachmentTypeSticker
		filename = att.AltText
		url = att.AttachmentWebpURL
		mimeType = "image/webp"
	} else if att.AttachmentMP4URL != "" {
		typ = table.AttachmentTypeAnimatedImage
		url = att.AttachmentMP4URL
		mimeType = "video/mp4"
	}
	return mediadl.ReuploadParams{
		AttachmentType: typ,
		URL:            url,
		PreviewWidth:   att.PreviewWidth,
		PreviewHeight:  att.PreviewHeight,
		MimeType:       mimeType,
		FileName:       filename,
	}
}

func (mc *MessageConverter) stickerReuploadParams(att *slidetypes.MessageContentSticker) mediadl.ReuploadParams {
	if att == nil {
		return mediadl.ReuploadParams{}
	}
	return mediadl.ReuploadParams{
		AttachmentType: table.AttachmentTypeSticker,
		URL:            att.PreviewURL,
		FileName:       att.AltText,
		PreviewWidth:   att.PreviewWidth,
		PreviewHeight:  att.PreviewHeight,
	}
}

func (mc *MessageConverter) musicStickerReuploadParams(att *slidetypes.MessageContentMusicSticker) mediadl.ReuploadParams {
	if att == nil {
		return mediadl.ReuploadParams{}
	}
	return mediadl.ReuploadParams{
		AttachmentType: table.AttachmentTypeAudio,
		URL:            att.AudioTrack.Web30SPreviewDownloadURL,
		RefreshMeta:    &mediadl.MediaRefreshMeta{AttachmentFBID: att.MediaContentFBID},
	}
}

func errorToNotice(err error, attachmentContainerType string, content any) *bridgev2.ConvertedMessagePart {
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
		Extra: unsupportedContentToExtra(content),
	}
}

func unsupportedContentToExtra(content any) map[string]any {
	if marshaled, _ := json.Marshal(content); len(marshaled) > 50000 {
		return map[string]any{
			"fi.mau.meta.unsupported_slide_content_too_long": true,
		}
	}
	return map[string]any{
		"fi.mau.meta.unsupported_slide_content": content,
	}
}

func (mc *MessageConverter) wrapUnsupportedContent(content any) *bridgev2.ConvertedMessagePart {
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    "Unsupported message. Use the Instagram app to view.",
		},
		Extra: unsupportedContentToExtra(content),
	}
}
