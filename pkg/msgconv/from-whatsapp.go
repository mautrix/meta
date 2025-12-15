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
	"encoding/json"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/armadilloutil"
	"go.mau.fi/whatsmeow/proto/instamadilloAddMessage"
	"go.mau.fi/whatsmeow/proto/waArmadilloApplication"
	"go.mau.fi/whatsmeow/proto/waArmadilloXMA"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	metaTypes "go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (mc *MessageConverter) WhatsAppTextToMatrix(ctx context.Context, text *waCommon.MessageText) *bridgev2.ConvertedMessagePart {
	content := &event.MessageEventContent{
		MsgType:  event.MsgText,
		Body:     text.GetText(),
		Mentions: &event.Mentions{},
	}
	silent := false
	if len(text.Commands) > 0 {
		for _, cmd := range text.Commands {
			switch cmd.GetCommandType() {
			case waCommon.Command_SILENT:
				silent = true
				content.Mentions.Room = false
			case waCommon.Command_EVERYONE:
				if !silent {
					content.Mentions.Room = true
				}
			case waCommon.Command_AI:
				// TODO ???
			}
		}
	}
	if len(text.GetMentionedJID()) > 0 {
		content.Format = event.FormatHTML
		content.FormattedBody = event.TextToHTML(content.Body)
		for _, jid := range text.GetMentionedJID() {
			parsed, err := types.ParseJID(jid)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Str("jid", jid).Msg("Failed to parse mentioned JID")
				continue
			}
			mxid, displayname, err := mc.getBasicUserInfo(ctx, metaid.MakeWAUserID(parsed))
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Str("jid", jid).Msg("Failed to get user info for mentioned JID")
				continue
			}
			if !silent {
				content.Mentions.UserIDs = append(content.Mentions.UserIDs, mxid)
			}
			mentionText := "@" + jid
			content.Body = strings.ReplaceAll(content.Body, mentionText, displayname)
			content.FormattedBody = strings.ReplaceAll(content.FormattedBody, mentionText, fmt.Sprintf(`<a href="%s">%s</a>`, mxid.URI().MatrixToURL(), event.TextToHTML(displayname)))
		}
	}
	return &bridgev2.ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: content,
	}
}

type MediaTransportContainer interface {
	GetTransport() *waMediaTransport.WAMediaTransport
}

type AttachmentTransport[Integral MediaTransportContainer, Ancillary any] interface {
	GetIntegral() Integral
	GetAncillary() Ancillary
}

type AttachmentMessage[Integral MediaTransportContainer, Ancillary any, Transport AttachmentTransport[Integral, Ancillary]] interface {
	Decode() (Transport, error)
}

type AttachmentMessageWithCaption[Integral MediaTransportContainer, Ancillary any, Transport AttachmentTransport[Integral, Ancillary]] interface {
	GetCaption() *waCommon.MessageText
}

type convertFunc func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error)

type userVisibleError struct {
	Message string
}

func (u userVisibleError) Error() string {
	return u.Message
}

func convertWhatsAppAttachment[
	Transport AttachmentTransport[Integral, Ancillary],
	Integral MediaTransportContainer,
	Ancillary any,
](
	ctx context.Context,
	mc *MessageConverter,
	msg AttachmentMessage[Integral, Ancillary, Transport],
	mediaType whatsmeow.MediaType,
	convert convertFunc,
) (metadata Ancillary, media, caption *bridgev2.ConvertedMessagePart, err error) {
	var typedTransport Transport
	typedTransport, err = msg.Decode()
	if err != nil {
		return
	}
	untypedTransport := any(typedTransport)
	if stickerTransport, ok := untypedTransport.(*waMediaTransport.StickerTransport); ok {
		if stickerTransport.Ancillary.GetReceiverFetchID() != "" {
			err = userVisibleError{Message: "Unsupported sticker, view in Messenger"}
			return
		}
	}
	msgWithCaption, ok := msg.(AttachmentMessageWithCaption[Integral, Ancillary, Transport])
	if ok && len(msgWithCaption.GetCaption().GetText()) > 0 {
		caption = mc.WhatsAppTextToMatrix(ctx, msgWithCaption.GetCaption())
		caption.Content.MsgType = event.MsgText
	}
	metadata = typedTransport.GetAncillary()
	transport := typedTransport.GetIntegral().GetTransport()
	media, err = mc.reuploadWhatsAppAttachment(ctx, transport, mediaType, convert)
	return
}

func (mc *MessageConverter) reuploadWhatsAppAttachment(
	ctx context.Context,
	transport *waMediaTransport.WAMediaTransport,
	mediaType whatsmeow.MediaType,
	convert convertFunc,
) (*bridgev2.ConvertedMessagePart, error) {
	client := ctx.Value(contextKeyWAClient).(*whatsmeow.Client)
	intent := ctx.Value(contextKeyIntent).(bridgev2.MatrixAPI)
	portal := ctx.Value(contextKeyPortal).(*bridgev2.Portal)

	if mc.DirectMedia {
		msgID := ctx.Value(contextKeyMsgID).(networkid.MessageID)
		var partID networkid.PartID
		if ctx.Value(contextKeyPartID) != nil {
			partID = ctx.Value(contextKeyPartID).(networkid.PartID)
		}
		mediaID := metaid.MakeMediaID(metaid.DirectMediaTypeWhatsAppV2, portal.Receiver, msgID, partID)
		content := &event.MessageEventContent{
			Info: &event.FileInfo{
				MimeType: transport.GetAncillary().GetMimetype(),
				Size:     int(transport.GetAncillary().GetFileLength()),
			},
		}
		var err error
		content.URL, err = mc.Bridge.Matrix.GenerateContentURI(ctx, mediaID)
		if err != nil {
			return nil, err
		}
		directMediaMeta, err := json.Marshal(DirectMediaWhatsApp{
			Key:        transport.Integral.MediaKey,
			Type:       mediaType,
			SHA256:     transport.Integral.FileSHA256,
			EncSHA256:  transport.Integral.FileEncSHA256,
			DirectPath: *transport.Integral.DirectPath,
		})
		if err != nil {
			return nil, err
		}
		return &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: content,
			DBMetadata: &metaid.MessageMetadata{
				DirectMediaMeta: directMediaMeta,
			},
			Extra: make(map[string]any),
		}, nil
	}
	data, err := client.DownloadFB(ctx, transport.GetIntegral(), mediaType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", bridgev2.ErrMediaDownloadFailed, err)
	}
	var fileName string
	mimeType := transport.GetAncillary().GetMimetype()
	if convert != nil {
		data, mimeType, fileName, err = convert(ctx, data, mimeType)
		if err != nil {
			return nil, err
		}
	}
	mxc, file, err := intent.UploadMedia(ctx, portal.MXID, data, fileName, mimeType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", bridgev2.ErrMediaReuploadFailed, err)
	}
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			Body: fileName,
			URL:  mxc,
			Info: &event.FileInfo{
				MimeType: mimeType,
				Size:     len(data),
			},
			File: file,
		},
		Extra: make(map[string]any),
	}, nil
}

func (mc *MessageConverter) convertWhatsAppImage(ctx context.Context, image *waConsumerApplication.ConsumerApplication_ImageMessage) (converted, caption *bridgev2.ConvertedMessagePart, err error) {
	metadata, converted, caption, err := convertWhatsAppAttachment[*waMediaTransport.ImageTransport](ctx, mc, image, whatsmeow.MediaImage, func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error) {
		fileName := "image" + exmime.ExtensionFromMimetype(mimeType)
		return data, mimeType, fileName, nil
	})
	if converted != nil {
		converted.Content.MsgType = event.MsgImage
		converted.Content.Info.Width = int(metadata.GetWidth())
		converted.Content.Info.Height = int(metadata.GetHeight())
	}
	return
}

func (mc *MessageConverter) convertWhatsAppSticker(ctx context.Context, sticker *waConsumerApplication.ConsumerApplication_StickerMessage) (converted, caption *bridgev2.ConvertedMessagePart, err error) {
	metadata, converted, caption, err := convertWhatsAppAttachment[*waMediaTransport.StickerTransport](ctx, mc, sticker, whatsmeow.MediaImage, func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error) {
		fileName := "sticker" + exmime.ExtensionFromMimetype(mimeType)
		return data, mimeType, fileName, nil
	})
	if converted != nil {
		converted.Type = event.EventSticker
		converted.Content.Info.Width = int(metadata.GetWidth())
		converted.Content.Info.Height = int(metadata.GetHeight())
	}
	return
}

func (mc *MessageConverter) convertWhatsAppDocument(ctx context.Context, document *waConsumerApplication.ConsumerApplication_DocumentMessage) (converted, caption *bridgev2.ConvertedMessagePart, err error) {
	_, converted, caption, err = convertWhatsAppAttachment[*waMediaTransport.DocumentTransport](ctx, mc, document, whatsmeow.MediaDocument, func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error) {
		fileName := document.GetFileName()
		if fileName == "" {
			fileName = "file" + exmime.ExtensionFromMimetype(mimeType)
		}
		return data, mimeType, fileName, nil
	})
	if converted != nil {
		converted.Content.MsgType = event.MsgFile
	}
	return
}

func (mc *MessageConverter) convertWhatsAppAudio(ctx context.Context, audio *waConsumerApplication.ConsumerApplication_AudioMessage) (converted, caption *bridgev2.ConvertedMessagePart, err error) {
	// Treat all audio messages as voice messages, official clients don't set the flag for some reason
	isVoiceMessage := true // audio.GetPTT()
	metadata, converted, caption, err := convertWhatsAppAttachment[*waMediaTransport.AudioTransport](ctx, mc, audio, whatsmeow.MediaAudio, func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error) {
		fileName := "audio" + exmime.ExtensionFromMimetype(mimeType)
		if isVoiceMessage && !strings.HasPrefix(mimeType, "audio/ogg") {
			data, err = ffmpeg.ConvertBytes(ctx, data, ".ogg", []string{}, []string{"-c:a", "libopus"}, mimeType)
			if err != nil {
				return data, mimeType, fileName, fmt.Errorf("%w audio to ogg/opus: %w", bridgev2.ErrMediaConvertFailed, err)
			}
			fileName += ".ogg"
			mimeType = "audio/ogg"
		}
		return data, mimeType, fileName, nil
	})
	if converted != nil {
		converted.Content.MsgType = event.MsgAudio
		converted.Content.Info.Duration = int(metadata.GetSeconds() * 1000)
		if isVoiceMessage {
			converted.Content.MSC3245Voice = &event.MSC3245Voice{}
			converted.Content.MSC1767Audio = &event.MSC1767Audio{
				Duration: converted.Content.Info.Duration,
				Waveform: []int{},
			}
		}
	}
	return
}

func (mc *MessageConverter) convertWhatsAppVideo(ctx context.Context, video *waConsumerApplication.ConsumerApplication_VideoMessage) (converted, caption *bridgev2.ConvertedMessagePart, err error) {
	metadata, converted, caption, err := convertWhatsAppAttachment[*waMediaTransport.VideoTransport](ctx, mc, video, whatsmeow.MediaVideo, func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error) {
		fileName := "video" + exmime.ExtensionFromMimetype(mimeType)
		return data, mimeType, fileName, nil
	})
	if converted != nil {
		converted.Content.MsgType = event.MsgVideo
		converted.Content.Info.Width = int(metadata.GetWidth())
		converted.Content.Info.Height = int(metadata.GetHeight())
		converted.Content.Info.Duration = int(metadata.GetSeconds() * 1000)
		// FB is annoying and sends images in video containers sometimes
		if strings.HasPrefix(converted.Content.Info.MimeType, "image/") {
			converted.Content.MsgType = event.MsgImage
		} else if metadata.GetGifPlayback() {
			converted.Extra["info"] = map[string]any{
				"fi.mau.gif":           true,
				"fi.mau.loop":          true,
				"fi.mau.autoplay":      true,
				"fi.mau.hide_controls": true,
				"fi.mau.no_audio":      true,
			}
		}
	}
	return
}

func (mc *MessageConverter) convertWhatsAppMedia(ctx context.Context, rawContent *waConsumerApplication.ConsumerApplication_Content) (converted, caption *bridgev2.ConvertedMessagePart, err error) {
	switch content := rawContent.GetContent().(type) {
	case *waConsumerApplication.ConsumerApplication_Content_ImageMessage:
		return mc.convertWhatsAppImage(ctx, content.ImageMessage)
	case *waConsumerApplication.ConsumerApplication_Content_StickerMessage:
		return mc.convertWhatsAppSticker(ctx, content.StickerMessage)
	case *waConsumerApplication.ConsumerApplication_Content_ViewOnceMessage:
		switch realContent := content.ViewOnceMessage.GetViewOnceContent().(type) {
		case *waConsumerApplication.ConsumerApplication_ViewOnceMessage_ImageMessage:
			return mc.convertWhatsAppImage(ctx, realContent.ImageMessage)
		case *waConsumerApplication.ConsumerApplication_ViewOnceMessage_VideoMessage:
			return mc.convertWhatsAppVideo(ctx, realContent.VideoMessage)
		default:
			return nil, nil, fmt.Errorf("unrecognized view once message type %T", realContent)
		}
	case *waConsumerApplication.ConsumerApplication_Content_DocumentMessage:
		return mc.convertWhatsAppDocument(ctx, content.DocumentMessage)
	case *waConsumerApplication.ConsumerApplication_Content_AudioMessage:
		return mc.convertWhatsAppAudio(ctx, content.AudioMessage)
	case *waConsumerApplication.ConsumerApplication_Content_VideoMessage:
		return mc.convertWhatsAppVideo(ctx, content.VideoMessage)
	default:
		return nil, nil, fmt.Errorf("unrecognized media message type %T", content)
	}
}

func (mc *MessageConverter) appName() string {
	if mc.BridgeMode == metaTypes.Instagram {
		return "Instagram app"
	} else {
		return "Messenger app"
	}
}

func (mc *MessageConverter) waConsumerToMatrix(ctx context.Context, rawContent *waConsumerApplication.ConsumerApplication_Content) (parts []*bridgev2.ConvertedMessagePart) {
	parts = make([]*bridgev2.ConvertedMessagePart, 0, 2)
	switch content := rawContent.GetContent().(type) {
	case *waConsumerApplication.ConsumerApplication_Content_MessageText:
		parts = append(parts, mc.WhatsAppTextToMatrix(ctx, content.MessageText))
	case *waConsumerApplication.ConsumerApplication_Content_ExtendedTextMessage:
		part := mc.WhatsAppTextToMatrix(ctx, content.ExtendedTextMessage.GetText())
		// TODO convert url previews
		parts = append(parts, part)
	case *waConsumerApplication.ConsumerApplication_Content_ImageMessage,
		*waConsumerApplication.ConsumerApplication_Content_StickerMessage,
		*waConsumerApplication.ConsumerApplication_Content_ViewOnceMessage,
		*waConsumerApplication.ConsumerApplication_Content_DocumentMessage,
		*waConsumerApplication.ConsumerApplication_Content_AudioMessage,
		*waConsumerApplication.ConsumerApplication_Content_VideoMessage:
		converted, caption, err := mc.convertWhatsAppMedia(ctx, rawContent)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to convert media message")
			errmsg := "Failed to transfer media"
			if _, ok := err.(userVisibleError); ok {
				errmsg = err.Error()
			}
			converted = &bridgev2.ConvertedMessagePart{
				Type: event.EventMessage,
				Content: &event.MessageEventContent{
					MsgType: event.MsgNotice,
					Body:    errmsg,
				},
			}
		}
		parts = append(parts, converted)
		if caption != nil {
			parts = append(parts, caption)
		}
	case *waConsumerApplication.ConsumerApplication_Content_LocationMessage:
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgLocation,
				Body:    content.LocationMessage.GetLocation().GetName() + "\n" + content.LocationMessage.GetAddress(),
				GeoURI:  fmt.Sprintf("geo:%f,%f", content.LocationMessage.GetLocation().GetDegreesLatitude(), content.LocationMessage.GetLocation().GetDegreesLongitude()),
			},
		})
	case *waConsumerApplication.ConsumerApplication_Content_LiveLocationMessage:
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgLocation,
				Body:    "Live location sharing started\n\nYou can see the location in the " + mc.appName(),
				GeoURI:  fmt.Sprintf("geo:%f,%f", content.LiveLocationMessage.GetLocation().GetDegreesLatitude(), content.LiveLocationMessage.GetLocation().GetDegreesLongitude()),
			},
		})
	case *waConsumerApplication.ConsumerApplication_Content_ContactMessage:
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Unsupported message (contact)\n\nPlease open in " + mc.appName(),
			},
		})
	case *waConsumerApplication.ConsumerApplication_Content_ContactsArrayMessage:
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Unsupported message (contacts array)\n\nPlease open in " + mc.appName(),
			},
		})
	default:
		zerolog.Ctx(ctx).Warn().Type("content_type", content).Msg("Unrecognized content type")
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    fmt.Sprintf("Unsupported message (%T)\n\nPlease open in %s", content, mc.appName()),
			},
		})
	}
	return
}

func (mc *MessageConverter) waLocationMessageToMatrix(ctx context.Context, content *waArmadilloXMA.ExtendedContentMessage, parsedURL *url.URL) (parts []*bridgev2.ConvertedMessagePart) {
	lat := parsedURL.Query().Get("lat")
	long := parsedURL.Query().Get("long")
	return []*bridgev2.ConvertedMessagePart{{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgLocation,
			GeoURI:  fmt.Sprintf("geo:%s,%s", lat, long),
			Body:    fmt.Sprintf("%s\n%s", content.GetTitleText(), content.GetSubtitleText()),
		},
	}}
}

func (mc *MessageConverter) waStoryReplyMessageToMatrix(ctx context.Context, content *waArmadilloXMA.ExtendedContentMessage) (parts []*bridgev2.ConvertedMessagePart, err error) {
	var assocMsg waMsgApplication.MessageApplication
	_, err = armadilloutil.Unmarshal(&assocMsg, content.AssociatedMessage, 2)
	if err != nil {
		return
	}
	var consMsg waConsumerApplication.ConsumerApplication
	_, err = armadilloutil.Unmarshal(&consMsg, assocMsg.GetPayload().GetSubProtocol().GetConsumerMessage(), 1)
	if err != nil {
		return
	}
	parts = mc.waConsumerToMatrix(ctx, consMsg.GetPayload().GetContent())
	return
}

func (mc *MessageConverter) waExtendedContentMessageToMatrix(ctx context.Context, content *waArmadilloXMA.ExtendedContentMessage) (parts []*bridgev2.ConvertedMessagePart) {
	body := content.GetMessageText()
	nativeURL := ""
	for _, cta := range content.GetCtas() {
		parsedURL, err := url.Parse(cta.GetNativeURL())
		if err != nil {
			continue
		}
		nativeURL = parsedURL.String()
		if parsedURL.Scheme == "messenger" && parsedURL.Host == "location_share" {
			return mc.waLocationMessageToMatrix(ctx, content, parsedURL)
		}
		if parsedURL.Scheme == "https" && !strings.Contains(body, cta.GetNativeURL()) {
			if body == "" {
				body = cta.GetNativeURL()
			} else {
				body = fmt.Sprintf("%s\n\n%s", body, cta.GetNativeURL())
			}
		}
	}
	msgtype := event.MsgText
	if body == "" {
		body = fmt.Sprintf("Unsupported message\n\nPlease open in %s", mc.appName())
		msgtype = event.MsgNotice
	}
	switch content.GetTargetType() {
	case waArmadilloXMA.ExtendedContentMessage_FB_STORY_REPLY:
		parts, err := mc.waStoryReplyMessageToMatrix(ctx, content)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to parse story reply message")
			break
		}
		for _, part := range parts {
			part.Content.EnsureHasHTML()
			if nativeURL != "" {
				part.Content.FormattedBody = fmt.Sprintf(
					`<blockquote>Reply to <a href="%s">Facebook story</a>:</blockquote><p>%s</p>`,
					nativeURL,
					part.Content.FormattedBody,
				)
			} else {
				part.Content.FormattedBody = fmt.Sprintf(
					`<blockquote>Reply to Facebook story:</blockquote><p>%s</p>`,
					part.Content.FormattedBody,
				)
			}
		}
		return parts
	}
	return []*bridgev2.ConvertedMessagePart{{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: msgtype,
			Body:    body,
		},
		Extra: map[string]any{
			"fi.mau.meta.temporary_unsupported_type": "Armadillo ExtendedContentMessage",
		},
	}}
}

func (mc *MessageConverter) waArmadilloToMatrix(ctx context.Context, rawContent *waArmadilloApplication.Armadillo_Content) (parts []*bridgev2.ConvertedMessagePart, replyOverride *waCommon.MessageKey) {
	parts = make([]*bridgev2.ConvertedMessagePart, 0, 2)
	switch content := rawContent.GetContent().(type) {
	case *waArmadilloApplication.Armadillo_Content_ExtendedContentMessage:
		return mc.waExtendedContentMessageToMatrix(ctx, content.ExtendedContentMessage), nil
	case *waArmadilloApplication.Armadillo_Content_BumpExistingMessage_:
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgText,
				Body:    "Bumped a message",
			},
		})
		replyOverride = content.BumpExistingMessage.GetKey()
	//case *waArmadilloApplication.Armadillo_Content_RavenMessage_:
	//	// TODO
	default:
		zerolog.Ctx(ctx).Warn().Type("content_type", content).Msg("Unrecognized armadillo content type")
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    fmt.Sprintf("Unsupported message (%T)\n\nPlease open in %s", content, mc.appName()),
			},
		})
	}
	return
}

func (mc *MessageConverter) instamadilloToMatrix(ctx context.Context, rawContent *instamadilloAddMessage.AddMessagePayload) (parts []*bridgev2.ConvertedMessagePart, replyOverride *waCommon.MessageKey) {
	// TODO implement
	return []*bridgev2.ConvertedMessagePart{{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    "Unsupported encrypted Instagram message",
		},
	}}, nil
}

func (mc *MessageConverter) WhatsAppToMatrix(
	ctx context.Context,
	portal *bridgev2.Portal,
	client *whatsmeow.Client,
	intent bridgev2.MatrixAPI,
	messageID networkid.MessageID,
	evt *events.FBMessage,
) *bridgev2.ConvertedMessage {
	ctx = context.WithValue(ctx, contextKeyWAClient, client)
	ctx = context.WithValue(ctx, contextKeyIntent, intent)
	ctx = context.WithValue(ctx, contextKeyPortal, portal)
	ctx = context.WithValue(ctx, contextKeyMsgID, messageID)
	cm := &bridgev2.ConvertedMessage{}
	if disappear := evt.FBApplication.GetMetadata().GetChatEphemeralSetting(); disappear != nil {
		cm.Disappear = database.DisappearingSetting{
			Timer: time.Duration(disappear.GetEphemeralExpiration()) * time.Second,
		}
		switch disappear.GetEphemeralityType() {
		case waMsgApplication.MessageApplication_EphemeralSetting_SEEN_BASED_WITH_TIMER:
			cm.Disappear.Type = event.DisappearingTypeAfterRead
		case waMsgApplication.MessageApplication_EphemeralSetting_SEND_BASED_WITH_TIMER,
			waMsgApplication.MessageApplication_EphemeralSetting_UNKNOWN:
			cm.Disappear.Type = event.DisappearingTypeAfterSend
		case waMsgApplication.MessageApplication_EphemeralSetting_SEEN_ONCE:
			cm.Disappear.Timer = 5 * time.Minute
			cm.Disappear.Type = event.DisappearingTypeAfterRead
		}
		if cm.Disappear.Timer == 0 {
			cm.Disappear.Type = event.DisappearingTypeNone
		}
		if evt.Message != nil && cm.Disappear != portal.Disappear && disappear.EphemeralSettingTimestamp != nil {
			portal.Metadata.(*metaid.PortalMetadata).EphemeralSettingTimestamp = *disappear.EphemeralSettingTimestamp
			portal.UpdateDisappearingSetting(ctx, cm.Disappear, bridgev2.UpdateDisappearingSettingOpts{
				Sender:     intent,
				Timestamp:  evt.Info.Timestamp,
				Implicit:   true,
				Save:       true,
				SendNotice: true,
			})
		}
	}

	var replyOverride *waCommon.MessageKey
	switch typedMsg := evt.Message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		cm.Parts = mc.waConsumerToMatrix(ctx, typedMsg.GetPayload().GetContent())
	case *waArmadilloApplication.Armadillo:
		cm.Parts, replyOverride = mc.waArmadilloToMatrix(ctx, typedMsg.GetPayload().GetContent())
	case *instamadilloAddMessage.AddMessagePayload:
		cm.Parts, replyOverride = mc.instamadilloToMatrix(ctx, typedMsg)
		if disappear := typedMsg.GetMetadata().GetEphemeralityParams(); disappear != nil {
			cm.Disappear = database.DisappearingSetting{
				Timer: time.Duration(disappear.GetEphemeralDurationSec()) * time.Second,
				Type:  event.DisappearingTypeAfterSend,
			}
		}
	default:
		if evt.Message == nil && evt.FBApplication.GetMetadata().GetChatEphemeralSetting() != nil {
			portal.Metadata.(*metaid.PortalMetadata).EphemeralSettingTimestamp = evt.Info.Timestamp.Unix()
			portal.UpdateDisappearingSetting(ctx, cm.Disappear, bridgev2.UpdateDisappearingSettingOpts{
				Sender:    intent,
				Timestamp: evt.Info.Timestamp,
				Save:      true,
			})
			cm.Parts = []*bridgev2.ConvertedMessagePart{{
				Type:    event.EventMessage,
				Content: bridgev2.DisappearingMessageNotice(cm.Disappear.Timer, false),
				Extra: map[string]any{
					"com.beeper.action_message": map[string]any{
						"type":       "disappearing_timer",
						"timer":      cm.Disappear.Timer.Milliseconds(),
						"timer_type": cm.Disappear.Type,
						"implicit":   false,
					},
				},
				DontBridge: cm.Disappear == portal.Disappear,
			}}
		} else {
			cm.Parts = []*bridgev2.ConvertedMessagePart{{
				Type: event.EventMessage,
				Content: &event.MessageEventContent{
					MsgType: event.MsgNotice,
					Body:    "Unsupported message content type",
				},
			}}
		}
	}
	if qm := evt.FBApplication.GetMetadata().GetQuotedMessage(); qm != nil {
		pcp, _ := types.ParseJID(qm.GetParticipant())
		// TODO what if participant is not set?
		cm.ReplyTo = &networkid.MessageOptionalPartID{
			MessageID: metaid.MakeWAMessageID(evt.Info.Chat, pcp, qm.GetStanzaID()),
		}
	} else if replyOverride != nil {
		pcp, _ := types.ParseJID(replyOverride.GetParticipant())
		// TODO what if participant is not set?
		cm.ReplyTo = &networkid.MessageOptionalPartID{
			MessageID: metaid.MakeWAMessageID(evt.Info.Chat, pcp, qm.GetStanzaID()),
		}
	}
	for i, part := range cm.Parts {
		part.ID = metaid.MakeMessagePartID(i)
		if part.Content.Mentions == nil {
			part.Content.Mentions = &event.Mentions{}
		}
	}
	return cm
}
