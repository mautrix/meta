package connector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

func (m *MetaClient) decodeArchiveMessage(portal *bridgev2.Portal, message *waArmadilloBackupMessage.BackupMessage, timestampMS int64) (*WAMessageEvent, error) {
	var decoded events.FBMessage
	switch payload := message.GetPayload().(type) {
	case *waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage:
		var application waMsgApplication.MessageApplication
		if err := proto.Unmarshal(payload.EncryptedTransportMessage, &application); err != nil {
			return nil, fmt.Errorf("invalid archived Messenger application: %w", err)
		}
		var err error
		decoded, err = whatsmeow.DecodeFBApplication(&application)
		if err != nil {
			return nil, err
		}
	case *waArmadilloBackupMessage.BackupMessage_EncryptedTransportEvent,
		*waArmadilloBackupMessage.BackupMessage_EncryptedTransportLocallyTransformedMessage:
		return nil, nil
	case *waArmadilloBackupMessage.BackupMessage_MiTransportAdminMessage:
	default:
		return nil, errors.New("missing Messenger archive message payload")
	}
	metadata := message.GetMetadata()
	if metadata.GetMessageID() == "" || metadata.GetSenderID() == "" || timestampMS < 0 {
		return nil, errors.New("missing Messenger archive message metadata")
	}
	senderID, err := metaid.ParseIDFromString(metadata.GetSenderID())
	if err != nil {
		return nil, fmt.Errorf("invalid Messenger archive sender: %w", err)
	}
	chat := portal.Metadata.(*metaid.PortalMetadata).JID(portal.ID)
	decoded.Info = types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   types.NewJID(metadata.GetSenderID(), types.MessengerServer),
			IsFromMe: metaid.MakeUserLoginID(senderID) == m.UserLogin.ID,
			IsGroup:  chat.Server == types.GroupServer,
		},
		ID:        metadata.GetMessageID(),
		Timestamp: time.UnixMilli(timestampMS),
	}
	return &WAMessageEvent{FBMessage: &decoded, m: m}, nil
}

func (m *MetaClient) convertArchiveMessage(ctx context.Context, portal *bridgev2.Portal, message *securehistory.DecryptedMessage) (*bridgev2.BackfillMessage, error) {
	if message == nil || message.Message == nil {
		return nil, errors.New("missing decrypted Messenger archive message")
	}
	parent, err := m.decodeArchiveMessage(portal, message.Message.Message, message.Message.TimestampMS)
	if err != nil || parent == nil {
		return nil, err
	}
	type archiveReaction struct {
		message     *WAMessageEvent
		timestampMS int64
		text        string
	}
	reactions := make(map[string]archiveReaction)
	var latestEdit *waConsumerApplication.ConsumerApplication_EditMessage
	var editTimestampMS int64
	for _, supplemental := range message.Supplementals {
		if supplemental == nil {
			return nil, errors.New("missing Messenger archive supplemental")
		}
		switch supplemental.Message.GetPayload().(type) {
		case *waArmadilloBackupMessage.BackupMessage_EncryptedTransportEvent,
			*waArmadilloBackupMessage.BackupMessage_EncryptedTransportLocallyTransformedMessage,
			*waArmadilloBackupMessage.BackupMessage_MiTransportAdminMessage:
			continue
		}
		if supplemental.Message.GetMetadata() == nil || supplemental.Message.GetMetadata().TimestampMS == nil {
			return nil, errors.New("missing Messenger archive supplemental metadata")
		}
		timestampMS := supplemental.Message.GetMetadata().GetTimestampMS()
		decoded, err := m.decodeArchiveMessage(portal, supplemental.Message, timestampMS)
		if err != nil {
			return nil, err
		} else if decoded == nil {
			continue
		}
		content := decoded.GetConsumerApplication().GetPayload().GetContent()
		if reaction := content.GetReactionMessage(); reaction != nil {
			if target := reaction.GetKey().GetID(); target != "" && target != parent.Info.ID {
				return nil, errors.New("archive reaction references a different message")
			}
			actor := decoded.Info.Sender.User
			previous, exists := reactions[actor]
			if !exists || timestampMS > previous.timestampMS || (timestampMS == previous.timestampMS && reaction.GetText() == "") {
				reactions[actor] = archiveReaction{message: decoded, timestampMS: timestampMS, text: reaction.GetText()}
			}
		} else if edit := content.GetEditMessage(); edit != nil {
			if target := edit.GetKey().GetID(); target != "" && target != parent.Info.ID {
				return nil, errors.New("archive edit references a different message")
			} else if decoded.Info.Sender != parent.Info.Sender || edit.GetMessage() == nil {
				return nil, errors.New("invalid Messenger archive edit")
			}
			if latestEdit == nil || timestampMS > editTimestampMS {
				latestEdit = edit
				editTimestampMS = timestampMS
			}
		} else if content.GetPollUpdateMessage() != nil && parent.GetConsumerApplication().GetPayload().GetContent().GetPollCreationMessage() != nil {
			continue
		} else {
			return nil, fmt.Errorf("unsupported Messenger archive supplemental %T", content.GetContent())
		}
	}
	if latestEdit != nil {
		parent.Message = &waConsumerApplication.ConsumerApplication{
			Payload: &waConsumerApplication.ConsumerApplication_Payload{
				Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
					Content: &waConsumerApplication.ConsumerApplication_Content{
						Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{MessageText: latestEdit.GetMessage()},
					},
				},
			},
		}
	}
	sender := parent.GetSender()
	intent, ok := portal.GetIntentFor(ctx, sender, m.UserLogin, bridgev2.RemoteEventBackfill)
	if !ok {
		return nil, nil
	}
	if len(message.Attachments) > 0 {
		paths := make(map[string]string, len(message.Attachments))
		for _, attachment := range message.Attachments {
			if attachment.ObjectID != "" && attachment.URL != "" {
				paths[attachment.ObjectID] = attachment.URL
			}
		}
		ctx = context.WithValue(ctx, mediadl.ContextKeyWAMediaPaths, paths)
	}
	converted := m.Main.MsgConv.WhatsAppToMatrix(ctx, portal, m.Client, m.E2EEClient, m.UserLogin, intent, parent.GetID(), parent.FBMessage)
	if latestEdit != nil && len(converted.Parts) > 0 {
		metadata, ok := converted.Parts[0].DBMetadata.(*metaid.MessageMetadata)
		if !ok {
			metadata = &metaid.MessageMetadata{}
			converted.Parts[0].DBMetadata = metadata
		}
		metadata.EditTimestamp = latestEdit.GetTimestampMS()
		if metadata.EditTimestamp == 0 {
			metadata.EditTimestamp = editTimestampMS
		}
	}
	result := &bridgev2.BackfillMessage{
		ConvertedMessage: converted,
		Sender:           sender,
		ID:               parent.GetID(),
		Timestamp:        parent.Info.Timestamp,
		StreamOrder:      parent.Info.Timestamp.UnixMilli(),
	}
	for _, reaction := range reactions {
		if reaction.text != "" {
			result.Reactions = append(result.Reactions, &bridgev2.BackfillReaction{
				Timestamp: reaction.message.GetTimestamp(),
				Sender:    reaction.message.GetSender(),
				Emoji:     reaction.text,
			})
		}
	}
	return result, nil
}
