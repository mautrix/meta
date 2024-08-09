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
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"google.golang.org/protobuf/proto"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (mc *MessageConverter) TextToWhatsApp(content *event.MessageEventContent) *waCommon.MessageText {
	// TODO mentions
	return &waCommon.MessageText{
		Text: proto.String(content.Body),
	}
}

func (mc *MessageConverter) ToWhatsApp(
	ctx context.Context,
	evt *event.Event,
	content *event.MessageEventContent,
	portal *bridgev2.Portal,
	client *whatsmeow.Client,
	relaybotFormatted bool,
	replyTo *database.Message,
) (*waConsumerApplication.ConsumerApplication, *waMsgApplication.MessageApplication_Metadata, error) {
	ctx = context.WithValue(ctx, contextKeyWAClient, client)
	ctx = context.WithValue(ctx, contextKeyPortal, portal)

	if evt.Type == event.EventSticker {
		content.MsgType = event.MessageType(event.EventSticker.Type)
	}
	if content.MsgType == event.MsgEmote && !relaybotFormatted {
		content.Body = "/me " + content.Body
		if content.FormattedBody != "" {
			content.FormattedBody = "/me " + content.FormattedBody
		}
	}
	var waContent waConsumerApplication.ConsumerApplication_Content
	switch content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		waContent.Content = &waConsumerApplication.ConsumerApplication_Content_MessageText{
			MessageText: mc.TextToWhatsApp(content),
		}
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile, event.MessageType(event.EventSticker.Type):
		reuploaded, fileName, err := mc.reuploadMediaToWhatsApp(ctx, evt, content)
		if err != nil {
			return nil, nil, err
		}
		var caption *waCommon.MessageText
		if content.FileName != "" && content.Body != content.FileName {
			caption = mc.TextToWhatsApp(content)
		} else {
			caption = &waCommon.MessageText{}
		}
		waContent.Content, err = mc.wrapWhatsAppMedia(evt, content, reuploaded, caption, fileName)
		if err != nil {
			return nil, nil, err
		}
	case event.MsgLocation:
		lat, long, err := parseGeoURI(content.GeoURI)
		if err != nil {
			return nil, nil, err
		}
		// TODO does this actually work with any of the messenger clients?
		waContent.Content = &waConsumerApplication.ConsumerApplication_Content_LocationMessage{
			LocationMessage: &waConsumerApplication.ConsumerApplication_LocationMessage{
				Location: &waConsumerApplication.ConsumerApplication_Location{
					DegreesLatitude:  proto.Float64(lat),
					DegreesLongitude: proto.Float64(long),
					Name:             proto.String(content.Body),
				},
				Address: proto.String("Earth"),
			},
		}
	default:
		return nil, nil, fmt.Errorf("%w %s", bridgev2.ErrUnsupportedMessageType, content.MsgType)
	}
	var meta waMsgApplication.MessageApplication_Metadata
	if replyTo != nil {
		_, replySender, stanzaID := metaid.ParseWAMessageID(replyTo.ID)
		meta.QuotedMessage = &waMsgApplication.MessageApplication_Metadata_QuotedMessage{
			StanzaID: proto.String(stanzaID),
			// TODO: this is hacky since it hardcodes the server
			// TODO 2: should this be included for DMs?
			Participant: proto.String(replySender.String()),
			Payload:     nil,
		}
	}
	return &waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
				Content: &waContent,
			},
		},
		Metadata: nil,
	}, &meta, nil
}

func parseGeoURI(uri string) (lat, long float64, err error) {
	if !strings.HasPrefix(uri, "geo:") {
		err = fmt.Errorf("uri doesn't have geo: prefix")
		return
	}
	// Remove geo: prefix and anything after ;
	coordinates := strings.Split(strings.TrimPrefix(uri, "geo:"), ";")[0]

	if splitCoordinates := strings.Split(coordinates, ","); len(splitCoordinates) != 2 {
		err = fmt.Errorf("didn't find exactly two numbers separated by a comma")
	} else if lat, err = strconv.ParseFloat(splitCoordinates[0], 64); err != nil {
		err = fmt.Errorf("latitude is not a number: %w", err)
	} else if long, err = strconv.ParseFloat(splitCoordinates[1], 64); err != nil {
		err = fmt.Errorf("longitude is not a number: %w", err)
	}
	return
}

func clampTo400(w, h int) (int, int) {
	if w > 400 {
		h = h * 400 / w
		w = 400
	}
	if h > 400 {
		w = w * 400 / h
		h = 400
	}
	return w, h
}

func (mc *MessageConverter) reuploadMediaToWhatsApp(ctx context.Context, evt *event.Event, content *event.MessageEventContent) (*waMediaTransport.WAMediaTransport, string, error) {
	mimeType := content.Info.MimeType
	fileName := content.FileName
	if fileName == "" {
		fileName = content.Body
	}
	data, err := mc.Bridge.Bot.DownloadMedia(ctx, content.URL, content.File)
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if content.MSC3245Voice != nil && ffmpeg.Supported() {
		data, err = ffmpeg.ConvertBytes(ctx, data, ".m4a", []string{}, []string{"-c:a", "aac"}, mimeType)
		if err != nil {
			return nil, "", fmt.Errorf("%w voice message to m4a: %w", bridgev2.ErrMediaConvertFailed, err)
		}
		mimeType = "audio/mp4"
		fileName += ".m4a"
	} else if mimeType == "image/gif" && content.MsgType == event.MsgImage && ffmpeg.Supported() {
		data, err = ffmpeg.ConvertBytes(ctx, data, ".mp4", []string{"-f", "gif"}, []string{
			"-pix_fmt", "yuv420p", "-c:v", "libx264", "-movflags", "+faststart",
			"-filter:v", "crop='floor(in_w/2)*2:floor(in_h/2)*2'",
		}, mimeType)
		if err != nil {
			return nil, "", fmt.Errorf("%w gif to mp4: %w", bridgev2.ErrMediaConvertFailed, err)
		}
		mimeType = "video/mp4"
		fileName += ".mp4"
		content.MsgType = event.MsgVideo
		customInfo, ok := evt.Content.Raw["info"].(map[string]any)
		if !ok {
			customInfo = make(map[string]any)
			evt.Content.Raw["info"] = customInfo
		}
		customInfo["fi.mau.gif"] = true
	}
	if content.MsgType == event.MsgImage && content.Info.Width == 0 {
		cfg, _, _ := image.DecodeConfig(bytes.NewReader(data))
		content.Info.Width, content.Info.Height = cfg.Width, cfg.Height
	}
	mediaType := msgToMediaType(content.MsgType)
	client := ctx.Value(contextKeyWAClient).(*whatsmeow.Client)
	uploaded, err := client.Upload(ctx, data, mediaType)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", bridgev2.ErrMediaReuploadFailed, err)
	}
	w, h := clampTo400(content.Info.Width, content.Info.Height)
	if w == 0 && content.MsgType == event.MsgImage {
		w, h = 400, 400
	}
	mediaTransport := &waMediaTransport.WAMediaTransport{
		Integral: &waMediaTransport.WAMediaTransport_Integral{
			FileSHA256:        uploaded.FileSHA256,
			MediaKey:          uploaded.MediaKey,
			FileEncSHA256:     uploaded.FileEncSHA256,
			DirectPath:        &uploaded.DirectPath,
			MediaKeyTimestamp: proto.Int64(time.Now().Unix()),
		},
		Ancillary: &waMediaTransport.WAMediaTransport_Ancillary{
			FileLength: proto.Uint64(uint64(len(data))),
			Mimetype:   &mimeType,
			// This field is extremely required for some reason.
			// Messenger iOS & Android will refuse to display the media if it's not present.
			// iOS also requires that width and height are non-empty.
			Thumbnail: &waMediaTransport.WAMediaTransport_Ancillary_Thumbnail{
				ThumbnailWidth:  proto.Uint32(uint32(w)),
				ThumbnailHeight: proto.Uint32(uint32(h)),
			},
			ObjectID: &uploaded.ObjectID,
		},
	}
	fmt.Printf("Uploaded media transport: %+v\n", mediaTransport)
	return mediaTransport, fileName, nil
}

func (mc *MessageConverter) wrapWhatsAppMedia(
	evt *event.Event,
	content *event.MessageEventContent,
	reuploaded *waMediaTransport.WAMediaTransport,
	caption *waCommon.MessageText,
	fileName string,
) (output waConsumerApplication.ConsumerApplication_Content_Content, err error) {
	switch content.MsgType {
	case event.MsgImage:
		imageMsg := &waConsumerApplication.ConsumerApplication_ImageMessage{
			Caption: caption,
		}
		err = imageMsg.Set(&waMediaTransport.ImageTransport{
			Integral: &waMediaTransport.ImageTransport_Integral{
				Transport: reuploaded,
			},
			Ancillary: &waMediaTransport.ImageTransport_Ancillary{
				Height: proto.Uint32(uint32(content.Info.Height)),
				Width:  proto.Uint32(uint32(content.Info.Width)),
			},
		})
		output = &waConsumerApplication.ConsumerApplication_Content_ImageMessage{ImageMessage: imageMsg}
	case event.MessageType(event.EventSticker.Type):
		stickerMsg := &waConsumerApplication.ConsumerApplication_StickerMessage{}
		err = stickerMsg.Set(&waMediaTransport.StickerTransport{
			Integral: &waMediaTransport.StickerTransport_Integral{
				Transport: reuploaded,
			},
			Ancillary: &waMediaTransport.StickerTransport_Ancillary{
				Height: proto.Uint32(uint32(content.Info.Height)),
				Width:  proto.Uint32(uint32(content.Info.Width)),
			},
		})
		output = &waConsumerApplication.ConsumerApplication_Content_StickerMessage{StickerMessage: stickerMsg}
	case event.MsgVideo:
		videoMsg := &waConsumerApplication.ConsumerApplication_VideoMessage{
			Caption: caption,
		}
		customInfo, _ := evt.Content.Raw["info"].(map[string]any)
		isGif, _ := customInfo["fi.mau.gif"].(bool)

		err = videoMsg.Set(&waMediaTransport.VideoTransport{
			Integral: &waMediaTransport.VideoTransport_Integral{
				Transport: reuploaded,
			},
			Ancillary: &waMediaTransport.VideoTransport_Ancillary{
				Height:      proto.Uint32(uint32(content.Info.Height)),
				Width:       proto.Uint32(uint32(content.Info.Width)),
				Seconds:     proto.Uint32(uint32(content.Info.Duration / 1000)),
				GifPlayback: &isGif,
			},
		})
		output = &waConsumerApplication.ConsumerApplication_Content_VideoMessage{VideoMessage: videoMsg}
	case event.MsgAudio:
		_, isVoice := evt.Content.Raw["org.matrix.msc3245.voice"]
		audioMsg := &waConsumerApplication.ConsumerApplication_AudioMessage{
			PTT: &isVoice,
		}
		err = audioMsg.Set(&waMediaTransport.AudioTransport{
			Integral: &waMediaTransport.AudioTransport_Integral{
				Transport: reuploaded,
			},
			Ancillary: &waMediaTransport.AudioTransport_Ancillary{
				Seconds: proto.Uint32(uint32(content.Info.Duration / 1000)),
			},
		})
		output = &waConsumerApplication.ConsumerApplication_Content_AudioMessage{AudioMessage: audioMsg}
	case event.MsgFile:
		documentMsg := &waConsumerApplication.ConsumerApplication_DocumentMessage{
			FileName: &fileName,
		}
		err = documentMsg.Set(&waMediaTransport.DocumentTransport{
			Integral: &waMediaTransport.DocumentTransport_Integral{
				Transport: reuploaded,
			},
			Ancillary: &waMediaTransport.DocumentTransport_Ancillary{},
		})
		output = &waConsumerApplication.ConsumerApplication_Content_DocumentMessage{DocumentMessage: documentMsg}
	}
	return
}

func msgToMediaType(msgType event.MessageType) whatsmeow.MediaType {
	switch msgType {
	case event.MsgImage, event.MessageType(event.EventSticker.Type):
		return whatsmeow.MediaImage
	case event.MsgVideo:
		return whatsmeow.MediaVideo
	case event.MsgAudio:
		return whatsmeow.MediaAudio
	case event.MsgFile:
		fallthrough
	default:
		return whatsmeow.MediaDocument
	}
}
