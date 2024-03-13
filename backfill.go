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

package main

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
)

func (user *User) StopBackfillLoop() {
	if fn := user.stopBackfillTask.Swap(nil); fn != nil {
		(*fn)()
	}
}

type BackfillCollector struct {
	*table.UpsertMessages
	Source      id.UserID
	MaxPages    int
	Forward     bool
	LastMessage *database.Message
	Task        *database.BackfillTask
	Done        func()
}

func (user *User) handleBackfillTask(ctx context.Context, task *database.BackfillTask) {
	log := zerolog.Ctx(ctx)
	log.Debug().Any("task", task).Msg("Got backfill task")
	portal := user.bridge.GetExistingPortalByThreadID(task.Key)
	task.DispatchedAt = time.Now()
	task.CompletedAt = time.Time{}
	if !portal.MoreToBackfill {
		log.Debug().Int64("portal_id", task.Key.ThreadID).Msg("Nothing more to backfill in portal")
		task.Finished = true
		task.CompletedAt = time.Now()
		if err := task.Upsert(ctx); err != nil {
			log.Err(err).Msg("Failed to save backfill task")
		}
		return
	}
	if err := task.Upsert(ctx); err != nil {
		log.Err(err).Msg("Failed to save backfill task")
	}
	ok := portal.requestMoreHistory(ctx, user, portal.OldestMessageTS, portal.OldestMessageID)
	if !ok {
		task.CooldownUntil = time.Now().Add(1 * time.Hour)
		if err := task.Upsert(ctx); err != nil {
			log.Err(err).Msg("Failed to save backfill task")
		}
		return
	}
	backfillDone := make(chan struct{})
	doneCallback := sync.OnceFunc(func() {
		close(backfillDone)
	})
	portal.backfillCollector = &BackfillCollector{
		UpsertMessages: &table.UpsertMessages{
			Range: &table.LSInsertNewMessageRange{
				ThreadKey:              portal.ThreadID,
				MinTimestampMsTemplate: portal.OldestMessageTS,
				MaxTimestampMsTemplate: portal.OldestMessageTS,
				MinMessageId:           portal.OldestMessageID,
				MaxMessageId:           portal.OldestMessageID,
				MinTimestampMs:         portal.OldestMessageTS,
				MaxTimestampMs:         portal.OldestMessageTS,
				HasMoreBefore:          true,
				HasMoreAfter:           true,
			},
		},
		Source:   user.MXID,
		MaxPages: user.bridge.Config.Bridge.Backfill.Queue.PagesAtOnce,
		Forward:  false,
		Task:     task,
		Done:     doneCallback,
	}
	select {
	case <-backfillDone:
	case <-ctx.Done():
		return
	}
	if !portal.MoreToBackfill {
		task.Finished = true
	}
	task.CompletedAt = time.Now()
	if err := task.Upsert(ctx); err != nil {
		log.Err(err).Msg("Failed to save backfill task")
	}
	log.Debug().Any("task", task).Msg("Finished backfill task")
}

func (user *User) BackfillLoop() {
	if !user.bridge.Config.Bridge.Backfill.Enabled {
		return
	}
	log := user.log.With().Str("action", "backfill loop").Logger()
	defer func() {
		log.Debug().Msg("Backfill loop stopped")
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldFn := user.stopBackfillTask.Swap(&cancel)
	if oldFn != nil {
		(*oldFn)()
	}
	ctx = log.WithContext(ctx)
	var extraTime time.Duration
	sleepBetweenTasks := user.bridge.Config.Bridge.Backfill.Queue.SleepBetweenTasks
	initialSleep := time.Duration(rand.Int63n(sleepBetweenTasks.Nanoseconds())) + (sleepBetweenTasks / 2)
	log.Debug().Stringer("sleep_duration", initialSleep).Msg("Starting backfill loop after initial delay")
	select {
	case <-time.After(initialSleep):
	case <-ctx.Done():
		return
	}
	log.Debug().Msg("Backfill loop started")
	for {
		task, err := user.bridge.DB.BackfillTask.GetNext(ctx, user.MXID)
		if err != nil {
			log.Err(err).Msg("Failed to get next backfill task")
		} else if task != nil {
			user.handleBackfillTask(ctx, task)
			extraTime = 0
		} else if extraTime < 1*time.Minute {
			extraTime += 5 * time.Second
		}
		select {
		case <-time.After(sleepBetweenTasks + extraTime):
		case <-ctx.Done():
			return
		}
	}
}

func (portal *Portal) requestMoreHistory(ctx context.Context, user *User, minTimestampMS int64, minMessageID string) bool {
	resp, err := user.Client.ExecuteTasks(&socket.FetchMessagesTask{
		ThreadKey:            portal.ThreadID,
		Direction:            0,
		ReferenceTimestampMs: minTimestampMS,
		ReferenceMessageId:   minMessageID,
		SyncGroup:            1,
		Cursor:               user.Client.SyncManager.GetCursor(1),
	})
	zerolog.Ctx(ctx).Trace().Any("resp_data", resp).Msg("Response data for fetching messages")
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to request more history")
		return false
	} else {
		zerolog.Ctx(ctx).Debug().
			Int64("min_timestamp_ms", minTimestampMS).
			Str("min_message_id", minMessageID).
			Msg("Requested more history")
		return true
	}
}

var globalUpsertCounter atomic.Int64

func (portal *Portal) handleMetaExistingRange(user *User, rng *table.LSUpdateExistingMessageRange) {
	portal.backfillLock.Lock()
	defer portal.backfillLock.Unlock()

	log := portal.log.With().
		Str("action", "handle meta existing range").
		Stringer("source_mxid", user.MXID).
		Int("global_upsert_counter", int(globalUpsertCounter.Add(1))).
		Logger()
	logEvt := log.Info().
		Int64("timestamp_ms", rng.TimestampMS).
		Bool("bool2", rng.UnknownBool2).
		Bool("bool3", rng.UnknownBool3)
	if portal.backfillCollector == nil {
		logEvt.Msg("Ignoring update existing message range command with no backfill collector")
	} else if portal.backfillCollector.Source != user.MXID {
		logEvt.Stringer("prev_mxid", portal.backfillCollector.Source).
			Msg("Ignoring update existing message range command for another user")
	} else if portal.backfillCollector.Range.MinTimestampMs != rng.TimestampMS {
		logEvt.Int64("prev_timestamp_ms", portal.backfillCollector.Range.MinTimestampMs).
			Msg("Ignoring update existing message range command with different timestamp")
	} else {
		if len(portal.backfillCollector.Messages) == 0 {
			logEvt.Msg("Update existing range marked backfill as done, no messages found")
			if portal.backfillCollector.Done != nil {
				portal.backfillCollector.Done()
			}
			portal.MoreToBackfill = false
			err := portal.Update(log.WithContext(context.TODO()))
			if err != nil {
				log.Err(err).Msg("Failed to save portal in database")
			}
		} else {
			logEvt.Msg("Update existing range marked backfill as done, processing collected history now")
			if rng.UnknownBool2 && !rng.UnknownBool3 {
				portal.backfillCollector.Range.HasMoreBefore = false
			} else {
				portal.backfillCollector.Range.HasMoreAfter = false
			}
			portal.handleMessageBatch(log.WithContext(context.TODO()), user, portal.backfillCollector.UpsertMessages, portal.backfillCollector.Forward, portal.backfillCollector.LastMessage, portal.backfillCollector.Done)
		}
		portal.backfillCollector = nil
	}
}

func (portal *Portal) handleMetaUpsertMessages(user *User, upsert *table.UpsertMessages) {
	portal.backfillLock.Lock()
	defer portal.backfillLock.Unlock()

	if !portal.bridge.Config.Bridge.Backfill.Enabled {
		return
	} else if upsert.Range == nil {
		portal.log.Warn().Int("message_count", len(upsert.Messages)).Msg("Ignoring upsert messages without range")
		return
	}
	log := portal.log.With().
		Str("action", "handle meta upsert").
		Stringer("source_mxid", user.MXID).
		Int("global_upsert_counter", int(globalUpsertCounter.Add(1))).
		Logger()
	log.Info().
		Int64("min_timestamp_ms", upsert.Range.MinTimestampMs).
		Str("min_message_id", upsert.Range.MinMessageId).
		Int64("max_timestamp_ms", upsert.Range.MaxTimestampMs).
		Str("max_message_id", upsert.Range.MaxMessageId).
		Bool("has_more_before", upsert.Range.HasMoreBefore).
		Bool("has_more_after", upsert.Range.HasMoreAfter).
		Int("message_count", len(upsert.Messages)).
		Msg("Received upsert messages")
	ctx := log.WithContext(context.TODO())

	// Check if someone is already collecting messages for backfill
	if portal.backfillCollector != nil {
		if user.MXID != portal.backfillCollector.Source {
			log.Warn().Stringer("prev_mxid", portal.backfillCollector.Source).Msg("Ignoring upsert for another user")
			return
		} else if upsert.Range.MaxTimestampMs > portal.backfillCollector.Range.MinTimestampMs {
			log.Warn().
				Int64("prev_min_timestamp_ms", portal.backfillCollector.Range.MinTimestampMs).
				Msg("Ignoring unexpected upsert messages while collecting history")
			return
		}
		if portal.backfillCollector.MaxPages > 0 {
			portal.backfillCollector.MaxPages--
		}
		portal.backfillCollector.UpsertMessages = portal.backfillCollector.Join(upsert)
		pageLimitReached := portal.backfillCollector.MaxPages == 0
		endOfChatReached := !upsert.Range.HasMoreBefore
		existingMessagesReached := portal.backfillCollector.LastMessage != nil && portal.backfillCollector.Range.MinTimestampMs <= portal.backfillCollector.LastMessage.Timestamp.UnixMilli()
		if portal.backfillCollector.Task != nil {
			portal.backfillCollector.Task.PageCount++
			if portal.bridge.Config.Bridge.Backfill.Queue.MaxPages >= 0 && portal.backfillCollector.Task.PageCount >= portal.bridge.Config.Bridge.Backfill.Queue.MaxPages {
				log.Debug().Any("task", portal.backfillCollector.Task).Msg("Marking backfill task as finished (reached page limit)")
				pageLimitReached = true
			}
		}
		logEvt := log.Debug().
			Bool("page_limit_reached", pageLimitReached).
			Bool("end_of_chat_reached", endOfChatReached).
			Bool("existing_messages_reached", existingMessagesReached)
		if !pageLimitReached && !endOfChatReached && !existingMessagesReached {
			logEvt.Msg("Requesting more history as collector still has room")
			portal.requestMoreHistory(ctx, user, upsert.Range.MinTimestampMs, upsert.Range.MinMessageId)
			return
		}
		logEvt.Msg("Processing collected history now")
		portal.handleMessageBatch(ctx, user, portal.backfillCollector.UpsertMessages, portal.backfillCollector.Forward, portal.backfillCollector.LastMessage, portal.backfillCollector.Done)
		portal.backfillCollector = nil
		return
	}

	// No active collector, check the last bridged message
	lastMessage, err := portal.bridge.DB.Message.GetLastByTimestamp(ctx, portal.PortalKey, time.Now().Add(1*time.Minute))
	if err != nil {
		log.Err(err).Msg("Failed to get last message to check if upsert batch should be handled")
		return
	}
	if lastMessage == nil {
		// Chat is empty, request more history or bridge the one received message immediately depending on history_fetch_count
		if portal.bridge.Config.Bridge.Backfill.HistoryFetchPages != 0 {
			log.Debug().Msg("Got first historical message in empty chat, requesting more")
			portal.backfillCollector = &BackfillCollector{
				UpsertMessages: upsert,
				Source:         user.MXID,
				MaxPages:       portal.bridge.Config.Bridge.Backfill.HistoryFetchPages,
				Forward:        true,
			}
			portal.requestMoreHistory(ctx, user, upsert.Range.MinTimestampMs, upsert.Range.MinMessageId)
		} else {
			log.Debug().Msg("Got first historical message in empty chat, bridging it immediately")
			portal.handleMessageBatch(ctx, user, upsert, true, nil, nil)
		}
	} else if upsert.Range.MaxTimestampMs > lastMessage.Timestamp.UnixMilli() && upsert.Range.MaxMessageId != lastMessage.ID {
		// Chat is not empty and the upsert contains a newer message than the last bridged one,
		// request more history to fill the gap or bridge the received one immediately depending on catchup_fetch_count
		if portal.bridge.Config.Bridge.Backfill.CatchupFetchPages > 0 {
			log.Debug().Msg("Got upsert of new messages, requesting more")
			portal.backfillCollector = &BackfillCollector{
				UpsertMessages: upsert,
				Source:         user.MXID,
				MaxPages:       portal.bridge.Config.Bridge.Backfill.CatchupFetchPages,
				Forward:        true,
				LastMessage:    lastMessage,
			}
			portal.requestMoreHistory(ctx, user, upsert.Range.MinTimestampMs, upsert.Range.MinMessageId)
		} else {
			log.Debug().Msg("Got upsert of new messages, bridging them immediately")
			portal.handleMessageBatch(ctx, user, upsert, true, lastMessage, nil)
		}
	} else {
		// Chat is not empty and the upsert doesn't contain new messages (and it's not a part of a backfill collector), ignore it.
		log.Debug().
			Int64("last_message_ts", lastMessage.Timestamp.UnixMilli()).
			Str("last_message_id", lastMessage.ID).
			Int64("upsert_max_ts", upsert.Range.MaxTimestampMs).
			Str("upsert_max_id", upsert.Range.MaxMessageId).
			Msg("Ignoring unrequested upsert before last message")
		queueConfig := portal.bridge.Config.Bridge.Backfill.Queue
		if queueConfig.MaxPages != 0 && portal.bridge.SpecVersions.Supports(mautrix.BeeperFeatureBatchSending) {
			task := portal.bridge.DB.BackfillTask.NewWithValues(portal.PortalKey, user.MXID)
			err = task.InsertIfNotExists(ctx)
			if err != nil {
				log.Err(err).Msg("Failed to ensure backfill task exists")
			}
		}
	}
}

func (portal *Portal) deterministicEventID(msgID string, partIndex int) id.EventID {
	data := fmt.Sprintf("%s/%s", portal.MXID, msgID)
	if partIndex != 0 {
		data = fmt.Sprintf("%s/%d", data, partIndex)
	}
	sum := sha256.Sum256([]byte(data))
	return id.EventID(fmt.Sprintf("$%s:%s.com", base64.RawURLEncoding.EncodeToString(sum[:]), portal.bridge.BeeperNetworkName))
}

type BackfillPartMetadata struct {
	Intent       *appservice.IntentAPI
	MessageID    string
	OTID         int64
	Sender       int64
	PartIndex    int
	EditCount    int64
	Reactions    []*table.LSUpsertReaction
	InBatchReact *table.LSUpsertReaction
}

func (portal *Portal) handleMessageBatch(ctx context.Context, source *User, upsert *table.UpsertMessages, forward bool, lastMessage *database.Message, doneCallback func()) {
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
	log := zerolog.Ctx(ctx)
	upsert.Messages = slices.CompactFunc(upsert.Messages, func(a, b *table.WrappedMessage) bool {
		if a.MessageId == b.MessageId {
			log.Debug().
				Str("message_id", a.MessageId).
				Bool("attachment_counts_match", len(a.XMAAttachments) == len(b.XMAAttachments) && len(a.BlobAttachments) == len(b.BlobAttachments) && len(a.Stickers) == len(b.Stickers)).
				Msg("Backfill batch contained duplicate message")
			return true
		}
		return false
	})
	if lastMessage != nil {
		// For catchup backfills, delete any messages that are older than the last bridged message.
		upsert.Messages = slices.DeleteFunc(upsert.Messages, func(message *table.WrappedMessage) bool {
			return message.TimestampMs <= lastMessage.Timestamp.UnixMilli()
		})
	}
	if portal.OldestMessageTS == 0 || portal.OldestMessageTS > upsert.Range.MinTimestampMs {
		portal.OldestMessageTS = upsert.Range.MinTimestampMs
		portal.OldestMessageID = upsert.Range.MinMessageId
		portal.MoreToBackfill = upsert.Range.HasMoreBefore
		err := portal.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to save oldest message ID/timestamp in database")
		} else {
			log.Debug().
				Bool("more_to_backfill", portal.MoreToBackfill).
				Int64("oldest_message_ts", portal.OldestMessageTS).
				Str("oldest_message_id", portal.OldestMessageID).
				Msg("Saved oldest message ID/timestamp in database")
		}
	}
	if len(upsert.Messages) == 0 {
		log.Warn().Msg("Got empty batch of historical messages")
		return
	}
	log.Info().
		Int64("oldest_message_ts", upsert.Messages[0].TimestampMs).
		Str("oldest_message_id", upsert.Messages[0].MessageId).
		Int64("newest_message_ts", upsert.Messages[len(upsert.Messages)-1].TimestampMs).
		Str("newest_message_id", upsert.Messages[len(upsert.Messages)-1].MessageId).
		Int("message_count", len(upsert.Messages)).
		Bool("has_more_before", upsert.Range.HasMoreBefore).
		Msg("Handling batch of historical messages")
	if lastMessage == nil && (upsert.Messages[0].TimestampMs != upsert.Range.MinTimestampMs || upsert.Messages[0].MessageId != upsert.Range.MinMessageId) {
		log.Warn().
			Int64("min_timestamp_ms", upsert.Range.MinTimestampMs).
			Str("min_message_id", upsert.Range.MinMessageId).
			Int64("first_message_ts", upsert.Messages[0].TimestampMs).
			Str("first_message_id", upsert.Messages[0].MessageId).
			Msg("First message in batch doesn't match range")
	}
	if !forward {
		go func() {
			if doneCallback != nil {
				defer doneCallback()
			}
			portal.convertAndSendBackfill(ctx, source, upsert.Messages, upsert.MarkRead, forward)
		}()
	} else {
		if doneCallback != nil {
			defer doneCallback()
		}
		portal.convertAndSendBackfill(ctx, source, upsert.Messages, upsert.MarkRead, forward)
		queueConfig := portal.bridge.Config.Bridge.Backfill.Queue
		if lastMessage == nil && queueConfig.MaxPages != 0 && portal.bridge.SpecVersions.Supports(mautrix.BeeperFeatureBatchSending) {
			task := portal.bridge.DB.BackfillTask.NewWithValues(portal.PortalKey, source.MXID)
			err := task.Upsert(ctx)
			if err != nil {
				log.Err(err).Msg("Failed to save backfill task after initial backfill")
			} else {
				log.Debug().Msg("Saved backfill task after initial backfill")
			}
		}
	}
}

func (portal *Portal) convertAndSendBackfill(ctx context.Context, source *User, messages []*table.WrappedMessage, markRead, forward bool) {
	log := zerolog.Ctx(ctx)
	events := make([]*event.Event, 0, len(messages))
	metas := make([]*BackfillPartMetadata, 0, len(messages))
	ctx = context.WithValue(ctx, msgconvContextKeyClient, source.Client)
	if forward {
		ctx = context.WithValue(ctx, msgconvContextKeyBackfill, backfillTypeForward)
	} else {
		ctx = context.WithValue(ctx, msgconvContextKeyBackfill, backfillTypeHistorical)
	}
	sendReactionsInBatch := portal.bridge.SpecVersions.Supports(mautrix.BeeperFeatureBatchSending)
	for _, msg := range messages {
		intent := portal.bridge.GetPuppetByID(msg.SenderId).IntentFor(portal)
		if intent == nil {
			log.Warn().Int64("sender_id", msg.SenderId).Msg("Failed to get intent for sender")
			continue
		}
		ctx := context.WithValue(ctx, msgconvContextKeyIntent, intent)
		ctx = log.With().
			Str("message_id", msg.MessageId).
			Str("otid", msg.OfflineThreadingId).
			Int64("sender_id", msg.SenderId).
			Logger().WithContext(ctx)
		converted := portal.MsgConv.ToMatrix(ctx, msg)
		if portal.bridge.Config.Bridge.CaptionInMessage {
			converted.MergeCaption()
		}
		if len(converted.Parts) == 0 {
			log.Warn().Str("message_id", msg.MessageId).Msg("Message was empty after conversion")
			continue
		}
		var reactionsToSendSeparately []*table.LSUpsertReaction
		if !sendReactionsInBatch {
			reactionsToSendSeparately = msg.Reactions
		}
		for i, part := range converted.Parts {
			content := &event.Content{
				Parsed: part.Content,
				Raw:    part.Extra,
			}
			evtType, err := portal.encrypt(ctx, intent, content, part.Type)
			if err != nil {
				log.Err(err).Str("message_id", msg.MessageId).Int("part_index", i).Msg("Failed to encrypt event")
				continue
			}

			events = append(events, &event.Event{
				Sender:    intent.UserID,
				Type:      evtType,
				Timestamp: msg.TimestampMs,
				ID:        portal.deterministicEventID(msg.MessageId, i),
				RoomID:    portal.MXID,
				Content:   *content,
			})
			otid, _ := strconv.ParseInt(msg.OfflineThreadingId, 10, 64)
			metas = append(metas, &BackfillPartMetadata{
				Intent:    intent,
				MessageID: msg.MessageId,
				OTID:      otid,
				Sender:    msg.SenderId,
				PartIndex: i,
				EditCount: msg.EditCount,
				Reactions: reactionsToSendSeparately,
			})
			reactionsToSendSeparately = nil
		}
		if sendReactionsInBatch {
			reactionTargetEventID := portal.deterministicEventID(msg.MessageId, 0)
			for _, react := range msg.Reactions {
				reactSender := portal.bridge.GetPuppetByID(react.ActorId)
				events = append(events, &event.Event{
					Sender:    reactSender.IntentFor(portal).UserID,
					Type:      event.EventReaction,
					Timestamp: react.TimestampMs,
					RoomID:    portal.MXID,
					Content: event.Content{
						Parsed: &event.ReactionEventContent{
							RelatesTo: event.RelatesTo{
								Type:    event.RelAnnotation,
								EventID: reactionTargetEventID,
								Key:     variationselector.Add(react.Reaction),
							},
						},
					},
				})
				metas = append(metas, &BackfillPartMetadata{
					MessageID:    msg.MessageId,
					InBatchReact: react,
				})
			}
		}
	}
	if len(events) == 0 {
		log.Info().Msg("No events to send in backfill batch")
		return
	}
	if unreadHoursThreshold := portal.bridge.Config.Bridge.Backfill.UnreadHoursThreshold; unreadHoursThreshold > 0 && !markRead && len(messages) > 0 {
		markRead = messages[len(messages)-1].TimestampMs < time.Now().Add(-time.Duration(unreadHoursThreshold)*time.Hour).UnixMilli()
		if markRead {
			log.Debug().
				Int64("newest_timestamp_ms", messages[len(messages)-1].TimestampMs).
				Msg("Marking chat as read in backfill as it's older than the unread hours threshold")
		}
	}
	if portal.bridge.SpecVersions.Supports(mautrix.BeeperFeatureBatchSending) {
		log.Info().Int("event_count", len(events)).Msg("Sending events to Matrix using Beeper batch sending")
		portal.sendBackfillBeeper(ctx, source, events, metas, markRead, forward)
	} else {
		log.Info().Int("event_count", len(events)).Msg("Sending events to Matrix one by one")
		portal.sendBackfillLegacy(ctx, source, events, metas, markRead)
	}
	log.Info().Msg("Finished sending backfill batch")
}

func (portal *Portal) sendBackfillLegacy(ctx context.Context, source *User, events []*event.Event, metas []*BackfillPartMetadata, markRead bool) {
	var lastEventID id.EventID
	for i, evt := range events {
		resp, err := portal.sendMatrixEvent(ctx, metas[i].Intent, evt.Type, evt.Content.Parsed, evt.Content.Raw, evt.Timestamp)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Int("evt_index", i).Msg("Failed to send event")
		} else {
			portal.storeMessageInDB(ctx, resp.EventID, metas[i].MessageID, metas[i].OTID, metas[i].Sender, time.UnixMilli(evt.Timestamp), metas[i].PartIndex)
			lastEventID = resp.EventID
		}
		for _, react := range metas[i].Reactions {
			portal.handleMetaReaction(react)
		}
	}
	if markRead && lastEventID != "" {
		puppet := portal.bridge.GetPuppetByCustomMXID(source.MXID)
		if puppet != nil {
			err := portal.SendReadReceipt(ctx, puppet, lastEventID)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to send read receipt after backfill")
			}
		}
	}
}

func (portal *Portal) sendBackfillBeeper(ctx context.Context, source *User, events []*event.Event, metas []*BackfillPartMetadata, markRead, forward bool) {
	var markReadBy id.UserID
	if markRead && forward {
		markReadBy = source.MXID
	}
	resp, err := portal.MainIntent().BeeperBatchSend(ctx, portal.MXID, &mautrix.ReqBeeperBatchSend{
		Forward:          forward,
		SendNotification: forward && !markRead,
		MarkReadBy:       markReadBy,
		Events:           events,
	})
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to send backfill batch")
		return
	} else if len(resp.EventIDs) != len(metas) {
		zerolog.Ctx(ctx).Error().
			Int("event_count", len(events)).
			Int("meta_count", len(metas)).
			Msg("Got wrong number of event IDs for backfill batch")
		return
	}
	dbMessages := make([]*database.Message, 0, len(events))
	dbReactions := make([]*database.Reaction, 0)
	for i, evtID := range resp.EventIDs {
		meta := metas[i]
		if meta.InBatchReact != nil {
			dbReactions = append(dbReactions, &database.Reaction{
				MessageID: meta.MessageID,
				Sender:    meta.InBatchReact.ActorId,
				Emoji:     meta.InBatchReact.Reaction,
				MXID:      evtID,
			})
		} else {
			dbMessages = append(dbMessages, &database.Message{
				ID:        meta.MessageID,
				PartIndex: meta.PartIndex,
				Sender:    meta.Sender,
				OTID:      meta.OTID,
				MXID:      evtID,
				Timestamp: time.UnixMilli(events[i].Timestamp),
				EditCount: meta.EditCount,
			})
		}
	}
	err = portal.bridge.DB.Message.BulkInsert(ctx, portal.PortalKey, portal.MXID, dbMessages)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to save backfill batch messages to database")
	}
	err = portal.bridge.DB.Reaction.BulkInsert(ctx, portal.PortalKey, portal.MXID, dbReactions)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to save backfill batch reactions to database")
	}
}
