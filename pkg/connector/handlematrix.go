package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"go.mau.fi/util/variationselector"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	waTypes "go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.EditHandlingNetworkAPI        = (*MetaClient)(nil)
	_ bridgev2.ReactionHandlingNetworkAPI    = (*MetaClient)(nil)
	_ bridgev2.RedactionHandlingNetworkAPI   = (*MetaClient)(nil)
	_ bridgev2.ReadReceiptHandlingNetworkAPI = (*MetaClient)(nil)
	_ bridgev2.ChatViewingNetworkAPI         = (*MetaClient)(nil)
	_ bridgev2.TypingHandlingNetworkAPI      = (*MetaClient)(nil)
	_ bridgev2.DeleteChatHandlingNetworkAPI  = (*MetaClient)(nil)
	_ bridgev2.RoomNameHandlingNetworkAPI    = (*MetaClient)(nil)
	_ bridgev2.RoomAvatarHandlingNetworkAPI  = (*MetaClient)(nil)
)

var _ bridgev2.TransactionIDGeneratingNetwork = (*MetaConnector)(nil)

func (m *MetaConnector) GenerateTransactionID(userID id.UserID, roomID id.RoomID, eventType event.Type) networkid.RawTransactionID {
	return networkid.RawTransactionID(strconv.FormatInt(methods.GenerateEpochID(), 10))
}

var (
	ErrServerRejectedMessage = bridgev2.WrapErrorInStatus(errors.New("server rejected message")).WithErrorAsMessage().WithSendNotice(true)
	ErrNotConnected          = bridgev2.WrapErrorInStatus(errors.New("not connected")).WithErrorAsMessage().WithSendNotice(true)
)

const ConnectWaitTimeout = 1 * time.Minute

func getOTID(inputTxnID networkid.RawTransactionID) int64 {
	if inputTxnID != "" {
		otid, err := strconv.ParseInt(string(inputTxnID), 10, 64)
		if err == nil && otid > 0 {
			return otid
		}
	}
	return methods.GenerateEpochID()
}

func (m *MetaClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if m.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	log := zerolog.Ctx(ctx)

	portalMeta := msg.Portal.Metadata.(*metaid.PortalMetadata)
	otid := getOTID(msg.InputTransactionID)

	switch portalMeta.ThreadType {
	case table.ENCRYPTED_OVER_WA_ONE_TO_ONE, table.ENCRYPTED_OVER_WA_GROUP:
		if !m.e2eeConnectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return nil, ErrNotConnected
		}

		waMsg, waMeta, err := m.Main.MsgConv.ToWhatsApp(ctx, msg.Event, msg.Content, msg.Portal, m.E2EEClient, msg.OrigSender != nil, msg.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		messageID := strconv.FormatInt(otid, 10)
		chatJID := portalMeta.JID(msg.Portal.ID)
		senderJID := m.WADevice.ID
		resp, err := m.E2EEClient.SendFBMessage(ctx, chatJID, waMsg, waMeta, whatsmeow.SendRequestExtra{
			ID: messageID,
		})
		if err != nil {
			return nil, err
		}
		return &bridgev2.MatrixMessageResponse{
			DB: &database.Message{
				ID:        metaid.MakeWAMessageID(chatJID, senderJID.ToNonAD(), messageID),
				SenderID:  networkid.UserID(m.UserLogin.ID),
				Timestamp: resp.Timestamp,
			},
			// Note: WhatsApp timestamps are seconds, but we use unix millis in order to match FB stream orders.
			StreamOrder: resp.Timestamp.UnixMilli(),
		}, nil
	default:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return nil, ErrNotConnected
		}

		tasks, err := m.Main.MsgConv.ToMeta(
			ctx, m.Client, msg.Event, msg.Content, msg.ReplyTo, msg.ThreadRoot, otid, msg.OrigSender != nil, msg.Portal,
		)
		if errors.Is(err, types.ErrPleaseReloadPage) && m.canReconnect() {
			log.Debug().Err(err).Msg("Doing full reconnect and retrying upload")
			m.FullReconnect()
			tasks, err = m.Main.MsgConv.ToMeta(
				ctx, m.Client, msg.Event, msg.Content, msg.ReplyTo, msg.ThreadRoot, otid, msg.OrigSender != nil, msg.Portal,
			)
		}
		if errors.Is(err, messagix.ErrTokenInvalidated) {
			if m.canReconnect() {
				go m.FullReconnect()
			}
			return nil, err
		} else if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}

		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Int64("otid", otid)
		})
		log.Debug().Msg("Sending Matrix message to Meta")

		otidStr := strconv.FormatInt(otid, 10)

		var resp *table.LSTable

		for retries := 0; retries < 5; retries++ {
			if err = m.Client.WaitUntilCanSendMessages(ctx, 15*time.Second); err != nil {
				log.Err(err).Msg("Error waiting to be able to send messages, retrying")
			} else {
				resp, err = m.Client.ExecuteTasks(ctx, tasks...)
				if err == nil {
					break
				}
				log.Err(err).Msg("Failed to send message to Meta, retrying")
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		if err != nil {
			return nil, err
		}

		log.Trace().Any("response", resp).Msg("Meta send response")
		var msgID string
		if resp != nil {
			for _, replace := range resp.LSReplaceOptimsiticMessage {
				if replace.OfflineThreadingId == otidStr {
					msgID = replace.MessageId
				}
			}
			if len(msgID) == 0 {
				for _, failed := range resp.LSMarkOptimisticMessageFailed {
					if failed.OTID == otidStr {
						log.Warn().Str("message", failed.Message).Msg("Sending message failed (optimistic)")
						return nil, fmt.Errorf("%w: %s", ErrServerRejectedMessage, failed.Message)
					}
				}
				for _, failed := range resp.LSHandleFailedTask {
					if failed.OTID == otidStr {
						log.Warn().Str("message", failed.Message).Msg("Sending message failed (task)")
						return nil, fmt.Errorf("%w: %s", ErrServerRejectedMessage, failed.Message)
					}
				}
				log.Warn().Msg("Message send response didn't include message ID")
			}
		}
		var parsedTSTime time.Time
		parsedTS, err := methods.ParseMessageID(msgID)
		if err != nil {
			log.Warn().Err(err).Str("message_id", msgID).Msg("Failed to parse message ID")
		} else {
			parsedTSTime = time.UnixMilli(parsedTS)
			if parsedTSTime.Before(time.Now().Add(-1 * time.Hour)) {
				log.Warn().
					Time("parsed_ts", parsedTSTime).
					Str("message_id", msgID).
					Msg("Message ID timestamp is too far in the past")
				parsedTSTime = time.Time{}
				parsedTS = 0
			} else if parsedTSTime.After(time.Now().Add(1 * time.Hour)) {
				log.Warn().
					Time("parsed_ts", parsedTSTime).
					Str("message_id", msgID).
					Msg("Message ID timestamp is too far in the future")
				parsedTSTime = time.Time{}
				parsedTS = 0
			}
		}

		return &bridgev2.MatrixMessageResponse{
			DB: &database.Message{
				ID:        metaid.MakeFBMessageID(msgID),
				SenderID:  networkid.UserID(m.UserLogin.ID),
				Timestamp: parsedTSTime,
			},
			StreamOrder: parsedTS,
		}, nil
	}
}

func (m *MetaClient) PreHandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (bridgev2.MatrixReactionPreResponse, error) {
	return bridgev2.MatrixReactionPreResponse{
		SenderID:     networkid.UserID(m.UserLogin.ID),
		EmojiID:      "",
		Emoji:        variationselector.Remove(msg.Content.RelatesTo.Key),
		MaxReactions: 1,
	}, nil
}

func wrapReaction(message *waConsumerApplication.ConsumerApplication_ReactionMessage) *waConsumerApplication.ConsumerApplication {
	return &waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
				Content: &waConsumerApplication.ConsumerApplication_Content{
					Content: &waConsumerApplication.ConsumerApplication_Content_ReactionMessage{
						ReactionMessage: message,
					},
				},
			},
		},
	}
}

func wrapEdit(message *waConsumerApplication.ConsumerApplication_EditMessage) *waConsumerApplication.ConsumerApplication {
	return &waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
				Content: &waConsumerApplication.ConsumerApplication_Content{
					Content: &waConsumerApplication.ConsumerApplication_Content_EditMessage{
						EditMessage: message,
					},
				},
			},
		},
	}
}

func wrapRevoke(message *waConsumerApplication.ConsumerApplication_RevokeMessage) *waConsumerApplication.ConsumerApplication {
	return &waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_ApplicationData{
				ApplicationData: &waConsumerApplication.ConsumerApplication_ApplicationData{
					ApplicationContent: &waConsumerApplication.ConsumerApplication_ApplicationData_Revoke{
						Revoke: message,
					},
				},
			},
		},
	}
}

func (m *MetaClient) HandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (*database.Reaction, error) {
	if m.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	switch messageID := metaid.ParseMessageID(msg.TargetMessage.ID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return nil, ErrNotConnected
		}
		resp, err := m.Client.ExecuteTasks(ctx, &socket.SendReactionTask{
			ThreadKey:       metaid.ParseFBPortalID(msg.Portal.ID),
			TimestampMs:     msg.Event.Timestamp,
			MessageID:       messageID.ID,
			Reaction:        msg.PreHandleResp.Emoji,
			ActorID:         metaid.ParseUserID(msg.PreHandleResp.SenderID),
			SyncGroup:       1,
			SendAttribution: table.MESSENGER_INBOX_IN_THREAD,
		})
		if err != nil {
			return nil, err
		}
		// TODO fail if response doesn't contain LSReplaceOptimisticReaction?
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("Meta reaction response")
		return &database.Reaction{}, nil
	case metaid.ParsedWAMessageID:
		if !m.e2eeConnectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return nil, ErrNotConnected
		}
		consumerMsg := wrapReaction(&waConsumerApplication.ConsumerApplication_ReactionMessage{
			Key:               m.messageIDToWAKey(messageID),
			Text:              ptr.Ptr(msg.PreHandleResp.Emoji),
			SenderTimestampMS: ptr.Ptr(msg.Event.Timestamp),
		})
		portalJID := msg.Portal.Metadata.(*metaid.PortalMetadata).JID(msg.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil, whatsmeow.SendRequestExtra{
			ID: waTypes.MessageID(msg.InputTransactionID),
		})
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("WhatsApp reaction response")
		return nil, err
	default:
		return nil, fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixReactionRemove(ctx context.Context, msg *bridgev2.MatrixReactionRemove) error {
	if m.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	switch messageID := metaid.ParseMessageID(msg.TargetReaction.MessageID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		resp, err := m.Client.ExecuteTasks(ctx, &socket.SendReactionTask{
			ThreadKey:       metaid.ParseFBPortalID(msg.Portal.ID),
			TimestampMs:     msg.Event.Timestamp,
			MessageID:       messageID.ID,
			Reaction:        "",
			ActorID:         metaid.ParseUserID(msg.TargetReaction.SenderID),
			SyncGroup:       1,
			SendAttribution: table.MESSENGER_INBOX_IN_THREAD,
		})
		if err != nil {
			return err
		}
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("Meta reaction remove response")
		return nil
	case metaid.ParsedWAMessageID:
		if !m.e2eeConnectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		consumerMsg := wrapReaction(&waConsumerApplication.ConsumerApplication_ReactionMessage{
			Key:               m.messageIDToWAKey(messageID),
			Text:              ptr.Ptr(""),
			SenderTimestampMS: ptr.Ptr(msg.Event.Timestamp),
		})
		portalJID := msg.Portal.Metadata.(*metaid.PortalMetadata).JID(msg.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil, whatsmeow.SendRequestExtra{
			ID: waTypes.MessageID(msg.InputTransactionID),
		})
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("WhatsApp reaction response")
		return err
	default:
		return fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixEdit(ctx context.Context, edit *bridgev2.MatrixEdit) error {
	if m.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	log := zerolog.Ctx(ctx)
	otid := getOTID(edit.InputTransactionID)
	switch messageID := metaid.ParseMessageID(edit.EditTarget.ID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		fakeSendTasks, err := m.Main.MsgConv.ToMeta(
			ctx, m.Client, edit.Event, edit.Content, nil, nil, otid, false, edit.Portal,
		)
		if err != nil {
			return fmt.Errorf("failed to convert message: %w", err)
		}

		fakeTask := fakeSendTasks[0].(*socket.SendMessageTask)

		editTask := &socket.EditMessageTask{
			MessageID: messageID.ID,
			Text:      fakeTask.Text,
		}

		newEditCount := int64(edit.EditTarget.EditCount) + 1

		// This channel will receive any extra edit responses we receive that AREN'T listed
		// as responses for the edit request we are about to submit. Sometimes you submit a
		// request ID for an edit and you get a response for that request ID indicating your
		// edit was reverted, but it actually wasn't, and you get a subsequent message, not
		// associated with your request ID, telling you that the edit was successful. So in
		// case the normal response is bad, we have to wait a bit to see if there is a
		// follow-up response that corrects it. AFAICT, there is no way to look at the
		// initial response and tell if it is real or fake. ^_^
		cursedExtraEdits := make(chan *FBEditEvent, 5)
		m.editChannels.Set(editTask.MessageID, cursedExtraEdits)
		defer m.editChannels.Delete(editTask.MessageID)

		var resp *table.LSTable
		resp, err = m.Client.ExecuteTasks(ctx, editTask)
		log.Trace().Any("response", resp).Msg("Meta edit response")
		if err != nil {
			return fmt.Errorf("failed to send edit to Meta: %w", err)
		}

		if len(resp.LSEditMessage) == 0 {
			log.Debug().Msg("Edit response didn't contain new edit?")
			return nil
		}
		editMsg := resp.LSEditMessage[0]

		if editMsg.MessageID != editTask.MessageID {
			log.Debug().Msg("Edit response contained different message ID")
			return nil
		}

		if editMsg.Text != editTask.Text {
			log.Warn().Msg("Server returned edit with different text, waiting to see if this is corrected")
			// Wait at most 5 seconds for a success acknowledgment to show up. It
			// usually shows up after around 100ms, but in case of network delays, it
			// could be longer. Abort immediately in case we do receive a success
			// acknowledgment, so that the delay only occurs in the rare case that an
			// edit actually is rejected.
			after := time.After(5 * time.Second)
		loop:
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-after:
					log.Warn().Msg("Timed out waiting for edit correction, edit was actually rejected")
					return fmt.Errorf("edit reverted")
				case newEditMsg := <-cursedExtraEdits:
					if newEditMsg.Text == editTask.Text {
						log.Info().Msg("Server accepted edit after previously rejecting it")
						editMsg = newEditMsg.LSEditMessage
						break loop
					} else {
						log.Warn().Msg("Server returned another edit with different text, continuing to wait")
					}
				}
			}
		}

		if editMsg.EditCount != newEditCount {
			log.Warn().
				Int64("expected_edit_count", newEditCount).
				Int64("actual_edit_count", resp.LSEditMessage[0].EditCount).
				Msg("Edit count mismatch")
		}
		edit.EditTarget.EditCount = int(editMsg.EditCount)

		return nil
	case metaid.ParsedWAMessageID:
		if !m.e2eeConnectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		consumerMsg := wrapEdit(&waConsumerApplication.ConsumerApplication_EditMessage{
			Key:         m.messageIDToWAKey(messageID),
			Message:     m.Main.MsgConv.TextToWhatsApp(edit.Content),
			TimestampMS: ptr.Ptr(edit.Event.Timestamp),
		})
		edit.EditTarget.Metadata.(*metaid.MessageMetadata).EditTimestamp = edit.Event.Timestamp
		portalJID := edit.Portal.Metadata.(*metaid.PortalMetadata).JID(edit.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil, whatsmeow.SendRequestExtra{
			ID: strconv.FormatInt(otid, 10),
		})
		log.Trace().Any("response", resp).Msg("WhatsApp edit response")
		return err
	default:
		return fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixMessageRemove(ctx context.Context, msg *bridgev2.MatrixMessageRemove) error {
	if m.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	log := zerolog.Ctx(ctx)
	switch messageID := metaid.ParseMessageID(msg.TargetMessage.ID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		resp, err := m.Client.ExecuteTasks(ctx, &socket.DeleteMessageTask{MessageId: messageID.ID})
		// TODO does the response data need to be checked?
		log.Trace().Any("response", resp).Msg("Meta delete response")
		return err
	case metaid.ParsedWAMessageID:
		if !m.e2eeConnectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		consumerMsg := wrapRevoke(&waConsumerApplication.ConsumerApplication_RevokeMessage{
			Key: m.messageIDToWAKey(messageID),
		})
		portalJID := msg.Portal.Metadata.(*metaid.PortalMetadata).JID(msg.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil, whatsmeow.SendRequestExtra{
			ID: waTypes.MessageID(msg.InputTransactionID),
		})
		log.Trace().Any("response", resp).Msg("WhatsApp delete response")
		return err
	default:
		return fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixReadReceipt(ctx context.Context, receipt *bridgev2.MatrixReadReceipt) error {
	if m.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	if !receipt.ReadUpTo.After(receipt.LastRead) {
		return nil
	}
	if receipt.LastRead.IsZero() {
		receipt.LastRead = receipt.ReadUpTo.Add(-5 * time.Second)
	}
	messages, err := receipt.Portal.Bridge.DB.Message.GetMessagesBetweenTimeQuery(ctx, receipt.Portal.PortalKey, receipt.LastRead, receipt.ReadUpTo)
	if err != nil {
		return fmt.Errorf("failed to get messages to mark as read: %w", err)
	} else if len(messages) == 0 {
		return nil
	}
	log := zerolog.Ctx(ctx)
	log.Trace().
		Time("last_read", receipt.LastRead).
		Time("read_up_to", receipt.ReadUpTo).
		Int("message_count", len(messages)).
		Msg("Handling read receipt")
	waMessagesToRead := make(map[waTypes.JID][]string)
	var fbMessageToReadTS time.Time
	for _, msg := range messages {
		switch messageID := metaid.ParseMessageID(msg.ID).(type) {
		case metaid.ParsedFBMessageID:
			if fbMessageToReadTS.Before(msg.Timestamp) {
				fbMessageToReadTS = msg.Timestamp
			}
		case metaid.ParsedWAMessageID:
			if msg.SenderID == networkid.UserID(m.UserLogin.ID) {
				continue
			}
			var key waTypes.JID
			// In group chats, group receipts by sender. In DMs, just use blank key (no participant field).
			if messageID.Sender != messageID.Chat {
				key = messageID.Sender
			}
			waMessagesToRead[key] = append(waMessagesToRead[key], messageID.ID)
		}
	}
	threadID := metaid.ParseFBPortalID(receipt.Portal.ID)
	if !fbMessageToReadTS.IsZero() && threadID != 0 && !receipt.Implicit {
		var syncGroup int64 = 1
		// TODO set sync group to 104 for community groups?
		resp, err := m.Client.ExecuteTasks(ctx, &socket.ThreadMarkReadTask{
			ThreadId:            threadID,
			LastReadWatermarkTs: fbMessageToReadTS.UnixMilli(),
			SyncGroup:           syncGroup,
		})
		log.Trace().Any("response", resp).Msg("Read receipt send response")
		if err != nil {
			log.Err(err).Time("read_watermark", fbMessageToReadTS).Msg("Failed to send read receipt")
		} else {
			log.Debug().Time("read_watermark", fbMessageToReadTS).Msg("Read receipt sent")
		}
	}
	portalJID := receipt.Portal.Metadata.(*metaid.PortalMetadata).JID(receipt.Portal.ID)
	if len(waMessagesToRead) > 0 && !portalJID.IsEmpty() {
		for messageSender, ids := range waMessagesToRead {
			err = m.E2EEClient.MarkRead(ctx, ids, receipt.Receipt.Timestamp, portalJID, messageSender)
			if err != nil {
				log.Err(err).Strs("ids", ids).Msg("Failed to mark messages as read")
			}
		}
	}
	return nil
}

// Note: the handling of typing notifications for facebook E2EE is
// very similar to the handling of typing notifications in the
// whatsapp bridge, because they both use the same API.

func (m *MetaClient) HandleMatrixViewingChat(ctx context.Context, msg *bridgev2.MatrixViewingChat) error {
	if m.E2EEClient == nil {
		return nil
	}

	// WhatsApp only sends typing notifications if the user is set
	// to online, and Facebook uses WhatsApp for E2EE chats,
	// therefore we need to set online status for typing
	// notifications to work properly.
	presence := waTypes.PresenceUnavailable
	if msg.Portal != nil {
		portalMeta := msg.Portal.Metadata.(*metaid.PortalMetadata)
		if portalMeta.ThreadType.IsWhatsApp() {
			presence = waTypes.PresenceAvailable
		}
	}

	if m.waLastPresence != presence {
		err := m.updateWAPresence(ctx, presence)
		if err != nil {
			zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to set presence when viewing chat")
		}
	}

	// No codepaths where we return an error, yet
	return nil
}

func (m *MetaClient) HandleMatrixTyping(ctx context.Context, msg *bridgev2.MatrixTyping) error {
	portalMeta := msg.Portal.Metadata.(*metaid.PortalMetadata)
	if portalMeta.ThreadType.IsWhatsApp() {
		portalJID := msg.Portal.Metadata.(*metaid.PortalMetadata).JID(msg.Portal.ID)
		var chatPresence waTypes.ChatPresence
		var mediaPresence waTypes.ChatPresenceMedia
		if msg.IsTyping {
			chatPresence = waTypes.ChatPresenceComposing
		} else {
			chatPresence = waTypes.ChatPresencePaused
		}
		switch msg.Type {
		case bridgev2.TypingTypeText:
			mediaPresence = waTypes.ChatPresenceMediaText
		case bridgev2.TypingTypeRecordingMedia:
			mediaPresence = waTypes.ChatPresenceMediaAudio
		case bridgev2.TypingTypeUploadingMedia:
			return nil
		}

		if m.Main.Config.SendPresenceOnTyping {
			err := m.updateWAPresence(ctx, waTypes.PresenceAvailable)
			if err != nil {
				zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to set presence on typing")
			}
		}
		return m.E2EEClient.SendChatPresence(ctx, portalJID, chatPresence, mediaPresence)
	}
	threadID := metaid.ParseFBPortalID(msg.Portal.ID)
	isGroupThread := int64(1)
	if portalMeta.ThreadType.IsOneToOne() {
		isGroupThread = 0
	}
	isTyping := int64(0)
	if msg.IsTyping {
		isTyping = 1
	}
	return m.Client.ExecuteStatelessTask(ctx, &socket.UpdatePresenceTask{
		ThreadKey:     threadID,
		IsGroupThread: isGroupThread,
		IsTyping:      isTyping,
		Attribution:   0,
		SyncGroup:     1,
		ThreadType:    int64(portalMeta.ThreadType),
	})
}

func (t *MetaClient) HandleMatrixDeleteChat(ctx context.Context, chat *bridgev2.MatrixDeleteChat) error {
	portalMeta := chat.Portal.Metadata.(*metaid.PortalMetadata)
	platform := t.LoginMeta.Platform
	threadID := metaid.ParseFBPortalID(chat.Portal.ID)

	zerolog.Ctx(ctx).Info().
		Int64("thread_id", threadID).
		Any("platform", platform).
		Bool("is_whatsapp_e2ee", portalMeta.ThreadType.IsWhatsApp()).
		Msg("Deleting chat")

	if platform.IsInstagram() {
		return t.Client.Instagram.DeleteThread(ctx, strconv.FormatInt(threadID, 10))
	} else if platform.IsMessenger() {
		_, err := t.Client.ExecuteTasks(ctx, &socket.DeleteThreadTask{
			ThreadKey:  threadID,
			RemoveType: 0,
			SyncGroup:  1,
		})
		if err != nil {
			return err
		}
		if portalMeta.ThreadType.IsWhatsApp() {
			_, err := t.Client.ExecuteTasks(ctx, &socket.DeleteThreadTask{
				ThreadKey:  threadID, // TODO: use e2ee thread ID
				RemoveType: 0,
				SyncGroup:  95,
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown platform for deleting chat: %v", platform)
}

func (m *MetaClient) HandleMatrixRoomName(ctx context.Context, msg *bridgev2.MatrixRoomName) (bool, error) {
	if msg.Portal.RoomType == database.RoomTypeDM {
		return false, fmt.Errorf("renaming not supported in DMs")
	}
	platform := m.LoginMeta.Platform
	threadID := metaid.ParseFBPortalID(msg.Portal.ID)
	if platform == types.Instagram {
		err := m.Client.Instagram.EditGroupTitle(ctx, strconv.FormatInt(threadID, 10), msg.Content.Name)
		if err != nil {
			return false, err
		}
		return true, nil
	} else if platform.IsMessenger() {
		_, err := m.Client.ExecuteTasks(ctx, &socket.RenameThreadTask{
			ThreadKey:  threadID,
			ThreadName: msg.Content.Name,
			SyncGroup:  1,
		})
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("unknown platform for renaming chat: %v", platform)
}

func (m *MetaClient) HandleMatrixRoomAvatar(ctx context.Context, msg *bridgev2.MatrixRoomAvatar) (bool, error) {
	if msg.Portal.RoomType == database.RoomTypeDM {
		return false, fmt.Errorf("changing avatar not supported in DMs")
	}
	if m.LoginMeta.Platform == types.Instagram {
		// TODO: implement Instagram avatar changing. IG Web doesn't support this.
		return false, fmt.Errorf("changing avatar not supported on Instagram")
	}
	threadID := metaid.ParseFBPortalID(msg.Portal.ID)
	var imageID int64
	if msg.Content.URL == "" {
		// TODO: handle removing avatar. Messenger web doesn't have a remove option?
		return false, fmt.Errorf("removing avatar not implemented")
	} else {
		data, err := m.Main.Bridge.Bot.DownloadMedia(ctx, msg.Content.URL, nil)
		if err != nil {
			return false, fmt.Errorf("failed to download avatar: %w", err)
		}
		mimeType := http.DetectContentType(data)
		resp, err := m.Client.SendMercuryUploadRequest(ctx, threadID, &messagix.MercuryUploadMedia{
			Filename:  "avatar.jpg",
			MimeType:  mimeType,
			MediaData: data,
		})
		if err != nil {
			return false, fmt.Errorf("failed to upload avatar: %w", err)
		}

		imageID = resp.Payload.RealMetadata.GetFbId()
		if imageID == 0 {
			return false, fmt.Errorf("no image ID received from upload")
		}
	}
	_, err := m.Client.ExecuteTasks(ctx, &socket.SetThreadImageTask{
		ThreadKey: threadID,
		ImageID:   imageID,
		SyncGroup: 1,
	})
	if err != nil {
		return false, err
	}
	// TODO update portal metadata
	return true, nil
}

func (m *MetaClient) HandleMatrixMembership(ctx context.Context, msg *bridgev2.MatrixMembershipChange) (*bridgev2.MatrixMembershipResult, error) {
	if msg.Portal.RoomType == database.RoomTypeDM {
		return nil, errors.New("cannot change members for DM")
	}

	var targetID int64
	switch target := msg.Target.(type) {
	case *bridgev2.Ghost:
		targetID = metaid.ParseUserID(target.ID)
	case *bridgev2.UserLogin:
		targetID = metaid.ParseUserLoginID(target.ID)
	default:
		return nil, fmt.Errorf("unknown membership target type %T", target)
	}
	if targetID == 0 {
		return nil, fmt.Errorf("invalid target user ID")
	}

	portalMeta := msg.Portal.Metadata.(*metaid.PortalMetadata)
	if portalMeta.ThreadType == table.ENCRYPTED_OVER_WA_GROUP {
		portalJID := portalMeta.JID(msg.Portal.ID)
		targetJID := waTypes.NewJID(strconv.FormatInt(targetID, 10), waTypes.MessengerServer)
		var action whatsmeow.ParticipantChange
		switch msg.Type {
		case bridgev2.Invite:
			action = whatsmeow.ParticipantChangeAdd
		case bridgev2.Kick:
			action = whatsmeow.ParticipantChangeRemove
		default:
			return nil, nil
		}
		resp, err := m.E2EEClient.UpdateGroupParticipants(ctx, portalJID, []waTypes.JID{targetJID}, action)
		if err != nil {
			return nil, err
		} else if len(resp) == 0 {
			return nil, fmt.Errorf("no response for participant change")
		} else if resp[0].Error != 0 {
			return nil, fmt.Errorf("failed to change participant: code %d", resp[0].Error)
		}
		return &bridgev2.MatrixMembershipResult{RedirectTo: metaid.MakeWAUserID(resp[0].JID)}, nil
	}

	threadID := metaid.ParseFBPortalID(msg.Portal.ID)
	var task socket.Task
	switch msg.Type {
	case bridgev2.Invite:
		task = &socket.AddParticipantsTask{
			ThreadKey:  threadID,
			ContactIDs: []int64{targetID},
			SyncGroup:  1,
		}
	case bridgev2.Kick:
		task = &socket.RemoveParticipantTask{
			ThreadID:  threadID,
			ContactID: targetID,
		}
	default:
		return nil, nil
	}
	_, err := m.Client.ExecuteTasks(ctx, task)
	if err != nil {
		return nil, err
	}
	return &bridgev2.MatrixMembershipResult{}, nil
}
