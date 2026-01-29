package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"go.mau.fi/whatsmeow/proto/instamadilloAddMessage"
	"go.mau.fi/whatsmeow/proto/instamadilloDeleteMessage"
	"go.mau.fi/whatsmeow/proto/instamadilloSupplementMessage"
	"go.mau.fi/whatsmeow/proto/waArmadilloApplication"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	folderInbox                     = "inbox"
	folderOther                     = "other"
	folderSpam                      = "spam"
	folderPending                   = "pending"
	folderMontage                   = "montage"
	folderHidden                    = "hidden"
	folderLegacy                    = "legacy"
	folderDisabled                  = "disabled"
	folderPageBackground            = "page_background"
	folderPageDone                  = "page_done"
	folderBlocked                   = "blocked"
	folderCommunity                 = "community"
	folderRestricted                = "restricted"
	folderBCPartnership             = "bc_partnership"
	folderE2EECutover               = "e2ee_cutover"
	folderE2EECutoverArchived       = "e2ee_cutover_archived"
	folderE2EECutoverPending        = "e2ee_cutover_pending"
	folderE2EECutoverOther          = "e2ee_cutover_other"
	folderInterop                   = "interop"
	folderArchived                  = "archived"
	folderAIActive                  = "ai_active"
	folderSalsaRestricted           = "salsa_restricted"
	folderMessengerMarketingMessage = "messenger_marketing_message"
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
	return evt.FolderName != folderPending && evt.FolderName != folderSpam
}

func (evt *VerifyThreadExistsEvent) GetPortalKey() networkid.PortalKey {
	return evt.m.makeFBPortalKey(evt.ThreadKey, evt.ThreadType)
}

func (evt *VerifyThreadExistsEvent) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.
		Int64("thread_id", evt.ThreadKey).
		Int64("thread_type", int64(evt.ThreadType)).
		Str("thread_folder", evt.FolderName)
}

func (evt *VerifyThreadExistsEvent) GetSender() bridgev2.EventSender {
	return bridgev2.EventSender{}
}

func (evt *VerifyThreadExistsEvent) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	if portal.MXID == "" {
		if portal.Metadata.(*metaid.PortalMetadata).FetchAttempted.Swap(true) {
			zerolog.Ctx(ctx).Warn().Msg("Not resending create request for thread that was already requested")
			return nil, fmt.Errorf("thread resync was already requested")
		}
		zerolog.Ctx(ctx).Debug().Msg("Sending create thread request for unknown thread in verifyThreadExists")
		resp, err := evt.m.Client.ExecuteTasks(ctx, &socket.CreateThreadTask{
			ThreadFBID:                evt.ThreadKey,
			ForceUpsert:               0,
			UseOpenMessengerTransport: 0,
			SyncGroup:                 1,
			MetadataOnly:              0,
			PreviewOnly:               0,
		})
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
	_ bridgev2.RemoteEventWithStreamOrder             = (*FBMessageEvent)(nil)
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

func (evt *FBMessageEvent) GetStreamOrder() int64 {
	parsedTS, err := methods.ParseMessageID(evt.MessageId)
	if err != nil {
		evt.m.UserLogin.Log.Warn().Err(err).Str("message_id", evt.MessageId).Msg("Failed to parse message ID")
	} else if parsedTS != evt.TimestampMs {
		evt.m.UserLogin.Log.Warn().
			Int64("parsed_ts", parsedTS).
			Int64("timestamp_ms", evt.TimestampMs).
			Str("message_id", evt.MessageId).
			Msg("Message ID timestamp mismatch")
	}
	return evt.TimestampMs
}

func (evt *FBMessageEvent) ConvertMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI) (*bridgev2.ConvertedMessage, error) {
	cli := evt.m.Client
	if cli == nil {
		return nil, messagix.ErrClientIsNil
	}
	return evt.m.Main.MsgConv.ToMatrix(ctx, portal, cli, intent, evt.GetID(), evt.WrappedMessage, evt.m.Main.Config.DisableXMAAlways), nil
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

type EnsureWAChatStateEvent struct {
	JID types.JID
	m   *MetaClient
}

var (
	_ bridgev2.RemoteChatResyncWithInfo       = (*EnsureWAChatStateEvent)(nil)
	_ bridgev2.RemoteEventThatMayCreatePortal = (*EnsureWAChatStateEvent)(nil)
)

func (evt *EnsureWAChatStateEvent) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventChatResync
}

func (evt *EnsureWAChatStateEvent) GetPortalKey() networkid.PortalKey {
	return evt.m.makeWAPortalKey(evt.JID)
}

func (evt *EnsureWAChatStateEvent) ShouldCreatePortal() bool {
	return true
}

func (evt *EnsureWAChatStateEvent) AddLogContext(c zerolog.Context) zerolog.Context {
	return c
}

func (evt *EnsureWAChatStateEvent) GetSender() bridgev2.EventSender {
	return bridgev2.EventSender{}
}

func (evt *EnsureWAChatStateEvent) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	switch evt.JID.Server {
	case types.GroupServer:
		if portal.MXID != "" {
			return &bridgev2.ChatInfo{
				Type:         ptr.Ptr(database.RoomTypeDefault),
				ExtraUpdates: updateServerAndThreadType(evt.JID, table.ENCRYPTED_OVER_WA_GROUP),
			}, nil
		}
		groupInfo, err := evt.m.E2EEClient.GetGroupInfo(ctx, evt.JID)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to fetch WhatsApp group info")
			return nil, err
		}
		return evt.m.wrapWAGroupInfo(groupInfo), nil
	case types.MessengerServer, types.DefaultUserServer:
		if portal.MXID != "" {
			return &bridgev2.ChatInfo{
				Type:         ptr.Ptr(database.RoomTypeDM),
				ExtraUpdates: updateServerAndThreadType(evt.JID, table.ENCRYPTED_OVER_WA_ONE_TO_ONE),
			}, nil
		}
		return evt.m.makeWADirectChatInfo(evt.JID), nil
	default:
		return nil, fmt.Errorf("unknown WhatsApp server %s", evt.JID.Server)
	}
}

type WAMessageEvent struct {
	*events.FBMessage
	m *MetaClient
}

var (
	_ bridgev2.RemoteMessage              = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteEdit                 = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteEventWithTimestamp   = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteReaction             = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteReactionRemove       = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteMessageRemove        = (*WAMessageEvent)(nil)
	_ bridgev2.RemoteEventWithStreamOrder = (*WAMessageEvent)(nil)
)

func (evt *WAMessageEvent) GetTargetMessage() networkid.MessageID {
	if evt.Message == nil {
		return ""
	}
	consumerApp, ok := evt.Message.(*waConsumerApplication.ConsumerApplication)
	if !ok {
		//payload, ok := evt.Message.(*instamadilloSupplementMessage.SupplementMessagePayload)
		//if ok {
		//	// TODO return target message
		//}
		return ""
	}
	switch typedPayload := consumerApp.GetPayload().GetPayload().(type) {
	case *waConsumerApplication.ConsumerApplication_Payload_Content:
		switch content := typedPayload.Content.GetContent().(type) {
		case *waConsumerApplication.ConsumerApplication_Content_EditMessage:
			return evt.m.waKeyToMessageID(evt.Info.Chat, evt.Info.Sender, content.EditMessage.GetKey())
		case *waConsumerApplication.ConsumerApplication_Content_ReactionMessage:
			return evt.m.waKeyToMessageID(evt.Info.Chat, evt.Info.Sender, content.ReactionMessage.GetKey())
		}
	case *waConsumerApplication.ConsumerApplication_Payload_ApplicationData:
		switch applicationContent := typedPayload.ApplicationData.GetApplicationContent().(type) {
		case *waConsumerApplication.ConsumerApplication_ApplicationData_Revoke:
			return evt.m.waKeyToMessageID(evt.Info.Chat, evt.Info.Sender, applicationContent.Revoke.GetKey())
		}
	}
	return ""
}

func (m *MetaClient) messageIDToWAKey(id metaid.ParsedWAMessageID) *waCommon.MessageKey {
	key := &waCommon.MessageKey{
		RemoteJID: ptr.Ptr(id.Chat.String()),
		ID:        ptr.Ptr(id.ID),
	}
	if id.Sender.User == string(m.UserLogin.ID) {
		key.FromMe = ptr.Ptr(true)
	}
	if id.Chat.Server != types.MessengerServer && id.Chat.Server != types.DefaultUserServer {
		key.Participant = ptr.Ptr(id.Sender.String())
	}
	return key
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
	switch typed := evt.Message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		return typed.GetPayload().GetContent().GetReactionMessage().GetText(), ""
	case *instamadilloSupplementMessage.SupplementMessagePayload:
		switch innerTyped := typed.GetContent().GetSupplementMessageContent().(type) {
		case *instamadilloSupplementMessage.SupplementMessageContent_Reaction:
			return innerTyped.Reaction.GetEmoji(), ""
		case *instamadilloSupplementMessage.SupplementMessageContent_MediaReaction:
			return innerTyped.MediaReaction.GetReaction().GetEmoji(), ""
		}
		return "", ""
	default:
		return "", ""
	}
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
	case *instamadilloSupplementMessage.SupplementMessagePayload:
		switch typedMsg.GetContent().GetSupplementMessageContent().(type) {
		case *instamadilloSupplementMessage.SupplementMessageContent_Reaction:
			// TODO reaction removal
			return bridgev2.RemoteEventReaction
		case *instamadilloSupplementMessage.SupplementMessageContent_MediaReaction:
			return bridgev2.RemoteEventReaction
		case *instamadilloSupplementMessage.SupplementMessageContent_EditText:
			return bridgev2.RemoteEventEdit
		}
	case *instamadilloAddMessage.AddMessagePayload:
		return bridgev2.RemoteEventMessage
	case *instamadilloDeleteMessage.DeleteMessagePayload:
		return bridgev2.RemoteEventMessageRemove
	default:
		if evt.Message == nil && evt.FBApplication.GetMetadata().GetChatEphemeralSetting() != nil {
			return bridgev2.RemoteEventMessage
		}
		log.Warn().Type("message_type", evt.Message).Msg("Unrecognized message type")
	}
	return bridgev2.RemoteEventUnknown
}

func (evt *WAMessageEvent) GetPortalKey() networkid.PortalKey {
	return evt.m.makeWAPortalKey(evt.Info.Chat)
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
	switch typedMsg := evt.Message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		consumerContent := typedMsg.GetPayload().GetContent()
		if reactionSenderTS := consumerContent.GetReactionMessage().GetSenderTimestampMS(); reactionSenderTS != 0 {
			return time.UnixMilli(reactionSenderTS)
		} else if editTS := consumerContent.GetEditMessage().GetTimestampMS(); editTS != 0 {
			return time.UnixMilli(editTS)
		}
	}
	return evt.Info.Timestamp
}

func (evt *WAMessageEvent) GetStreamOrder() int64 {
	// Note: WhatsApp timestamps are seconds, but we use unix millis in order to match FB stream orders.
	return evt.GetTimestamp().UnixMilli()
}

func (evt *WAMessageEvent) ConvertMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI) (*bridgev2.ConvertedMessage, error) {
	return evt.m.Main.MsgConv.WhatsAppToMatrix(ctx, portal, evt.m.E2EEClient, intent, evt.GetID(), evt.FBMessage), nil
}

func (evt *WAMessageEvent) ConvertEdit(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message) (*bridgev2.ConvertedEdit, error) {
	if len(existing) > 1 {
		zerolog.Ctx(ctx).Warn().Msg("Got edit to message with multiple parts")
	}
	switch typedMsg := evt.Message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		editMsg := typedMsg.GetPayload().GetContent().GetEditMessage()
		if existing[0].Metadata.(*metaid.MessageMetadata).EditTimestamp >= editMsg.GetTimestampMS() {
			return nil, fmt.Errorf("%w: duplicate edit", bridgev2.ErrIgnoringRemoteEvent)
		}
		converted := evt.m.Main.MsgConv.WhatsAppTextToMatrix(ctx, editMsg.GetMessage())
		editPart := converted.ToEditPart(existing[0])
		editPart.Part.Metadata.(*metaid.MessageMetadata).EditTimestamp = editMsg.GetTimestampMS()
		return &bridgev2.ConvertedEdit{
			ModifiedParts: []*bridgev2.ConvertedEditPart{editPart},
		}, nil
	case *instamadilloSupplementMessage.SupplementMessagePayload:
		editText := typedMsg.GetContent().GetEditText()
		if existing[0].EditCount >= int(editText.GetEditCount()) {
			return nil, fmt.Errorf("%w: duplicate edit", bridgev2.ErrIgnoringRemoteEvent)
		}
		converted := &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType:  event.MsgText,
				Body:     editText.GetNewContent(),
				Mentions: &event.Mentions{},
			},
		}
		editPart := converted.ToEditPart(existing[0])
		editPart.Part.EditCount = int(editText.GetEditCount())
		return &bridgev2.ConvertedEdit{
			ModifiedParts: []*bridgev2.ConvertedEditPart{editPart},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported message type %T for edit conversion", evt.Message)
	}
}

type FBChatResync struct {
	Raw       *table.LSDeleteThenInsertThread
	PortalKey networkid.PortalKey
	Info      *bridgev2.ChatInfo
	Members   map[int64]bridgev2.ChatMember
	Backfill  *table.UpsertMessages
	UpsertID  int64
	m         *MetaClient

	filled bool
}

var (
	_ bridgev2.RemoteChatResyncWithInfo       = (*FBChatResync)(nil)
	_ bridgev2.RemoteEventThatMayCreatePortal = (*FBChatResync)(nil)
	_ bridgev2.RemoteChatResyncBackfillBundle = (*FBChatResync)(nil)
)

func (r *FBChatResync) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventChatResync
}

func (r *FBChatResync) GetPortalKey() networkid.PortalKey {
	return r.PortalKey
}

func (r *FBChatResync) ShouldCreatePortal() bool {
	if r.Raw == nil {
		return false
	}
	return r.Raw.FolderName != folderPending && r.Raw.FolderName != folderSpam
}

func (r *FBChatResync) AddLogContext(c zerolog.Context) zerolog.Context {
	if r.UpsertID != 0 {
		c = c.Int64("global_upsert_counter", r.UpsertID)
	}
	if r.Raw == nil {
		return c
	}
	c = c.
		Int64("thread_id", r.Raw.ThreadKey).
		Int("thread_type", int(r.Raw.ThreadType)).
		Dict("debug_info", zerolog.Dict().
			Str("thread_folder", r.Raw.FolderName).
			Int64("ig_folder", r.Raw.IgFolder).
			Int64("group_notification_settings", r.Raw.GroupNotificationSettings))
	return c
}

func (r *FBChatResync) GetSender() bridgev2.EventSender {
	return bridgev2.EventSender{}
}

func (r *FBChatResync) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	if r.Raw == nil {
		return nil, nil
	}
	if len(r.Members) > 0 && !r.filled {
		self := r.Info.Members.MemberMap[networkid.UserID(r.m.UserLogin.ID)]
		r.Info.Members.MemberMap = make(map[networkid.UserID]bridgev2.ChatMember, len(r.Members))
		for id, member := range r.Members {
			r.Info.Members.MemberMap[metaid.MakeUserID(id)] = member
		}
		if r.Info.Members.TotalMemberCount > 0 {
			r.Info.Members.IsFull = len(r.Members) == r.Info.Members.TotalMemberCount
		}
		hasSelf := false
		for _, member := range r.Members {
			if member.IsFromMe {
				hasSelf = true
				break
			}
		}
		if !hasSelf && self.IsFromMe {
			r.Info.Members.MemberMap[self.Sender] = self
		}
		r.filled = true
		if len(r.Info.Members.MemberMap) == 1 && portal.OtherUserID != "" && portal.OtherUserID == networkid.UserID(portal.Receiver) {
			r.Info.Members.MemberMap = makeNoteToSelfMembers(portal.OtherUserID, r.Info.Members.MemberMap[portal.OtherUserID].UserInfo)
		}
	}
	return r.Info, nil
}

func (r *FBChatResync) CheckNeedsBackfill(ctx context.Context, lastMessage *database.Message) (bool, error) {
	// Check for forward backfill if we're handling a remote update, we need to fill any gap between
	// the last message we know of and the last activity timestamp specified on the thread.
	if r.Backfill == nil && r.Raw != nil && lastMessage != nil && r.Raw.LastActivityTimestampMs > lastMessage.Timestamp.UnixMilli() {
		zerolog.Ctx(ctx).Debug().
			Int64("last_message_ts", lastMessage.Timestamp.UnixMilli()).
			Int64("thread_last_activity_ts", r.Raw.LastActivityTimestampMs).
			Msg("Thread has newer activity than last known message, triggering forward backfill")
		return true, nil
	}
	if r.Backfill == nil {
		return false, nil
	} else if lastMessage == nil {
		return true, nil
	} else if metaid.MakeFBMessageID(r.Backfill.Range.MaxMessageId) == lastMessage.ID {
		return false, nil
	} else if r.Backfill.Range.MaxTimestampMs > lastMessage.Timestamp.UnixMilli() {
		return true, nil
	} else {
		zerolog.Ctx(ctx).Debug().
			Int64("last_message_ts", lastMessage.Timestamp.UnixMilli()).
			Str("last_message_id", string(lastMessage.ID)).
			Int64("upsert_max_ts", r.Backfill.Range.MaxTimestampMs).
			Str("upsert_max_id", r.Backfill.Range.MaxMessageId).
			Msg("Ignoring unrequested upsert before last message")
		return false, nil
	}
}

func (r *FBChatResync) GetBundledBackfillData() any {
	return r.Backfill
}
