// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
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

package igconnector

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exerrors"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	chatBackfillSlotsOnce sync.Once
	chatBackfillSlots     chan struct{}
)

func (ic *IGClient) processMailbox(ctx, retryCtx context.Context, mailbox *slidetypes.Mailbox) {
	ic.waitMailboxProcessed = make(chan struct{})
	ic.mailboxProcessed.Store(false)
	done := func() {
		ic.mailboxProcessed.Store(true)
		close(ic.waitMailboxProcessed)
	}
	startTS := time.Now()

	events := make([]bridgev2.RemoteEvent, 0, len(mailbox.ThreadsByFolder.Edges))
	for _, node := range mailbox.ThreadsByFolder.Edges {
		if retryCtx.Err() != nil {
			done()
			return
		}
		err := ic.saveThreadMappings(ctx, node.Node.AsIGDirectThread)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save thread mappings")
		}
		events = append(events, ic.wrapChatResync(node.Node.AsIGDirectThread, false, "mailbox"))
	}
	if retryCtx.Err() != nil {
		done()
		return
	}
	resyncSince := ic.resyncChatsSince.Swap(nil)
	go func() {
		defer done()

		backfilling := ic.updateChatBackfillState(ctx, mailbox, resyncSince != nil)
		for _, evt := range events {
			if ctx.Err() != nil {
				return
			}
			res := ic.UserLogin.QueueRemoteEvent(evt)
			if !res.Success {
				zerolog.Ctx(ctx).Err(res.Error).Msg("Failed to queue remote event for mailbox sync")
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		edges := mailbox.ThreadsByFolder.Edges
		if resyncSince != nil && !backfilling && !ic.Main.Bridge.Background {
			var inboxCursor string
			if mailbox.ThreadsByFolder.PageInfo.HasNextPage && len(edges) > 0 &&
				!edges[len(edges)-1].Node.AsIGDirectThread.LastActivityTimestampMS.Before(*resyncSince) {
				zerolog.Ctx(ctx).Info().
					Time("since", *resyncSince).
					Msg("First inbox page doesn't cover time since last seq ID, resyncing more chats")
				inboxCursor = mailbox.ThreadsByFolder.PageInfo.EndCursor
			}
			go ic.doChatBackfill(ctx, inboxCursor, *resyncSince)
		}
		err := ic.Main.DB.PutIGSeqID(ctx, ic.UserLogin.ID, mailbox.UQSeqID, startTS)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save reconnection state after mailbox processing")
		}
	}()
}

func (ic *IGClient) updateChatBackfillState(ctx context.Context, mailbox *slidetypes.Mailbox, resnapshot bool) (backfilling bool) {
	ic.chatBackfillLock.Lock()
	defer ic.chatBackfillLock.Unlock()
	oldCursor, oldCompleted := ic.LoginMeta.ThreadBackfillCursor, ic.LoginMeta.BackfillCompleted
	if resnapshot {
		ic.LoginMeta.ThreadBackfillCursor = ""
	}
	if !ic.LoginMeta.BackfillCompleted && !ic.Main.Bridge.Background {
		if !mailbox.ThreadsByFolder.PageInfo.HasNextPage {
			zerolog.Ctx(ctx).Info().Msg("Only one page in inbox, marking backfill as completed")
			ic.LoginMeta.BackfillCompleted = true
		} else if ic.Main.Config.ThreadBackfill.Enabled() {
			backfilling = true
			if ic.LoginMeta.ThreadBackfillCursor == "" {
				ic.LoginMeta.ThreadBackfillCursor = mailbox.ThreadsByFolder.PageInfo.EndCursor
			}
		}
	}
	if ic.LoginMeta.ThreadBackfillCursor != oldCursor || ic.LoginMeta.BackfillCompleted != oldCompleted {
		err := ic.UserLogin.Save(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save chat backfill state after mailbox processing")
		}
	}
	return
}

func (ic *IGConnector) waitForChatBackfillSlot(ctx context.Context) (func(), error) {
	limit := ic.Config.ThreadBackfill.MaxConcurrent
	if limit <= 0 {
		return func() {}, nil
	}
	chatBackfillSlotsOnce.Do(func() {
		chatBackfillSlots = make(chan struct{}, limit)
	})
	select {
	case chatBackfillSlots <- struct{}{}:
		return func() { <-chatBackfillSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (ic *IGClient) doChatBackfill(ctx context.Context, cursor string, since time.Time) {
	defer func() {
		if v := recover(); v != nil {
			zerolog.Ctx(ctx).Err(exerrors.RecoverToError(v)).
				Bytes(zerolog.ErrorStackFieldName, debug.Stack()).
				Msg("Panic in chat backfill")
		}
	}()
	viewerFBID := metaid.ParseUserLoginID(ic.UserLogin.ID)
	if viewerFBID == 0 {
		zerolog.Ctx(ctx).Warn().Msg("No viewer FBID for chat backfill")
		return
	}
	ic.chatBackfillLock.Lock()
	defer ic.chatBackfillLock.Unlock()
	if since.IsZero() {
		if ic.LoginMeta.PendingBackfillCompleted {
			return
		}
		cursor = ic.LoginMeta.ThreadBackfillCursor
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ic.stopChatBackfill.Store(&cancel)
	defer ic.stopChatBackfill.Store(nil)

	log := zerolog.Ctx(ctx).With().Str("action", "chat backfill").Logger()
	ctx = log.WithContext(ctx)

	if since.IsZero() && !ic.newLogin {
		release, err := ic.Main.waitForChatBackfillSlot(ctx)
		if err != nil {
			return
		}
		defer release()
	}

	processThread := func(thread *slidetypes.ThreadInfo) error {
		err := ic.saveThreadMappings(ctx, thread)
		if err != nil {
			return fmt.Errorf("failed to save thread mappings: %w", err)
		}
		source := "backfill"
		if !since.IsZero() {
			source = "resnapshot"
		}
		res := ic.UserLogin.QueueRemoteEvent(ic.wrapChatResync(thread, false, source))
		if !res.Success {
			return res.Error
		}
		return nil
	}
	hasMore := true
	folder := "INBOX"
	processThreads := func(threads slidetypes.Edged[slidetypes.Node[slidetypes.WrappedThreadInfo]]) bool {
		cursor = threads.PageInfo.EndCursor
		hasMore = threads.PageInfo.HasNextPage
		for _, edge := range threads.Edges {
			if edge.Node.AsIGDirectThread == nil {
				log.Warn().Str("thread_igid", edge.Node.ID).Msg("Skipping null thread in chat list")
				continue
			}
			if edge.Node.AsIGDirectThread.Users == nil {
				log.Warn().Str("thread_igid", edge.Node.AsIGDirectThread.ID).Msg("Skipping thread without users in chat list")
				continue
			}
			if !since.IsZero() && edge.Node.AsIGDirectThread.LastActivityTimestampMS.Before(since) {
				hasMore = false
				return true
			}
			if err := processThread(edge.Node.AsIGDirectThread); err != nil {
				log.Err(err).Str("thread_igid", edge.Node.AsIGDirectThread.ID).Msg("Failed to process thread")
				return false
			}
		}
		if ctx.Err() != nil {
			return false
		} else if since.IsZero() {
			ic.LoginMeta.ThreadBackfillCursor = cursor
			if err := ic.UserLogin.Save(ctx); err != nil {
				log.Err(err).Msg("Failed to save chat backfill cursor")
			}
		}
		return true
	}
	fetchPage := func() bool {
		select {
		case <-time.After(ic.Main.Config.ThreadBackfill.BatchDelay):
		case <-ctx.Done():
			return false
		}
		resp, err := ic.Client.PaginateMailbox(ctx, slidetypes.MakePaginateMailboxRequest(viewerFBID, cursor, folder, nil))
		if err != nil && resp != nil && resp.Mailbox != nil {
			log.Warn().Err(err).Str("folder", folder).Msg("Chat list page contained errors, ignoring")
			err = nil
		}
		if err != nil {
			log.Err(err).Str("folder", folder).Msg("Failed to fetch more chats")
			return false
		} else if resp.Mailbox == nil {
			log.Warn().Str("folder", folder).Msg("Pagination response didn't contain a mailbox")
			return false
		}
		return processThreads(resp.Mailbox.ThreadsByFolder)
	}

	batchCount := 0
	maxBatches := ic.Main.Config.ThreadBackfill.BatchCount
	pendingCursor := ""
	if since.IsZero() && ic.LoginMeta.BackfillCompleted {
		pendingCursor = cursor
	} else if since.IsZero() || cursor != "" {
		if cursor == "" {
			log.Info().Msg("No start cursor, loading from scratch")
			resp, err := ic.Client.GetMailbox(ctx)
			if err != nil {
				log.Err(err).Msg("Failed to fetch initial inbox")
				return
			} else if resp.Mailbox == nil {
				log.Warn().Msg("Initial inbox response didn't contain a mailbox")
				return
			}
			if !processThreads(resp.Mailbox.ThreadsByFolder) {
				return
			}
		} else {
			batchCount++
		}
		for hasMore && cursor != "" && (!since.IsZero() || maxBatches < 0 || batchCount < maxBatches) {
			if !fetchPage() {
				return
			}
			batchCount++
		}
		if since.IsZero() {
			log.Info().Int("total_batch_count", batchCount).Msg("Completed inbox chat backfill, syncing message requests")
			ic.LoginMeta.BackfillCompleted = true
			ic.LoginMeta.ThreadBackfillCursor = ""
			if err := ic.UserLogin.Save(ctx); err != nil {
				log.Err(err).Msg("Failed to save user login after inbox chat backfill")
			}
		}
	}
	folder, cursor, hasMore = "PENDING", pendingCursor, true
	for pendingBatches := 0; hasMore && (!since.IsZero() || maxBatches < 0 || pendingBatches < maxBatches); pendingBatches++ {
		if !fetchPage() {
			return
		}
		batchCount++
		if cursor == "" {
			break
		}
	}
	if !since.IsZero() {
		log.Info().Int("total_batch_count", batchCount).Msg("Completed resyncing chats after resnapshot")
		return
	}
	log.Info().Int("total_batch_count", batchCount).Msg("Completed chat backfill successfully")
	ic.LoginMeta.PendingBackfillCompleted = true
	ic.LoginMeta.ThreadBackfillCursor = ""
	err := ic.UserLogin.Save(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to save user login after chat backfill")
	}
}
