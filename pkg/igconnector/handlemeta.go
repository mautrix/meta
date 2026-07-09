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
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
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
	MetaNotInstagram           status.BridgeStateErrorCode = "meta-not-instagram-account"
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
		MetaNotInstagram:           "Non-Instagram login present on Instagram-only bridge",
	})
}

func (ic *IGClient) doWaitMailboxProcessed(ctx context.Context) error {
	if !ic.mailboxProcessed.Load() {
		zerolog.Ctx(ctx).Warn().Msg("Blocking new event handling until mailbox is processed")
		select {
		case <-ic.waitMailboxProcessed:
			zerolog.Ctx(ctx).Warn().Msg("Mailbox processed, unblocking new event handling")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (ic *IGClient) handleIGEvent(ctx context.Context, rawEvt slidetypes.ClientEvent) error {
	switch evt := rawEvt.(type) {
	case *slidetypes.Connected:
		ic.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
		if evt.SubscribedSeqID >= evt.LatestSeqID {
			ic.catchingUpTo = 0
			go func() {
				_ = ic.doWaitMailboxProcessed(ctx)
				ic.caughtUp.Set()
			}()
		} else {
			ic.catchingUpTo = evt.LatestSeqID
		}
		if !ic.LoginMeta.BackfillCompleted && !ic.Main.Bridge.Background && ic.Main.Config.ThreadBackfill.Enabled() {
			go ic.doChatBackfill(ctx, "")
		}
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
		_ = ic.doWaitMailboxProcessed(ctx)
		err := ic.Main.DB.PutIGSeqID(ctx, ic.UserLogin.ID, evt.SeqID, evt.Timestamp)
		if err != nil {
			return err
		}
		if c := ic.catchingUpTo; c > 0 && evt.SeqID >= c {
			ic.caughtUp.Set()
		}
		return nil
	case *slidetypes.ReconnectionStateUpdate:
		return ic.Main.DB.PutReconnectionState(ctx, ic.UserLogin.ID, evt.State)
	case *slidetypes.ResnapshotRequired:
		_ = ic.doWaitMailboxProcessed(ctx)
		go ic.FullReconnect(true)
		return nil
	case *slidetypes.Delta:
		if err := ic.doWaitMailboxProcessed(ctx); err != nil {
			return err
		}
		return ic.handleDelta(ctx, evt)
	case *slidetypes.TypingNotification:
		return ic.handleTyping(ctx, evt)
	default:
		return fmt.Errorf("unrecognized event type: %T", rawEvt)
	}
}

func (ic *IGClient) wrapChatResync(thread *slidetypes.ThreadInfo, useBundle bool) *simplevent.ChatResync {
	var bundle any
	// Fetching the entire inbox only returns stub messages which can't be used safely for backfilling.
	// Let FetchMessages fetch the full thread info in such cases (which happens after checking if backfill is needed).
	if useBundle {
		bundle = thread
	}
	return &simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:         bridgev2.RemoteEventChatResync,
			PortalKey:    ic.makePortalKey(thread.ThreadKey, thread.IsGroup),
			CreatePortal: true,
		},
		ChatInfo:            ic.wrapChatInfo(thread),
		LatestMessageTS:     thread.LastActivityTimestampMS.Time,
		BundledBackfillData: bundle,
	}
}

func (ic *IGClient) getAndResyncThread(ctx context.Context, threadIGID string) (networkid.PortalKey, error) {
	resp, err := ic.Client.GetThread(ctx, slidetypes.MakeGetThreadInfoRequest(threadIGID))
	if err != nil {
		return networkid.PortalKey{}, fmt.Errorf("failed to get thread info for %s: %w", threadIGID, err)
	}
	zerolog.Ctx(ctx).Trace().
		Any("thread_resp", resp.ThreadInfo.AsIGDirectThread).
		Msg("Response for thread resync fetch")
	// This should be done by extra updates in the chat resync anyway, but do it here just to be safe
	err = ic.saveThreadMappings(ctx, resp.ThreadInfo.AsIGDirectThread)
	if err != nil {
		return networkid.PortalKey{}, fmt.Errorf("failed to save FBID for IG thread %s: %w", threadIGID, err)
	}
	evt := ic.wrapChatResync(resp.ThreadInfo.AsIGDirectThread, true)
	res := ic.UserLogin.QueueRemoteEvent(evt)
	if !res.Success {
		return evt.PortalKey, res.Error
	}
	return evt.PortalKey, nil
}

func (ic *IGClient) ensurePortal(ctx context.Context, threadIGID string, allowCreate bool) (networkid.PortalKey, bool, error) {
	if threadIGID == "" {
		return networkid.PortalKey{}, false, nil
	} else if fbid, err := ic.Main.DB.GetFBIDForIGChat(ctx, threadIGID, ic.UserLogin.ID); err != nil {
		return networkid.PortalKey{}, false, err
	} else if fbid == 0 {
		// resync
	} else if portal, err := ic.Main.Bridge.GetExistingPortalByKey(ctx, ic.makeUncertainPortalKey(fbid)); err != nil {
		return networkid.PortalKey{}, false, fmt.Errorf("failed to get existing portal for thread %s/%d: %w", threadIGID, fbid, err)
	} else if portal == nil {
		// resync
	} else {
		return portal.PortalKey, false, nil
	}
	if !allowCreate {
		return networkid.PortalKey{}, false, nil
	}
	key, err := ic.getAndResyncThread(ctx, threadIGID)
	return key, true, err
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
	log.Trace().
		Type("event_struct", d.Data).
		RawJSON("event_data", d.Raw).
		Msg("Handling delta")

	allowCreate := true
	switch evt := d.Data.(type) {
	case *slidetypes.DeleteThreadEvent, *slidetypes.DeleteMessageEvent, *slidetypes.DeleteReactionEvent,
		*slidetypes.ParticipantLeaveEvent:
		allowCreate = false
	case *slidetypes.AdminMessageEvent:
		if ic.pendingGroupCreations.Has(evt.Message.OfflineThreadingID) {
			log.Debug().
				Str("offline_threading_id", evt.Message.OfflineThreadingID).
				Str("thread_id", d.ThreadIGID).
				Msg("Ignoring create notice for pending creation")
			return nil
		}
	}

	portalKey, didResync, err := ic.ensurePortal(ctx, d.ThreadIGID, allowCreate)
	if err != nil {
		return fmt.Errorf("failed to ensure portal for thread %s: %w", d.ThreadIGID, err)
	} else if portalKey.IsEmpty() {
		log.Warn().
			Str("typename", d.TypeName).
			Str("thread_fbid", d.ThreadIGID).
			Msg("Ignoring event with no portal")
		return nil
	}

	var res bridgev2.EventHandlingResult
	switch evt := d.Data.(type) {
	case *slidetypes.NewMessageEvent:
		res = ic.handleMessage(portalKey, evt.Message)
	case *slidetypes.AdminMessageEvent:
		res = ic.handleMessage(portalKey, evt.Message)
	case *slidetypes.EditMessageEvent:
		res = ic.handleEdit(portalKey, evt)
	case *slidetypes.CreateReactionEvent:
		res = ic.handleReaction(ctx, portalKey, evt)
	case *slidetypes.DeleteReactionEvent:
		res = ic.handleReactionDelete(ctx, portalKey, evt)
	case *slidetypes.DeleteMessageEvent:
		res = ic.handleMessageDelete(portalKey, evt.MessageID)
	case *slidetypes.DeleteThreadEvent:
		res = ic.handleThreadDelete(portalKey)
	case *slidetypes.PinThreadEvent:
		res = ic.handleThreadPin(portalKey, evt.IsPinned)
	case *slidetypes.UpdateThreadFolderEvent:
		res = ic.handleThreadFolder(portalKey, evt.Folder)
	case *slidetypes.UpdateThreadNameEvent:
		res = ic.handleThreadName(portalKey, evt)
	case *slidetypes.UpdateThreadImageEvent:
		res = ic.handleThreadImage(portalKey, evt)
	case *slidetypes.ParticipantJoinEvent:
		res = ic.handleGroupJoin(portalKey, evt)
	case *slidetypes.ParticipantLeaveEvent:
		res = ic.handleGroupLeave(portalKey, evt)
	case *slidetypes.AdminChangeEvent:
		// The event shape isn't great for making a chat info change event, just resync the chat info entirely
		if !didResync {
			_, err = ic.getAndResyncThread(ctx, d.ThreadIGID)
		}
		return err
	case *slidetypes.MarkReadEvent:
		res = ic.dispatchRead(portalKey, ic.selfEventSender(), evt.ReadTimestampMS.Time)
	case *slidetypes.MarkUnreadEvent:
		res = ic.dispatchUnread(portalKey, evt.MarkedAsUnread)
	case *slidetypes.ReadReceiptEvent:
		res = ic.dispatchRead(portalKey, ic.makeEventSender(evt.ReadReceipt.ParticipantFBID), evt.ReadReceipt.WatermarkTimestampMS.Time)
	case *slidetypes.MuteThreadEvent:
		res = ic.handleMuteThread(portalKey, evt.IsMutedNow)
	case *slidetypes.PinMessageEvent:
		res = ic.handlePinMessages(portalKey, evt)
	case slidetypes.UnknownEvent:
		log.Warn().
			Str("typename", d.TypeName).
			Str("thread_fbid", d.ThreadIGID).
			Msg("Unrecognized event type in socket")
		return nil
	default:
		return fmt.Errorf("unrecognized event type: %T", d.Data)
	}
	if !res.Success {
		return res.Error
	}
	return nil
}

func (ic *IGClient) makeMessageEventMeta(portalKey networkid.PortalKey, msg *slidetypes.Message, evtType bridgev2.RemoteEventType) simplevent.EventMeta {
	return simplevent.EventMeta{
		Type:         evtType,
		PortalKey:    portalKey,
		Sender:       ic.makeEventSender(msg.SenderFBID),
		CreatePortal: true,
		Timestamp:    msg.TimestampMS.Time,
		StreamOrder:  msg.TimestampMS.UnixMilli(),
	}
}

func (ic *IGClient) handleMessage(portalKey networkid.PortalKey, msg *slidetypes.Message) bridgev2.EventHandlingResult {
	msgID := metaid.MakeFBMessageID(msg.ID)
	return ic.UserLogin.QueueRemoteEvent(&simplevent.Message[*slidetypes.Message]{
		EventMeta: ic.makeMessageEventMeta(portalKey, msg, bridgev2.RemoteEventMessage),
		//TransactionID: msg.OfflineThreadingID,
		Data: msg,
		ID:   msgID,
		ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, data *slidetypes.Message) (*bridgev2.ConvertedMessage, error) {
			return ic.Main.MsgConv.ToMatrix(ctx, portal, ic.Client, intent, msgID, data, ic.Main.Config.DisableXMAAlways), ctx.Err()
		},
	})
}

func (ic *IGClient) handleEdit(portalKey networkid.PortalKey, evt *slidetypes.EditMessageEvent) bridgev2.EventHandlingResult {
	msgID := metaid.MakeFBMessageID(evt.MessageID)
	return ic.UserLogin.QueueRemoteEvent(&simplevent.Message[string]{
		EventMeta: simplevent.EventMeta{
			Type:        bridgev2.RemoteEventEdit,
			PortalKey:   portalKey,
			Sender:      bridgev2.EventSender{ForceEditOrigSender: true},
			Timestamp:   evt.SlideEditHistoryEntry.TimestampMS.Time,
			StreamOrder: evt.SlideEditHistoryEntry.TimestampMS.UnixMilli(),
		},
		Data: evt.TextBody,
		ID:   msgID,
		ConvertEditFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, existing []*database.Message, newText string) (*bridgev2.ConvertedEdit, error) {
			if len(existing) == 0 {
				return nil, fmt.Errorf("no existing message found for edit event %s", msgID)
			} else if len(existing) > 1 {
				zerolog.Ctx(ctx).Warn().
					Int("existing_count", len(existing)).
					Msg("Multiple existing messages found for edit event, using the first one")
			}
			return &bridgev2.ConvertedEdit{
				ModifiedParts: []*bridgev2.ConvertedEditPart{{
					Part:    existing[0],
					Type:    event.EventMessage,
					Content: ic.Main.MsgConv.MetaToMatrixText(ctx, newText, nil),
				}},
			}, nil
		},
	})
}

func (ic *IGClient) handleReaction(ctx context.Context, portalKey networkid.PortalKey, evt *slidetypes.CreateReactionEvent) bridgev2.EventHandlingResult {
	err := ic.Main.DB.PutIGReaction(ctx, portalKey, evt.MessageID, evt.Reaction.SenderFBID, evt.Reaction.LogMessageID)
	if err != nil {
		return bridgev2.EventHandlingResultFailed.WithError(fmt.Errorf("failed to store reaction mapping in db: %w", err))
	}
	return ic.UserLogin.QueueRemoteEvent(&simplevent.Reaction{
		EventMeta: simplevent.EventMeta{
			Type:        bridgev2.RemoteEventReaction,
			PortalKey:   portalKey,
			Sender:      ic.makeEventSender(evt.Reaction.SenderFBID),
			Timestamp:   evt.Reaction.ReactionTimestampMS.Time,
			StreamOrder: evt.Reaction.ReactionTimestampMS.UnixMilli(),
		},
		TargetMessage: metaid.MakeFBMessageID(evt.MessageID),
		Emoji:         evt.Reaction.Reaction,
	})
}

func (ic *IGClient) handleReactionDelete(ctx context.Context, portalKey networkid.PortalKey, evt *slidetypes.DeleteReactionEvent) bridgev2.EventHandlingResult {
	targetMsgID := evt.MessageID
	reactionSenderFBID := evt.Reaction.SenderFBID
	if reactionSenderFBID == 0 {
		var err error
		targetMsgID, reactionSenderFBID, err = ic.Main.DB.GetIGReactionTarget(ctx, portalKey, evt.Reaction.LogMessageID)
		if err != nil {
			return bridgev2.EventHandlingResultFailed.WithError(fmt.Errorf("failed to get reaction target from db: %w", err))
		} else if targetMsgID == "" {
			zerolog.Ctx(ctx).Warn().
				Stringer("portal_key", portalKey).
				Str("delete_id", evt.MessageID).
				Str("reaction_message_id", evt.Reaction.LogMessageID).
				Msg("Dropping reaction delete of unknown reaction message ID")
			return bridgev2.EventHandlingResultIgnored
		}
	}
	return ic.UserLogin.QueueRemoteEvent(&simplevent.Reaction{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventReactionRemove,
			PortalKey: portalKey,
			Sender:    ic.makeEventSender(reactionSenderFBID),
		},
		TargetMessage: metaid.MakeFBMessageID(targetMsgID),
	})
}

func (ic *IGClient) handleMessageDelete(portalKey networkid.PortalKey, id string) bridgev2.EventHandlingResult {
	return ic.UserLogin.QueueRemoteEvent(&simplevent.MessageRemove{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventMessageRemove,
			PortalKey: portalKey,
		},
		TargetMessage: metaid.MakeFBMessageID(id),
	})
}

func (ic *IGClient) handleThreadDelete(portalKey networkid.PortalKey) bridgev2.EventHandlingResult {
	return ic.UserLogin.QueueRemoteEvent(&simplevent.ChatDelete{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventChatDelete,
			PortalKey: portalKey,
		},
		OnlyForMe: true,
	})
}

func (ic *IGClient) handleThreadFolder(portalKey networkid.PortalKey, folder string) bridgev2.EventHandlingResult {
	isRequest := folder == "PENDING" || folder == "SPAM"
	return ic.UserLogin.QueueRemoteEvent(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type:         bridgev2.RemoteEventChatInfoChange,
			PortalKey:    portalKey,
			CreatePortal: !isRequest,
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			ChatInfo: &bridgev2.ChatInfo{MessageRequest: &isRequest},
		},
	})
}

func (ic *IGClient) handleThreadPin(portalKey networkid.PortalKey, isPinned bool) bridgev2.EventHandlingResult {
	var tag event.RoomTag
	if isPinned {
		tag = event.RoomTagFavourite
	}
	return ic.UserLogin.QueueRemoteEvent(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type:         bridgev2.RemoteEventChatInfoChange,
			PortalKey:    portalKey,
			CreatePortal: true,
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			ChatInfo: &bridgev2.ChatInfo{
				UserLocal: &bridgev2.UserLocalPortalInfo{
					Tag: &tag,
				},
			},
		},
	})
}

func (ic *IGClient) handleMessageChatInfoChange(
	portalKey networkid.PortalKey,
	msg *slidetypes.Message,
	change *bridgev2.ChatInfo,
	members ...bridgev2.ChatMember,
) bridgev2.EventHandlingResult {
	var memberChanges *bridgev2.ChatMemberList
	if len(members) > 0 {
		memberChanges = &bridgev2.ChatMemberList{
			MemberMap: make(bridgev2.ChatMemberMap, len(members)),
		}
		for _, m := range members {
			memberChanges.MemberMap.Add(m)
		}
	}
	return ic.UserLogin.QueueRemoteEvent(&simplevent.ChatInfoChange{
		EventMeta: ic.makeMessageEventMeta(portalKey, msg, bridgev2.RemoteEventChatInfoChange),
		ChatInfoChange: &bridgev2.ChatInfoChange{
			ChatInfo:      change,
			MemberChanges: memberChanges,
		},
	})
}

func (ic *IGClient) handleThreadName(portalKey networkid.PortalKey, evt *slidetypes.UpdateThreadNameEvent) bridgev2.EventHandlingResult {
	return ic.handleMessageChatInfoChange(portalKey, evt.Message, &bridgev2.ChatInfo{
		Name: &evt.ThreadName,
	})
}

func (ic *IGClient) handleThreadImage(portalKey networkid.PortalKey, evt *slidetypes.UpdateThreadImageEvent) bridgev2.EventHandlingResult {
	return ic.handleMessageChatInfoChange(portalKey, evt.Message, &bridgev2.ChatInfo{
		Avatar: wrapAvatar(evt.ThreadImage.URI),
	})
}

func (ic *IGClient) handleGroupJoin(portalKey networkid.PortalKey, evt *slidetypes.ParticipantJoinEvent) bridgev2.EventHandlingResult {
	members := &bridgev2.ChatMemberList{
		MemberMap: make(bridgev2.ChatMemberMap, len(evt.Thread.AsIGDirectThread.Users)),
	}
	for _, member := range evt.Thread.AsIGDirectThread.Users {
		members.MemberMap.Add(bridgev2.ChatMember{
			EventSender: ic.makeEventSender(member.InteropMessagingUserFBID),
			Membership:  event.MembershipJoin,
			UserInfo:    ic.wrapUserInfo(member),
		})
	}
	return ic.handleMessageChatInfoChange(portalKey, evt.Message, &bridgev2.ChatInfo{
		Name:    &evt.Thread.AsIGDirectThread.ThreadTitle,
		Members: members,
	})
}

func (ic *IGClient) handleGroupLeave(portalKey networkid.PortalKey, evt *slidetypes.ParticipantLeaveEvent) bridgev2.EventHandlingResult {
	return ic.handleMessageChatInfoChange(portalKey, evt.Message, &bridgev2.ChatInfo{
		Name: &evt.Thread.AsIGDirectThread.ThreadTitle,
	}, bridgev2.ChatMember{
		EventSender: ic.makeEventSender(evt.LeftParticipantFBID),
		Membership:  event.MembershipLeave,
	})
}

func (ic *IGClient) dispatchRead(portalKey networkid.PortalKey, sender bridgev2.EventSender, ts time.Time) bridgev2.EventHandlingResult {
	return ic.UserLogin.QueueRemoteEvent(&simplevent.Receipt{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventReadReceipt,
			PortalKey: portalKey,
			Sender:    sender,
		},
		ReadUpTo:            ts,
		ReadUpToStreamOrder: ts.UnixMilli(),
	})
}

func (ic *IGClient) dispatchUnread(portalKey networkid.PortalKey, unread bool) bridgev2.EventHandlingResult {
	return ic.UserLogin.QueueRemoteEvent(&simplevent.MarkUnread{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventMarkUnread,
			PortalKey: portalKey,
			Sender:    ic.selfEventSender(),
		},
		Unread: unread,
	})
}

func (ic *IGClient) handleMuteThread(portalKey networkid.PortalKey, isMuted bool) bridgev2.EventHandlingResult {
	// The event doesn't tell us when the chat gets unmuted
	mutedUntil := event.MutedForever
	if !isMuted {
		mutedUntil = bridgev2.Unmuted
	}
	return ic.UserLogin.QueueRemoteEvent(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventChatInfoChange,
			PortalKey: portalKey,
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			ChatInfo: &bridgev2.ChatInfo{
				UserLocal: &bridgev2.UserLocalPortalInfo{
					MutedUntil: &mutedUntil,
				},
			},
		},
	})
}

func (ic *IGClient) handlePinMessages(key networkid.PortalKey, evt *slidetypes.PinMessageEvent) bridgev2.EventHandlingResult {
	// TODO pinned messages aren't plumbed through bridgev2 yet
	return bridgev2.EventHandlingResultIgnored
}

func (ic *IGClient) handleTyping(ctx context.Context, evt *slidetypes.TypingNotification) error {
	threadKey, err := ic.Main.DB.GetFBIDForIGThread(ctx, evt.ThreadID, ic.UserLogin.ID)
	if err != nil {
		return fmt.Errorf("failed to get FBID for IG thread %s: %w", evt.ThreadID, err)
	} else if threadKey == 0 {
		return nil
	}
	userID, err := ic.Main.DB.GetFBIDForIGUser(ctx, strconv.FormatInt(evt.SenderID, 10))
	if err != nil {
		return fmt.Errorf("failed to get FBID for IG user %d: %w", evt.SenderID, err)
	} else if userID == 0 {
		return nil
	}
	timeout := 6 * time.Second
	if evt.ActivityStatus == 0 {
		timeout = 0
	}
	res := ic.UserLogin.QueueRemoteEvent(&simplevent.Typing{
		EventMeta: simplevent.EventMeta{
			Type:              bridgev2.RemoteEventTyping,
			PortalKey:         ic.makeUncertainPortalKey(threadKey),
			UncertainReceiver: true,
			Sender:            ic.makeEventSender(userID),
			Timestamp:         evt.Timestamp.Time,
		},
		Timeout: timeout,
		Type:    bridgev2.TypingTypeText,
	})
	if !res.Success {
		return res.Error
	}
	return nil
}
