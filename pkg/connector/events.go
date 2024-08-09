package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"go.mau.fi/whatsmeow/proto/waArmadilloApplication"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type VerifyThreadExistsEvent struct {
	*table.LSVerifyThreadExists
	m *MetaClient
}

var (
	_ bridgev2.RemoteChatResyncWithInfo       = (*VerifyThreadExistsEvent)(nil)
	_ bridgev2.RemoteEventThatMayCreatePortal = (*VerifyThreadExistsEvent)(nil)
)

func (evt *VerifyThreadExistsEvent) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventChatResync
}

func (evt *VerifyThreadExistsEvent) ShouldCreatePortal() bool {
	return true
}

func (evt *VerifyThreadExistsEvent) GetPortalKey() networkid.PortalKey {
	return evt.m.makeFBPortalKey(evt.ThreadKey, evt.ThreadType)
}

func (evt *VerifyThreadExistsEvent) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.Int64("thread_id", evt.ThreadKey).Int64("thread_type", int64(evt.ThreadType))
}

func (evt *VerifyThreadExistsEvent) GetSender() bridgev2.EventSender {
	return bridgev2.EventSender{}
}

func (evt *VerifyThreadExistsEvent) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	if portal.MXID == "" {
		if portal.Metadata.(*PortalMetadata).fetchAttempted.Swap(true) {
			zerolog.Ctx(ctx).Warn().Msg("Not resending create request for thread that was already requested")
			return nil, fmt.Errorf("thread resync was already requested")
		}
		zerolog.Ctx(ctx).Debug().Msg("Sending create thread request for unknown thread in verifyThreadExists")
		resp, err := evt.m.Client.ExecuteTasks(
			&socket.CreateThreadTask{
				ThreadFBID:                evt.ThreadKey,
				ForceUpsert:               0,
				UseOpenMessengerTransport: 0,
				SyncGroup:                 1,
				MetadataOnly:              0,
				PreviewOnly:               0,
			},
		)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to request full thread info")
		} else {
			zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("Requested full thread info")
		}
	}
	return evt.m.makeMinimalChatInfo(evt.ThreadKey, evt.ThreadType), nil
}

type FBMessageEvent struct {
	*table.WrappedMessage
	portalKey         networkid.PortalKey
	uncertainReceiver bool
	m                 *MetaClient
}

var (
	_ bridgev2.RemoteMessage                          = (*FBMessageEvent)(nil)
	_ bridgev2.RemoteEventWithUncertainPortalReceiver = (*FBMessageEvent)(nil)
	_ bridgev2.RemoteEventWithTimestamp               = (*FBMessageEvent)(nil)
)

func (evt *FBMessageEvent) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventMessage
}

func (evt *FBMessageEvent) GetPortalKey() networkid.PortalKey {
	return evt.portalKey
}

func (evt *FBMessageEvent) PortalReceiverIsUncertain() bool {
	return evt.uncertainReceiver
}

func (evt *FBMessageEvent) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.Str("message_id", evt.MessageId).Int64("sender_id", evt.SenderId)
}

func (evt *FBMessageEvent) GetSender() bridgev2.EventSender {
	return evt.m.makeEventSender(evt.SenderId)
}

func (evt *FBMessageEvent) GetID() networkid.MessageID {
	return metaid.MakeFBMessageID(evt.MessageId)
}

func (evt *FBMessageEvent) GetTimestamp() time.Time {
	return time.UnixMilli(evt.TimestampMs)
}

func (evt *FBMessageEvent) ConvertMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI) (*bridgev2.ConvertedMessage, error) {
	return evt.m.Main.MsgConv.ToMatrix(ctx, portal, evt.m.Client, intent, evt.WrappedMessage), nil
}

type FBEditEvent struct {
	*table.LSEditMessage
	orig *database.Message
	m    *MetaClient
}

var (
	_ bridgev2.RemoteEdit = (*FBEditEvent)(nil)
)

func (evt *FBEditEvent) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventEdit
}

func (evt *FBEditEvent) GetPortalKey() networkid.PortalKey {
	return evt.orig.Room
}

func (evt *FBEditEvent) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.Str("message_id", evt.MessageID)
}

func (evt *FBEditEvent) GetSender() bridgev2.EventSender {
	return bridgev2.EventSender{
		IsFromMe:    networkid.UserLoginID(evt.orig.SenderID) == evt.m.UserLogin.ID,
		SenderLogin: networkid.UserLoginID(evt.orig.SenderID),
		Sender:      evt.orig.SenderID,
	}
}

func (evt *FBEditEvent) GetTargetMessage() networkid.MessageID {
	return metaid.MakeFBMessageID(evt.MessageID)
}

func (evt *FBEditEvent) ConvertEdit(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message) (*bridgev2.ConvertedEdit, error) {
	textPart := existing[0] // TODO: Figure out a better way to get the text part, esp. if there are attachments etc.

	return &bridgev2.ConvertedEdit{
		ModifiedParts: []*bridgev2.ConvertedEditPart{
			{
				Part:    textPart,
				Type:    event.EventMessage,
				Content: evt.m.Main.MsgConv.MetaToMatrixText(ctx, evt.Text, nil, portal),
			},
		},
	}, nil
}

type WAMessageEvent struct {
	*events.FBMessage
	evtType   bridgev2.RemoteEventType
	portalKey networkid.PortalKey
	m         *MetaClient
}

var (
	_ bridgev2.RemoteMessage                  = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteEdit                     = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteEventWithTimestamp       = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteEventThatMayCreatePortal = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteReaction                 = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteReactionRemove           = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteMessageRemove            = (*WAMessageEvent)(nil)
)

func (evt *WAMessageEvent) GetTargetMessage() networkid.MessageID {
	consumerApp, ok := evt.Message.(*waConsumerApplication.ConsumerApplication)
	if !ok {
		panic(fmt.Errorf("GetTargetMessage called for non-ConsumerApplication message %T", evt.Message))
	}
	switch typedPayload := consumerApp.GetPayload().GetPayload().(type) {
	case *waConsumerApplication.ConsumerApplication_Payload_Content:
		switch content := typedPayload.Content.GetContent().(type) {
		case *waConsumerApplication.ConsumerApplication_Content_EditMessage:
			return evt.m.waKeyToMessageID(evt.Info.Chat, evt.Info.Sender, content.EditMessage.GetKey())
		case *waConsumerApplication.ConsumerApplication_Content_ReactionMessage:
			return evt.m.waKeyToMessageID(evt.Info.Chat, evt.Info.Sender, content.ReactionMessage.GetKey())
		default:
			panic(fmt.Errorf("GetTargetMessage called for non-edit/reaction content message (%T)", content))
		}
	case *waConsumerApplication.ConsumerApplication_Payload_ApplicationData:
		switch applicationContent := typedPayload.ApplicationData.GetApplicationContent().(type) {
		case *waConsumerApplication.ConsumerApplication_ApplicationData_Revoke:
			return evt.m.waKeyToMessageID(evt.Info.Chat, evt.Info.Sender, applicationContent.Revoke.GetKey())
		default:
			panic(fmt.Errorf("GetTargetMessage called for non-Revoke application data message (%T)", applicationContent))
		}
	default:
		panic(fmt.Errorf("GetTargetMessage called for non-Content/ApplicationData consumer message (%T)", typedPayload))
	}
}

func (m *MetaClient) waKeyToMessageID(chat, sender types.JID, key *waCommon.MessageKey) networkid.MessageID {
	sender = sender.ToNonAD()
	var err error
	if !key.GetFromMe() {
		if key.GetParticipant() != "" {
			sender, err = types.ParseJID(key.GetParticipant())
			if err != nil {
				// TODO log somehow?
				return ""
			}
			if sender.Server == types.LegacyUserServer {
				sender.Server = types.DefaultUserServer
			}
		} else if chat.Server == types.DefaultUserServer || chat.Server == types.MessengerServer {
			ownID := ptr.Val(m.WADevice.ID).ToNonAD()
			if sender.User == ownID.User {
				sender = chat
			} else {
				sender = ownID
			}
		} else {
			// TODO log somehow?
			return ""
		}
	}
	remoteJID, err := types.ParseJID(key.GetRemoteJID())
	if err == nil && !remoteJID.IsEmpty() {
		// TODO use remote jid in other cases?
		if remoteJID.Server == types.GroupServer {
			chat = remoteJID
		}
	}
	return metaid.MakeWAMessageID(chat, sender, key.GetID())
}

func (evt *WAMessageEvent) GetReactionEmoji() (string, networkid.EmojiID) {
	consumerApp, _ := evt.Message.(*waConsumerApplication.ConsumerApplication)
	return consumerApp.GetPayload().GetContent().GetReactionMessage().GetText(), ""
}

func (evt *WAMessageEvent) GetRemovedEmojiID() networkid.EmojiID {
	return ""
}

func (evt *WAMessageEvent) GetType() bridgev2.RemoteEventType {
	// FIXME proper log
	log := zerolog.Ctx(context.TODO())
	switch typedMsg := evt.Message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		switch payload := typedMsg.GetPayload().GetPayload().(type) {
		case *waConsumerApplication.ConsumerApplication_Payload_Content:
			switch content := payload.Content.GetContent().(type) {
			case *waConsumerApplication.ConsumerApplication_Content_EditMessage:
				return bridgev2.RemoteEventEdit
			case *waConsumerApplication.ConsumerApplication_Content_ReactionMessage:
				if content.ReactionMessage.GetText() == "" {
					return bridgev2.RemoteEventReactionRemove
				}
				return bridgev2.RemoteEventReaction
			default:
				return bridgev2.RemoteEventMessage
			}
		case *waConsumerApplication.ConsumerApplication_Payload_ApplicationData:
			switch applicationContent := payload.ApplicationData.GetApplicationContent().(type) {
			case *waConsumerApplication.ConsumerApplication_ApplicationData_Revoke:
				return bridgev2.RemoteEventMessageRemove
			default:
				log.Warn().Type("content_type", applicationContent).Msg("Unrecognized application content type")
			}
		case *waConsumerApplication.ConsumerApplication_Payload_Signal:
			log.Warn().Msg("Unsupported consumer signal payload message")
		case *waConsumerApplication.ConsumerApplication_Payload_SubProtocol:
			log.Warn().Msg("Unsupported consumer subprotocol payload message")
		default:
			log.Warn().Type("payload_type", payload).Msg("Unrecognized consumer message payload type")
		}
	case *waArmadilloApplication.Armadillo:
		switch payload := typedMsg.GetPayload().GetPayload().(type) {
		case *waArmadilloApplication.Armadillo_Payload_Content:
			return bridgev2.RemoteEventMessage
		case *waArmadilloApplication.Armadillo_Payload_ApplicationData:
			log.Warn().Msg("Unsupported armadillo application data message")
		case *waArmadilloApplication.Armadillo_Payload_Signal:
			log.Warn().Msg("Unsupported armadillo signal payload message")
		case *waArmadilloApplication.Armadillo_Payload_SubProtocol:
			log.Warn().Msg("Unsupported armadillo subprotocol payload message")
		default:
			log.Warn().Type("payload_type", payload).Msg("Unrecognized armadillo message payload type")
		}
	default:
		log.Warn().Type("message_type", evt.Message).Msg("Unrecognized message type")
	}
	return bridgev2.RemoteEventUnknown
}

func (evt *WAMessageEvent) ShouldCreatePortal() bool {
	return true
}

func (evt *WAMessageEvent) GetPortalKey() networkid.PortalKey {
	return evt.portalKey
}

func (evt *WAMessageEvent) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.Str("message_id", evt.Info.ID).Stringer("sender_id", evt.Info.Sender)
}

func (evt *WAMessageEvent) GetSender() bridgev2.EventSender {
	return evt.m.makeWAEventSender(evt.Info.Sender)
}

func (evt *WAMessageEvent) GetID() networkid.MessageID {
	return metaid.MakeWAMessageID(evt.Info.Chat, evt.Info.Sender, evt.Info.ID)
}

func (evt *WAMessageEvent) GetTimestamp() time.Time {
	consumerContent := evt.GetConsumerApplication().GetPayload().GetContent()
	if reactionSenderTS := consumerContent.GetReactionMessage().GetSenderTimestampMS(); reactionSenderTS != 0 {
		return time.UnixMilli(reactionSenderTS)
	} else if editTS := consumerContent.GetEditMessage().GetTimestampMS(); editTS != 0 {
		return time.UnixMilli(editTS)
	}
	return evt.Info.Timestamp
}

func (evt *WAMessageEvent) ConvertMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI) (*bridgev2.ConvertedMessage, error) {
	return evt.m.Main.MsgConv.WhatsAppToMatrix(ctx, portal, evt.m.E2EEClient, intent, evt.FBMessage), nil
}

func (evt *WAMessageEvent) ConvertEdit(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message) (*bridgev2.ConvertedEdit, error) {
	if len(existing) > 1 {
		zerolog.Ctx(ctx).Warn().Msg("Got edit to message with multiple parts")
	}
	editMsg := evt.GetConsumerApplication().GetPayload().GetContent().GetEditMessage()
	if existing[0].Metadata.(*MessageMetadata).EditTimestamp <= editMsg.GetTimestampMS() {
		return nil, fmt.Errorf("%w: duplicate edit", bridgev2.ErrIgnoringRemoteEvent)
	}
	converted := evt.m.Main.MsgConv.WhatsAppTextToMatrix(ctx, editMsg.GetMessage())
	editPart := converted.ToEditPart(existing[0])
	editPart.Part.Metadata.(*MessageMetadata).EditTimestamp = editMsg.GetTimestampMS()
	return &bridgev2.ConvertedEdit{
		ModifiedParts: []*bridgev2.ConvertedEditPart{editPart},
	}, nil
}
