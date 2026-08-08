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
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metadb"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var _ bridgev2.BackfillingNetworkAPI = (*IGClient)(nil)

func (ic *IGClient) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	meta, err := ic.ensureIGID(ctx, params.Portal)
	if err != nil {
		return nil, err
	}
	if params.Forward {
		return ic.fetchForwardBackfill(ctx, params, meta)
	}
	return ic.fetchBackwardBackfill(ctx, params, meta)
}

func (ic *IGClient) fetchForwardBackfill(ctx context.Context, params bridgev2.FetchMessagesParams, meta *metaid.PortalMetadata) (*bridgev2.FetchMessagesResponse, error) {
	thread, ok := params.BundledData.(*slidetypes.ThreadInfo)
	if !ok {
		zerolog.Ctx(ctx).Debug().Msg("Fetching thread for forward backfill request...")
		resp, err := ic.Client.GetThread(ctx, slidetypes.MakeGetThreadInfoRequest(meta.IGID))
		if err != nil {
			return nil, fmt.Errorf("failed to get thread info: %w", err)
		}
		zerolog.Ctx(ctx).Trace().
			Any("thread_response", resp.ThreadInfo.AsIGDirectThread).
			Msg("Response for initial thread fetch")
		thread = resp.ThreadInfo.AsIGDirectThread
		params.Portal.UpdateInfo(ctx, ic.wrapChatInfo(thread), ic.UserLogin, nil, time.Time{})
	}
	canBackwardsBackfill := params.AnchorMessage == nil && ic.Main.Bridge.Config.Backfill.Queue.AnyEnabled()
	needsMore := len(thread.SlideMessages.Edges) < params.Count &&
		thread.SlideMessages.PageInfo.HasNextPage && !canBackwardsBackfill
	cursor := thread.SlideMessages.PageInfo.EndCursor
	var anchorID string
	var anchorTS time.Time
	if params.AnchorMessage != nil {
		rawParsed := metaid.ParseMessageID(params.AnchorMessage.ID)
		parsed, ok := rawParsed.(metaid.ParsedFBMessageID)
		if !ok {
			return nil, fmt.Errorf("unexpected message ID type %T", rawParsed)
		}
		anchorID = parsed.ID
		anchorTS = params.AnchorMessage.Timestamp
	}
	foundAnchor := false
	var messages []slidetypes.Node[*slidetypes.Message]
	appendMessages := func(t *slidetypes.SlideMessages) {
		if anchorID != "" {
			for i, msg := range t.Edges {
				if msg.Node.ID == anchorID || msg.Node.TimestampMS.Before(anchorTS) {
					foundAnchor = true
					t.Edges = t.Edges[:i]
					break
				}
			}
		}
		messages = append(messages, t.Edges...)
		cursor = t.PageInfo.EndCursor
		needsMore = len(messages) < params.Count && t.PageInfo.HasNextPage
	}
	appendMessages(thread.SlideMessages)
	for needsMore && !foundAnchor && !canBackwardsBackfill {
		zerolog.Ctx(ctx).Debug().
			Int("collected_count", len(messages)).
			Int("limit", params.Count).
			Str("cursor", cursor).
			Msg("Fetching additional batch of messages for forward backfill")
		resp, err := ic.Client.PaginateMessages(ctx, &slidetypes.PaginateMessagesRequest{
			AfterCursor: &cursor,
			ThreadID:    meta.IGID,
			FirstN:      20,

			InitialMessagePageCount: 20,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to paginate messages: %w", err)
		}
		zerolog.Ctx(ctx).Trace().
			Any("paginate_response", resp.ThreadInfo.AsIGDirectThread.Messages).
			Msg("Response for pagination")
		appendMessages(resp.ThreadInfo.AsIGDirectThread.Messages)
	}
	var markRead bool
	for _, rr := range thread.SlideReadReceipts {
		if metaid.MakeUserLoginID(rr.ParticipantFBID) == ic.UserLogin.ID {
			markRead = !rr.WatermarkTimestampMS.Before(thread.LastActivityTimestampMS.Time)
			break
		}
	}
	if thread.MarkedAsUnread {
		markRead = false
	}
	return &bridgev2.FetchMessagesResponse{
		Messages: ic.wrapBackfillMessages(ctx, params.Portal, messages),
		Forward:  true,
		MarkRead: markRead,
	}, ctx.Err()
}

const BackfillCursorPrefix = "ig:"

func (ic *IGClient) fetchBackwardBackfill(ctx context.Context, params bridgev2.FetchMessagesParams, meta *metaid.PortalMetadata) (*bridgev2.FetchMessagesResponse, error) {
	cursorVal, ok := strings.CutPrefix(string(params.Cursor), BackfillCursorPrefix)
	var beforeMessageID *string
	cursor := &cursorVal
	if !ok || cursorVal == "" {
		cursor = nil
		if params.AnchorMessage == nil {
			// TODO is it possible to hit this for non-empty chats?
			zerolog.Ctx(ctx).Debug().Msg("Returning empty response to backwards backfill request with no cursor nor anchor")
			return &bridgev2.FetchMessagesResponse{}, nil
		}
		rawParsed := metaid.ParseMessageID(params.AnchorMessage.ID)
		parsed, ok := rawParsed.(metaid.ParsedFBMessageID)
		if !ok {
			return nil, fmt.Errorf("unexpected message ID type %T", rawParsed)
		}
		beforeMessageID = &parsed.ID
	}
	hasMore := true
	var messages []slidetypes.Node[*slidetypes.Message]
	for hasMore && len(messages) < params.Count {
		resp, err := ic.Client.PaginateMessages(ctx, &slidetypes.PaginateMessagesRequest{
			AfterCursor:        cursor,
			OlderThanMessageID: beforeMessageID,
			ThreadID:           meta.IGID,
			FirstN:             20,

			InitialMessagePageCount: 20,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to paginate messages: %w", err)
		}
		zerolog.Ctx(ctx).Trace().
			Any("paginate_response", resp.ThreadInfo.AsIGDirectThread.Messages).
			Msg("Response for backwards pagination")
		messages = append(messages, resp.ThreadInfo.AsIGDirectThread.Messages.Edges...)
		beforeMessageID = nil
		cursor = &resp.ThreadInfo.AsIGDirectThread.Messages.PageInfo.EndCursor
		hasMore = resp.ThreadInfo.AsIGDirectThread.Messages.PageInfo.HasNextPage
	}
	if len(messages) == 0 || cursor == nil {
		return &bridgev2.FetchMessagesResponse{}, nil
	}
	return &bridgev2.FetchMessagesResponse{
		Messages: ic.wrapBackfillMessages(ctx, params.Portal, messages),
		Cursor:   networkid.PaginationCursor(BackfillCursorPrefix + *cursor),
		HasMore:  hasMore,
	}, ctx.Err()
}

func (ic *IGClient) wrapBackfillMessages(
	ctx context.Context, portal *bridgev2.Portal, messages []slidetypes.Node[*slidetypes.Message],
) []*bridgev2.BackfillMessage {
	// Instagram returns messages newest to oldest, bridgev2 wants oldest to newest
	slices.Reverse(messages)
	out := make([]*bridgev2.BackfillMessage, len(messages))
	var reactions []*metadb.IGReactionEntry
	for i, msg := range messages {
		if ctx.Err() != nil {
			return nil
		}
		msgID := metaid.MakeFBMessageID(msg.Node.ID)
		sender := ic.makeEventSender(msg.Node.SenderFBID)
		intent, ok := portal.GetIntentFor(ctx, sender, ic.UserLogin, bridgev2.RemoteEventBackfill)
		if !ok {
			intent = ic.Main.Bridge.Bot
		}
		out[i] = &bridgev2.BackfillMessage{
			ConvertedMessage: ic.Main.MsgConv.ToMatrix(
				ctx, portal, ic.Client, ic.UserLogin, intent, msgID, msg.Node,
				ic.Main.Config.DisableXMABackfill || ic.Main.Config.DisableXMAAlways,
			),
			Sender:      sender,
			ID:          msgID,
			Timestamp:   msg.Node.TimestampMS.Time,
			StreamOrder: msg.Node.TimestampMS.UnixMilli(),
			Reactions:   make([]*bridgev2.BackfillReaction, len(msg.Node.Reactions)),
			//TxnID: msg.Node.OfflineThreadingID,
		}
		for j, react := range msg.Node.Reactions {
			out[i].Reactions[j] = &bridgev2.BackfillReaction{
				Timestamp: react.ReactionTimestampMS.Time,
				Sender:    ic.makeEventSender(react.SenderFBID),
				Emoji:     react.Reaction,
			}
			if react.LogMessageID != "" {
				reactions = append(reactions, &metadb.IGReactionEntry{
					TargetMsgID:   msg.Node.ID,
					Sender:        react.SenderFBID,
					ReactionMsgID: react.LogMessageID,
				})
			}
		}
	}
	err := ic.Main.DB.PutManyIGReactions(ctx, portal.PortalKey, reactions)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to store IG reaction mappings from backfill")
	}
	return out
}
