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

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	DGWConnectionError         status.BridgeStateErrorCode = "ig-dgw-connection-error"
	MetaConnectionUnauthorized status.BridgeStateErrorCode = "meta-connection-unauthorized"
	MetaCookieRemoved          status.BridgeStateErrorCode = "meta-cookie-removed"
	MetaUserIDIsZero           status.BridgeStateErrorCode = "meta-user-id-is-zero"
	MetaRedirectedToLoginPage  status.BridgeStateErrorCode = "meta-redirected-to-login"
	MetaNotLoggedIn            status.BridgeStateErrorCode = "meta-not-logged-in"
	MetaConnectError           status.BridgeStateErrorCode = "meta-connect-error"
	MetaGraphQLError           status.BridgeStateErrorCode = "meta-graphql-error"
	IGChallengeRequired        status.BridgeStateErrorCode = "ig-challenge-required"
	IGAccountSuspended         status.BridgeStateErrorCode = "ig-account-suspended"
	IGConsentRequired          status.BridgeStateErrorCode = "ig-consent-required"
	FBCheckpointRequired       status.BridgeStateErrorCode = "fb-checkpoint-required"
	MetaProxyUpdateFail        status.BridgeStateErrorCode = "meta-proxy-update-fail"
)

func init() {
	status.BridgeStateHumanErrors.Update(status.BridgeStateErrorMap{
		DGWConnectionError:         "Disconnected from server, trying to reconnect",
		MetaConnectionUnauthorized: "Logged out, please relogin to continue",
		MetaCookieRemoved:          "Logged out, please relogin to continue",
		MetaUserIDIsZero:           "Logged out, please relogin to continue",
		MetaRedirectedToLoginPage:  "Logged out, please relogin to continue",
		MetaNotLoggedIn:            "Logged out, please relogin to continue",
		IGAccountSuspended:         "Logged out, please check the Instagram website to continue",
		IGChallengeRequired:        "Challenge required, please check the Instagram website to continue",
		IGConsentRequired:          "Consent required, please check the Instagram website to continue",
		FBCheckpointRequired:       "Checkpoint required, please check the Facebook website to continue",
		MetaConnectError:           "Unknown connection error",
		MetaProxyUpdateFail:        "Failed to update proxy",
	})
}

func (ic *IGClient) saveReconnectionState(ctx context.Context) error {
	state, err := ic.Client.DumpState()
	if err != nil {
		return fmt.Errorf("failed to dump state for seqid update: %w", err)
	}
	err = ic.Main.DB.PutReconnectionState(ctx, ic.UserLogin.ID, state)
	if err != nil {
		return fmt.Errorf("failed to save reconnection state for seqid update: %w", err)
	}
	return nil
}

func (ic *IGClient) handleIGEvent(ctx context.Context, rawEvt slidetypes.ClientEvent) error {
	if !ic.mailboxProcessed.Load() {
		zerolog.Ctx(ctx).Warn().Msg("Blocking new event handling until mailbox is processed")
		select {
		case <-ic.waitMailboxProcessed:
			zerolog.Ctx(ctx).Warn().Msg("Mailbox processed, unblocking new event handling")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	switch evt := rawEvt.(type) {
	case *slidetypes.Connected:
		ic.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
		return nil
	case *slidetypes.Disconnected:
		ic.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      DGWConnectionError,
			Info: map[string]any{
				"go_error": evt.Error.Error(),
			},
		})
		return nil
	case *slidetypes.SeqIDUpdate:
		return ic.saveReconnectionState(ctx)
	case *slidetypes.ResnapshotRequired:
		go ic.FullReconnect()
		return nil
	case *slidetypes.Delta:
		return ic.handleDelta(ctx, evt)
	default:
		return fmt.Errorf("unrecognized event type: %T", rawEvt)
	}
}

func (ic *IGClient) wrapChatResync(thread *slidetypes.ThreadInfo) *simplevent.ChatResync {
	return &simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:         bridgev2.RemoteEventChatResync,
			PortalKey:    ic.makePortalKey(thread.ThreadKey, thread.IsGroup),
			CreatePortal: true,
		},
		ChatInfo: ic.wrapChatInfo(thread),
		// TODO use CheckNeedsBackfillFunc instead?
		LatestMessageTS:     thread.LastActivityTimestampMS.Time,
		BundledBackfillData: thread,
	}
}

func (ic *IGClient) getAndResyncThread(ctx context.Context, threadIGID string) (networkid.PortalKey, error) {
	resp, err := ic.Client.GetThread(ctx, slidetypes.MakeGetThreadInfoRequest(threadIGID))
	if err != nil {
		return networkid.PortalKey{}, fmt.Errorf("failed to get thread info for %s: %w", threadIGID, err)
	}
	// This should be done by extra updates in the chat resync anyway, but do it here just to be safe
	err = ic.Main.DB.PutFBIDForIGChat(ctx, resp.ThreadInfo.AsIGDirectThread.ID, resp.ThreadInfo.AsIGDirectThread.ThreadKey)
	if err != nil {
		return networkid.PortalKey{}, fmt.Errorf("failed to save FBID for IG thread %s: %w", threadIGID, err)
	}
	evt := ic.wrapChatResync(resp.ThreadInfo.AsIGDirectThread)
	ic.UserLogin.QueueRemoteEvent(evt)
	return evt.PortalKey, nil
}

func (ic *IGClient) ensurePortal(ctx context.Context, threadIGID string) (networkid.PortalKey, error) {
	if threadIGID == "" {
		return networkid.PortalKey{}, nil
	} else if fbid, err := ic.Main.DB.GetFBIDForIGChat(ctx, threadIGID); err != nil {
		return networkid.PortalKey{}, err
	} else if fbid == 0 {
		return ic.getAndResyncThread(ctx, threadIGID)
	} else if portal, err := ic.Main.Bridge.GetExistingPortalByKey(ctx, ic.makeUncertainPortalKey(fbid)); err != nil {
		return networkid.PortalKey{}, fmt.Errorf("failed to get existing portal for thread %s/%d: %w", threadIGID, fbid, err)
	} else if portal == nil {
		return ic.getAndResyncThread(ctx, threadIGID)
	} else {
		return portal.PortalKey, nil
	}
}

func (ic *IGClient) handleDelta(ctx context.Context, d *slidetypes.Delta) error {
	defer func() {
		v := recover()
		if v != nil {
			err, ok := v.(error)
			if !ok {
				err = fmt.Errorf("%v", v)
			}
			stack := debug.Stack()
			zerolog.Ctx(ctx).Err(err).
				Bytes(zerolog.ErrorStackFieldName, stack).
				Msg("Panic in delta handler")
			ic.UserLogin.TrackAnalytics("Bridge Event Handler Panic", map[string]any{
				"error":        err.Error(),
				"stack":        string(stack),
				"handler_type": "*slidetypes.Delta",
			})
		}
	}()
	log := zerolog.Ctx(ctx)
	log.Trace().Any("event_data", d.Data).Msg("Handling delta")

	portalKey, err := ic.ensurePortal(ctx, d.ThreadFBID)
	if err != nil {
		return fmt.Errorf("failed to ensure portal for thread %s: %w", d.ThreadFBID, err)
	}

	var res bridgev2.EventHandlingResult
	switch evt := d.Data.(type) {
	case *slidetypes.NewMessageEvent:
		res = ic.UserLogin.QueueRemoteEvent(&simplevent.Message[*slidetypes.Message]{
			EventMeta: simplevent.EventMeta{
				Type:         bridgev2.RemoteEventMessage,
				PortalKey:    portalKey,
				Sender:       ic.makeEventSender(evt.Message.Sender.UserDict.InteropMessagingUserFBID),
				CreatePortal: true,
				Timestamp:    evt.Message.TimestampMS.Time,
				StreamOrder:  evt.Message.TimestampMS.UnixMilli(),
			},
			//TransactionID:      evt.Message.OfflineThreadingID,
			Data: &evt.Message,
			ID:   metaid.MakeFBMessageID(evt.Message.ID),
			ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, data *slidetypes.Message) (*bridgev2.ConvertedMessage, error) {
				return &bridgev2.ConvertedMessage{
					Parts: []*bridgev2.ConvertedMessagePart{{
						Type: event.EventMessage,
						Content: &event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    data.TextBody,
						},
					}},
				}, nil
			},
		})
	case *slidetypes.CreateReactionEvent:
		// TODO implement
		return nil
	case *slidetypes.MarkReadEvent:
		res = ic.UserLogin.QueueRemoteEvent(&simplevent.Receipt{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventReadReceipt,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Time("read_up_to", evt.ReadTimestampMS.Time)
				},
				PortalKey: portalKey,
				Sender:    ic.selfEventSender(),
			},
			ReadUpTo: evt.ReadTimestampMS.Time,
		})
	case slidetypes.UnknownEvent:
		log.Warn().
			Str("typename", d.TypeName).
			Str("thread_fbid", d.ThreadFBID).
			Msg("Unrecognized event type in socket")
		return nil
	default:
		return nil
		//return fmt.Errorf("unrecognized event type: %T", d.Data)
	}
	if !res.Success {
		return res.Error
	}
	return nil
}
