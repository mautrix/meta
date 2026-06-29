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
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
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
		err := ic.Main.DB.PutIGSeqID(ctx, ic.UserLogin.ID, mailbox.UQSeqID, startTS)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save reconnection state after mailbox processing")
		}
	}()
}
