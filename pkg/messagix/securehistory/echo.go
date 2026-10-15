package securehistory

import (
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.mau.fi/whatsmeow/proto/waArmadilloApplication"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupCommon"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waArmadilloXMA"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"google.golang.org/protobuf/proto"
)

func (m *ArchiveMessage) decryptEcho(device *Device, threadID string) (*DecryptedMessage, error) {
	echo := m.Echo
	var epoch Epoch
	if echo.EpochID != nil {
		epoch = device.Epochs[*echo.EpochID]
	} else {
		for _, candidate := range device.Epochs {
			if string(candidate.AnonID) == echo.EpochAnonID {
				epoch = candidate
				break
			}
		}
	}
	if echo.EpochAnonID == "" || string(epoch.AnonID) != echo.EpochAnonID || len(epoch.RootKey) != 32 {
		return nil, errors.New("missing Messenger archive Echo epoch")
	}
	version, err := archiveEncryptionVersion(echo.EncryptionVersion)
	if err != nil {
		return nil, err
	}
	plaintext, err := decryptArchivePayload(epoch.RootKey, echo.EpochAnonID, threadID, version, echo.EchoDocumentString)
	if err != nil {
		return nil, err
	}
	result, err := decodeEcho(plaintext, m.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid Messenger archive Echo document: %w", err)
	}
	result.Attachments = m.Attachments
	return result, nil
}

type echoFields map[string]string

func parseEcho(data []byte) (echoFields, error) {
	if !utf8.Valid(data) || !strings.Contains(string(data), "\r\n") {
		return nil, errors.New("invalid Echo field encoding")
	}
	fields := make(echoFields)
	for line := range strings.SplitSeq(string(data), "\r\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, errors.New("invalid Echo field")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if value == "" {
			delete(fields, key)
		} else {
			fields[key] = value
		}
	}
	return fields, nil
}

func (f echoFields) text(key string) string {
	value := f[key]
	if !strings.HasPrefix(value, `"`) {
		return value
	}
	var out strings.Builder
	for i := 1; i < len(value); i++ {
		switch value[i] {
		case '"':
			return out.String()
		case '\\':
			i++
			if i == len(value) {
				return ""
			}
			switch value[i] {
			case 'r':
				out.WriteByte('\r')
			case 'n':
				out.WriteByte('\n')
			default:
				out.WriteByte(value[i])
			}
		default:
			out.WriteByte(value[i])
		}
	}
	return ""
}

func (f echoFields) integer(key string) (int64, error) {
	return strconv.ParseInt(f[key], 10, 64)
}

func echoSender(value string) (string, error) {
	address, err := mail.ParseAddress(value)
	if err != nil {
		return "", errors.New("invalid Echo sender address")
	}
	id, _, _ := strings.Cut(address.Address, "@")
	if n, err := strconv.ParseUint(id, 10, 64); err != nil || n == 0 {
		return "", errors.New("invalid Echo sender ID")
	}
	return id, nil
}

func echoProtobuf(
	content *waConsumerApplication.ConsumerApplication_Content,
	metadata *waMsgApplication.MessageApplication_Metadata,
	id, sender string,
	timestamp int64,
) (*DecryptedProtobuf, error) {
	sub := &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{}
	err := sub.Set(&waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
				Content: content,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return echoApplication(&waMsgApplication.MessageApplication{
		Metadata: metadata,
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
				SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload{
					SubProtocol: sub,
				},
			},
		},
	}, id, sender, timestamp)
}

func echoApplication(app *waMsgApplication.MessageApplication, id, sender string, timestamp int64) (*DecryptedProtobuf, error) {
	application, err := proto.Marshal(app)
	if err != nil {
		return nil, err
	}
	return &DecryptedProtobuf{
		TimestampMS: timestamp,
		Message: &waArmadilloBackupMessage.BackupMessage{
			Metadata: &waArmadilloBackupCommon.Metadata{
				MessageID:   &id,
				SenderID:    &sender,
				TimestampMS: &timestamp,
			},
			Payload: &waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage{
				EncryptedTransportMessage: application,
			},
		},
	}, nil
}

func decodeEcho(data []byte, id string) (*DecryptedMessage, error) {
	fields, err := parseEcho(data)
	if err != nil {
		return nil, err
	}
	timestamp, err := fields.integer("Sort-Order")
	if err != nil ||
		timestamp < 0 ||
		fields.text("Message-ID") == "" ||
		fields.text("Thread-ID") == "" ||
		fields.text("X-Offline-Threading-ID") != id {
		return nil, errors.New("missing or mismatched Echo message metadata")
	}
	sender, err := echoSender(fields["From"])
	if err != nil {
		return nil, err
	}
	if fields["Is-Tombstoned"] == "1" ||
		fields.text("Message-Content-Type") == "decryption_failure" ||
		(fields.text("Message-Content-Type") == "placeholder" && fields.text("Message-Content-Subtype") == "unsend") {
		return &DecryptedMessage{
			ID: id,
			Message: &DecryptedProtobuf{
				TimestampMS: timestamp,
				Message: &waArmadilloBackupMessage.BackupMessage{
					Metadata: &waArmadilloBackupCommon.Metadata{
						MessageID:   &id,
						SenderID:    &sender,
						TimestampMS: &timestamp,
					},
					Payload: &waArmadilloBackupMessage.BackupMessage_EncryptedTransportEvent{},
				},
			},
		}, nil
	}
	metadata := &waMsgApplication.MessageApplication_Metadata{
		IsForwarded: proto.Bool(fields["Is-Forwarded"] == "1"),
	}
	if quote := fields.text("Reply-Message-Id"); quote != "" {
		participant, err := echoSender(fields["Reply-Message-Sender"])
		if err != nil {
			return nil, err
		}
		metadata.QuotedMessage = &waMsgApplication.MessageApplication_Metadata_QuotedMessage{
			StanzaID:    &quote,
			Participant: proto.String(participant + "@msgr"),
		}
	}
	var mainMessage *DecryptedProtobuf
	var content *waConsumerApplication.ConsumerApplication_Content
	switch fields.text("Message-Content-Type") {
	case "text", "admin_message", "unsupported_needs_update", "null_state", "placeholder", "":
		content = &waConsumerApplication.ConsumerApplication_Content{
			Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{
				MessageText: &waCommon.MessageText{
					Text: proto.String(fields.text("Text")),
				},
			},
		}
	case "xma":
		mainMessage, err = fields.xmaProtobuf(metadata, id, sender, timestamp)
		if err != nil {
			return nil, err
		}
	case "media":
		content, err = fields.mediaContent()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported Echo content type %q", fields.text("Message-Content-Type"))
	}
	result := &DecryptedMessage{ID: id, Supplementals: make(map[string]*DecryptedProtobuf)}
	result.Message = mainMessage
	if result.Message == nil {
		result.Message, err = echoProtobuf(content, metadata, id, sender, timestamp)
	}
	if err != nil {
		return nil, err
	}
	for key := range fields {
		editID, ok := strings.CutPrefix(key, "Edit-Contents-")
		if !ok || editID == id {
			continue
		}
		ts, err := fields.integer("Edit-Timestamps-" + editID)
		if err != nil || ts < 0 {
			return nil, errors.New("invalid Echo edit timestamp")
		}
		edit, err := echoProtobuf(&waConsumerApplication.ConsumerApplication_Content{
			Content: &waConsumerApplication.ConsumerApplication_Content_EditMessage{
				EditMessage: &waConsumerApplication.ConsumerApplication_EditMessage{
					Key:         &waCommon.MessageKey{ID: &id},
					Message:     &waCommon.MessageText{Text: proto.String(fields.text(key))},
					TimestampMS: &ts,
				},
			},
		}, nil, editID, sender, ts)
		if err != nil {
			return nil, err
		}
		result.Supplementals["edit:"+editID] = edit
	}
	if fields["Reactions"] != "" {
		reactions, err := mail.ParseAddressList(fields["Reactions"])
		if err != nil {
			return nil, errors.New("invalid Echo reactions")
		}
		ids, err := mail.ParseAddressList(fields["X-Reaction-Offline-Threading-IDs"])
		if err != nil {
			return nil, errors.New("invalid Echo reaction IDs")
		}
		timestamps, err := mail.ParseAddressList(fields["Reaction-Authoritative-Timestamps"])
		if err != nil || len(ids) != len(reactions) || len(timestamps) != len(reactions) {
			return nil, errors.New("mismatched Echo reaction metadata")
		}
		for i, reaction := range reactions {
			actor, err := echoSender(reaction.Address)
			if err != nil {
				return nil, err
			}
			ts, err := strconv.ParseInt(timestamps[i].Name, 10, 64)
			if err != nil || ts < 0 || ids[i].Name == "" {
				return nil, errors.New("invalid Echo reaction metadata")
			}
			supplemental, err := echoProtobuf(&waConsumerApplication.ConsumerApplication_Content{
				Content: &waConsumerApplication.ConsumerApplication_Content_ReactionMessage{
					ReactionMessage: &waConsumerApplication.ConsumerApplication_ReactionMessage{
						Key:               &waCommon.MessageKey{ID: &id},
						Text:              &reaction.Name,
						SenderTimestampMS: &ts,
					},
				},
			}, nil, ids[i].Name, actor, ts)
			if err != nil {
				return nil, err
			}
			result.Supplementals["reaction:"+actor] = supplemental
		}
	}
	return result, nil
}

func (f echoFields) mediaContent() (*waConsumerApplication.ConsumerApplication_Content, error) {
	objectID := f.text("X-Object-Id")
	prefix := ""
	if objects := f.text("X-Attachment-Object-Ids"); objects != "" {
		ids := strings.Split(objects, ",")
		objectID = strings.TrimSpace(ids[0])
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if f.text(id+"-X-Backup-Status") != "" || f.text(id+"-X-Content-Type") == "full" {
				objectID = id
				break
			}
		}
		prefix = objectID + "-"
	}
	decode := func(key string) ([]byte, error) {
		value := f.text(prefix + key)
		data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
		if err != nil || len(data) != 32 {
			return nil, fmt.Errorf("invalid Echo media %s", key)
		}
		return data, nil
	}
	key, err := decode("X-Media-Key")
	if err != nil {
		return nil, err
	}
	hash, err := decode("X-Plaintext-Hash")
	if err != nil {
		return nil, err
	}
	encHash, err := decode("X-Encrypted-Hash")
	if err != nil {
		return nil, err
	}
	path := f.text(prefix + "X-Direct-Path")
	if objectID == "" || path == "" {
		return nil, errors.New("missing Echo attachment metadata")
	}
	size, _ := f.integer("Size")
	width, _ := f.integer("Width")
	height, _ := f.integer("Height")
	duration, _ := f.integer("Playable-Duration")
	keyTimestamp, _ := f.integer(prefix + "X-Media-Key-Timestamp")
	if size < 0 || width < 0 || width > 1<<32-1 || height < 0 || height > 1<<32-1 || duration < 0 || duration > 1<<32-1 {
		return nil, errors.New("invalid Echo media dimensions")
	}
	transport := &waMediaTransport.WAMediaTransport{
		Integral: &waMediaTransport.WAMediaTransport_Integral{
			MediaKey:          key,
			FileSHA256:        hash,
			FileEncSHA256:     encHash,
			DirectPath:        &path,
			MediaKeyTimestamp: &keyTimestamp,
		},
		Ancillary: &waMediaTransport.WAMediaTransport_Ancillary{
			ObjectID:   &objectID,
			FileLength: proto.Uint64(uint64(size)),
			Mimetype:   proto.String(f.text("Content-Type")),
		},
	}
	caption := &waCommon.MessageText{Text: proto.String(f.text("Text"))}
	content := &waConsumerApplication.ConsumerApplication_Content{}
	switch f.text("Attachment-Type") {
	case "image", "animated_image":
		image := &waConsumerApplication.ConsumerApplication_ImageMessage{Caption: caption}
		err = image.Set(&waMediaTransport.ImageTransport{
			Integral: &waMediaTransport.ImageTransport_Integral{Transport: transport},
			Ancillary: &waMediaTransport.ImageTransport_Ancillary{
				Width:  proto.Uint32(uint32(width)),
				Height: proto.Uint32(uint32(height)),
			},
		})
		content.Content = &waConsumerApplication.ConsumerApplication_Content_ImageMessage{ImageMessage: image}
	case "sticker":
		sticker := &waConsumerApplication.ConsumerApplication_StickerMessage{}
		err = sticker.Set(&waMediaTransport.StickerTransport{
			Integral: &waMediaTransport.StickerTransport_Integral{Transport: transport},
			Ancillary: &waMediaTransport.StickerTransport_Ancillary{
				Width:  proto.Uint32(uint32(width)),
				Height: proto.Uint32(uint32(height)),
			},
		})
		content.Content = &waConsumerApplication.ConsumerApplication_Content_StickerMessage{StickerMessage: sticker}
	case "video":
		video := &waConsumerApplication.ConsumerApplication_VideoMessage{Caption: caption}
		err = video.Set(&waMediaTransport.VideoTransport{
			Integral: &waMediaTransport.VideoTransport_Integral{Transport: transport},
			Ancillary: &waMediaTransport.VideoTransport_Ancillary{
				Width:   proto.Uint32(uint32(width)),
				Height:  proto.Uint32(uint32(height)),
				Seconds: proto.Uint32(uint32(duration)),
			},
		})
		content.Content = &waConsumerApplication.ConsumerApplication_Content_VideoMessage{VideoMessage: video}
	case "audio":
		audio := &waConsumerApplication.ConsumerApplication_AudioMessage{
			PTT: proto.Bool(f.text(prefix+"X-Attachment-Type") == "1"),
		}
		if duration >= 1000 {
			duration /= 1000
		}
		err = audio.Set(&waMediaTransport.AudioTransport{
			Integral: &waMediaTransport.AudioTransport_Integral{Transport: transport},
			Ancillary: &waMediaTransport.AudioTransport_Ancillary{
				Seconds: proto.Uint32(uint32(duration)),
			},
		})
		content.Content = &waConsumerApplication.ConsumerApplication_Content_AudioMessage{AudioMessage: audio}
	case "document", "file":
		document := &waConsumerApplication.ConsumerApplication_DocumentMessage{
			FileName: proto.String(f.text("File-Name")),
		}
		err = document.Set(&waMediaTransport.DocumentTransport{
			Integral:  &waMediaTransport.DocumentTransport_Integral{Transport: transport},
			Ancillary: &waMediaTransport.DocumentTransport_Ancillary{},
		})
		content.Content = &waConsumerApplication.ConsumerApplication_Content_DocumentMessage{DocumentMessage: document}
	default:
		return nil, fmt.Errorf("unsupported Echo attachment type %q", f.text("Attachment-Type"))
	}
	return content, err
}

func (f echoFields) xmaProtobuf(metadata *waMsgApplication.MessageApplication_Metadata, id, sender string, timestamp int64) (*DecryptedProtobuf, error) {
	target, ok := waArmadilloXMA.ExtendedContentMessage_ExtendedContentType_value[f.text("XMA-Target-Type")]
	if !ok {
		return nil, errors.New("unsupported Echo XMA target type")
	}
	xma := &waArmadilloXMA.ExtendedContentMessage{
		TargetType:     waArmadilloXMA.ExtendedContentMessage_ExtendedContentType(target).Enum(),
		TargetID:       proto.String(f.text("XMA-Target-ID")),
		TargetUsername: proto.String(f.text("XMA-Target-Username")),
		TitleText:      proto.String(f.text("XMA-Title-Text")),
		SubtitleText:   proto.String(f.text("XMA-Subtitle-Text")),
		HeaderTitle:    proto.String(f.text("XMA-Header-Title")),
		HeaderSubtitle: proto.String(f.text("XMA-Header-Subtitle")),
		MessageText:    proto.String(f.text("Text")),
		ContentRef:     proto.String(f.text("XMA-Content-Ref")),
		XmaDataclass:   proto.String(f.text("XMA-Dataclass")),
	}
	if expiry, err := f.integer("XMA-Target-Expiry"); err == nil {
		xma.TargetExpiringAtSec = &expiry
	}
	if layout, ok := waArmadilloXMA.ExtendedContentMessage_XmaLayoutType_value[f.text("XMA-Layout-Type")]; ok {
		xma.XmaLayoutType = waArmadilloXMA.ExtendedContentMessage_XmaLayoutType(layout).Enum()
	}
	if gating, ok := waArmadilloXMA.ExtendedContentMessage_OverlayIconGlyph_value[f.text("XMA-Gating-Type")]; ok {
		xma.OverlayIconGlyph = waArmadilloXMA.ExtendedContentMessage_OverlayIconGlyph(gating).Enum()
	}
	if raw := f.text("XMA-Default-CTA"); raw != "" {
		xma.Ctas = []*waArmadilloXMA.ExtendedContentMessage_CTA{{
			ActionURL:  &raw,
			NativeURL:  &raw,
			ButtonType: waArmadilloXMA.ExtendedContentMessage_OPEN_NATIVE.Enum(),
		}}
	}
	if f.text("Attachment-Type") != "" {
		mediaFields := maps.Clone(f)
		mediaFields["Attachment-Type"] = "image"
		media, err := mediaFields.mediaContent()
		if err != nil {
			return nil, err
		}
		preview := media.GetImageMessage().GetImage()
		preview.Version = proto.Int32(2)
		xma.Previews = append(xma.Previews, preview)
	}
	sub := &waMsgApplication.MessageApplication_SubProtocolPayload_Armadillo{}
	err := sub.Set(&waArmadilloApplication.Armadillo{
		Payload: &waArmadilloApplication.Armadillo_Payload{
			Payload: &waArmadilloApplication.Armadillo_Payload_Content{
				Content: &waArmadilloApplication.Armadillo_Content{
					Content: &waArmadilloApplication.Armadillo_Content_ExtendedContentMessage{
						ExtendedContentMessage: xma,
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return echoApplication(&waMsgApplication.MessageApplication{
		Metadata: metadata,
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
				SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload{SubProtocol: sub},
			},
		},
	}, id, sender, timestamp)
}
