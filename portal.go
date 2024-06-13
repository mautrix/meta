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
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/variationselector"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary/armadillo/waArmadilloApplication"
	"go.mau.fi/whatsmeow/binary/armadillo/waCommon"
	"go.mau.fi/whatsmeow/binary/armadillo/waConsumerApplication"
	"go.mau.fi/whatsmeow/binary/armadillo/waMsgApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/bridge"
	"maunium.net/go/mautrix/bridge/bridgeconfig"
	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/config"
	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	metaTypes "go.mau.fi/mautrix-meta/messagix/types"
	"go.mau.fi/mautrix-meta/msgconv"
)

func (br *MetaBridge) GetPortalByMXID(mxid id.RoomID) *Portal {
	br.portalsLock.Lock()
	defer br.portalsLock.Unlock()

	portal, ok := br.portalsByMXID[mxid]
	if !ok {
		dbPortal, err := br.DB.Portal.GetByMXID(context.TODO(), mxid)
		if err != nil {
			br.ZLog.Err(err).Msg("Failed to get portal from database")
			return nil
		}
		return br.loadPortal(context.TODO(), dbPortal, nil, table.UNKNOWN_THREAD_TYPE)
	}

	return portal
}

func (br *MetaBridge) GetExistingPortalByThreadID(key database.PortalKey) *Portal {
	return br.GetPortalByThreadID(key, table.UNKNOWN_THREAD_TYPE)
}

func (br *MetaBridge) GetPortalByThreadID(key database.PortalKey, threadType table.ThreadType) *Portal {
	br.portalsLock.Lock()
	defer br.portalsLock.Unlock()
	if threadType != table.UNKNOWN_THREAD_TYPE && !threadType.IsOneToOne() {
		key.Receiver = 0
	}
	portal, ok := br.portalsByID[key]
	if !ok && threadType == table.UNKNOWN_THREAD_TYPE && key.Receiver != 0 {
		// If the thread type is unknown and a DM portal wasn't found, try to find a group portal (zeroed receiver)
		portal, ok = br.portalsByID[database.PortalKey{ThreadID: key.ThreadID}]
	}
	if !ok {
		dbPortal, err := br.DB.Portal.GetByThreadID(context.TODO(), key)
		if err != nil {
			br.ZLog.Err(err).Msg("Failed to get portal from database")
			return nil
		}
		return br.loadPortal(context.TODO(), dbPortal, &key, threadType)
	}
	return portal
}

func (br *MetaBridge) GetAllPortalsWithMXID() []*Portal {
	portals, err := br.dbPortalsToPortals(br.DB.Portal.GetAllWithMXID(context.TODO()))
	if err != nil {
		br.ZLog.Err(err).Msg("Failed to get all portals with mxid")
		return nil
	}
	return portals
}

func (br *MetaBridge) FindPrivateChatPortalsWith(userID int64) []*Portal {
	portals, err := br.dbPortalsToPortals(br.DB.Portal.FindPrivateChatsWith(context.TODO(), userID))
	if err != nil {
		br.ZLog.Err(err).Msg("Failed to get all DM portals with user")
		return nil
	}
	return portals
}

func (br *MetaBridge) GetAllIPortals() (iportals []bridge.Portal) {
	portals, err := br.dbPortalsToPortals(br.DB.Portal.GetAllWithMXID(context.TODO()))
	if err != nil {
		br.ZLog.Err(err).Msg("Failed to get all portals with mxid")
		return nil
	}
	iportals = make([]bridge.Portal, len(portals))
	for i, portal := range portals {
		iportals[i] = portal
	}
	return iportals
}

func (br *MetaBridge) loadPortal(ctx context.Context, dbPortal *database.Portal, key *database.PortalKey, threadType table.ThreadType) *Portal {
	if dbPortal == nil {
		if key == nil || threadType == table.UNKNOWN_THREAD_TYPE {
			return nil
		}

		dbPortal = br.DB.Portal.New()
		dbPortal.PortalKey = *key
		dbPortal.ThreadType = threadType
		err := dbPortal.Insert(ctx)
		if err != nil {
			br.ZLog.Err(err).Msg("Failed to insert new portal")
			return nil
		}
	}

	portal := br.NewPortal(dbPortal)

	br.portalsByID[portal.PortalKey] = portal
	if portal.MXID != "" {
		br.portalsByMXID[portal.MXID] = portal
	}

	return portal
}

func (br *MetaBridge) dbPortalsToPortals(dbPortals []*database.Portal, err error) ([]*Portal, error) {
	if err != nil {
		return nil, err
	}
	br.portalsLock.Lock()
	defer br.portalsLock.Unlock()

	output := make([]*Portal, len(dbPortals))
	for index, dbPortal := range dbPortals {
		if dbPortal == nil {
			continue
		}

		portal, ok := br.portalsByID[dbPortal.PortalKey]
		if !ok {
			portal = br.loadPortal(context.TODO(), dbPortal, nil, table.UNKNOWN_THREAD_TYPE)
		}

		output[index] = portal
	}

	return output, nil
}

type portalMetaMessage struct {
	evt  any
	user *User
}

type portalMatrixMessage struct {
	evt  *event.Event
	user *User
}

type Portal struct {
	*database.Portal

	MsgConv *msgconv.MessageConverter

	bridge *MetaBridge
	log    zerolog.Logger

	roomCreateLock sync.Mutex
	encryptLock    sync.Mutex

	metaMessages   chan portalMetaMessage
	matrixMessages chan portalMatrixMessage

	currentlyTyping     []id.UserID
	currentlyTypingLock sync.Mutex

	pendingMessages     map[int64]id.EventID
	pendingMessagesLock sync.Mutex

	backfillLock      sync.Mutex
	backfillCollector *BackfillCollector

	fetchAttempted atomic.Bool

	relayUser *User
}

func (br *MetaBridge) NewPortal(dbPortal *database.Portal) *Portal {
	logWith := br.ZLog.With().Int64("thread_id", dbPortal.ThreadID)
	if dbPortal.Receiver != 0 {
		logWith = logWith.Int64("thread_receiver", dbPortal.Receiver)
	}
	if dbPortal.MXID != "" {
		logWith = logWith.Stringer("room_id", dbPortal.MXID)
	}

	portal := &Portal{
		Portal: dbPortal,
		bridge: br,
		log:    logWith.Logger(),

		metaMessages:   make(chan portalMetaMessage, br.Config.Bridge.PortalMessageBuffer),
		matrixMessages: make(chan portalMatrixMessage, br.Config.Bridge.PortalMessageBuffer),

		pendingMessages: make(map[int64]id.EventID),
	}
	portal.MsgConv = &msgconv.MessageConverter{
		PortalMethods:        portal,
		ConvertVoiceMessages: true,
		MaxFileSize:          br.MediaConfig.UploadSize,
	}
	go portal.messageLoop()

	return portal
}

func init() {
	event.TypeMap[event.StateBridge] = reflect.TypeOf(CustomBridgeInfoContent{})
	event.TypeMap[event.StateHalfShotBridge] = reflect.TypeOf(CustomBridgeInfoContent{})
}

var (
	_ bridge.Portal                    = (*Portal)(nil)
	_ bridge.ReadReceiptHandlingPortal = (*Portal)(nil)
	//_ bridge.TypingPortal              = (*Portal)(nil)
	//_ bridge.DisappearingPortal        = (*Portal)(nil)
	//_ bridge.MembershipHandlingPortal  = (*Portal)(nil)
	//_ bridge.MetaHandlingPortal        = (*Portal)(nil)
)

func (portal *Portal) IsEncrypted() bool {
	return portal.Encrypted
}

func (portal *Portal) MarkEncrypted() {
	portal.Encrypted = true
	err := portal.Update(context.TODO())
	if err != nil {
		portal.log.Err(err).Msg("Failed to update portal in database after marking as encrypted")
	}
}

func (portal *Portal) ReceiveMatrixEvent(user bridge.User, evt *event.Event) {
	if user.GetPermissionLevel() >= bridgeconfig.PermissionLevelUser || portal.HasRelaybot() {
		portal.matrixMessages <- portalMatrixMessage{user: user.(*User), evt: evt}
	}
}

func (portal *Portal) GetRelayUser() *User {
	if !portal.HasRelaybot() {
		return nil
	} else if portal.relayUser == nil {
		portal.relayUser = portal.bridge.GetUserByMXID(portal.RelayUserID)
	}
	return portal.relayUser
}

func (portal *Portal) GetDMPuppet() *Puppet {
	if !portal.IsPrivateChat() {
		return nil
	}
	return portal.bridge.GetPuppetByID(portal.ThreadID)
}

func (portal *Portal) MainIntent() *appservice.IntentAPI {
	if dmPuppet := portal.GetDMPuppet(); dmPuppet != nil {
		return dmPuppet.DefaultIntent()
	}
	return portal.bridge.Bot
}

type CustomBridgeInfoContent struct {
	event.BridgeEventContent
	RoomType string `json:"com.beeper.room_type,omitempty"`
}

func (portal *Portal) getBridgeInfoStateKey() string {
	return fmt.Sprintf("fi.mau.meta://%s/%d", portal.bridge.BeeperNetworkName, portal.ThreadID)
}

func (portal *Portal) GetThreadURL(_ context.Context) (protocol, channel string) {
	switch portal.bridge.Config.Meta.Mode {
	case config.ModeInstagram:
		protocol = "https://www.instagram.com/"
		channel = fmt.Sprintf("https://www.instagram.com/direct/t/%d/", portal.ThreadID)
	case config.ModeFacebook, config.ModeFacebookTor:
		protocol = "https://www.facebook.com/"
		channel = fmt.Sprintf("https://www.facebook.com/messages/t/%d", portal.ThreadID)
	case config.ModeMessenger:
		protocol = "https://www.messenger.com/"
		channel = fmt.Sprintf("https://www.messenger.com/t/%d", portal.ThreadID)
	}
	if portal.ThreadType.IsWhatsApp() {
		// TODO store fb-side thread ID? (the whatsapp chat id is not the same as the fb-side thread id used in urls)
		channel = ""
	}
	return
}

func (portal *Portal) getBridgeInfo() (string, CustomBridgeInfoContent) {
	bridgeInfo := event.BridgeEventContent{
		BridgeBot: portal.bridge.Bot.UserID,
		Creator:   portal.MainIntent().UserID,
		Protocol: event.BridgeInfoSection{
			ID:          portal.bridge.BeeperServiceName,
			DisplayName: portal.bridge.ProtocolName,
			AvatarURL:   portal.bridge.Config.AppService.Bot.ParsedAvatar.CUString(),
		},
		Channel: event.BridgeInfoSection{
			ID:          strconv.FormatInt(portal.ThreadID, 10),
			DisplayName: portal.Name,
			AvatarURL:   portal.AvatarURL.CUString(),
		},
	}
	bridgeInfo.Protocol.ExternalURL, bridgeInfo.Channel.ExternalURL = portal.GetThreadURL(nil)
	var roomType string
	if portal.IsPrivateChat() {
		roomType = "dm"
	}
	return portal.getBridgeInfoStateKey(), CustomBridgeInfoContent{bridgeInfo, roomType}
}

func (portal *Portal) UpdateBridgeInfo(ctx context.Context) {
	if len(portal.MXID) == 0 {
		portal.log.Debug().Msg("Not updating bridge info: no Matrix room created")
		return
	}
	portal.log.Debug().Msg("Updating bridge info...")
	stateKey, content := portal.getBridgeInfo()
	_, err := portal.MainIntent().SendStateEvent(ctx, portal.MXID, event.StateBridge, stateKey, content)
	if err != nil {
		portal.log.Warn().Err(err).Msg("Failed to update m.bridge")
	}
	// TODO remove this once https://github.com/matrix-org/matrix-doc/pull/2346 is in spec
	_, err = portal.MainIntent().SendStateEvent(ctx, portal.MXID, event.StateHalfShotBridge, stateKey, content)
	if err != nil {
		portal.log.Warn().Err(err).Msg("Failed to update uk.half-shot.bridge")
	}
}

func (portal *Portal) messageLoop() {
	for {
		select {
		case msg := <-portal.matrixMessages:
			portal.handleMatrixMessages(msg)
		case msg := <-portal.metaMessages:
			portal.handleMetaMessage(msg)
		}
	}
}

func (portal *Portal) handleMatrixMessages(msg portalMatrixMessage) {
	log := portal.log.With().
		Str("action", "handle matrix event").
		Str("event_id", msg.evt.ID.String()).
		Str("event_type", msg.evt.Type.String()).
		Logger()
	ctx := log.WithContext(context.TODO())

	evtTS := time.UnixMilli(msg.evt.Timestamp)
	timings := messageTimings{
		initReceive:  msg.evt.Mautrix.ReceivedAt.Sub(evtTS),
		decrypt:      msg.evt.Mautrix.DecryptionDuration,
		totalReceive: time.Since(evtTS),
	}
	implicitRRStart := time.Now()
	if portal.ThreadType.IsWhatsApp() {
		portal.handleMatrixReadReceiptForWhatsApp(ctx, msg.user, "", evtTS, false)
	}
	timings.implicitRR = time.Since(implicitRRStart)

	switch msg.evt.Type {
	case event.EventMessage, event.EventSticker:
		portal.handleMatrixMessage(ctx, msg.user, msg.evt, timings)
	case event.EventRedaction:
		portal.handleMatrixRedaction(ctx, msg.user, msg.evt)
	case event.EventReaction:
		portal.handleMatrixReaction(ctx, msg.user, msg.evt)
	default:
		log.Warn().Str("type", msg.evt.Type.Type).Msg("Unhandled matrix message type")
	}
}

func (portal *Portal) HandleMatrixReadReceipt(brUser bridge.User, eventID id.EventID, receipt event.ReadReceipt) {
	user := brUser.(*User)
	log := portal.log.With().
		Str("action", "handle matrix receipt").
		Stringer("event_id", eventID).
		Stringer("user_mxid", user.MXID).
		Int64("user_meta_id", user.MetaID).
		Logger()
	ctx := log.WithContext(context.TODO())
	if portal.ThreadType.IsWhatsApp() {
		portal.handleMatrixReadReceiptForWhatsApp(ctx, user, eventID, receipt.Timestamp, true)
	} else {
		portal.handleMatrixReadReceiptForMessenger(ctx, user, eventID, receipt.Timestamp)
	}
}

func (portal *Portal) handleMatrixReadReceiptForMessenger(ctx context.Context, sender *User, eventID id.EventID, receiptTimestamp time.Time) {
	log := zerolog.Ctx(ctx)
	if !sender.IsLoggedIn() {
		log.Debug().Msg("Ignoring read receipt: user is not connected to Meta")
		return
	}
	readWatermark := receiptTimestamp
	targetMsg, err := portal.bridge.DB.Message.GetByMXID(ctx, eventID)
	if err != nil {
		log.Err(err).Msg("Failed to get read receipt target message")
	} else if targetMsg != nil {
		readWatermark = targetMsg.Timestamp
	}
	resp, err := sender.Client.ExecuteTasks(&socket.ThreadMarkReadTask{
		ThreadId:            portal.ThreadID,
		LastReadWatermarkTs: receiptTimestamp.UnixMilli(),
		SyncGroup:           1,
	})
	log.Trace().Any("response", resp).Msg("Read receipt send response")
	if err != nil {
		log.Err(err).Time("read_watermark", readWatermark).Msg("Failed to send read receipt")
	} else {
		log.Debug().Time("read_watermark", readWatermark).Msg("Read receipt sent")
	}
}

func (portal *Portal) handleMatrixReadReceiptForWhatsApp(ctx context.Context, sender *User, eventID id.EventID, receiptTimestamp time.Time, isExplicit bool) {
	log := zerolog.Ctx(ctx)
	if !sender.IsE2EEConnected() {
		if isExplicit {
			log.Debug().Msg("Ignoring read receipt: user is not connected to WhatsApp")
		}
		return
	}

	maxTimestamp := receiptTimestamp
	// Implicit read receipts don't have an event ID that's already bridged
	if isExplicit {
		if message, err := portal.bridge.DB.Message.GetByMXID(ctx, eventID); err != nil {
			log.Err(err).Msg("Failed to get read receipt target message")
		} else if message != nil {
			maxTimestamp = message.Timestamp
		}
	}

	prevTimestamp := sender.GetLastReadTS(ctx, portal.PortalKey)
	lastReadIsZero := false
	if prevTimestamp.IsZero() {
		prevTimestamp = maxTimestamp.Add(-2 * time.Second)
		lastReadIsZero = true
	}

	messages, err := portal.bridge.DB.Message.GetAllBetweenTimestamps(ctx, portal.PortalKey, prevTimestamp, maxTimestamp)
	if err != nil {
		log.Err(err).Msg("Failed to get messages for read receipt")
		return
	}
	if len(messages) > 0 {
		sender.SetLastReadTS(ctx, portal.PortalKey, messages[len(messages)-1].Timestamp)
	}
	groupedMessages := make(map[types.JID][]types.MessageID)
	for _, msg := range messages {
		var key types.JID
		if msg.Sender == sender.MetaID || msg.IsUnencrypted() {
			// Don't send read receipts for own messages or unencrypted messages
			continue
		} else if !portal.IsPrivateChat() {
			// TODO: this is hacky since it hardcodes the server
			key = types.JID{User: strconv.FormatInt(msg.Sender, 10), Server: types.MessengerServer}
		} // else: blank key (participant field isn't needed in direct chat read receipts)
		groupedMessages[key] = append(groupedMessages[key], msg.ID)
	}
	// For explicit read receipts, log even if there are no targets. For implicit ones only log when there are targets
	if len(groupedMessages) > 0 || isExplicit {
		log.Debug().
			Time("last_read", prevTimestamp).
			Bool("last_read_was_zero", lastReadIsZero).
			Bool("explicit", isExplicit).
			Any("receipts", groupedMessages).
			Msg("Sending read receipts")
	}
	for messageSender, ids := range groupedMessages {
		err = sender.E2EEClient.MarkRead(ids, receiptTimestamp, portal.JID(), messageSender)
		if err != nil {
			log.Err(err).Strs("ids", ids).Msg("Failed to mark messages as read")
		}
	}
}

const MaxEditCount = 5
const MaxEditTime = 15 * time.Minute

func (portal *Portal) handleMatrixMessage(ctx context.Context, sender *User, evt *event.Event, timings messageTimings) {
	log := zerolog.Ctx(ctx)
	start := time.Now()

	messageAge := timings.totalReceive
	ms := metricSender{portal: portal, timings: &timings, ctx: ctx}
	log.Debug().
		Str("sender", evt.Sender.String()).
		Dur("age", messageAge).
		Msg("Received message")

	errorAfter := portal.bridge.Config.Bridge.MessageHandlingTimeout.ErrorAfter
	deadline := portal.bridge.Config.Bridge.MessageHandlingTimeout.Deadline
	isScheduled, _ := evt.Content.Raw["com.beeper.scheduled"].(bool)
	if isScheduled {
		log.Debug().Msg("Message is a scheduled message, extending handling timeouts")
		errorAfter *= 10
		deadline *= 10
	}

	if errorAfter > 0 {
		remainingTime := errorAfter - messageAge
		if remainingTime < 0 {
			go ms.sendMessageMetrics(evt, errTimeoutBeforeHandling, "Timeout handling", true)
			return
		} else if remainingTime < 1*time.Second {
			log.Warn().
				Dur("remaining_time", remainingTime).
				Dur("max_timeout", errorAfter).
				Msg("Message was delayed before reaching the bridge")
		}
		go func() {
			time.Sleep(remainingTime)
			ms.sendMessageMetrics(evt, errMessageTakingLong, "Timeout handling", false)
		}()
	}

	if deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, deadline)
		defer cancel()
	}

	timings.preproc = time.Since(start)
	start = time.Now()

	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		log.Error().Type("content_type", content).Msg("Unexpected parsed content type")
		go ms.sendMessageMetrics(evt, fmt.Errorf("%w %T", errUnexpectedParsedContentType, evt.Content.Parsed), "Error converting", true)
		return
	}
	if content.MsgType == event.MsgNotice && !portal.bridge.Config.Bridge.BridgeNotices {
		go ms.sendMessageMetrics(evt, errMNoticeDisabled, "Error converting", true)
		return
	}

	realSenderMXID := sender.MXID
	isRelay := false
	// TODO check login for correct client (e2ee vs not e2ee)
	if !sender.IsLoggedIn() {
		sender = portal.GetRelayUser()
		if sender == nil {
			go ms.sendMessageMetrics(evt, errUserNotLoggedIn, "Ignoring", true)
			return
		} else if !sender.IsLoggedIn() {
			go ms.sendMessageMetrics(evt, errRelaybotNotLoggedIn, "Ignoring", true)
			return
		}
		isRelay = true
	}

	if editTarget := content.RelatesTo.GetReplaceID(); editTarget != "" {
		portal.handleMatrixEdit(ctx, sender, isRelay, realSenderMXID, &ms, evt, content)
		return
	}

	relaybotFormatted := isRelay && portal.addRelaybotFormat(ctx, realSenderMXID, evt, content)
	var otid int64
	var tasks []socket.Task
	var waMsg *waConsumerApplication.ConsumerApplication
	var waMeta *waMsgApplication.MessageApplication_Metadata
	var err error
	if portal.ThreadType.IsWhatsApp() {
		ctx = context.WithValue(ctx, msgconvContextKeyE2EEClient, sender.E2EEClient)
		waMsg, waMeta, err = portal.MsgConv.ToWhatsApp(ctx, evt, content, relaybotFormatted)
	} else {
		ctx = context.WithValue(ctx, msgconvContextKeyClient, sender.Client)
		tasks, otid, err = portal.MsgConv.ToMeta(ctx, evt, content, relaybotFormatted)
		if errors.Is(err, metaTypes.ErrPleaseReloadPage) && sender.canReconnect() {
			log.Err(err).Msg("Got please reload page error while converting message, reloading page in background")
			go sender.FullReconnect()
			err = errReloading
		} else if errors.Is(err, messagix.ErrTokenInvalidated) {
			go sender.DisconnectFromError(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      MetaCookieRemoved,
			})
			err = errLoggedOut
		}
	}
	if err != nil {
		log.Err(err).Msg("Failed to convert message")
		go ms.sendMessageMetrics(evt, err, "Error converting", true)
		return
	}

	timings.convert = time.Since(start)
	start = time.Now()

	if waMsg != nil {
		messageID := sender.E2EEClient.GenerateMessageID()
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("message_id", messageID)
		})
		log.Debug().Msg("Sending Matrix message to WhatsApp")
		var resp whatsmeow.SendResponse
		resp, err = sender.E2EEClient.SendFBMessage(ctx, portal.JID(), waMsg, waMeta, whatsmeow.SendRequestExtra{
			ID: messageID,
		})
		// TODO save message in db before sending and only update timestamp later
		portal.storeMessageInDB(ctx, evt.ID, messageID, 0, sender.MetaID, resp.Timestamp, 0)
	} else {
		log.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Int64("otid", otid)
		})
		log.Debug().Msg("Sending Matrix message to Meta")
		otidStr := strconv.FormatInt(otid, 10)
		portal.pendingMessages[otid] = evt.ID
		messageTS := time.Now()
		var resp *table.LSTable
		resp, err = sender.Client.ExecuteTasks(tasks...)
		log.Trace().Any("response", resp).Msg("Meta send response")
		var msgID string
		if err == nil {
			for _, replace := range resp.LSReplaceOptimsiticMessage {
				if replace.OfflineThreadingId == otidStr {
					msgID = replace.MessageId
				}
			}
			if len(msgID) == 0 {
				for _, failed := range resp.LSMarkOptimisticMessageFailed {
					if failed.OTID == otidStr {
						log.Warn().Str("message", failed.Message).Msg("Sending message failed")
						go ms.sendMessageMetrics(evt, fmt.Errorf("%w: %s", errServerRejected, failed.Message), "Error sending", true)
						return
					}
				}
				for _, failed := range resp.LSHandleFailedTask {
					if failed.OTID == otidStr {
						log.Warn().Str("message", failed.Message).Msg("Sending message failed")
						go ms.sendMessageMetrics(evt, fmt.Errorf("%w: %s", errServerRejected, failed.Message), "Error sending", true)
						return
					}
				}
				log.Warn().Msg("Message send response didn't include message ID")
			}
		}
		if msgID != "" {
			portal.pendingMessagesLock.Lock()
			_, ok = portal.pendingMessages[otid]
			if ok {
				portal.storeMessageInDB(ctx, evt.ID, msgID, otid, sender.MetaID, messageTS, 0)
				delete(portal.pendingMessages, otid)
			} else {
				log.Debug().Msg("Not storing message send response: pending message was already removed from map")
			}
			portal.pendingMessagesLock.Unlock()
		}
	}

	timings.totalSend = time.Since(start)
	go ms.sendMessageMetrics(evt, err, "Error sending", true)
}

func (portal *Portal) redactFailedEdit(ctx context.Context, evtID id.EventID, reason string) {
	_, err := portal.MainIntent().RedactEvent(ctx, portal.MXID, evtID, mautrix.ReqRedact{
		Reason: reason,
	})
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to redact failed Matrix edit")
	}
}

func (portal *Portal) handleMatrixEdit(ctx context.Context, sender *User, isRelay bool, realSenderMXID id.UserID, ms *metricSender, evt *event.Event, content *event.MessageEventContent) {
	log := zerolog.Ctx(ctx)
	editTarget := content.RelatesTo.GetReplaceID()
	editTargetMsg, err := portal.bridge.DB.Message.GetByMXID(ctx, editTarget)
	if err != nil {
		log.Err(err).Stringer("edit_target_mxid", editTarget).Msg("Failed to get edit target message")
		go ms.sendMessageMetrics(evt, errFailedToGetEditTarget, "Error converting", true)
		return
	} else if editTargetMsg == nil {
		log.Err(err).Stringer("edit_target_mxid", editTarget).Msg("Edit target message not found")
		go ms.sendMessageMetrics(evt, errEditUnknownTarget, "Error converting", true)
		return
	} else if editTargetMsg.Sender != sender.MetaID {
		go ms.sendMessageMetrics(evt, errEditDifferentSender, "Error converting", true)
		return
	} else if !portal.ThreadType.IsWhatsApp() && editTargetMsg.EditCount >= MaxEditCount {
		go ms.sendMessageMetrics(evt, errEditCountExceeded, "Error converting", true)
		go portal.redactFailedEdit(ctx, evt.ID, errEditCountExceeded.Error())
		return
	} else if !portal.ThreadType.IsWhatsApp() && time.Since(editTargetMsg.Timestamp) > MaxEditTime {
		go ms.sendMessageMetrics(evt, errEditTooOld, "Error converting", true)
		go portal.redactFailedEdit(ctx, evt.ID, errEditTooOld.Error())
		return
	}
	if content.NewContent != nil {
		content = content.NewContent
		evt.Content.Parsed = content
	}

	if isRelay {
		portal.addRelaybotFormat(ctx, realSenderMXID, evt, content)
	}
	newEditCount := editTargetMsg.EditCount + 1
	if portal.ThreadType.IsWhatsApp() {
		consumerMsg := wrapEdit(&waConsumerApplication.ConsumerApplication_EditMessage{
			Key:         portal.buildMessageKey(sender, editTargetMsg),
			Message:     portal.MsgConv.TextToWhatsApp(content),
			TimestampMS: evt.Timestamp,
		})
		var resp whatsmeow.SendResponse
		resp, err = sender.E2EEClient.SendFBMessage(ctx, portal.JID(), consumerMsg, nil)
		log.Trace().Any("response", resp).Msg("WhatsApp delete response")
	} else {
		editTask := &socket.EditMessageTask{
			MessageID: editTargetMsg.ID,
			Text:      content.Body,
		}
		var resp *table.LSTable
		resp, err = sender.Client.ExecuteTasks(editTask)
		log.Trace().Any("response", resp).Msg("Meta edit response")
		if err == nil {
			if len(resp.LSEditMessage) == 0 {
				log.Debug().Msg("Edit response didn't contain new edit?")
			} else if resp.LSEditMessage[0].MessageID != editTargetMsg.ID {
				log.Debug().Msg("Edit response contained different message ID")
			} else if resp.LSEditMessage[0].Text != content.Body {
				log.Warn().Msg("Server returned edit with different text")
				err = errEditReverted
				go portal.redactFailedEdit(ctx, evt.ID, err.Error())
			} else if resp.LSEditMessage[0].EditCount != newEditCount {
				log.Warn().
					Int64("expected_edit_count", newEditCount).
					Int64("actual_edit_count", resp.LSEditMessage[0].EditCount).
					Msg("Edit count mismatch")
			}
		}
	}
	go ms.sendMessageMetrics(evt, err, "Error sending", true)
	if err == nil {
		// TODO does the response contain the edit count?
		err = editTargetMsg.UpdateEditCount(ctx, newEditCount)
		if err != nil {
			log.Err(err).Msg("Failed to update edit count")
		}
	}
}

func (portal *Portal) handleMatrixRedaction(ctx context.Context, sender *User, evt *event.Event) {
	log := zerolog.Ctx(ctx)
	dbMessage, err := portal.bridge.DB.Message.GetByMXID(ctx, evt.Redacts)
	if err != nil {
		log.Err(err).Msg("Failed to get redaction target message")
	}
	dbReaction, err := portal.bridge.DB.Reaction.GetByMXID(ctx, evt.Redacts)
	if err != nil {
		log.Err(err).Msg("Failed to get redaction target reaction")
	}

	if !sender.IsLoggedIn() {
		sender = portal.GetRelayUser()
		if sender == nil {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, errUserNotLoggedIn)
			return
		} else if !sender.IsLoggedIn() {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, errRelaybotNotLoggedIn)
			return
		}
	}

	if dbMessage != nil {
		if dbMessage.Sender != sender.MetaID {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, errRedactionTargetSentBySomeoneElse)
			return
		}
		if !dbMessage.IsUnencrypted() {
			consumerMsg := wrapRevoke(&waConsumerApplication.ConsumerApplication_RevokeMessage{
				Key: portal.buildMessageKey(sender, dbMessage),
			})
			var resp whatsmeow.SendResponse
			resp, err = sender.E2EEClient.SendFBMessage(ctx, portal.JID(), consumerMsg, nil)
			log.Trace().Any("response", resp).Msg("WhatsApp delete response")
		} else {
			var resp *table.LSTable
			resp, err = sender.Client.ExecuteTasks(&socket.DeleteMessageTask{MessageId: dbMessage.ID})
			// TODO does the response data need to be checked?
			log.Trace().Any("response", resp).Msg("Instagram delete response")
		}
		if err != nil {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, err)
			log.Err(err).Msg("Failed to send message redaction to Meta")
			return
		}
		err = dbMessage.Delete(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to delete redacted message from database")
		} else if otherParts, err := portal.bridge.DB.Message.GetAllPartsByID(ctx, dbMessage.ID, portal.Receiver); err != nil {
			log.Err(err).Msg("Failed to get other parts of redacted message from database")
		} else if len(otherParts) > 0 {
			// If there are other parts of the message, send a redaction for each of them
			for _, otherPart := range otherParts {
				_, err = portal.MainIntent().RedactEvent(ctx, portal.MXID, otherPart.MXID, mautrix.ReqRedact{
					Reason: "Other part of redacted message",
					TxnID:  "mxmeta_partredact_" + otherPart.MXID.String(),
				})
				if err != nil {
					log.Err(err).
						Str("part_event_id", otherPart.MXID.String()).
						Int("part_index", otherPart.PartIndex).
						Msg("Failed to redact other part of redacted message")
				}
				err = otherPart.Delete(ctx)
				if err != nil {
					log.Err(err).
						Str("part_event_id", otherPart.MXID.String()).
						Int("part_index", otherPart.PartIndex).
						Msg("Failed to delete other part of redacted message from database")
				}
			}
		}
		portal.sendMessageStatusCheckpointSuccess(ctx, evt)
	} else if dbReaction != nil {
		if dbReaction.Sender != sender.MetaID {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, errUnreactTargetSentBySomeoneElse)
			return
		}
		targetMsg, err := portal.bridge.DB.Message.GetByID(ctx, dbReaction.MessageID, 0, portal.Receiver)
		if err != nil {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, err)
			log.Err(err).Msg("Failed to get removed reaction target message")
			return
		} else if targetMsg == nil {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, errReactionTargetNotFound)
			log.Warn().Msg("Reaction target message not found")
			return
		}
		err = portal.sendReaction(ctx, sender, targetMsg, "", evt.Timestamp)
		if err != nil {
			portal.sendMessageStatusCheckpointFailed(ctx, evt, err)
			log.Err(err).Msg("Failed to send reaction redaction to Meta")
			return
		}
		err = dbReaction.Delete(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to delete redacted reaction from database")
		}
		portal.sendMessageStatusCheckpointSuccess(ctx, evt)
	} else {
		portal.sendMessageStatusCheckpointFailed(ctx, evt, errRedactionTargetNotFound)
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

func (portal *Portal) buildMessageKey(user *User, targetMsg *database.Message) *waCommon.MessageKey {
	var messageKeyParticipant string
	if !portal.IsPrivateChat() {
		// TODO: this is hacky since it hardcodes the server
		messageKeyParticipant = types.JID{User: strconv.FormatInt(targetMsg.Sender, 10), Server: types.MessengerServer}.String()
	}
	return &waCommon.MessageKey{
		RemoteJID:   portal.JID().String(),
		FromMe:      targetMsg.Sender == user.MetaID,
		ID:          targetMsg.ID,
		Participant: messageKeyParticipant,
	}
}

func (portal *Portal) sendReaction(ctx context.Context, sender *User, targetMsg *database.Message, metaEmoji string, timestamp int64) error {
	if !targetMsg.IsUnencrypted() {
		consumerMsg := wrapReaction(&waConsumerApplication.ConsumerApplication_ReactionMessage{
			Key:               portal.buildMessageKey(sender, targetMsg),
			Text:              metaEmoji,
			SenderTimestampMS: timestamp,
		})
		resp, err := sender.E2EEClient.SendFBMessage(ctx, portal.JID(), consumerMsg, nil)
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("WhatsApp reaction response")
		return err
	} else {
		resp, err := sender.Client.ExecuteTasks(&socket.SendReactionTask{
			ThreadKey:       portal.ThreadID,
			TimestampMs:     timestamp,
			MessageID:       targetMsg.ID,
			ActorID:         sender.MetaID,
			Reaction:        metaEmoji,
			SyncGroup:       1,
			SendAttribution: table.MESSENGER_INBOX_IN_THREAD,
		})
		// TODO save the hidden thread message from the response too?
		zerolog.Ctx(ctx).Trace().Any("response", resp).Msg("Instagram reaction response")
		return err
	}
}

func (portal *Portal) handleMatrixReaction(ctx context.Context, sender *User, evt *event.Event) {
	log := zerolog.Ctx(ctx)
	if !sender.IsLoggedIn() {
		portal.sendMessageStatusCheckpointFailed(ctx, evt, errCantRelayReactions)
		return
	}
	relatedEventID := evt.Content.AsReaction().RelatesTo.EventID
	targetMsg, err := portal.bridge.DB.Message.GetByMXID(ctx, relatedEventID)
	if err != nil {
		portal.sendMessageStatusCheckpointFailed(ctx, evt, err)
		log.Err(err).Msg("Failed to get reaction target message")
		return
	} else if targetMsg == nil {
		portal.sendMessageStatusCheckpointFailed(ctx, evt, errReactionTargetNotFound)
		log.Warn().Msg("Reaction target message not found")
		return
	}
	emoji := evt.Content.AsReaction().RelatesTo.Key
	metaEmoji := variationselector.Remove(emoji)

	err = portal.sendReaction(ctx, sender, targetMsg, metaEmoji, evt.Timestamp)
	if err != nil {
		portal.sendMessageStatusCheckpointFailed(ctx, evt, err)
		log.Err(err).Msg("Failed to send reaction")
		return
	}
	dbReaction, err := portal.bridge.DB.Reaction.GetByID(
		ctx,
		targetMsg.ID,
		portal.Receiver,
		sender.MetaID,
	)
	if err != nil {
		log.Err(err).Msg("Failed to get existing reaction from database")
	} else if dbReaction != nil {
		log.Debug().Stringer("existing_event_id", dbReaction.MXID).Msg("Redacting existing reaction after sending new one")
		_, err = portal.MainIntent().RedactEvent(ctx, portal.MXID, dbReaction.MXID)
		if err != nil {
			log.Err(err).Msg("Failed to redact existing reaction")
		}
	}
	if dbReaction != nil {
		dbReaction.MXID = evt.ID
		dbReaction.Emoji = metaEmoji
		err = dbReaction.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to update reaction in database")
		}
	} else {
		dbReaction = portal.bridge.DB.Reaction.New()
		dbReaction.MXID = evt.ID
		dbReaction.RoomID = portal.MXID
		dbReaction.MessageID = targetMsg.ID
		dbReaction.ThreadID = portal.ThreadID
		dbReaction.ThreadReceiver = portal.Receiver
		dbReaction.Sender = sender.MetaID
		dbReaction.Emoji = metaEmoji
		err = dbReaction.Insert(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to insert reaction to database")
		}
	}

	portal.sendMessageStatusCheckpointSuccess(ctx, evt)
}

func (portal *Portal) sendMessageStatusCheckpointSuccess(ctx context.Context, evt *event.Event) {
	portal.sendDeliveryReceipt(ctx, evt.ID)
	portal.bridge.SendMessageSuccessCheckpoint(evt, status.MsgStepRemote, 0)
	portal.sendStatusEvent(ctx, evt.ID, "", nil, nil)
}

func (portal *Portal) sendMessageStatusCheckpointFailed(ctx context.Context, evt *event.Event, err error) {
	portal.sendDeliveryReceipt(ctx, evt.ID)
	portal.bridge.SendMessageErrorCheckpoint(evt, status.MsgStepRemote, err, true, 0)
	portal.sendStatusEvent(ctx, evt.ID, "", err, nil)
}

type msgconvContextKey int

const (
	msgconvContextKeyIntent msgconvContextKey = iota
	msgconvContextKeyClient
	msgconvContextKeyE2EEClient
	msgconvContextKeyBackfill
)

type backfillType int

const (
	backfillTypeForward backfillType = iota + 1
	backfillTypeHistorical
)

func (portal *Portal) ShouldFetchXMA(ctx context.Context) bool {
	xmaDisabled := ctx.Value(msgconvContextKeyBackfill) == backfillTypeHistorical && portal.bridge.Config.Bridge.Backfill.Queue.DontFetchXMA
	return !xmaDisabled && !portal.bridge.Config.Bridge.DisableXMA
}

func (portal *Portal) UploadMatrixMedia(ctx context.Context, data []byte, fileName, contentType string) (id.ContentURIString, error) {
	intent := ctx.Value(msgconvContextKeyIntent).(*appservice.IntentAPI)
	req := mautrix.ReqUploadMedia{
		ContentBytes: data,
		ContentType:  contentType,
		FileName:     fileName,
	}
	if portal.bridge.Config.Homeserver.AsyncMedia {
		uploaded, err := intent.UploadAsync(ctx, req)
		if err != nil {
			return "", err
		}
		return uploaded.ContentURI.CUString(), nil
	} else {
		uploaded, err := intent.UploadMedia(ctx, req)
		if err != nil {
			return "", err
		}
		return uploaded.ContentURI.CUString(), nil
	}
}

func (portal *Portal) DownloadMatrixMedia(ctx context.Context, uriString id.ContentURIString) ([]byte, error) {
	parsedURI, err := uriString.Parse()
	if err != nil {
		return nil, fmt.Errorf("malformed content URI: %w", err)
	}
	return portal.MainIntent().DownloadBytes(ctx, parsedURI)
}

func (portal *Portal) GetData(ctx context.Context) *database.Portal {
	return portal.Portal
}

func (portal *Portal) GetClient(ctx context.Context) *messagix.Client {
	return ctx.Value(msgconvContextKeyClient).(*messagix.Client)
}

func (portal *Portal) GetE2EEClient(ctx context.Context) *whatsmeow.Client {
	return ctx.Value(msgconvContextKeyE2EEClient).(*whatsmeow.Client)
}

func (portal *Portal) GetMatrixReply(ctx context.Context, replyToID string, replyToUser int64) (replyTo id.EventID, replyTargetSender id.UserID) {
	if replyToID == "" {
		return
	}
	log := zerolog.Ctx(ctx).With().
		Str("reply_target_id", replyToID).
		Logger()
	if message, err := portal.bridge.DB.Message.GetByID(ctx, replyToID, 0, portal.Receiver); err != nil {
		log.Err(err).Msg("Failed to get reply target message from database")
	} else if message == nil {
		if ctx.Value(msgconvContextKeyBackfill) != nil && portal.bridge.Config.Homeserver.Software == bridgeconfig.SoftwareHungry {
			replyTo = portal.deterministicEventID(replyToID, 0)
		} else {
			log.Warn().Msg("Reply target message not found")
			return
		}
	} else {
		replyTo = message.MXID
		if replyToUser != 0 && message.Sender != replyToUser {
			log.Warn().
				Int64("message_sender", message.Sender).
				Int64("reply_to_user", replyToUser).
				Msg("Mismatching reply to user and found message sender")
		}
		replyToUser = message.Sender
	}
	targetUser := portal.bridge.GetUserByMetaID(replyToUser)
	if targetUser != nil {
		replyTargetSender = targetUser.MXID
	} else {
		replyTargetSender = portal.bridge.FormatPuppetMXID(replyToUser)
	}
	return
}

func (portal *Portal) GetMetaReply(ctx context.Context, content *event.MessageEventContent) *socket.ReplyMetaData {
	replyToID := content.RelatesTo.GetReplyTo()
	if len(replyToID) == 0 {
		return nil
	}
	replyToMsg, err := portal.bridge.DB.Message.GetByMXID(ctx, replyToID)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).
			Str("reply_to_mxid", replyToID.String()).
			Msg("Failed to get reply target message from database")
	} else if replyToMsg == nil {
		zerolog.Ctx(ctx).Warn().
			Str("reply_to_mxid", replyToID.String()).
			Msg("Reply target message not found")
	} else {
		return &socket.ReplyMetaData{
			ReplyMessageId:  replyToMsg.ID,
			ReplySourceType: 1,
			ReplyType:       0,
			ReplySender:     replyToMsg.Sender,
		}
	}
	return nil
}

func (portal *Portal) GetUserMXID(ctx context.Context, userID int64) id.UserID {
	user := portal.bridge.GetUserByMetaID(userID)
	if user != nil {
		return user.MXID
	}
	return portal.bridge.FormatPuppetMXID(userID)
}

func (portal *Portal) handleMetaMessage(portalMessage portalMetaMessage) {
	switch typedEvt := portalMessage.evt.(type) {
	case *events.FBMessage:
		portal.handleEncryptedMessage(portalMessage.user, typedEvt)
	case *events.Receipt:
		portal.handleWhatsAppReceipt(portalMessage.user, typedEvt)
	case *table.WrappedMessage:
		portal.handleMetaInsertMessage(portalMessage.user, typedEvt)
	case *table.UpsertMessages:
		portal.handleMetaUpsertMessages(portalMessage.user, typedEvt)
	case *table.LSUpdateExistingMessageRange:
		portal.handleMetaExistingRange(portalMessage.user, typedEvt)
	case *table.LSEditMessage:
		portal.handleMetaEditMessage(typedEvt)
	case *table.LSDeleteMessage:
		portal.handleMetaDelete(typedEvt.MessageId)
	case *table.LSDeleteThenInsertMessage:
		if typedEvt.IsUnsent {
			portal.handleMetaDelete(typedEvt.MessageId)
		} else {
			portal.log.Warn().
				Str("message_id", typedEvt.MessageId).
				Int64("edit_count", typedEvt.EditCount).
				Msg("Got unexpected non-unsend DeleteThenInsertMessage command")
		}
	case *table.LSUpsertReaction:
		portal.handleMetaReaction(typedEvt)
	case *table.LSDeleteReaction:
		portal.handleMetaReactionDelete(typedEvt)
	case *table.LSUpdateReadReceipt:
		portal.handleMetaReadReceipt(typedEvt)
	case *table.LSMarkThreadRead:
		portal.handleMetaReadReceipt(&table.LSUpdateReadReceipt{
			ReadWatermarkTimestampMs: typedEvt.LastReadWatermarkTimestampMs,
			ContactId:                portalMessage.user.MetaID,
			ReadActionTimestampMs:    time.Now().UnixMilli(),
		})
	case *table.LSUpdateTypingIndicator:
		portal.handleMetaTypingIndicator(typedEvt)
	case *table.LSSyncUpdateThreadName:
		portal.handleMetaNameChange(typedEvt)
	case *table.LSSetThreadImageURL:
		portal.handleMetaAvatarChange(typedEvt)
	case *table.LSMoveThreadToE2EECutoverFolder:
		if portal.ThreadType == table.ONE_TO_ONE {
			portal.log.Debug().Msg("Updating thread type to WA 1:1 after MoveThreadToE2EECutoverFolder event")
			portal.ThreadType = table.ENCRYPTED_OVER_WA_ONE_TO_ONE
			err := portal.Update(context.TODO())
			if err != nil {
				portal.log.Err(err).Msg("Failed to save portal")
			}
		}
	case *table.LSDeleteThread:
		portal.log.Info().Msg("Deleting portal due to delete thread event")
		ctx := context.TODO()
		portal.Delete()
		portal.Cleanup(ctx, false)
	default:
		portal.log.Error().
			Type("data_type", typedEvt).
			Msg("Invalid inner event type inside meta message")
	}
}

func (portal *Portal) checkPendingMessage(ctx context.Context, messageID string, otid, sender int64, timestamp time.Time) bool {
	if otid == 0 {
		return false
	}
	portal.pendingMessagesLock.Lock()
	defer portal.pendingMessagesLock.Unlock()
	pendingEventID, ok := portal.pendingMessages[otid]
	if !ok {
		return false
	}
	portal.storeMessageInDB(ctx, pendingEventID, messageID, otid, sender, timestamp, 0)
	delete(portal.pendingMessages, otid)
	zerolog.Ctx(ctx).Debug().Stringer("pending_event_id", pendingEventID).Msg("Saved pending message ID")
	return true
}

func (portal *Portal) handleWhatsAppReceipt(source *User, receipt *events.Receipt) {
	if receipt.Type != types.ReceiptTypeRead && receipt.Type != types.ReceiptTypeReadSelf {
		return
	}
	senderID := int64(receipt.Sender.UserInt())
	if senderID == 0 {
		return
	}
	log := portal.log.With().
		Str("action", "handle whatsapp receipt").
		Stringer("chat_jid", receipt.Chat).
		Stringer("receipt_sender_jid", receipt.Sender).
		Strs("message_ids", receipt.MessageIDs).
		Time("receipt_timestamp", receipt.Timestamp).
		Logger()
	ctx := log.WithContext(context.TODO())
	markAsRead := make([]*database.Message, 0, 1)
	var bestTimestamp time.Time
	for _, msgID := range receipt.MessageIDs {
		msg, err := portal.bridge.DB.Message.GetLastPartByID(ctx, msgID, portal.Receiver)
		if err != nil {
			log.Err(err).Msg("Failed to get message from database")
		}
		if msg == nil {
			continue
		}
		if msg.Timestamp.After(bestTimestamp) {
			bestTimestamp = msg.Timestamp
			markAsRead = append(markAsRead[:0], msg)
		} else if msg != nil && msg.Timestamp.Equal(bestTimestamp) {
			markAsRead = append(markAsRead, msg)
		}
	}
	if senderID == source.MetaID {
		if len(markAsRead) > 0 {
			source.SetLastReadTS(ctx, portal.PortalKey, markAsRead[0].Timestamp)
		} else {
			source.SetLastReadTS(ctx, portal.PortalKey, receipt.Timestamp)
		}
	}
	sender := portal.bridge.GetPuppetByID(senderID)
	for _, msg := range markAsRead {
		// TODO bridge read-self as m.read.private?
		err := portal.SendReadReceipt(ctx, sender, msg.MXID)
		if err != nil {
			log.Err(err).Stringer("event_id", msg.MXID).Msg("Failed to mark event as read")
		} else {
			log.Debug().Stringer("event_id", msg.MXID).Msg("Marked event as read")
		}
	}
}

func (portal *Portal) handleEncryptedMessage(source *User, evt *events.FBMessage) {
	sender := portal.bridge.GetPuppetByID(int64(evt.Info.Sender.UserInt()))
	log := portal.log.With().
		Str("action", "handle whatsapp message").
		Stringer("chat_jid", evt.Info.Chat).
		Stringer("sender_jid", evt.Info.Sender).
		Str("message_id", evt.Info.ID).
		Time("message_ts", evt.Info.Timestamp).
		Logger()
	ctx := log.WithContext(context.TODO())
	sender.FetchAndUpdateInfoIfNecessary(ctx, source)

	switch typedMsg := evt.Message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		switch payload := typedMsg.GetPayload().GetPayload().(type) {
		case *waConsumerApplication.ConsumerApplication_Payload_Content:
			switch content := payload.Content.GetContent().(type) {
			case *waConsumerApplication.ConsumerApplication_Content_EditMessage:
				portal.handleWhatsAppEditMessage(ctx, sender, content.EditMessage)
			case *waConsumerApplication.ConsumerApplication_Content_ReactionMessage:
				portal.handleMetaOrWhatsAppReaction(ctx, sender, content.ReactionMessage.GetKey().GetID(), content.ReactionMessage.GetText(), content.ReactionMessage.GetSenderTimestampMS())
			default:
				portal.handleMetaOrWhatsAppMessage(ctx, source, sender, evt, nil)
			}
		case *waConsumerApplication.ConsumerApplication_Payload_ApplicationData:
			switch applicationContent := payload.ApplicationData.GetApplicationContent().(type) {
			case *waConsumerApplication.ConsumerApplication_ApplicationData_Revoke:
				portal.handleMetaOrWhatsAppDelete(ctx, sender, applicationContent.Revoke.GetKey().GetID())
			default:
				log.Warn().Type("content_type", applicationContent).Msg("Unrecognized application content type")
			}
		case *waConsumerApplication.ConsumerApplication_Payload_Signal:
			log.Warn().Msg("Unsupported consumer signal payload message")
		case *waConsumerApplication.ConsumerApplication_Payload_SubProtocol:
			log.Warn().Msg("Unsupported consumer subprotocol payload message")
		default:
			log.Warn().Type("payload_type", payload).Msg("Unrecognized consumer message payload type")
		}
	case *waArmadilloApplication.Armadillo:
		switch payload := typedMsg.GetPayload().GetPayload().(type) {
		case *waArmadilloApplication.Armadillo_Payload_Content:
			portal.handleMetaOrWhatsAppMessage(ctx, source, sender, evt, nil)
		case *waArmadilloApplication.Armadillo_Payload_ApplicationData:
			log.Warn().Msg("Unsupported armadillo application data message")
		case *waArmadilloApplication.Armadillo_Payload_Signal:
			log.Warn().Msg("Unsupported armadillo signal payload message")
		case *waArmadilloApplication.Armadillo_Payload_SubProtocol:
			log.Warn().Msg("Unsupported armadillo subprotocol payload message")
		default:
			log.Warn().Type("payload_type", payload).Msg("Unrecognized armadillo message payload type")
		}
	default:
		log.Warn().Type("message_type", evt.Message).Msg("Unrecognized message type")
	}
}

func (portal *Portal) handleMetaInsertMessage(source *User, message *table.WrappedMessage) {
	sender := portal.bridge.GetPuppetByID(message.SenderId)
	log := portal.log.With().
		Str("action", "insert meta message").
		Int64("sender_id", sender.ID).
		Str("message_id", message.MessageId).
		Str("otid", message.OfflineThreadingId).
		Logger()
	ctx := log.WithContext(context.TODO())
	portal.handleMetaOrWhatsAppMessage(ctx, source, sender, nil, message)
	log.Debug().Msg("Finished handling Meta message")
}

func (portal *Portal) handleMetaOrWhatsAppMessage(ctx context.Context, source *User, sender *Puppet, waMsg *events.FBMessage, metaMsg *table.WrappedMessage) {
	log := zerolog.Ctx(ctx)

	if portal.MXID == "" {
		log.Debug().Msg("Creating Matrix room from incoming message")
		if err := portal.CreateMatrixRoom(ctx, source); err != nil {
			log.Error().Err(err).Msg("Failed to create portal room")
			return
		}
	}

	var messageID string
	var messageTime time.Time
	var otidInt int64
	if waMsg != nil {
		messageID = waMsg.Info.ID
		messageTime = waMsg.Info.Timestamp
	} else {
		messageID = metaMsg.MessageId
		otidInt, _ = strconv.ParseInt(metaMsg.OfflineThreadingId, 10, 64)
		messageTime = time.UnixMilli(metaMsg.TimestampMs)
		if portal.checkPendingMessage(ctx, metaMsg.MessageId, otidInt, sender.ID, messageTime) {
			return
		}
	}

	existingMessage, err := portal.bridge.DB.Message.GetByID(ctx, messageID, 0, portal.Receiver)
	if err != nil {
		log.Err(err).Msg("Failed to check if message was already bridged")
		return
	} else if existingMessage != nil {
		log.Debug().Msg("Ignoring duplicate message")
		return
	}

	intent := sender.IntentFor(portal)
	ctx = context.WithValue(ctx, msgconvContextKeyIntent, intent)
	var converted *msgconv.ConvertedMessage
	if waMsg != nil {
		ctx = context.WithValue(ctx, msgconvContextKeyE2EEClient, source.E2EEClient)
		converted = portal.MsgConv.WhatsAppToMatrix(ctx, waMsg)
	} else {
		ctx = context.WithValue(ctx, msgconvContextKeyClient, source.Client)
		converted = portal.MsgConv.ToMatrix(ctx, metaMsg)
	}
	if portal.bridge.Config.Bridge.CaptionInMessage {
		converted.MergeCaption()
	}
	if len(converted.Parts) == 0 {
		log.Warn().Msg("Message was empty after conversion")
		return
	}
	for i, part := range converted.Parts {
		user, err := portal.bridge.DB.User.GetByMetaID(context.TODO(), sender.ID)
		if err == nil && user != nil {
			part.Extra["mx_sender_id"] = user.MXID
		}
		resp, err := portal.sendMatrixEvent(ctx, intent, part.Type, part.Content, part.Extra, messageTime.UnixMilli())
		if err != nil {
			log.Err(err).Int("part_index", i).Msg("Failed to send message to Matrix")
			continue
		}
		portal.storeMessageInDB(ctx, resp.EventID, messageID, otidInt, sender.ID, messageTime, i)
	}
}

func (portal *Portal) handleWhatsAppEditMessage(ctx context.Context, sender *Puppet, edit *waConsumerApplication.ConsumerApplication_EditMessage) {
	log := zerolog.Ctx(ctx).With().
		Int64("edit_ts", edit.TimestampMS).
		Logger()
	ctx = log.WithContext(context.TODO())
	targetMsg, err := portal.bridge.DB.Message.GetAllPartsByID(ctx, edit.GetKey().GetID(), portal.Receiver)
	if err != nil {
		log.Err(err).Msg("Failed to get edit target message")
		return
	} else if len(targetMsg) == 0 {
		log.Warn().Msg("Edit target message not found")
		return
	} else if len(targetMsg) > 1 {
		log.Warn().Msg("Ignoring edit of multipart message")
		return
	} else if targetMsg[0].EditTimestamp() >= edit.TimestampMS {
		log.Debug().Int64("existing_edit_ts", targetMsg[0].EditTimestamp()).Msg("Ignoring duplicate edit")
		return
	}
	converted := portal.MsgConv.WhatsAppTextToMatrix(ctx, edit.GetMessage())
	converted.Content.SetEdit(targetMsg[0].MXID)
	resp, err := portal.sendMatrixEvent(ctx, sender.IntentFor(portal), converted.Type, converted.Content, converted.Extra, edit.TimestampMS)
	if err != nil {
		log.Err(err).Msg("Failed to send edit to Matrix")
	} else if err := targetMsg[0].UpdateEditTimestamp(ctx, edit.TimestampMS); err != nil {
		log.Err(err).Stringer("event_id", resp.EventID).Msg("Failed to save message edit count to database")
	} else {
		log.Debug().Stringer("event_id", resp.EventID).Msg("Handled Meta message edit")
	}
}

func (portal *Portal) handleMetaEditMessage(edit *table.LSEditMessage) {
	log := portal.log.With().
		Str("action", "edit meta message").
		Str("message_id", edit.MessageID).
		Int64("edit_count", edit.EditCount).
		Logger()
	ctx := log.WithContext(context.TODO())
	targetMsg, err := portal.bridge.DB.Message.GetAllPartsByID(ctx, edit.MessageID, portal.Receiver)
	if err != nil {
		log.Err(err).Msg("Failed to get edit target message")
		return
	} else if len(targetMsg) == 0 {
		log.Warn().Msg("Edit target message not found")
		return
	} else if len(targetMsg) > 1 {
		log.Warn().Msg("Ignoring edit of multipart message")
		return
	} else if targetMsg[0].EditCount >= edit.EditCount {
		log.Debug().Int64("existing_edit_count", targetMsg[0].EditCount).Msg("Ignoring duplicate edit")
		return
	}
	sender := portal.bridge.GetPuppetByID(targetMsg[0].Sender)
	content := &event.MessageEventContent{
		MsgType:  event.MsgText,
		Body:     edit.Text,
		Mentions: &event.Mentions{},
	}
	content.SetEdit(targetMsg[0].MXID)
	resp, err := portal.sendMatrixEvent(ctx, sender.IntentFor(portal), event.EventMessage, content, map[string]any{}, 0)
	if err != nil {
		log.Err(err).Msg("Failed to send edit to Matrix")
	} else if err := targetMsg[0].UpdateEditCount(ctx, edit.EditCount); err != nil {
		log.Err(err).Stringer("event_id", resp.EventID).Msg("Failed to save message edit count to database")
	} else {
		log.Debug().Stringer("event_id", resp.EventID).Msg("Handled Meta message edit")
	}
}

func (portal *Portal) handleMetaReaction(react *table.LSUpsertReaction) {
	sender := portal.bridge.GetPuppetByID(react.ActorId)
	ctx := portal.log.With().
		Str("action", "upsert meta reaction").
		Int64("sender_id", sender.ID).
		Str("target_msg_id", react.MessageId).
		Logger().
		WithContext(context.TODO())
	portal.handleMetaOrWhatsAppReaction(ctx, sender, react.MessageId, react.Reaction, 0)
}

func (portal *Portal) handleMetaReactionDelete(react *table.LSDeleteReaction) {
	sender := portal.bridge.GetPuppetByID(react.ActorId)
	log := portal.log.With().
		Str("action", "delete meta reaction").
		Int64("sender_id", sender.ID).
		Str("target_msg_id", react.MessageId).
		Logger()
	ctx := log.WithContext(context.TODO())
	portal.handleMetaOrWhatsAppReaction(ctx, sender, react.MessageId, "", 0)
}

func (portal *Portal) handleMetaOrWhatsAppReaction(ctx context.Context, sender *Puppet, messageID, reaction string, timestamp int64) {
	log := zerolog.Ctx(ctx)
	targetMsg, err := portal.bridge.DB.Message.GetByID(ctx, messageID, 0, portal.Receiver)
	if err != nil {
		log.Err(err).Msg("Failed to get target message from database")
		return
	} else if targetMsg == nil {
		log.Warn().Msg("Target message not found")
		return
	}
	existingReaction, err := portal.bridge.DB.Reaction.GetByID(ctx, targetMsg.ID, portal.Receiver, sender.ID)
	if err != nil {
		log.Err(err).Msg("Failed to get existing reaction from database")
		return
	} else if existingReaction != nil && existingReaction.Emoji == reaction {
		// TODO should reactions be deduplicated by some ID instead of the emoji?
		log.Debug().Msg("Ignoring duplicate reaction")
		return
	}
	intent := sender.IntentFor(portal)
	if existingReaction != nil {
		_, err = intent.RedactEvent(ctx, portal.MXID, existingReaction.MXID, mautrix.ReqRedact{
			TxnID: "mxmeta_unreact_" + existingReaction.MXID.String(),
		})
		if err != nil {
			log.Err(err).Msg("Failed to redact reaction")
		}
	}
	if reaction == "" {
		if existingReaction == nil {
			log.Warn().Msg("Existing reaction to delete not found")
			return
		}
		err = existingReaction.Delete(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to delete reaction from database")
		}
		return
	}
	content := &event.ReactionEventContent{
		RelatesTo: event.RelatesTo{
			Type:    event.RelAnnotation,
			Key:     variationselector.Add(reaction),
			EventID: targetMsg.MXID,
		},
	}
	resp, err := portal.sendMatrixEvent(ctx, intent, event.EventReaction, content, nil, timestamp)
	if err != nil {
		log.Err(err).Msg("Failed to send reaction")
		return
	}
	if existingReaction == nil {
		dbReaction := portal.bridge.DB.Reaction.New()
		dbReaction.MXID = resp.EventID
		dbReaction.RoomID = portal.MXID
		dbReaction.MessageID = targetMsg.ID
		dbReaction.ThreadID = portal.ThreadID
		dbReaction.ThreadReceiver = portal.Receiver
		dbReaction.Sender = sender.ID
		dbReaction.Emoji = reaction
		// TODO save timestamp?
		err = dbReaction.Insert(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to insert reaction to database")
		}
	} else {
		existingReaction.Emoji = reaction
		existingReaction.MXID = resp.EventID
		err = existingReaction.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to update reaction in database")
		}
	}
}

func (portal *Portal) handleMetaDelete(messageID string) {
	log := portal.log.With().
		Str("action", "delete meta message").
		Str("message_id", messageID).
		Logger()
	ctx := log.WithContext(context.TODO())
	portal.handleMetaOrWhatsAppDelete(ctx, nil, messageID)
}

func (portal *Portal) handleMetaOrWhatsAppDelete(ctx context.Context, sender *Puppet, messageID string) {
	log := zerolog.Ctx(ctx)
	targetMsg, err := portal.bridge.DB.Message.GetAllPartsByID(ctx, messageID, portal.Receiver)
	if err != nil {
		log.Err(err).Msg("Failed to get target message from database")
		return
	} else if len(targetMsg) == 0 {
		log.Warn().Msg("Target message not found")
		return
	}
	intent := portal.MainIntent()
	if sender != nil {
		intent = sender.IntentFor(portal)
	}
	for _, part := range targetMsg {
		_, err = intent.RedactEvent(ctx, portal.MXID, part.MXID, mautrix.ReqRedact{
			TxnID: "mxmeta_delete_" + part.MXID.String(),
		})
		if err != nil {
			log.Err(err).
				Int("part_index", part.PartIndex).
				Str("event_id", part.MXID.String()).
				Msg("Failed to redact message")
		}
		err = part.Delete(ctx)
		if err != nil {
			log.Err(err).
				Int("part_index", part.PartIndex).
				Msg("Failed to delete message from database")
		}
	}
}

type customReadReceipt struct {
	Timestamp          int64  `json:"ts,omitempty"`
	DoublePuppetSource string `json:"fi.mau.double_puppet_source,omitempty"`
}

type customReadMarkers struct {
	mautrix.ReqSetReadMarkers
	ReadExtra      customReadReceipt `json:"com.beeper.read.extra"`
	FullyReadExtra customReadReceipt `json:"com.beeper.fully_read.extra"`
}

func (portal *Portal) SendReadReceipt(ctx context.Context, sender *Puppet, eventID id.EventID) error {
	intent := sender.IntentFor(portal)
	if intent.IsCustomPuppet {
		extra := customReadReceipt{DoublePuppetSource: portal.bridge.Name}
		return intent.SetReadMarkers(ctx, portal.MXID, &customReadMarkers{
			ReqSetReadMarkers: mautrix.ReqSetReadMarkers{
				Read:      eventID,
				FullyRead: eventID,
			},
			ReadExtra:      extra,
			FullyReadExtra: extra,
		})
	} else {
		return intent.MarkRead(ctx, portal.MXID, eventID)
	}
}

func (portal *Portal) handleMetaReadReceipt(read *table.LSUpdateReadReceipt) {
	if portal.MXID == "" {
		portal.log.Debug().Msg("Dropping read receipt in chat with no portal")
		return
	}
	sender := portal.bridge.GetPuppetByID(read.ContactId)
	log := portal.log.With().
		Str("action", "handle meta read receipt").
		Int64("sender_id", sender.ID).
		Int64("read_up_to_ms", read.ReadWatermarkTimestampMs).
		Int64("read_at_ms", read.ReadActionTimestampMs).
		Logger()
	ctx := log.WithContext(context.TODO())
	message, err := portal.bridge.DB.Message.GetLastByTimestamp(ctx, portal.PortalKey, time.UnixMilli(read.ReadWatermarkTimestampMs))
	if err != nil {
		log.Err(err).Msg("Failed to get message to mark as read")
	} else if message == nil {
		log.Warn().Msg("No message found to mark as read")
	} else if err = portal.SendReadReceipt(ctx, sender, message.MXID); err != nil {
		log.Err(err).Stringer("event_id", message.MXID).Msg("Failed to send read receipt")
	} else {
		log.Debug().Stringer("event_id", message.MXID).Msg("Sent read receipt to Matrix")
	}
}

// TODO find if this is the correct timeout
const MetaTypingTimeout = 15 * time.Second

func (portal *Portal) handleMetaTypingIndicator(typing *table.LSUpdateTypingIndicator) {
	if portal.MXID == "" {
		portal.log.Debug().Msg("Dropping typing message in chat with no portal")
		return
	}
	ctx := context.TODO()
	sender := portal.bridge.GetPuppetByID(typing.SenderId)
	intent := sender.IntentFor(portal)
	// Don't bridge double puppeted typing notifications to avoid echoing
	if intent.IsCustomPuppet {
		return
	}
	_, err := intent.UserTyping(ctx, portal.MXID, typing.IsTyping, MetaTypingTimeout)
	if err != nil {
		portal.log.Err(err).
			Int64("user_id", sender.ID).
			Msg("Failed to handle Meta typing notification")
	}
}

func (portal *Portal) handleMetaNameChange(typedEvt *table.LSSyncUpdateThreadName) {
	log := portal.log.With().
		Str("action", "meta name change").
		Logger()
	ctx := log.WithContext(context.TODO())
	if portal.updateName(ctx, typedEvt.ThreadName) {
		err := portal.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to save portal in database after name change")
		}
		portal.UpdateBridgeInfo(ctx)
	}
}

func (portal *Portal) handleMetaAvatarChange(evt *table.LSSetThreadImageURL) {
	log := portal.log.With().
		Str("action", "meta avatar change").
		Logger()
	ctx := log.WithContext(context.TODO())
	if portal.updateAvatar(ctx, evt.ImageURL) {
		err := portal.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to save portal in database after avatar change")
		}
		portal.UpdateBridgeInfo(ctx)
	}
}

func (portal *Portal) storeMessageInDB(ctx context.Context, eventID id.EventID, messageID string, otid, senderID int64, timestamp time.Time, partIndex int) {
	dbMessage := portal.bridge.DB.Message.New()
	dbMessage.MXID = eventID
	dbMessage.RoomID = portal.MXID
	dbMessage.ID = messageID
	dbMessage.OTID = otid
	dbMessage.Sender = senderID
	dbMessage.Timestamp = timestamp
	dbMessage.PartIndex = partIndex
	dbMessage.ThreadID = portal.ThreadID
	dbMessage.ThreadReceiver = portal.Receiver
	err := dbMessage.Insert(ctx)
	if err != nil {
		portal.log.Err(err).Msg("Failed to insert message into database")
	}
}

func (portal *Portal) sendMainIntentMessage(ctx context.Context, content *event.MessageEventContent) (*mautrix.RespSendEvent, error) {
	return portal.sendMatrixEvent(ctx, portal.MainIntent(), event.EventMessage, content, nil, 0)
}

func (portal *Portal) encrypt(ctx context.Context, intent *appservice.IntentAPI, content *event.Content, eventType event.Type) (event.Type, error) {
	if !portal.Encrypted || portal.bridge.Crypto == nil {
		return eventType, nil
	}
	intent.AddDoublePuppetValue(content)
	// TODO maybe the locking should be inside mautrix-go?
	portal.encryptLock.Lock()
	defer portal.encryptLock.Unlock()
	err := portal.bridge.Crypto.Encrypt(ctx, portal.MXID, eventType, content)
	if err != nil {
		return eventType, fmt.Errorf("failed to encrypt event: %w", err)
	}
	return event.EventEncrypted, nil
}

func (portal *Portal) sendMatrixEvent(ctx context.Context, intent *appservice.IntentAPI, eventType event.Type, content any, extraContent map[string]any, timestamp int64) (*mautrix.RespSendEvent, error) {
	wrappedContent := event.Content{Parsed: content, Raw: extraContent}
	if eventType != event.EventReaction {
		var err error
		eventType, err = portal.encrypt(ctx, intent, &wrappedContent, eventType)
		if err != nil {
			return nil, err
		}
	}

	_, _ = intent.UserTyping(ctx, portal.MXID, false, 0)
	return intent.SendMassagedMessageEvent(ctx, portal.MXID, eventType, &wrappedContent, timestamp)
}

func (portal *Portal) getEncryptionEventContent() (evt *event.EncryptionEventContent) {
	evt = &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}
	if rot := portal.bridge.Config.Bridge.Encryption.Rotation; rot.EnableCustom {
		evt.RotationPeriodMillis = rot.Milliseconds
		evt.RotationPeriodMessages = rot.Messages
	}
	return
}

func (portal *Portal) shouldSetDMRoomMetadata() bool {
	return !portal.IsPrivateChat() ||
		portal.bridge.Config.Bridge.PrivateChatPortalMeta == "always" ||
		(portal.IsEncrypted() && portal.bridge.Config.Bridge.PrivateChatPortalMeta != "never")
}

func (portal *Portal) ensureUserInvited(ctx context.Context, user *User) bool {
	return user.ensureInvited(ctx, portal.MainIntent(), portal.MXID, portal.IsPrivateChat())
}

func (portal *Portal) CreateMatrixRoom(ctx context.Context, user *User) error {
	portal.roomCreateLock.Lock()
	defer portal.roomCreateLock.Unlock()
	if portal.MXID != "" {
		portal.log.Debug().Msg("Not creating room: already exists")
		return nil
	}
	portal.log.Debug().Msg("Creating matrix room")

	intent := portal.MainIntent()

	if err := intent.EnsureRegistered(ctx); err != nil {
		portal.log.Error().Err(err).Msg("failed to ensure registered")
		return err
	}

	bridgeInfoStateKey, bridgeInfo := portal.getBridgeInfo()
	initialState := []*event.Event{{
		Type:     event.StateBridge,
		Content:  event.Content{Parsed: bridgeInfo},
		StateKey: &bridgeInfoStateKey,
	}, {
		// TODO remove this once https://github.com/matrix-org/matrix-doc/pull/2346 is in spec
		Type:     event.StateHalfShotBridge,
		Content:  event.Content{Parsed: bridgeInfo},
		StateKey: &bridgeInfoStateKey,
	}}

	creationContent := make(map[string]interface{})
	if !portal.bridge.Config.Bridge.FederateRooms {
		creationContent["m.federate"] = false
	}

	var invite []id.UserID
	autoJoinInvites := portal.bridge.SpecVersions.Supports(mautrix.BeeperFeatureAutojoinInvites)
	if autoJoinInvites {
		invite = append(invite, user.MXID)
	}
	var waGroupInfo *types.GroupInfo
	var participants []id.UserID
	if portal.ThreadType == table.ENCRYPTED_OVER_WA_GROUP {
		waGroupInfo, participants = portal.UpdateWAGroupInfo(ctx, user, nil)
		invite = append(invite, participants...)
		slices.Sort(invite)
		invite = slices.Compact(invite)
	}

	if portal.bridge.Config.Bridge.Encryption.Default {
		initialState = append(initialState, &event.Event{
			Type: event.StateEncryption,
			Content: event.Content{
				Parsed: portal.getEncryptionEventContent(),
			},
		})
		portal.Encrypted = true

		if portal.IsPrivateChat() {
			invite = append(invite, portal.bridge.Bot.UserID)
		}
	}
	dmPuppet := portal.GetDMPuppet()
	if dmPuppet != nil {
		dmPuppet.FetchAndUpdateInfoIfNecessary(ctx, user)
		portal.UpdateInfoFromPuppet(ctx, dmPuppet)
	}
	if !portal.AvatarURL.IsEmpty() {
		initialState = append(initialState, &event.Event{
			Type: event.StateRoomAvatar,
			Content: event.Content{Parsed: &event.RoomAvatarEventContent{
				URL: portal.AvatarURL,
			}},
		})
	}

	req := &mautrix.ReqCreateRoom{
		Visibility:      "private",
		Name:            portal.Name,
		Invite:          invite,
		Preset:          "private_chat",
		IsDirect:        portal.IsPrivateChat(),
		InitialState:    initialState,
		CreationContent: creationContent,

		BeeperAutoJoinInvites: autoJoinInvites,
	}
	resp, err := intent.CreateRoom(ctx, req)
	if err != nil {
		portal.log.Warn().Err(err).Msg("failed to create room")
		return err
	}
	portal.log = portal.log.With().Stringer("room_id", resp.RoomID).Logger()

	portal.NameSet = len(req.Name) > 0
	portal.AvatarSet = !portal.AvatarURL.IsEmpty()
	portal.MXID = resp.RoomID
	portal.MoreToBackfill = true
	portal.bridge.portalsLock.Lock()
	portal.bridge.portalsByMXID[portal.MXID] = portal
	portal.bridge.portalsLock.Unlock()
	err = portal.Update(ctx)
	if err != nil {
		portal.log.Err(err).Msg("Failed to save portal room ID")
		return err
	}
	portal.log.Info().Msg("Created matrix room for portal")

	if !autoJoinInvites {
		if portal.Encrypted {
			err = portal.bridge.Bot.EnsureJoined(ctx, portal.MXID, appservice.EnsureJoinedParams{BotOverride: portal.MainIntent().Client})
			if err != nil {
				portal.log.Error().Err(err).Msg("Failed to ensure bridge bot is joined to private chat portal")
			}
		}
		user.ensureInvited(ctx, portal.MainIntent(), portal.MXID, portal.IsPrivateChat())
	}
	go portal.addToPersonalSpace(portal.log.WithContext(context.TODO()), user)

	if portal.IsPrivateChat() {
		user.AddDirectChat(ctx, portal.MXID, dmPuppet.MXID)
	}
	if waGroupInfo != nil && !autoJoinInvites {
		portal.SyncWAParticipants(ctx, user, waGroupInfo.Participants)
	}

	return nil
}

func (portal *Portal) UpdateInfoFromPuppet(ctx context.Context, puppet *Puppet) {
	if !portal.shouldSetDMRoomMetadata() {
		return
	}
	update := false
	update = portal.updateName(ctx, puppet.Name) || update
	// Note: DM avatars will also go through the main UpdateInfo route
	// (for some reason DMs have thread pictures, but not thread names)
	update = portal.updateAvatarWithURL(ctx, puppet.AvatarID, puppet.AvatarURL) || update
	if update {
		err := portal.Update(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save portal in database after updating DM info")
		}
		portal.UpdateBridgeInfo(ctx)
	}
}

func (portal *Portal) UpdateWAGroupInfo(ctx context.Context, source *User, groupInfo *types.GroupInfo) (*types.GroupInfo, []id.UserID) {
	log := zerolog.Ctx(ctx)
	if groupInfo == nil {
		var err error
		groupInfo, err = source.E2EEClient.GetGroupInfo(portal.JID())
		if err != nil {
			log.Err(err).Msg("Failed to fetch WhatsApp group info")
			return nil, nil
		}
	}
	update := false
	update = portal.updateName(ctx, groupInfo.Name) || update
	//update = portal.updateTopic(ctx, groupInfo.Topic) || update
	//update = portal.updateWAAvatar(ctx)
	participants := portal.SyncWAParticipants(ctx, source, groupInfo.Participants)
	if update {
		err := portal.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to save portal in database after updating group info")
		}
		portal.UpdateBridgeInfo(ctx)
	}
	return groupInfo, participants
}

func (portal *Portal) SyncWAParticipants(ctx context.Context, source *User, participants []types.GroupParticipant) []id.UserID {
	var userIDs []id.UserID
	for _, pcp := range participants {
		puppet := portal.bridge.GetPuppetByID(int64(pcp.JID.UserInt()))
		puppet.FetchAndUpdateInfoIfNecessary(ctx, source)
		userIDs = append(userIDs, puppet.IntentFor(portal).UserID)
		if portal.MXID != "" {
			err := puppet.IntentFor(portal).EnsureJoined(ctx, portal.MXID)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to ensure participant is joined to group")
			}
		}
	}
	return userIDs
}

func (portal *Portal) UpdateInfo(ctx context.Context, info table.ThreadInfo) {
	log := zerolog.Ctx(ctx).With().
		Str("function", "UpdateInfo").
		Logger()
	ctx = log.WithContext(ctx)
	update := false
	if portal.ThreadType != info.GetThreadType() && !portal.ThreadType.IsWhatsApp() {
		portal.ThreadType = info.GetThreadType()
		update = true
	}
	if !portal.IsPrivateChat() || portal.shouldSetDMRoomMetadata() {
		if info.GetThreadName() != "" || !portal.IsPrivateChat() {
			update = portal.updateName(ctx, info.GetThreadName()) || update
		}
		if info.GetThreadPictureUrl() != "" || !portal.IsPrivateChat() {
			update = portal.updateAvatar(ctx, info.GetThreadPictureUrl()) || update
		}
	}
	if update {
		err := portal.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to save portal in database after updating group info")
		}
		portal.UpdateBridgeInfo(ctx)
	}
	return
}

func (portal *Portal) updateName(ctx context.Context, newName string) bool {
	if portal.Name == newName && (portal.NameSet || portal.MXID == "") {
		return false
	}
	portal.Name = newName
	portal.NameSet = false
	if portal.MXID != "" {
		_, err := portal.MainIntent().SetRoomName(ctx, portal.MXID, portal.Name)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to update room name")
		} else {
			portal.NameSet = true
		}
	}
	return true
}

func (portal *Portal) updateAvatarWithURL(ctx context.Context, avatarID string, avatarMXC id.ContentURI) bool {
	if portal.AvatarID == avatarID && (portal.AvatarSet || portal.MXID == "") {
		return false
	}
	if (avatarID == "") != avatarMXC.IsEmpty() {
		return false
	}
	portal.AvatarID = avatarID
	portal.AvatarURL = avatarMXC
	portal.AvatarSet = false
	if portal.MXID != "" {
		_, err := portal.MainIntent().SetRoomAvatar(ctx, portal.MXID, portal.AvatarURL)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to update room avatar")
		} else {
			portal.AvatarSet = true
		}
	}
	return true
}

func (portal *Portal) updateAvatar(ctx context.Context, avatarURL string) bool {
	var setAvatar func(context.Context, id.ContentURI) error
	if portal.MXID != "" {
		setAvatar = func(ctx context.Context, uri id.ContentURI) error {
			_, err := portal.MainIntent().SetRoomAvatar(ctx, portal.MXID, uri)
			return err
		}
	}
	return msgconv.UpdateAvatar(
		ctx, avatarURL,
		&portal.AvatarID, &portal.AvatarSet, &portal.AvatarURL,
		portal.MainIntent().UploadBytes, setAvatar,
	)
}

func (portal *Portal) addToPersonalSpace(ctx context.Context, user *User) bool {
	spaceID := user.GetSpaceRoom(ctx)
	if len(spaceID) == 0 || user.IsInSpace(ctx, portal.PortalKey) {
		return false
	}
	_, err := portal.bridge.Bot.SendStateEvent(ctx, spaceID, event.StateSpaceChild, portal.MXID.String(), &event.SpaceChildEventContent{
		Via: []string{portal.bridge.Config.Homeserver.Domain},
	})
	if err != nil {
		zerolog.Ctx(ctx).Err(err).
			Str("user_id", user.MXID.String()).
			Str("space_id", spaceID.String()).
			Msg("Failed to add room to user's personal filtering space")
		return false
	} else {
		zerolog.Ctx(ctx).Debug().
			Str("user_id", user.MXID.String()).
			Str("space_id", spaceID.String()).
			Msg("Added room to user's personal filtering space")
		user.MarkInSpace(ctx, portal.PortalKey)
		return true
	}
}

func (portal *Portal) HasRelaybot() bool {
	return portal.bridge.Config.Bridge.Relay.Enabled && len(portal.RelayUserID) > 0
}

func (portal *Portal) addRelaybotFormat(ctx context.Context, userID id.UserID, evt *event.Event, content *event.MessageEventContent) bool {
	member := portal.MainIntent().Member(ctx, portal.MXID, userID)
	if member == nil {
		member = &event.MemberEventContent{}
	}
	// Stickers can't have captions, so force them into images when relaying
	if evt.Type == event.EventSticker {
		content.MsgType = event.MsgImage
		evt.Type = event.EventMessage
	}
	content.EnsureHasHTML()
	data, err := portal.bridge.Config.Bridge.Relay.FormatMessage(content, userID, *member)
	if err != nil {
		portal.log.Err(err).Msg("Failed to apply relaybot format")
	}
	content.FormattedBody = data
	// Force FileName field so the formatted body is used as a caption
	if content.FileName == "" {
		content.FileName = content.Body
	}
	content.Body = format.HTMLToText(content.FormattedBody)
	return true
}

func (portal *Portal) Delete() {
	err := portal.Portal.Delete(context.TODO())
	if err != nil {
		portal.log.Err(err).Msg("Failed to delete portal from db")
	}
	portal.bridge.portalsLock.Lock()
	delete(portal.bridge.portalsByID, portal.PortalKey)
	if len(portal.MXID) > 0 {
		delete(portal.bridge.portalsByMXID, portal.MXID)
	}
	if portal.Receiver == 0 {
		portal.bridge.usersLock.Lock()
		for _, user := range portal.bridge.usersByMetaID {
			user.RemoveInSpaceCache(portal.PortalKey)
		}
		portal.bridge.usersLock.Unlock()
	} else {
		user := portal.bridge.GetUserByMetaID(portal.Receiver)
		if user != nil {
			user.RemoveInSpaceCache(portal.PortalKey)
		}
	}
	portal.bridge.portalsLock.Unlock()
}

func (portal *Portal) Cleanup(ctx context.Context, puppetsOnly bool) {
	portal.bridge.CleanupRoom(ctx, &portal.log, portal.MainIntent(), portal.MXID, puppetsOnly)
}

func (br *MetaBridge) CleanupRoom(ctx context.Context, log *zerolog.Logger, intent *appservice.IntentAPI, mxid id.RoomID, puppetsOnly bool) {
	if len(mxid) == 0 {
		return
	}
	if br.SpecVersions.Supports(mautrix.BeeperFeatureRoomYeeting) {
		err := intent.BeeperDeleteRoom(ctx, mxid)
		if err == nil || errors.Is(err, mautrix.MNotFound) {
			return
		}
		log.Warn().Err(err).Msg("Failed to delete room using beeper yeet endpoint, falling back to normal behavior")
	}
	members, err := intent.JoinedMembers(ctx, mxid)
	if err != nil {
		log.Err(err).Msg("Failed to get portal members for cleanup")
		return
	}
	for member := range members.Joined {
		if member == intent.UserID {
			continue
		}
		puppet := br.GetPuppetByMXID(member)
		if puppet != nil {
			_, err = puppet.DefaultIntent().LeaveRoom(ctx, mxid)
			if err != nil {
				log.Err(err).Msg("Failed to leave as puppet while cleaning up portal")
			}
		} else if !puppetsOnly {
			_, err = intent.KickUser(ctx, mxid, &mautrix.ReqKickUser{UserID: member, Reason: "Deleting portal"})
			if err != nil {
				log.Err(err).Msg("Failed to kick user while cleaning up portal")
			}
		}
	}
	_, err = intent.LeaveRoom(ctx, mxid)
	if err != nil {
		log.Err(err).Msg("Failed to leave room while cleaning up portal")
	}
}
