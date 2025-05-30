package connector

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/exp/maps"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	WADisconnected             status.BridgeStateErrorCode = "wa-transient-disconnect"
	WAPermanentError           status.BridgeStateErrorCode = "wa-unknown-permanent-error"
	WACATError                 status.BridgeStateErrorCode = "wa-cat-refresh-error"
	WAConnectError             status.BridgeStateErrorCode = "wa-unknown-connect-error"
	MetaConnectionUnauthorized status.BridgeStateErrorCode = "meta-connection-unauthorized"
	MetaPermanentError         status.BridgeStateErrorCode = "meta-unknown-permanent-error"
	MetaCookieRemoved          status.BridgeStateErrorCode = "meta-cookie-removed"
	MetaUserIDIsZero           status.BridgeStateErrorCode = "meta-user-id-is-zero"
	MetaRedirectedToLoginPage  status.BridgeStateErrorCode = "meta-redirected-to-login"
	MetaNotLoggedIn            status.BridgeStateErrorCode = "meta-not-logged-in"
	MetaConnectError           status.BridgeStateErrorCode = "meta-connect-error"
	MetaTransientDisconnect    status.BridgeStateErrorCode = "meta-transient-disconnect"
	IGChallengeRequired        status.BridgeStateErrorCode = "ig-challenge-required"
	IGChallengeRequiredMaybe   status.BridgeStateErrorCode = "ig-challenge-required-maybe"
	IGAccountSuspended         status.BridgeStateErrorCode = "ig-account-suspended"
	MetaServerUnavailable      status.BridgeStateErrorCode = "meta-server-unavailable"
	IGConsentRequired          status.BridgeStateErrorCode = "ig-consent-required"
	FBConsentRequired          status.BridgeStateErrorCode = "fb-consent-required"
	MetaProxyUpdateFail        status.BridgeStateErrorCode = "meta-proxy-update-fail"
)

func init() {
	status.BridgeStateHumanErrors.Update(status.BridgeStateErrorMap{
		WADisconnected:             "Disconnected from encrypted chat server. Trying to reconnect.",
		MetaTransientDisconnect:    "Disconnected from server, trying to reconnect",
		MetaConnectionUnauthorized: "Logged out, please relogin to continue",
		MetaCookieRemoved:          "Logged out, please relogin to continue",
		MetaUserIDIsZero:           "Logged out, please relogin to continue",
		MetaRedirectedToLoginPage:  "Logged out, please relogin to continue",
		MetaNotLoggedIn:            "Logged out, please relogin to continue",
		IGAccountSuspended:         "Logged out, please check the Instagram website to continue",
		IGChallengeRequired:        "Challenge required, please check the Instagram website to continue",
		IGChallengeRequiredMaybe:   "Connection refused, please check the Instagram website to continue",
		IGConsentRequired:          "Consent required, please check the Instagram website to continue",
		FBConsentRequired:          "Consent required, please check the Facebook website to continue",
		MetaServerUnavailable:      "Connection refused by server",
		MetaConnectError:           "Unknown connection error",
		MetaProxyUpdateFail:        "Failed to update proxy",
	})
}

func (m *MetaClient) handleMetaEvent(rawEvt any) {
	log := m.UserLogin.Log

	switch evt := rawEvt.(type) {
	case *messagix.Event_PublishResponse:
		log.Trace().Any("table", &evt.Table).Msg("Got new event")
		select {
		case m.incomingTables <- evt.Table:
		default:
			log.Warn().Msg("Incoming tables channel full, event order not guaranteed")
			go func() {
				m.incomingTables <- evt.Table
			}()
		}
	case *messagix.Event_Ready:
		log.Debug().Msg("Initial connect to Meta socket completed")
		m.connectWaiter.Set()
		if tbl := m.initialTable.Swap(nil); tbl != nil {
			log.Debug().Msg("Sending cached initial table to handler")
			m.incomingTables <- tbl
		}
		if m.LoginMeta.Platform.IsMessenger() || m.Main.Config.IGE2EE {
			m.firstE2EEConnectDone = true
			go m.tryConnectE2EE(false)
		}
		m.metaState = status.BridgeState{StateEvent: status.StateConnected}
		m.UserLogin.BridgeState.Send(m.metaState)
	case *messagix.Event_SocketError:
		log.Debug().Err(evt.Err).Msg("Disconnected from Meta socket")
		m.metaState = status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      MetaTransientDisconnect,
		}
		m.UserLogin.BridgeState.Send(m.metaState)
	case *messagix.Event_Reconnected:
		if !m.firstE2EEConnectDone && (m.LoginMeta.Platform.IsMessenger() || m.Main.Config.IGE2EE) {
			m.firstE2EEConnectDone = true
			go m.tryConnectE2EE(false)
		}
		log.Debug().Msg("Reconnected to Meta socket")
		m.connectWaiter.Set()
		m.metaState = status.BridgeState{StateEvent: status.StateConnected}
		m.UserLogin.BridgeState.Send(m.metaState)
	case *messagix.Event_PermanentError:
		if errors.Is(evt.Err, messagix.CONNECTION_REFUSED_UNAUTHORIZED) {
			m.metaState = status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      MetaConnectionUnauthorized,
			}
		} else if errors.Is(evt.Err, messagix.CONNECTION_REFUSED_SERVER_UNAVAILABLE) {
			if m.Main.Config.Mode.IsMessenger() {
				m.metaState = status.BridgeState{
					StateEvent: status.StateUnknownError,
					Error:      MetaServerUnavailable,
				}
				if m.canReconnect() {
					log.Debug().Msg("Doing full reconnect after server unavailable error")
					go m.FullReconnect()
				}
			} else {
				m.metaState = status.BridgeState{
					StateEvent: status.StateBadCredentials,
					Error:      IGChallengeRequiredMaybe,
				}
			}
		} else {
			m.metaState = status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaPermanentError,
				Message:    evt.Err.Error(),
			}
		}
		m.UserLogin.BridgeState.Send(m.metaState)
		if stopPeriodicReconnect := m.stopPeriodicReconnect.Swap(nil); stopPeriodicReconnect != nil {
			(*stopPeriodicReconnect)()
		}
	default:
		log.Warn().Type("event_type", evt).Msg("Unrecognized event type from messagix")
	}
}

func (m *MetaClient) handleTableLoop() {
	ctx, cancel := context.WithCancel(m.Main.Bridge.BackgroundCtx)
	defer cancel()
	if oldCancel := m.stopHandlingTables.Swap(&cancel); oldCancel != nil {
		(*oldCancel)()
	}
	if m.wrappedEvents != nil {
		go m.handleEventLoop(ctx)
	}
	log := m.UserLogin.Log.With().Str("action", "handle table").Logger()
	ctx = log.WithContext(ctx)
	for {
		select {
		case tbl := <-m.incomingTables:
			m.notifyBackgroundConnAboutEvent(true)
			m.handleTable(ctx, tbl)
			m.notifyBackgroundConnAboutEvent(false)
		case <-ctx.Done():
			return
		}
	}
}

func (m *MetaClient) handleEventLoop(ctx context.Context) {
	zerolog.Ctx(ctx).Info().Msg("Starting inner event queue")
	for {
		select {
		case evts := <-m.wrappedEvents:
			for _, evt := range evts {
				m.UserLogin.QueueRemoteEvent(evt)
				if ctx.Err() != nil {
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (m *MetaClient) syncGhost(ctx context.Context, info types.UserInfo) {
	log := zerolog.Ctx(ctx).With().Int64("ghost_id", info.GetFBID()).Logger()
	ctx = log.WithContext(ctx)
	ghost, err := m.Main.Bridge.GetGhostByID(ctx, metaid.MakeUserID(info.GetFBID()))
	if err != nil {
		log.Err(err).Msg("Failed to get ghost")
		return
	}
	ghost.UpdateInfo(ctx, m.wrapUserInfo(info))
}

func (m *MetaClient) handleTable(ctx context.Context, tbl *table.LSTable) {
	for _, contact := range tbl.LSDeleteThenInsertContact {
		m.syncGhost(ctx, contact)
	}
	for _, contact := range tbl.LSVerifyContactRowExists {
		m.syncGhost(ctx, contact)
	}

	threadExists := make(map[int64]*table.LSVerifyThreadExists, len(tbl.LSVerifyThreadExists))
	threadResyncs := make(map[int64]*FBChatResync, len(tbl.LSDeleteThenInsertThread))
	params := threadMaps{
		ctx:   ctx,
		m:     m,
		vtes:  threadExists,
		syncs: threadResyncs,
	}
	innerQueue := make([]bridgev2.RemoteEvent, 0, 8)

	for _, thread := range tbl.LSVerifyThreadExists {
		threadExists[thread.ThreadKey] = thread
	}
	for _, thread := range tbl.LSDeleteThenInsertThread {
		threadResyncs[thread.ThreadKey] = &FBChatResync{
			PortalKey: m.makeFBPortalKey(thread.ThreadKey, thread.ThreadType),
			Info:      m.wrapChatInfo(thread),
			Raw:       thread,
			Members:   make(map[int64]bridgev2.ChatMember, thread.MemberCount),
			m:         m,
		}
	}
	// TODO resync threads with LSUpdateOrInsertThread?

	// Deleting a thread will cancel all further events, so handle those first
	handlePortalEvents(params, tbl.LSDeleteThread, m.handleDeleteThread, &innerQueue)

	for _, verifyExists := range threadExists {
		if _, resyncing := threadResyncs[verifyExists.ThreadKey]; resyncing {
			continue
		}
		innerQueue = append(innerQueue, &VerifyThreadExistsEvent{LSVerifyThreadExists: verifyExists, m: m})
	}

	// Handle events that are merged into thread resyncs before dispatching the resyncs
	handlePortalEvents(params, tbl.LSAddParticipantIdToGroupThread, m.handleAddParticipant, &innerQueue)
	handlePortalEvents(params, tbl.LSUpdateThreadMuteSetting, m.handleUpdateMuteSetting, &innerQueue)
	handlePortalEvents(params, tbl.LSMoveThreadToE2EECutoverFolder, m.handleMoveThreadToE2EE, &innerQueue)
	upsert, insert := tbl.WrapMessages()
	handlePortalEvents(params, maps.Values(upsert), m.handleUpsertMessages, &innerQueue)
	handlePortalEvents(params, tbl.LSUpdateExistingMessageRange, m.handleUpdateExistingMessageRange, &innerQueue)

	for _, resync := range threadResyncs {
		innerQueue = append(innerQueue, resync)
	}

	handlePortalEvents(params, insert, m.handleMessageInsert, &innerQueue)
	// Edits are special snowflakes that don't include the thread key
	for _, edit := range tbl.LSEditMessage {
		m.handleEdit(ctx, edit, &innerQueue)
	}
	handlePortalEvents(params, tbl.LSSyncUpdateThreadName, m.handleUpdateThreadName, &innerQueue)
	handlePortalEvents(params, tbl.LSSetThreadImageURL, m.handleSetThreadImage, &innerQueue)
	handlePortalEvents(params, tbl.LSUpdateReadReceipt, m.handleUpdateReadReceipt, &innerQueue)
	handlePortalEvents(params, tbl.LSMarkThreadReadV2, m.handleMarkThreadRead, &innerQueue)
	handlePortalEvents(params, tbl.LSUpdateTypingIndicator, m.handleTypingIndicator, &innerQueue)
	handlePortalEvents(params, tbl.LSDeleteMessage, m.handleDeleteMessage, &innerQueue)
	handlePortalEvents(params, tbl.LSDeleteThenInsertMessage, m.handleDeleteThenInsertMessage, &innerQueue)
	handlePortalEvents(params, tbl.LSUpsertReaction, m.handleUpsertReaction, &innerQueue)
	handlePortalEvents(params, tbl.LSDeleteReaction, m.handleDeleteReaction, &innerQueue)
	handlePortalEvents(params, tbl.LSRemoveParticipantFromThread, m.handleRemoveParticipant, &innerQueue)
	// TODO request more inbox if applicable

	if m.wrappedEvents == nil {
		for _, evt := range innerQueue {
			m.UserLogin.QueueRemoteEvent(evt)
		}
	} else {
		m.wrappedEvents <- innerQueue
	}
}

func (m *MetaClient) handleMarkThreadRead(tk handlerParams, msg *table.LSMarkThreadReadV2) bridgev2.RemoteEvent {
	return &simplevent.Receipt{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventReadReceipt,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Int64("read_up_to", msg.LastReadWatermarkTimestampMs)
			},
			PortalKey:         tk.Portal,
			UncertainReceiver: tk.UncertainReceiver,
			Sender:            m.selfEventSender(),
		},
		ReadUpTo: time.UnixMilli(msg.LastReadWatermarkTimestampMs),
	}
}

func (m *MetaClient) handleUpdateReadReceipt(tk handlerParams, msg *table.LSUpdateReadReceipt) bridgev2.RemoteEvent {
	return &simplevent.Receipt{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventReadReceipt,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Int64("read_up_to", msg.ReadWatermarkTimestampMs)
			},
			PortalKey:         tk.Portal,
			UncertainReceiver: tk.UncertainReceiver,
			Sender:            m.makeEventSender(msg.ContactId),
			Timestamp:         time.UnixMilli(msg.ReadActionTimestampMs),
		},
		ReadUpTo: time.UnixMilli(msg.ReadWatermarkTimestampMs),
	}
}

func (m *MetaClient) handleTypingIndicator(tk handlerParams, msg *table.LSUpdateTypingIndicator) bridgev2.RemoteEvent {
	var timeout time.Duration
	if msg.IsTyping {
		// TODO find if this is the correct timeout
		timeout = 15 * time.Second
	}
	return &simplevent.Typing{
		EventMeta: simplevent.EventMeta{
			Type:              bridgev2.RemoteEventTyping,
			PortalKey:         tk.Portal,
			UncertainReceiver: tk.UncertainReceiver,
			Sender:            m.makeEventSender(msg.SenderId),
		},
		Timeout: timeout,
	}
}

func wrapMessageDelete(portal networkid.PortalKey, uncertain bool, messageID string) *simplevent.MessageRemove {
	return &simplevent.MessageRemove{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventMessageRemove,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_id", messageID)
			},
			PortalKey:         portal,
			UncertainReceiver: uncertain,
		},
		TargetMessage: metaid.MakeFBMessageID(messageID),
	}
}

func (m *MetaClient) handleDeleteMessage(tk handlerParams, msg *table.LSDeleteMessage) bridgev2.RemoteEvent {
	return wrapMessageDelete(tk.Portal, tk.UncertainReceiver, msg.MessageId)
}

func (m *MetaClient) handleDeleteThenInsertMessage(tk handlerParams, msg *table.LSDeleteThenInsertMessage) bridgev2.RemoteEvent {
	if !msg.IsUnsent {
		zerolog.Ctx(tk.ctx).Warn().
			Str("message_id", msg.MessageId).
			Int64("edit_count", msg.EditCount).
			Msg("Got unexpected non-unsend DeleteThenInsertMessage command")
		return nil
	}
	return wrapMessageDelete(tk.Portal, tk.UncertainReceiver, msg.MessageId)
}

func (m *MetaClient) handleDeleteThread(tk handlerParams, msg *table.LSDeleteThread) bridgev2.RemoteEvent {
	// TODO figure out how to handle meta's false delete events
	// Delete the thread from the sync maps to prevent future events finding it
	delete(tk.syncs, msg.ThreadKey)
	delete(tk.vtes, msg.ThreadKey)
	return &simplevent.ChatDelete{
		EventMeta: simplevent.EventMeta{
			Type:              bridgev2.RemoteEventChatDelete,
			PortalKey:         tk.Portal,
			UncertainReceiver: tk.UncertainReceiver,
		},
		// TODO can deletes be only for me?
		OnlyForMe: false,
	}
}

func markPortalAsEncrypted(ctx context.Context, portal *bridgev2.Portal) bool {
	meta := portal.Metadata.(*metaid.PortalMetadata)
	if meta.ThreadType == table.ONE_TO_ONE {
		meta.ThreadType = table.ENCRYPTED_OVER_WA_ONE_TO_ONE
		return true
	}
	return false
}

func (m *MetaClient) handleMoveThreadToE2EE(tk handlerParams, msg *table.LSMoveThreadToE2EECutoverFolder) bridgev2.RemoteEvent {
	if tk.Sync != nil {
		tk.Sync.Info.ExtraUpdates = bridgev2.MergeExtraUpdaters(tk.Sync.Info.ExtraUpdates, markPortalAsEncrypted)
		return nil
	}
	return m.wrapChatInfoChange(tk.ID, 0, tk.Type, &bridgev2.ChatInfoChange{
		ChatInfo: &bridgev2.ChatInfo{
			ExtraUpdates: markPortalAsEncrypted,
		},
	})
}

func (m *MetaClient) wrapReaction(portalKey networkid.PortalKey, uncertainReceiver bool, sender, timestamp int64, messageID, emoji string) *simplevent.Reaction {
	evt := &simplevent.Reaction{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventReaction,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("target_message_id", messageID).Int64("sender_id", sender)
			},
			PortalKey:         portalKey,
			UncertainReceiver: uncertainReceiver,
			Sender:            m.makeEventSender(sender),
		},
		TargetMessage: metaid.MakeFBMessageID(messageID),
		Emoji:         emoji,
	}
	if timestamp != 0 {
		evt.Timestamp = time.UnixMilli(timestamp)
	}
	if emoji == "" {
		evt.Type = bridgev2.RemoteEventReactionRemove
	}
	return evt
}

func (m *MetaClient) handleUpsertReaction(tk handlerParams, evt *table.LSUpsertReaction) bridgev2.RemoteEvent {
	return m.wrapReaction(tk.Portal, tk.UncertainReceiver, evt.ActorId, evt.TimestampMs, evt.MessageId, evt.Reaction)
}

func (m *MetaClient) handleDeleteReaction(tk handlerParams, evt *table.LSDeleteReaction) bridgev2.RemoteEvent {
	return m.wrapReaction(tk.Portal, tk.UncertainReceiver, evt.ActorId, 0, evt.MessageId, "")
}

func (m *MetaClient) handleUpdateThreadName(tk handlerParams, evt *table.LSSyncUpdateThreadName) bridgev2.RemoteEvent {
	if tk.Type.IsOneToOne() && !m.Main.Bridge.Config.PrivateChatPortalMeta {
		return nil
	}
	return m.wrapChatInfoChange(tk.ID, 0, tk.Type, &bridgev2.ChatInfoChange{
		ChatInfo: &bridgev2.ChatInfo{
			Name: &evt.ThreadName,
		},
	})
}

func (m *MetaClient) handleSetThreadImage(tk handlerParams, evt *table.LSSetThreadImageURL) bridgev2.RemoteEvent {
	if tk.Type.IsOneToOne() && !m.Main.Bridge.Config.PrivateChatPortalMeta {
		return nil
	}
	return m.wrapChatInfoChange(tk.ID, 0, tk.Type, &bridgev2.ChatInfoChange{
		ChatInfo: &bridgev2.ChatInfo{
			Avatar: wrapAvatar(evt.ImageURL),
		},
	})
}

func (m *MetaClient) handleUpdateMuteSetting(tk handlerParams, evt *table.LSUpdateThreadMuteSetting) bridgev2.RemoteEvent {
	mutedUntil := time.UnixMilli(evt.MuteExpireTimeMS)
	if evt.MuteExpireTimeMS < 0 {
		mutedUntil = event.MutedForever
	}
	if tk.Sync != nil {
		if tk.Sync.Info.UserLocal == nil {
			tk.Sync.Info.UserLocal = &bridgev2.UserLocalPortalInfo{}
		}
		tk.Sync.Info.UserLocal.MutedUntil = &mutedUntil
		return nil
	}
	return m.wrapChatInfoChange(tk.ID, 0, tk.Type, &bridgev2.ChatInfoChange{
		ChatInfo: &bridgev2.ChatInfo{
			UserLocal: &bridgev2.UserLocalPortalInfo{
				MutedUntil: &mutedUntil,
			},
		},
	})
}

func (m *MetaClient) handleAddParticipant(tk handlerParams, evt *table.LSAddParticipantIdToGroupThread) bridgev2.RemoteEvent {
	if tk.Sync != nil {
		tk.Sync.Members[evt.ContactId] = m.wrapChatMember(evt)
		return nil
	}
	return m.wrapChatInfoChange(evt.ThreadKey, evt.ContactId, tk.Type, &bridgev2.ChatInfoChange{
		MemberChanges: &bridgev2.ChatMemberList{
			Members: []bridgev2.ChatMember{
				m.wrapChatMember(evt),
			},
		},
	})
}

func (m *MetaClient) handleRemoveParticipant(tk handlerParams, evt *table.LSRemoveParticipantFromThread) bridgev2.RemoteEvent {
	return m.wrapChatInfoChange(evt.ThreadKey, evt.ParticipantId, tk.Type, &bridgev2.ChatInfoChange{
		MemberChanges: &bridgev2.ChatMemberList{
			Members: []bridgev2.ChatMember{{
				EventSender:    m.makeEventSender(evt.ParticipantId),
				Membership:     event.MembershipLeave,
				PrevMembership: event.MembershipJoin,
			}},
		},
	})
}

func (m *MetaClient) handleSubthread(ctx context.Context, msg *table.WrappedMessage) {
	if msg.SubthreadKey != 0 {
		err := m.Main.DB.PutThread(ctx, msg.ThreadKey, msg.SubthreadKey, msg.MessageId)
		if err != nil {
			zerolog.Ctx(ctx).Warn().
				Err(err).
				Int64("thread_key", msg.ThreadKey).
				Int64("subthread_key", msg.SubthreadKey).
				Str("message_id", msg.MessageId).
				Msg("Failed to insert subthread")
		}
	} else if len(msg.XMAAttachments) == 1 {
		xma := msg.XMAAttachments[0]
		parsedURL, err := url.Parse(xma.ActionUrl)
		if err != nil || parsedURL.Scheme != "fb-messenger" || parsedURL.Host != "community_subthread" {
			return
		}
		msg.XMAAttachments = nil
		msg.Text = xma.TitleText
		msg.IsSubthreadStart = true
		err = m.Main.DB.PutThread(ctx, msg.ThreadKey, xma.TargetId, msg.ReplySourceId)
		if err != nil {
			zerolog.Ctx(ctx).Warn().
				Err(err).
				Str("xma_url", xma.ActionUrl).
				Int64("thread_key", msg.ThreadKey).
				Int64("subthread_key", xma.TargetId).
				Str("message_id", msg.ReplySourceId).
				Msg("Failed to insert subthread")
		}
	}
}

func (m *MetaClient) handleMessageInsert(tk handlerParams, msg *table.WrappedMessage) bridgev2.RemoteEvent {
	m.handleSubthread(tk.ctx, msg)
	msg.ThreadID = tk.ThreadMsgID
	return &FBMessageEvent{
		WrappedMessage:    msg,
		portalKey:         tk.Portal,
		uncertainReceiver: tk.UncertainReceiver,
		m:                 m,
	}
}

func (m *MetaClient) handleEdit(ctx context.Context, edit *table.LSEditMessage, innerQueue *[]bridgev2.RemoteEvent) {
	editID := metaid.MakeFBMessageID(edit.MessageID)
	originalMsg, err := m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, m.UserLogin.ID, editID)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Str("message_id", edit.MessageID).Msg("Failed to get edit target message")
	} else if originalMsg == nil {
		zerolog.Ctx(ctx).Warn().Str("message_id", edit.MessageID).Msg("Edit target message not found")
	} else {
		*innerQueue = append(*innerQueue, &FBEditEvent{
			LSEditMessage: edit,
			orig:          originalMsg,
			m:             m,
		})
	}
}

type ThreadKeyable interface {
	GetThreadKey() int64
}

type threadMaps struct {
	ctx   context.Context
	m     *MetaClient
	vtes  map[int64]*table.LSVerifyThreadExists
	syncs map[int64]*FBChatResync
}

type handlerParams struct {
	ctx context.Context

	ID                int64
	Type              table.ThreadType
	Portal            networkid.PortalKey
	UncertainReceiver bool
	Sync              *FBChatResync

	ThreadMsgID string

	vtes  map[int64]*table.LSVerifyThreadExists
	syncs map[int64]*FBChatResync
}

func handlePortalEvents[T ThreadKeyable](
	p threadMaps,
	msgs []T,
	fn func(tk handlerParams, msg T) bridgev2.RemoteEvent,
	innerQueue *[]bridgev2.RemoteEvent,
) {
	for _, msg := range msgs {
		threadKey := msg.GetThreadKey()
		sync, syncOK := p.syncs[threadKey]
		v, ok := p.vtes[threadKey]
		var threadType table.ThreadType
		uncertain := false
		if ok {
			threadType = v.ThreadType
		} else if syncOK {
			threadType = sync.Raw.ThreadType
		} else {
			uncertain = true
		}
		// TODO this check isn't needed for all types
		parentKey, threadMsgID, err := p.m.Main.DB.GetThreadByKey(p.ctx, threadKey)
		if err != nil {
			zerolog.Ctx(p.ctx).Warn().Err(err).Int64("thread_key", threadKey).Msg("Failed to get subthread key")
		} else if threadMsgID != "" {
			threadType = table.UNKNOWN_THREAD_TYPE
			uncertain = true
			threadKey = parentKey
		}
		if fn == nil {
			zerolog.Ctx(p.ctx).Warn().Type("event_type", msg).Msg("No handler for event")
			return
		}
		evt := fn(handlerParams{
			ctx: p.ctx,

			ID:                threadKey,
			Type:              threadType,
			Portal:            p.m.makeFBPortalKey(threadKey, threadType),
			UncertainReceiver: uncertain,
			Sync:              sync,
			ThreadMsgID:       threadMsgID,

			vtes:  p.vtes,
			syncs: p.syncs,
		}, msg)
		if evt != nil {
			*innerQueue = append(*innerQueue, evt)
		}
	}
}
