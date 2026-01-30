package connector

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var _ bridgev2.BackfillingNetworkAPI = (*MetaClient)(nil)

var ReRequestBackfillOnTimeout = false
var ErrorOnBackfillTimeout = false
var BackfillTimeout = 30 * time.Second
var BackfillForwardTimeout = 15 * time.Second
var BackfillBackgroundTimeout = 8 * time.Second

type BackfillCollector struct {
	*table.UpsertMessages
	MaxMessages int
	Forward     bool
	Anchor      *database.Message
	Done        func()
}

var globalUpsertCounter atomic.Int64

func (m *MetaClient) handleUpsertMessages(tk handlerParams, upsert *table.UpsertMessages) bridgev2.RemoteEvent {
	upsertID := globalUpsertCounter.Add(1)
	log := m.UserLogin.Log.With().
		Str("action", "handle meta upsert").
		Int64("thread_key", tk.ID).
		Int64("thread_type", int64(tk.Type)).
		Int64("global_upsert_counter", upsertID).
		Logger()
	ctx := log.WithContext(tk.ctx)
	m.backfillLock.Lock()
	defer m.backfillLock.Unlock()
	log.Info().
		Int64("min_timestamp_ms", upsert.Range.MinTimestampMs).
		Str("min_message_id", upsert.Range.MinMessageId).
		Int64("max_timestamp_ms", upsert.Range.MaxTimestampMs).
		Str("max_message_id", upsert.Range.MaxMessageId).
		Bool("has_more_before", upsert.Range.HasMoreBefore).
		Bool("has_more_after", upsert.Range.HasMoreAfter).
		Int("message_count", len(upsert.Messages)).
		Msg("Received upsert messages")
	if collector, ok := m.backfillCollectors[tk.ID]; ok {
		if upsert.Range.MaxTimestampMsTemplate > collector.Range.MinTimestampMs {
			log.Warn().
				Int64("prev_min_timestamp_ms", collector.Range.MinTimestampMs).
				Msg("Ignoring unexpected upsert messages while collecting history")
			return nil
		}
		if collector.MaxMessages > 0 {
			collector.MaxMessages = max(collector.MaxMessages-len(upsert.Messages), 0)
		}
		collector.UpsertMessages = collector.UpsertMessages.Join(upsert)
		messageLimitReached := collector.MaxMessages == 0
		endOfChatReached := !upsert.Range.HasMoreBefore
		existingMessagesReached := collector.Forward && collector.Anchor != nil && collector.Range.MinTimestampMs <= collector.Anchor.Timestamp.UnixMilli()
		logEvt := log.Debug().
			Bool("message_limit_reached", messageLimitReached).
			Bool("end_of_chat_reached", endOfChatReached).
			Bool("existing_messages_reached", existingMessagesReached)
		if !messageLimitReached && !endOfChatReached && !existingMessagesReached {
			logEvt.Msg("Requesting more history as collector still has room")
			go m.requestMoreHistory(ctx, tk.ID, upsert.Range.MinTimestampMs, upsert.Range.MinMessageId)
			return nil
		}
		logEvt.Msg("Processing collected history now")
		delete(m.backfillCollectors, tk.ID)
		collector.Done()
		return nil
	} else if tk.Sync != nil {
		// The thread is already being resynced, attach the backfill to the existing resync
		tk.Sync.Backfill = upsert
		tk.Sync.UpsertID = upsertID
		return nil
	} else {
		// Random upsert request, send a standalone event
		return &FBChatResync{
			PortalKey: tk.Portal,
			Backfill:  upsert,
			UpsertID:  upsertID,
			m:         m,
		}
	}
}

func (m *MetaClient) handleUpdateExistingMessageRange(tk handlerParams, rng *table.LSUpdateExistingMessageRange) bridgev2.RemoteEvent {
	logEvt := m.UserLogin.Log.Info().
		Str("action", "handle meta existing range").
		Int64("thread_key", tk.ID).
		Int64("thread_type", int64(tk.Type)).
		Int("global_upsert_counter", int(globalUpsertCounter.Add(1))).
		Int64("timestamp_ms", rng.TimestampMS).
		Bool("bool2", rng.UnknownBool2).
		Bool("bool3", rng.UnknownBool3)
	if collector, ok := m.backfillCollectors[tk.ID]; !ok {
		logEvt.Msg("Ignoring update existing message range command with no backfill collector")
	} else if collector.Range.MinTimestampMs != rng.TimestampMS {
		logEvt.Int64("prev_timestamp_ms", collector.Range.MinTimestampMs).
			Msg("Ignoring update existing message range command with different timestamp")
	} else {
		if len(collector.Messages) == 0 {
			logEvt.Msg("Update existing range marked backfill as done, no messages found")
			collector.Range.HasMoreBefore = false
		} else {
			logEvt.Msg("Update existing range marked backfill as done, processing collected history now")
			if rng.UnknownBool2 && !rng.UnknownBool3 {
				collector.Range.HasMoreBefore = false
			} else {
				collector.Range.HasMoreAfter = false
			}
		}
		collector.Done()
		delete(m.backfillCollectors, tk.ID)
	}
	return nil
}

func (m *MetaClient) requestMoreHistory(ctx context.Context, threadID, minTimestampMS int64, minMessageID string) bool {
	resp, err := m.Client.ExecuteTasks(ctx, &socket.FetchMessagesTask{
		ThreadKey:            threadID,
		Direction:            0,
		ReferenceTimestampMs: minTimestampMS,
		ReferenceMessageId:   minMessageID,
		SyncGroup:            1,
		Cursor:               m.Client.GetCursor(1),
	})
	zerolog.Ctx(ctx).Trace().
		Int64("thread_id", threadID).
		Any("resp_data", resp).
		Msg("Response data for fetching messages")
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Int64("thread_id", threadID).Msg("Failed to request more history")
		return false
	} else {
		zerolog.Ctx(ctx).Debug().
			Int64("thread_id", threadID).
			Int64("min_timestamp_ms", minTimestampMS).
			Str("min_message_id", minMessageID).
			Msg("Requested more history")
		return true
	}
}

func (m *MetaClient) addBackfillCollector(threadID int64, collector *BackfillCollector) bool {
	m.backfillLock.Lock()
	defer m.backfillLock.Unlock()
	_, ok := m.backfillCollectors[threadID]
	if ok {
		return false
	}
	m.backfillCollectors[threadID] = collector
	return true
}

func (m *MetaClient) removeBackfillCollector(threadID int64, collector *BackfillCollector) {
	m.backfillLock.Lock()
	defer m.backfillLock.Unlock()
	existing, ok := m.backfillCollectors[threadID]
	if ok && existing == collector {
		delete(m.backfillCollectors, threadID)
	}
}

func (m *MetaClient) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	if m.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	if params.Portal.Metadata.(*metaid.PortalMetadata).ThreadType == table.ENCRYPTED_OVER_WA_GROUP {
		return nil, nil
	}
	threadID := metaid.ParseFBPortalID(params.Portal.ID)
	if threadID == 0 {
		return nil, fmt.Errorf("invalid thread ID")
	}
	if params.Forward && params.BundledData == nil {
		zerolog.Ctx(ctx).Debug().Msg("Ignoring forward backfill without bundled data")
		return nil, nil
	}
	upsert, _ := params.BundledData.(*table.UpsertMessages)
	if upsert == nil || len(upsert.Messages) < params.Count {
		// Shortcut: bundled data min TS <= anchor TS, meaning we have everything
		if params.Forward && params.BundledData != nil && params.AnchorMessage != nil {
			anchorTS := params.AnchorMessage.Timestamp.UnixMilli()
			if upsert.Range.MinTimestampMs <= anchorTS {
				zerolog.Ctx(ctx).Debug().
					Int64("bundled_min_ts", upsert.Range.MinTimestampMs).
					Int64("anchor_ts", anchorTS).
					Msg("Forward backfill has all messages already")
				return m.wrapBackfillEvents(ctx, params.Portal, upsert, params.AnchorMessage, params.Forward), nil
			}
		}
		var oldestMessageID string
		var oldestMessageTS int64
		if upsert != nil {
			oldestMessageID = upsert.Range.MinMessageId
			oldestMessageTS = upsert.Range.MinTimestampMs
		} else if params.AnchorMessage != nil {
			parsedID, ok := metaid.ParseMessageID(params.AnchorMessage.ID).(metaid.ParsedFBMessageID)
			if !ok {
				zerolog.Ctx(ctx).Warn().Msg("Can't backfill with non-FB message ID")
				return nil, nil
			}
			oldestMessageID = parsedID.ID
			oldestMessageTS = params.AnchorMessage.Timestamp.UnixMilli()
			upsert = &table.UpsertMessages{
				Range: &table.LSInsertNewMessageRange{
					ThreadKey:              threadID,
					MinTimestampMsTemplate: oldestMessageTS,
					MaxTimestampMsTemplate: oldestMessageTS,
					MinMessageId:           oldestMessageID,
					MaxMessageId:           oldestMessageID,
					MinTimestampMs:         oldestMessageTS,
					MaxTimestampMs:         oldestMessageTS,
					HasMoreBefore:          true,
					HasMoreAfter:           true,
				},
			}
		} else {
			zerolog.Ctx(ctx).Warn().Msg("Can't backfill chat with no messages")
			return nil, nil
		}
		doneCh := make(chan struct{})
		collector := &BackfillCollector{
			UpsertMessages: upsert,
			MaxMessages:    params.Count,
			Forward:        params.Forward,
			Anchor:         params.AnchorMessage,
			Done: sync.OnceFunc(func() {
				close(doneCh)
			}),
		}
		if !m.addBackfillCollector(threadID, collector) {
			return nil, fmt.Errorf("backfill collector already exists for thread %d", threadID)
		}
		defer m.removeBackfillCollector(threadID, collector)
		start := time.Now()
		timeout := BackfillTimeout
		if params.Forward && bridgev2.PortalEventBuffer == 0 {
			timeout = BackfillForwardTimeout
		}
		if m.Main.Bridge.Background {
			timeout = BackfillBackgroundTimeout
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		// Finally kick off history request, we'll keep retrying this if we timeout
		if !m.requestMoreHistory(ctx, threadID, oldestMessageTS, oldestMessageID) {
			return nil, fmt.Errorf("failed to request more history for thread %d", threadID)
		}
	Loop:
		for {
			select {
			case <-doneCh:
				upsert = collector.UpsertMessages
				break Loop
			case <-ticker.C:
				if params.Forward && collector.UpsertMessages.Range.MinTimestampMs >= oldestMessageTS {
					zerolog.Ctx(ctx).Warn().Msg("Backfill collector did not call done, but has all messages")
					break Loop
				} else if time.Since(start) > timeout {
					zerolog.Ctx(ctx).Error().Msg("Waiting for backfill collector timed out")
					if ErrorOnBackfillTimeout {
						return nil, fmt.Errorf("failed to backfill history")
					}
					break Loop
				}
				// No response? Let's re-request that history again
				if ReRequestBackfillOnTimeout && !m.requestMoreHistory(ctx, threadID, oldestMessageTS, oldestMessageID) {
					return nil, fmt.Errorf("failed to request more history for thread %d", threadID)
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return m.wrapBackfillEvents(ctx, params.Portal, upsert, params.AnchorMessage, params.Forward), nil
}

func (m *MetaClient) wrapBackfillEvents(ctx context.Context, portal *bridgev2.Portal, upsert *table.UpsertMessages, anchor *database.Message, forward bool) *bridgev2.FetchMessagesResponse {
	// The messages are probably already sorted in reverse order (newest to oldest). We want to sort them again to be safe,
	// but reverse first to make the sorting algorithm's job easier if it's already sorted.
	slices.Reverse(upsert.Messages)
	slices.SortFunc(upsert.Messages, func(a, b *table.WrappedMessage) int {
		key := cmp.Compare(a.PrimarySortKey, b.PrimarySortKey)
		if key == 0 {
			key = cmp.Compare(a.SecondarySortKey, b.SecondarySortKey)
		}
		return key
	})
	upsert.Messages = slices.CompactFunc(upsert.Messages, func(a, b *table.WrappedMessage) bool {
		if a.MessageId == b.MessageId {
			zerolog.Ctx(ctx).Debug().
				Str("message_id", a.MessageId).
				Bool("attachment_counts_match", len(a.XMAAttachments) == len(b.XMAAttachments) && len(a.BlobAttachments) == len(b.BlobAttachments) && len(a.Stickers) == len(b.Stickers)).
				Msg("Backfill batch contained duplicate message")
			return true
		}
		return false
	})
	if anchor != nil {
		if forward {
			upsert.Messages = slices.DeleteFunc(upsert.Messages, func(message *table.WrappedMessage) bool {
				return message.TimestampMs <= anchor.Timestamp.UnixMilli()
			})
		} else {
			upsert.Messages = slices.DeleteFunc(upsert.Messages, func(message *table.WrappedMessage) bool {
				return message.TimestampMs >= anchor.Timestamp.UnixMilli()
			})
		}
	}
	wrappedMessages := make([]*bridgev2.BackfillMessage, len(upsert.Messages))
	for i, msg := range upsert.Messages {
		m.handleSubthread(ctx, msg)
		log := zerolog.Ctx(ctx).With().Str("message_id", msg.MessageId).Logger()
		ctx := log.WithContext(ctx)
		sender := m.makeEventSender(msg.SenderId)
		intent, ok := portal.GetIntentFor(ctx, sender, m.UserLogin, bridgev2.RemoteEventBackfill)
		if !ok {
			continue
		}
		parsedTS, err := methods.ParseMessageID(msg.MessageId)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to parse message ID in backfill")
		} else if parsedTS != msg.TimestampMs {
			log.Warn().
				Int64("parsed_ts", parsedTS).
				Int64("timestamp_ms", msg.TimestampMs).
				Msg("Message ID timestamp mismatch in backfill")
		}
		msgID := metaid.MakeFBMessageID(msg.MessageId)
		wrappedMessages[i] = &bridgev2.BackfillMessage{
			ConvertedMessage: m.Main.MsgConv.ToMatrix(ctx, portal, m.Client, intent, msgID, msg, m.Main.Config.DisableXMABackfill || m.Main.Config.DisableXMAAlways),
			Sender:           sender,
			ID:               msgID,
			Timestamp:        time.UnixMilli(msg.TimestampMs),
			Reactions:        make([]*bridgev2.BackfillReaction, len(msg.Reactions)),
			StreamOrder:      msg.TimestampMs,

			//ShouldBackfillThread: msg.SubthreadKey != 0,
		}
		for j, reaction := range msg.Reactions {
			wrappedMessages[i].Reactions[j] = &bridgev2.BackfillReaction{
				Timestamp: time.UnixMilli(reaction.TimestampMs),
				Sender:    m.makeEventSender(reaction.ActorId),
				Emoji:     reaction.Reaction,
			}
		}
	}
	return &bridgev2.FetchMessagesResponse{
		Messages: wrappedMessages,
		HasMore:  upsert.Range.HasMoreBefore,
		MarkRead: upsert.MarkRead,
	}
}
