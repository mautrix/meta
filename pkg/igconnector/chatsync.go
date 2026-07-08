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
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
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
		events = append(events, ic.wrapChatResync(node.Node.AsIGDirectThread))
	}
	if retryCtx.Err() != nil {
		done()
		return
	}
	go func() {
		defer done()

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
		if !ic.LoginMeta.BackfillCompleted && !ic.Main.Bridge.Background {
			if !mailbox.ThreadsByFolder.PageInfo.HasNextPage {
				zerolog.Ctx(ctx).Info().Msg("Only one page in inbox, marking backfill as completed")
				ic.LoginMeta.BackfillCompleted = true
				err := ic.UserLogin.Save(ctx)
				if err != nil {
					zerolog.Ctx(ctx).Err(err).Msg("Failed to save user login after mailbox processing")
				}
			} else if ic.Main.Config.ThreadBackfill.Enabled() {
				go ic.doChatBackfill(ctx, mailbox.ThreadsByFolder.PageInfo.EndCursor)
			}
		}
		err := ic.Main.DB.PutIGSeqID(ctx, ic.UserLogin.ID, mailbox.UQSeqID, startTS)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save reconnection state after mailbox processing")
		}
	}()
}

func (ic *IGClient) doChatBackfill(ctx context.Context, startCursor string) {
	viewerFBID := metaid.ParseUserLoginID(ic.UserLogin.ID)
	if viewerFBID == 0 {
		zerolog.Ctx(ctx).Warn().Msg("No viewer FBID for chat backfill")
		return
	}
	if !ic.chatBackfillLock.TryLock() {
		zerolog.Ctx(ctx).Debug().Msg("Chat backfill already in progress, not starting new backfill")
		return
	}
	defer ic.chatBackfillLock.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ic.stopChatBackfill.Store(&cancel)
	defer ic.stopChatBackfill.Store(nil)

	log := zerolog.Ctx(ctx).With().Str("action", "chat backfill").Logger()
	ctx = log.WithContext(ctx)

	processThread := func(thread *slidetypes.ThreadInfo) error {
		err := ic.saveThreadMappings(ctx, thread)
		if err != nil {
			return fmt.Errorf("failed to save thread mappings: %w", err)
		}
		res := ic.UserLogin.QueueRemoteEvent(ic.wrapChatResync(thread))
		if !res.Success {
			return res.Error
		}
		return nil
	}
	processThreads := func(threads slidetypes.Edged[slidetypes.Node[slidetypes.WrappedThreadInfo]]) bool {
		for _, edge := range threads.Edges {
			if err := processThread(edge.Node.AsIGDirectThread); err != nil {
				log.Err(err).Str("thread_igid", edge.Node.AsIGDirectThread.ID).Msg("Failed to process thread")
				return false
			}
		}
		return true
	}

	batchCount := 0
	if startCursor == "" {
		log.Info().Msg("No start cursor, loading from scratch")
		resp, err := ic.Client.GetMailbox(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to fetch initial inbox")
			return
		}
		if !processThreads(resp.Mailbox.ThreadsByFolder) {
			return
		}
	} else {
		batchCount++
	}
	for batchCount < ic.Main.Config.ThreadBackfill.BatchCount {
		select {
		case <-time.After(ic.Main.Config.ThreadBackfill.BatchDelay):
		case <-ctx.Done():
			return
		}
		resp, err := ic.Client.PaginateMailbox(ctx, slidetypes.MakePaginateMailboxRequest(viewerFBID, startCursor, "INBOX", nil))
		if err != nil {
			log.Err(err).Msg("Failed to fetch more chats")
			return
		}
		if !processThreads(resp.Mailbox.ThreadsByFolder) {
			return
		}
		batchCount++
		if !resp.Mailbox.ThreadsByFolder.PageInfo.HasNextPage {
			break
		}
	}
	log.Info().Int("total_batch_count", batchCount).Msg("Completed chat backfill successfully")
	ic.LoginMeta.BackfillCompleted = true
	err := ic.UserLogin.Save(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to save user login after chat backfill")
	}
}
