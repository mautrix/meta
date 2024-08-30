package connector

import (
	"context"
	"errors"
	"fmt"
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

	"go.mau.fi/mautrix-meta/pkg/messagix"
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
)

var (
	ErrServerRejectedMessage = bridgev2.WrapErrorInStatus(errors.New("server rejected message")).WithErrorAsMessage().WithSendNotice(true)
	ErrNotConnected          = bridgev2.WrapErrorInStatus(errors.New("not connected")).WithErrorAsMessage().WithSendNotice(true)
)

const ConnectWaitTimeout = 1 * time.Minute

func (m *MetaClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	log := zerolog.Ctx(ctx)

	portalMeta := msg.Portal.Metadata.(*PortalMetadata)

	switch portalMeta.ThreadType {
	case table.ENCRYPTED_OVER_WA_ONE_TO_ONE, table.ENCRYPTED_OVER_WA_GROUP:
		if !m.e2eeConnectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return nil, ErrNotConnected
		}

		waMsg, waMeta, err := m.Main.MsgConv.ToWhatsApp(ctx, msg.Event, msg.Content, msg.Portal, m.E2EEClient, msg.OrigSender != nil, msg.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		messageID := m.E2EEClient.GenerateMessageID()
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
		}, nil
	default:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return nil, ErrNotConnected
		}

		tasks, otid, err := m.Main.MsgConv.ToMeta(ctx, m.Client, msg.Event, msg.Content, msg.ReplyTo, msg.OrigSender != nil, msg.Portal)
		if errors.Is(err, types.ErrPleaseReloadPage) {
			// TODO handle properly
			return nil, err
		} else if errors.Is(err, messagix.ErrTokenInvalidated) {
			// TODO handle properly
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

		retries := 0
		for retries < 5 {
			if err = m.Client.WaitUntilCanSendMessages(15 * time.Second); err != nil {
				log.Err(err).Msg("Error waiting to be able to send messages, retrying")
			} else {
				resp, err = m.Client.ExecuteTasks(tasks...)
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

		return &bridgev2.MatrixMessageResponse{
			DB: &database.Message{
				ID:       metaid.MakeFBMessageID(msgID),
				SenderID: networkid.UserID(m.UserLogin.ID),
			},
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
	switch messageID := metaid.ParseMessageID(msg.TargetMessage.ID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return nil, ErrNotConnected
		}
		resp, err := m.Client.ExecuteTasks(&socket.SendReactionTask{
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
		portalJID := msg.Portal.Metadata.(*PortalMetadata).JID(msg.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil)
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("WhatsApp reaction response")
		return nil, err
	default:
		return nil, fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixReactionRemove(ctx context.Context, msg *bridgev2.MatrixReactionRemove) error {
	switch messageID := metaid.ParseMessageID(msg.TargetReaction.MessageID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		resp, err := m.Client.ExecuteTasks(&socket.SendReactionTask{
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
		portalJID := msg.Portal.Metadata.(*PortalMetadata).JID(msg.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil)
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("WhatsApp reaction response")
		return err
	default:
		return fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixEdit(ctx context.Context, edit *bridgev2.MatrixEdit) error {
	log := zerolog.Ctx(ctx)
	switch messageID := metaid.ParseMessageID(edit.EditTarget.ID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		fakeSendTasks, _, err := m.Main.MsgConv.ToMeta(ctx, m.Client, edit.Event, edit.Content, nil, false, edit.Portal)
		if err != nil {
			return fmt.Errorf("failed to convert message: %w", err)
		}

		fakeTask := fakeSendTasks[0].(*socket.SendMessageTask)

		editTask := &socket.EditMessageTask{
			MessageID: messageID.ID,
			Text:      fakeTask.Text,
		}

		newEditCount := int64(edit.EditTarget.EditCount) + 1

		var resp *table.LSTable
		resp, err = m.Client.ExecuteTasks(editTask)
		log.Trace().Any("response", resp).Msg("Meta edit response")
		if err != nil {
			return fmt.Errorf("failed to send edit to Meta: %w", err)
		}

		if len(resp.LSEditMessage) > 0 {
			edit.EditTarget.EditCount = int(resp.LSEditMessage[0].EditCount)
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
	case metaid.ParsedWAMessageID:
		if !m.e2eeConnectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		consumerMsg := wrapEdit(&waConsumerApplication.ConsumerApplication_EditMessage{
			Key:         m.messageIDToWAKey(messageID),
			Message:     m.Main.MsgConv.TextToWhatsApp(edit.Content),
			TimestampMS: ptr.Ptr(edit.Event.Timestamp),
		})
		portalJID := edit.Portal.Metadata.(*PortalMetadata).JID(edit.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil)
		log.Trace().Any("response", resp).Msg("WhatsApp edit response")
		return err
	default:
		return fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixMessageRemove(ctx context.Context, msg *bridgev2.MatrixMessageRemove) error {
	log := zerolog.Ctx(ctx)
	switch messageID := metaid.ParseMessageID(msg.TargetMessage.ID).(type) {
	case metaid.ParsedFBMessageID:
		if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
			return ErrNotConnected
		}
		resp, err := m.Client.ExecuteTasks(&socket.DeleteMessageTask{MessageId: messageID.ID})
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
		portalJID := msg.Portal.Metadata.(*PortalMetadata).JID(msg.Portal.ID)
		resp, err := m.E2EEClient.SendFBMessage(ctx, portalJID, consumerMsg, nil)
		log.Trace().Any("response", resp).Msg("WhatsApp delete response")
		return err
	default:
		return fmt.Errorf("invalid message ID")
	}
}

func (m *MetaClient) HandleMatrixReadReceipt(ctx context.Context, receipt *bridgev2.MatrixReadReceipt) error {
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
	if !fbMessageToReadTS.IsZero() && threadID != 0 {
		var syncGroup int64 = 1
		// TODO set sync group to 104 for community groups?
		resp, err := m.Client.ExecuteTasks(&socket.ThreadMarkReadTask{
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
	portalJID := receipt.Portal.Metadata.(*PortalMetadata).JID(receipt.Portal.ID)
	if len(waMessagesToRead) > 0 && !portalJID.IsEmpty() {
		for messageSender, ids := range waMessagesToRead {
			err = m.E2EEClient.MarkRead(ids, receipt.Receipt.Timestamp, portalJID, messageSender)
			if err != nil {
				log.Err(err).Strs("ids", ids).Msg("Failed to mark messages as read")
			}
		}
	}
	return nil
}
