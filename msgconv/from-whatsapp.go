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
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"slices"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary/armadillo/waCommon"
	"go.mau.fi/whatsmeow/binary/armadillo/waConsumerApplication"
	"go.mau.fi/whatsmeow/binary/armadillo/waMediaTransport"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func (mc *MessageConverter) whatsappTextToMatrix(ctx context.Context, text *waCommon.MessageText) *ConvertedMessagePart {
	content := &event.MessageEventContent{
		MsgType:  event.MsgText,
		Body:     text.GetText(),
		Mentions: &event.Mentions{},
	}
	silent := false
	if len(text.Commands) > 0 {
		for _, cmd := range text.Commands {
			switch cmd.CommandType {
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
			mxid := mc.GetUserMXID(ctx, int64(parsed.UserInt()))
			if !silent {
				content.Mentions.UserIDs = append(content.Mentions.UserIDs, mxid)
			}
			mentionText := "@" + jid
			content.Body = strings.ReplaceAll(content.Body, mentionText, mxid.String())
			content.FormattedBody = strings.ReplaceAll(content.FormattedBody, mentionText, fmt.Sprintf(`<a href="%s">%s</a>`, mxid.URI().MatrixToURL(), mxid.String()))
		}
	}
	return &ConvertedMessagePart{
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
) (metadata Ancillary, media, caption *ConvertedMessagePart, err error) {
	var typedTransport Transport
	typedTransport, err = msg.Decode()
	if err != nil {
		return
	}
	msgWithCaption, ok := msg.(AttachmentMessageWithCaption[Integral, Ancillary, Transport])
	if ok && len(msgWithCaption.GetCaption().GetText()) > 0 {
		caption = mc.whatsappTextToMatrix(ctx, msgWithCaption.GetCaption())
		caption.Content.MsgType = event.MsgNotice
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
) (*ConvertedMessagePart, error) {
	data, err := mc.GetE2EEClient(ctx).DownloadFB(transport.GetIntegral(), mediaType)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	var fileName string
	mimeType := transport.GetAncillary().GetMimetype()
	if convert != nil {
		data, fileName, mimeType, err = convert(ctx, data, mimeType)
		if err != nil {
			return nil, fmt.Errorf("failed to convert: %w", err)
		}
	}
	content, err := mc.uploadAttachment(ctx, data, fileName, mimeType)
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}
	return &ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: content,
		Extra:   make(map[string]any),
	}, nil
}

func (mc *MessageConverter) convertWhatsAppImage(ctx context.Context, image *waConsumerApplication.ConsumerApplication_ImageMessage) (converted, caption *ConvertedMessagePart, err error) {
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

func (mc *MessageConverter) convertWhatsAppSticker(ctx context.Context, sticker *waConsumerApplication.ConsumerApplication_StickerMessage) (converted, caption *ConvertedMessagePart, err error) {
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

func (mc *MessageConverter) convertWhatsAppDocument(ctx context.Context, document *waConsumerApplication.ConsumerApplication_DocumentMessage) (converted, caption *ConvertedMessagePart, err error) {
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

func (mc *MessageConverter) convertWhatsAppAudio(ctx context.Context, audio *waConsumerApplication.ConsumerApplication_AudioMessage) (converted, caption *ConvertedMessagePart, err error) {
	metadata, converted, caption, err := convertWhatsAppAttachment[*waMediaTransport.AudioTransport](ctx, mc, audio, whatsmeow.MediaAudio, func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error) {
		fileName := "audio" + exmime.ExtensionFromMimetype(mimeType)
		if audio.GetPTT() && !strings.HasPrefix(mimeType, "audio/ogg") {
			data, err = ffmpeg.ConvertBytes(ctx, data, ".ogg", []string{}, []string{"-c:a", "libopus"}, mimeType)
			if err != nil {
				return data, mimeType, fileName, fmt.Errorf("failed to convert audio to ogg/opus: %w", err)
			}
			fileName += ".ogg"
			mimeType = "audio/ogg"
		}
		return data, mimeType, fileName, nil
	})
	if converted != nil {
		converted.Content.MsgType = event.MsgAudio
		converted.Content.Info.Duration = int(metadata.GetSeconds() * 1000)
		if audio.GetPTT() {
			converted.Extra["org.matrix.msc3245.voice"] = map[string]any{}
			converted.Extra["org.matrix.msc1767.audio"] = map[string]any{}
		}
	}
	return
}

func (mc *MessageConverter) convertWhatsAppVideo(ctx context.Context, video *waConsumerApplication.ConsumerApplication_VideoMessage) (converted, caption *ConvertedMessagePart, err error) {
	metadata, converted, caption, err := convertWhatsAppAttachment[*waMediaTransport.VideoTransport](ctx, mc, video, whatsmeow.MediaVideo, func(ctx context.Context, data []byte, mimeType string) ([]byte, string, string, error) {
		fileName := "video" + exmime.ExtensionFromMimetype(mimeType)
		return data, mimeType, fileName, nil
	})
	if converted != nil {
		converted.Content.MsgType = event.MsgVideo
		converted.Content.Info.Width = int(metadata.GetWidth())
		converted.Content.Info.Height = int(metadata.GetHeight())
		converted.Content.Info.Duration = int(metadata.GetSeconds() * 1000)
		if metadata.GetGifPlayback() {
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

func (mc *MessageConverter) convertWhatsAppMedia(ctx context.Context, evt *events.FBConsumerMessage) (converted, caption *ConvertedMessagePart, err error) {
	switch content := evt.Message.GetPayload().GetContent().GetContent().(type) {
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

func (mc *MessageConverter) WhatsAppToMatrix(ctx context.Context, evt *events.FBConsumerMessage) *ConvertedMessage {
	cm := &ConvertedMessage{
		Parts: make([]*ConvertedMessagePart, 0),
	}
	switch content := evt.Message.GetPayload().GetContent().GetContent().(type) {
	case *waConsumerApplication.ConsumerApplication_Content_MessageText:
		cm.Parts = append(cm.Parts, mc.whatsappTextToMatrix(ctx, content.MessageText))
	case *waConsumerApplication.ConsumerApplication_Content_ExtendedTextMessage:
		part := mc.whatsappTextToMatrix(ctx, content.ExtendedTextMessage.GetText())
		// TODO convert url previews
		cm.Parts = append(cm.Parts, part)
	case *waConsumerApplication.ConsumerApplication_Content_ImageMessage,
		*waConsumerApplication.ConsumerApplication_Content_StickerMessage,
		*waConsumerApplication.ConsumerApplication_Content_ViewOnceMessage,
		*waConsumerApplication.ConsumerApplication_Content_DocumentMessage,
		*waConsumerApplication.ConsumerApplication_Content_AudioMessage,
		*waConsumerApplication.ConsumerApplication_Content_VideoMessage:
		converted, caption, err := mc.convertWhatsAppMedia(ctx, evt)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to convert media message")
			converted = &ConvertedMessagePart{
				Type: event.EventMessage,
				Content: &event.MessageEventContent{
					MsgType: event.MsgNotice,
					Body:    "Failed to transfer media",
				},
			}
		}
		cm.Parts = append(cm.Parts, converted)
		if caption != nil {
			cm.Parts = append(cm.Parts, caption)
		}
	case *waConsumerApplication.ConsumerApplication_Content_LocationMessage:
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgLocation,
				Body:    content.LocationMessage.GetLocation().GetName() + "\n" + content.LocationMessage.GetAddress(),
				GeoURI:  fmt.Sprintf("geo:%f,%f", content.LocationMessage.GetLocation().GetDegreesLatitude(), content.LocationMessage.GetLocation().GetDegreesLongitude()),
			},
		})
	case *waConsumerApplication.ConsumerApplication_Content_LiveLocationMessage:
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgLocation,
				Body:    "Live location sharing started",
				GeoURI:  fmt.Sprintf("geo:%f,%f", content.LiveLocationMessage.GetLocation().GetDegreesLatitude(), content.LiveLocationMessage.GetLocation().GetDegreesLongitude()),
			},
		})
	case *waConsumerApplication.ConsumerApplication_Content_ContactMessage:
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Unsupported message (contact)",
			},
		})
	case *waConsumerApplication.ConsumerApplication_Content_ContactsArrayMessage:
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Unsupported message (contacts array)",
			},
		})
	default:
		zerolog.Ctx(ctx).Warn().Type("content_type", content).Msg("Unrecognized content type")
		cm.Parts = append(cm.Parts, &ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "Unsupported message (unknown type)",
			},
		})
	}
	var replyTo id.EventID
	var sender id.UserID
	if qm := evt.Application.GetMetadata().GetQuotedMessage(); qm != nil {
		pcp, _ := types.ParseJID(qm.GetParticipant())
		replyTo, sender = mc.GetMatrixReply(ctx, qm.GetStanzaID(), int64(pcp.UserInt()))
	}
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
