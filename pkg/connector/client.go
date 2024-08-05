package connector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/util/variationselector"

	"go.mau.fi/mautrix-meta/config"
	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/socket"

	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"

	"go.mau.fi/mautrix-meta/pkg/connector/ids"
	"go.mau.fi/mautrix-meta/pkg/connector/msgconv"

	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
)

type metaEvent struct {
	context context.Context
	event   any
}

type MetaClient struct {
	Main   *MetaConnector
	client *messagix.Client

	log     zerolog.Logger
	cookies *cookies.Cookies
	login   *bridgev2.UserLogin

	incomingEvents   chan *metaEvent
	messageConverter *msgconv.MessageConverter
}

// Why are these separate?
func platformToMode(platform types.Platform) config.BridgeMode {
	switch platform {
	case types.Facebook:
		return config.ModeFacebook
	case types.Instagram:
		return config.ModeInstagram
	default:
		panic(fmt.Sprintf("unknown platform %d", platform))
	}
}

func NewMetaClient(ctx context.Context, main *MetaConnector, login *bridgev2.UserLogin) (*MetaClient, error) {
	log := zerolog.Ctx(ctx).With().Str("component", "meta_client").Logger()

	loginMetadata := login.Metadata.(*MetaLoginMetadata)

	log.Debug().Any("metadata", loginMetadata).Msg("Creating new Meta client")

	c := loginMetadata.Cookies
	c.Platform = types.Platform(loginMetadata.Platform)

	return &MetaClient{
		Main:           main,
		cookies:        c,
		log:            log,
		login:          login,
		incomingEvents: make(chan *metaEvent, 8),
		messageConverter: &msgconv.MessageConverter{
			BridgeMode: platformToMode(c.Platform),
		},
	}, nil
}

func (m *MetaClient) Update(ctx context.Context) error {
	m.login.Metadata.(*MetaLoginMetadata).Cookies = m.cookies
	err := m.login.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save updated cookies: %w", err)
	}
	zerolog.Ctx(ctx).Debug().Msg("Updated cookies")
	return nil
}

// We don't want to block while handling events, but they must be processed in order, so we use a channel to queue them.
func (m *MetaClient) metaEventHandler(rawEvt any) {
	ctx := m.log.WithContext(context.TODO())

	evt := metaEvent{
		context: ctx,
		event:   rawEvt,
	}

	m.incomingEvents <- &evt
}

func (m *MetaClient) handleMetaEventLoop() {
	for evt := range m.incomingEvents {
		if evt == nil {
			m.log.Debug().Msg("Received nil event, stopping event handling")
			return
		}
		m.handleMetaEvent(evt.context, evt.event)
	}
}

func (m *MetaClient) handleMetaEvent(ctx context.Context, evt any) {
	log := zerolog.Ctx(ctx)

	switch evt := evt.(type) {
	case *messagix.Event_PublishResponse:
		log.Trace().Any("table", &evt.Table).Msg("Got new event table")
		m.handleTable(ctx, evt.Table)
	case *messagix.Event_Ready:
		log.Trace().Msg("Initial connect to Meta socket completed, sending connected BridgeState")
		m.login.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
	default:
		log.Warn().Type("event_type", evt).Msg("Unrecognized event type from messagix")
	}
}

func (m *MetaClient) senderFromID(id int64) bridgev2.EventSender {
	return bridgev2.EventSender{
		IsFromMe:    ids.MakeUserLoginID(id) == m.login.ID,
		Sender:      ids.MakeUserID(id),
		SenderLogin: ids.MakeUserLoginID(id),
	}
}

func (m *MetaClient) handleTable(ctx context.Context, tbl *table.LSTable) {
	log := zerolog.Ctx(ctx)

	for _, contact := range tbl.LSDeleteThenInsertContact {
		log.Warn().Int64("contact_id", contact.Id).Msg("LSDeleteThenInsertContact")
	}
	for _, contact := range tbl.LSVerifyContactRowExists {
		log.Warn().Int64("contact_id", contact.ContactId).Msg("LSVerifyContactRowExists")
		ghost, err := m.Main.Bridge.GetGhostByID(ctx, ids.MakeUserID(contact.ContactId))
		if err != nil {
			log.Err(err).Int64("contact_id", contact.ContactId).Msg("Failed to get ghost")
			continue
		}
		ghost.UpdateInfo(ctx, &bridgev2.UserInfo{
			Name: &contact.Name,
		})
	}
	for _, thread := range tbl.LSDeleteThenInsertThread {
		log.Warn().Int64("thread_id", thread.ThreadKey).Msg("LSDeleteThenInsertThread")

		members := &bridgev2.ChatMemberList{
			Members: []bridgev2.ChatMember{
				{
					EventSender: bridgev2.EventSender{
						IsFromMe:    true,
						Sender:      networkid.UserID(m.login.ID),
						SenderLogin: m.login.ID,
					},
					Membership: event.MembershipJoin,
				},
			},
		}

		roomType := database.RoomTypeDefault
		if thread.ThreadType == table.ONE_TO_ONE {
			roomType = database.RoomTypeDM
			members.Members = append(members.Members, bridgev2.ChatMember{
				EventSender: m.senderFromID(thread.ThreadKey), // For One-to-One threads, the other participant is the thread key
				Membership:  event.MembershipJoin,
			})
		} else if thread.ThreadType == table.GROUP_THREAD {
			roomType = database.RoomTypeGroupDM
		}

		m.Main.Bridge.QueueRemoteEvent(m.login, &simplevent.ChatResync{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatResync,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Int64("thread_id", thread.ThreadKey)
				},
				PortalKey: networkid.PortalKey{
					ID: ids.MakePortalID(thread.ThreadKey),
				},
				CreatePortal: true,
			},

			ChatInfo: &bridgev2.ChatInfo{
				Name:    &thread.ThreadName,
				Topic:   &thread.ThreadDescription,
				Members: members,
				Type:    &roomType,
			},
		})
	}
	for _, participant := range tbl.LSAddParticipantIdToGroupThread {
		log.Warn().Int64("thread_id", participant.ThreadKey).Int64("contact_id", participant.ContactId).Msg("LSAddParticipantIdToGroupThread")

		m.Main.Bridge.QueueRemoteEvent(m.login, &simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatInfoChange,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Int64("thread_id", participant.ThreadKey).Int64("contact_id", participant.ContactId)

				},
				PortalKey: networkid.PortalKey{
					ID: ids.MakePortalID(participant.ThreadKey),
				},
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				MemberChanges: &bridgev2.ChatMemberList{
					Members: []bridgev2.ChatMember{
						{
							EventSender: m.senderFromID(participant.ContactId),
							Nickname:    participant.Nickname,
							Membership:  event.MembershipJoin,
						},
					},
				},
			},
		})
	}
	for _, participant := range tbl.LSRemoveParticipantFromThread {
		log.Warn().Int64("thread_id", participant.ThreadKey).Int64("contact_id", participant.ParticipantId).Msg("LSRemoveParticipantFromThread")

		m.Main.Bridge.QueueRemoteEvent(m.login, &simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatInfoChange,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Int64("thread_id", participant.ThreadKey).Int64("contact_id", participant.ParticipantId)

				},
				PortalKey: networkid.PortalKey{
					ID: ids.MakePortalID(participant.ThreadKey),
				},
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				MemberChanges: &bridgev2.ChatMemberList{
					Members: []bridgev2.ChatMember{
						{
							EventSender: m.senderFromID(participant.ParticipantId),
							Membership:  event.MembershipLeave,
						},
					},
				},
			},
		})
	}
	for _, thread := range tbl.LSVerifyThreadExists {
		log.Warn().Int64("thread_id", thread.ThreadKey).Msg("LSVerifyThreadExists")

		members := &bridgev2.ChatMemberList{
			Members: []bridgev2.ChatMember{
				{
					EventSender: bridgev2.EventSender{
						IsFromMe:    true,
						Sender:      networkid.UserID(m.login.ID),
						SenderLogin: m.login.ID,
					},
					Membership: event.MembershipJoin,
				},
			},
		}

		roomType := database.RoomTypeDefault
		if thread.ThreadType == table.ONE_TO_ONE {
			roomType = database.RoomTypeDM
			members.Members = append(members.Members, bridgev2.ChatMember{
				EventSender: m.senderFromID(thread.ThreadKey), // For One-to-One threads, the other participant is the thread key
				Membership:  event.MembershipJoin,
			})
		} else if thread.ThreadType == table.GROUP_THREAD {
			roomType = database.RoomTypeGroupDM
		}

		m.Main.Bridge.QueueRemoteEvent(m.login, &simplevent.ChatResync{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatResync,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Int64("thread_id", thread.ThreadKey)
				},
				PortalKey: networkid.PortalKey{
					ID: ids.MakePortalID(thread.ThreadKey),
				},
				CreatePortal: true,
			},

			GetChatInfoFunc: func(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
				// If the room isn't created yet, we're probably missing some info... this will create a stub room and ask Meta to send us the rest
				if portal.MXID == "" {
					resp, err := m.client.ExecuteTasks(
						&socket.CreateThreadTask{
							ThreadFBID:                thread.ThreadKey,
							ForceUpsert:               0,
							UseOpenMessengerTransport: 0,
							SyncGroup:                 1,
							MetadataOnly:              0,
							PreviewOnly:               0,
						},
					)
					if err != nil {
						log.Err(err).Msg("Failed to request more thread info")
					}
					log.Debug().Any("response", resp).Msg("Requested more thread info")
				}
				return &bridgev2.ChatInfo{
					Type:    &roomType,
					Members: members,
				}, nil
			},
		})
	}
	for _, mute := range tbl.LSUpdateThreadMuteSetting {
		log.Warn().Int64("thread_id", mute.ThreadKey).Msg("LSUpdateThreadMuteSetting")
	}
	for _, thread := range tbl.LSSyncUpdateThreadName {
		log.Warn().Int64("thread_id", thread.ThreadKey).Msg("LSUpdateThreadName")

		m.Main.Bridge.QueueRemoteEvent(m.login, &simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatInfoChange,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Int64("thread_id", thread.ThreadKey)

				},
				PortalKey: networkid.PortalKey{
					ID: ids.MakePortalID(thread.ThreadKey),
				},
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				ChatInfo: &bridgev2.ChatInfo{
					Name: &thread.ThreadName,
				},
			},
		})
	}

	upsert, insert := tbl.WrapMessages()
	for _, upsert := range upsert {
		log.Trace().Int64("thread_id", upsert.Range.ThreadKey).Msg("UpsertMessages")
	}
	for _, msg := range insert {
		log.Trace().Int64("thread_id", msg.ThreadKey).Str("message_id", msg.MessageId).Msg("InsertMessage")
		m.insertMessage(ctx, msg)
	}

	for _, reaction := range tbl.LSUpsertReaction {
		log.Warn().Str("message_id", reaction.MessageId).Msg("LSUpsertReaction")

		evt := &bridgev2.SimpleRemoteEvent[any]{
			Type: bridgev2.RemoteEventReaction,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.
					Any("reaction", reaction.Reaction).
					Str("message_id", string(reaction.MessageId))
			},
			Sender:        m.senderFromID(reaction.ActorId),
			PortalKey:     networkid.PortalKey{ID: networkid.PortalID(strconv.Itoa(int(reaction.ThreadKey)))},
			TargetMessage: networkid.MessageID(reaction.MessageId),
			EmojiID:       networkid.EmojiID(""),
			Emoji:         reaction.Reaction,
		}
		m.Main.Bridge.QueueRemoteEvent(m.login, evt)
	}

	for _, reaction := range tbl.LSDeleteReaction {
		log.Warn().Str("message_id", reaction.MessageId).Msg("LSDeleteReaction")

		evt := &bridgev2.SimpleRemoteEvent[any]{
			Type: bridgev2.RemoteEventReactionRemove,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.
					Str("message_id", string(reaction.MessageId))
			},
			Sender:        m.senderFromID(reaction.ActorId),
			PortalKey:     networkid.PortalKey{ID: networkid.PortalID(strconv.Itoa(int(reaction.ThreadKey)))},
			TargetMessage: networkid.MessageID(reaction.MessageId),
			EmojiID:       networkid.EmojiID(""),
		}
		m.Main.Bridge.QueueRemoteEvent(m.login, evt)
	}

	for _, edit := range tbl.LSEditMessage {
		// Get the existing message by ID
		editId := networkid.MessageID(edit.MessageID)
		originalMsg, err := m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, m.login.ID, editId)
		if err != nil {
			log.Err(err).Str("message_id", string(editId)).Msg("Failed to get original message")
			continue
		}

		m.Main.Bridge.QueueRemoteEvent(m.login, &simplevent.Message[*table.LSEditMessage]{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventEdit,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.
						Str("message_id", edit.MessageID)
				},
				PortalKey: originalMsg.Room,
			},
			Data:          edit,
			ID:            editId,
			TargetMessage: editId,
			ConvertEditFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message, data *table.LSEditMessage) (*bridgev2.ConvertedEdit, error) {
				textPart := existing[0] // TODO: Figure out a better way to get the text part, esp. if there are attachments etc.

				return &bridgev2.ConvertedEdit{
					ModifiedParts: []*bridgev2.ConvertedEditPart{
						{
							Part: textPart,
							Type: event.EventMessage,
							Content: m.messageConverter.MetaToMatrixText(ctx, data.Text, nil, portal),
						},
					},
				}, nil
			},
		},
		)
	}
}

func (m *MetaClient) insertMessage(ctx context.Context, msg *table.WrappedMessage) {
	log := zerolog.Ctx(ctx)

	log.Warn().Str("sender_id", strconv.Itoa(int(msg.SenderId))).Str("login_id", string(m.login.ID)).Msg("Inserting message")

	sender := m.senderFromID(msg.SenderId)

	log.Warn().Any("sender", sender).Msg("Sender")

	m.Main.Bridge.QueueRemoteEvent(m.login, &bridgev2.SimpleRemoteEvent[*table.WrappedMessage]{
		Type: bridgev2.RemoteEventMessage,
		LogContext: func(c zerolog.Context) zerolog.Context {
			return c.
				Str("message_id", msg.MessageId).
				Any("sender", sender)
		},
		ID:     networkid.MessageID(msg.MessageId),
		Sender: sender,
		PortalKey: networkid.PortalKey{
			ID: networkid.PortalID(strconv.Itoa(int(msg.ThreadKey))),
		},
		Data:         msg,
		CreatePortal: true,
		ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, msg *table.WrappedMessage) (*bridgev2.ConvertedMessage, error) {
			return m.messageConverter.ToMatrix(ctx, msg, portal), nil
		},
	})
}

func (m *MetaClient) Connect(ctx context.Context) error {
	client := messagix.NewClient(m.cookies, m.log.With().Str("component", "messagix").Logger())
	m.client = client

	_, initialTable, err := m.client.LoadMessagesPage()
	if err != nil {
		return fmt.Errorf("failed to load messages page: %w", err)
	}

	m.handleTable(ctx, initialTable)

	m.client.SetEventHandler(m.metaEventHandler)

	err = m.client.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to messagix: %w", err)
	}

	err = m.Update(ctx)
	if err != nil {
		return err
	}

	go m.handleMetaEventLoop()

	return nil
}

func (m *MetaClient) Disconnect() {
	m.incomingEvents <- nil
	close(m.incomingEvents)
	if m.client != nil {
		m.client.Disconnect()
	}
	m.client = nil
}

var metaCaps = &bridgev2.NetworkRoomCapabilities{
	FormattedText: true,
	UserMentions:  true,
	Replies:       true,
	Edits:         true,
	EditMaxCount:  10,
	EditMaxAge:    24 * time.Hour,
	Reactions:     true,
	ReactionCount: 1,
}

// GetCapabilities implements bridgev2.NetworkAPI.
func (m *MetaClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *bridgev2.NetworkRoomCapabilities {
	return metaCaps
}

func (m *MetaClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	panic("GetChatInfo should never be called")
}

func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	// This should never be called because ghost info is pre-populated when parsing the table
	panic("GetUserInfo should never be called")
}

// HandleMatrixMessage implements bridgev2.NetworkAPI.
func (m *MetaClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	log := zerolog.Ctx(ctx)

	if msg.Content.MsgType == event.MsgNotice /*&& !portal.bridge.Config.Bridge.BridgeNotices*/ {
		log.Warn().Msg("Ignoring notice message")
		return nil, nil
	}

	log.Trace().Any("event", msg.Event).Msg("Handling Matrix message")

	tasks, otid, err := m.messageConverter.ToMeta(ctx, msg.Event, msg.Content, false, ids.ParsePortalID(msg.Portal.ID), msg.Portal)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message: %w", err)
	}

	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int64("otid", otid)
	})
	log.Debug().Msg("Sending Matrix message to Meta")

	otidStr := strconv.FormatInt(otid, 10)

	var resp *table.LSTable

	retries := 0
	for retries < 5 {
		if err = m.client.WaitUntilCanSendMessages(15 * time.Second); err != nil {
			log.Err(err).Msg("Error waiting to be able to send messages, retrying")
		} else {
			resp, err = m.client.ExecuteTasks(tasks...)
			if err == nil {
				break
			}
			log.Err(err).Msg("Failed to send message to Meta, retrying")
		}
		retries++
	}

	log.Trace().Any("response", resp).Msg("Meta send response")
	var msgID string
	if resp != nil && err == nil {
		for _, replace := range resp.LSReplaceOptimsiticMessage {
			if replace.OfflineThreadingId == otidStr {
				msgID = replace.MessageId
			}
		}
		if len(msgID) == 0 {
			for _, failed := range resp.LSMarkOptimisticMessageFailed {
				if failed.OTID == otidStr {
					log.Warn().Str("message", failed.Message).Msg("Sending message failed")
					//go ms.sendMessageMetrics(evt, fmt.Errorf("%w: %s", errServerRejected, failed.Message), "Error sending", true)
					return nil, fmt.Errorf("sending message failed: %s", failed.Message)
				}
			}
			for _, failed := range resp.LSHandleFailedTask {
				if failed.OTID == otidStr {
					log.Warn().Str("message", failed.Message).Msg("Sending message failed")
					//go ms.sendMessageMetrics(evt, fmt.Errorf("%w: %s", errServerRejected, failed.Message), "Error sending", true)
					return nil, fmt.Errorf("sending message failed: %s", failed.Message)
				}
			}
			log.Warn().Msg("Message send response didn't include message ID")
		}
	}

	if m.login.User.MXID != msg.Event.Sender {
		return nil, fmt.Errorf("sender mismatch with user login: %s", msg.Event.Sender)
	}

	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        networkid.MessageID(msgID),
			MXID:      msg.Event.ID,
			Room:      networkid.PortalKey{ID: msg.Portal.ID},
			SenderID:  networkid.UserID(m.login.ID),
			Timestamp: time.Time{},
		},
	}, nil
}

func (m *MetaClient) HandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (*database.Reaction, error) {
	log := zerolog.Ctx(ctx)

	log.Debug().Any("reaction", msg).Msg("Handling Matrix reaction")

	resp, err := m.client.ExecuteTasks(&socket.SendReactionTask{
		ThreadKey:       ids.ParsePortalID(msg.Portal.ID),
		TimestampMs:     msg.Event.Timestamp,
		MessageID:       string(msg.TargetMessage.ID),
		Reaction:        msg.PreHandleResp.Emoji,
		ActorID:         ids.ParseUserID(msg.PreHandleResp.SenderID),
		SyncGroup:       1,
		SendAttribution: table.MESSENGER_INBOX_IN_THREAD,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send reaction to Meta: %w", err)
	}

	log.Trace().Any("response", resp).Msg("Meta reaction response")

	return &database.Reaction{}, nil
}

func (m *MetaClient) HandleMatrixReactionRemove(ctx context.Context, msg *bridgev2.MatrixReactionRemove) error {
	log := zerolog.Ctx(ctx)

	log.Debug().Any("reaction", msg).Msg("Removing Matrix reaction")

	resp, err := m.client.ExecuteTasks(&socket.SendReactionTask{
		ThreadKey:       ids.ParsePortalID(msg.Portal.ID),
		TimestampMs:     msg.Event.Timestamp,
		MessageID:       string(msg.TargetReaction.MessageID),
		Reaction:        "",
		ActorID:         ids.ParseUserID(msg.TargetReaction.SenderID),
		SyncGroup:       1,
		SendAttribution: table.MESSENGER_INBOX_IN_THREAD,
	})
	if err != nil {
		return fmt.Errorf("failed to send reaction to Meta: %w", err)
	}

	log.Trace().Any("response", resp).Msg("Meta reaction remove response")

	return nil
}

func (m *MetaClient) PreHandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (bridgev2.MatrixReactionPreResponse, error) {
	return bridgev2.MatrixReactionPreResponse{
		SenderID:     networkid.UserID(m.login.ID),
		EmojiID:      networkid.EmojiID(""),
		Emoji:        variationselector.Remove(msg.Content.RelatesTo.Key),
		MaxReactions: 1,
	}, nil
}

func (m *MetaClient) HandleMatrixEdit(ctx context.Context, edit *bridgev2.MatrixEdit) error {
	log := zerolog.Ctx(ctx)

	log.Debug().Any("edit", edit).Msg("Handling Matrix edit")

	// TODO: The conversion stuff wants a SendMessageTask, and I don't feel like rewriting it yet
	fakeSendTasks, _, err := m.messageConverter.ToMeta(ctx, edit.Event, edit.Content, false, ids.ParsePortalID(edit.Portal.ID), edit.Portal)
	if err != nil {
		return fmt.Errorf("failed to convert message: %w", err)
	}

	fakeTask := fakeSendTasks[0].(*socket.SendMessageTask)

	editTask := &socket.EditMessageTask{
		MessageID: string(edit.EditTarget.ID),
		Text:      fakeTask.Text,
	}

	newEditCount := int64(edit.EditTarget.EditCount) + 1

	var resp *table.LSTable
	resp, err = m.client.ExecuteTasks(editTask)
	log.Trace().Any("response", resp).Msg("Meta edit response")
	if err != nil {
		return fmt.Errorf("failed to send edit to Meta: %w", err)
	}

	if len(resp.LSEditMessage) == 0 {
		log.Debug().Msg("Edit response didn't contain new edit?")
	} else if resp.LSEditMessage[0].MessageID != editTask.MessageID {
		log.Debug().Msg("Edit response contained different message ID")
	} else if resp.LSEditMessage[0].Text != editTask.Text {
		log.Warn().Msg("Server returned edit with different text")
		return fmt.Errorf("edit reverted")
	} else if resp.LSEditMessage[0].EditCount != newEditCount {
		log.Warn().
			Int64("expected_edit_count", newEditCount).
			Int64("actual_edit_count", resp.LSEditMessage[0].EditCount).
			Msg("Edit count mismatch")
	}

	return nil
}

// IsLoggedIn implements bridgev2.NetworkAPI.
func (m *MetaClient) IsLoggedIn() bool {
	//panic("unimplemented")
	return true
}

// IsThisUser implements bridgev2.NetworkAPI.
func (m *MetaClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	panic("unimplemented")
}

// LogoutRemote implements bridgev2.NetworkAPI.
func (m *MetaClient) LogoutRemote(ctx context.Context) {
	panic("unimplemented")
}

func (m *MetaClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	log := zerolog.Ctx(ctx)
	log.Debug().Str("identifier", identifier).Bool("create_chat", createChat).Msg("Resolving identifier")

	// Make sure we can parse identifier as an int
	id, err := ids.ParseIDFromString(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identifier: %w", err)
	}

	var chat *bridgev2.CreateChatResponse
	if createChat {
		// Create the chat on the Meta side, not sure if this is necessary for DMs?
		resp, err := m.client.ExecuteTasks(
			&socket.CreateThreadTask{
				ThreadFBID:                id,
				ForceUpsert:               0,
				UseOpenMessengerTransport: 0,
				SyncGroup:                 1,
				MetadataOnly:              0,
				PreviewOnly:               0,
			},
		)

		log.Debug().Any("response_data", resp).Err(err).Msg("Create chat response")

		portalKey := networkid.PortalKey{ID: ids.MakePortalID(id)}

		roomtype := database.RoomTypeDM

		chat = &bridgev2.CreateChatResponse{
			PortalKey: portalKey,
			PortalInfo: &bridgev2.ChatInfo{
				Type: &roomtype,
				Members: &bridgev2.ChatMemberList{
					Members: []bridgev2.ChatMember{
						{
							EventSender: bridgev2.EventSender{
								IsFromMe:    true,
								Sender:      networkid.UserID(m.login.ID),
								SenderLogin: m.login.ID,
							},
							Membership: event.MembershipJoin,
						},
						{
							EventSender: m.senderFromID(id),
							Membership:  event.MembershipJoin,
						},
					},
				},
			},
		}
	}
	return &bridgev2.ResolveIdentifierResponse{
		UserID:   ids.MakeUserID(id),
		UserInfo: &bridgev2.UserInfo{},
		Chat:     chat,
	}, nil
}

func (m *MetaClient) SearchUsers(ctx context.Context, search string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	log := zerolog.Ctx(ctx)
	log.Debug().Str("search", search).Msg("Searching users")

	task := &socket.SearchUserTask{
		Query: search,
		SupportedTypes: []table.SearchType{
			table.SearchTypeContact, table.SearchTypeGroup, table.SearchTypePage, table.SearchTypeNonContact,
			table.SearchTypeIGContactFollowing, table.SearchTypeIGContactNonFollowing,
			table.SearchTypeIGNonContactFollowing, table.SearchTypeIGNonContactNonFollowing,
		},
		SurfaceType: 15,
		Secondary:   false,
	}
	if m.cookies.Platform.IsMessenger() {
		task.SurfaceType = 5
		task.SupportedTypes = append(task.SupportedTypes, table.SearchTypeCommunityMessagingThread)
	}
	taskCopy := *task
	taskCopy.Secondary = true
	secondaryTask := &taskCopy

	go func() {
		time.Sleep(10 * time.Millisecond)
		resp, err := m.client.ExecuteTasks(secondaryTask)
		log.Trace().Any("response_data", resp).Err(err).Msg("Resolve identifier secondary response")
		// The secondary response doesn't seem to have anything important, so just ignore it
	}()

	resp, err := m.client.ExecuteTasks(task)
	log.Trace().Any("response_data", resp).Err(err).Msg("Resolve identifier primary response")
	if err != nil {
		return nil, fmt.Errorf("failed to search for user: %w", err)
	}

	users := make([]*bridgev2.ResolveIdentifierResponse, 0)

	for _, result := range resp.LSInsertSearchResult {
		if result.ThreadType == table.ONE_TO_ONE && result.CanViewerMessage && result.GetFBID() != 0 {
			users = append(users, &bridgev2.ResolveIdentifierResponse{
				UserID: ids.MakeUserID(result.GetFBID()),
				UserInfo: &bridgev2.UserInfo{
					Name: &result.DisplayName,
				},
			})
		}
	}

	return users, nil
}

var (
	_ bridgev2.NetworkAPI                 = (*MetaClient)(nil)
	_ bridgev2.UserSearchingNetworkAPI    = (*MetaClient)(nil)
	_ bridgev2.EditHandlingNetworkAPI     = (*MetaClient)(nil)
	_ bridgev2.ReactionHandlingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.RedactionHandlingNetworkAPI   = (*MetaClient)(nil)
	// _ bridgev2.ReadReceiptHandlingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.ReadReceiptHandlingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.TypingHandlingNetworkAPI      = (*MetaClient)(nil)
	_ bridgev2.IdentifierResolvingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.GroupCreatingNetworkAPI       = (*MetaClient)(nil)
	// _ bridgev2.ContactListingNetworkAPI      = (*MetaClient)(nil)
)
