package connector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	//"reflect"
	//"strconv"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/config"
	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"

	//"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"

	"go.mau.fi/mautrix-meta/pkg/connector/msgconv"

	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	//"maunium.net/go/mautrix/event"
	metaTypes "go.mau.fi/mautrix-meta/messagix/types"
)

type metaEvent struct {
	context context.Context
	event   any
}

type MetaClient struct {
	Main   *MetaConnector
	client *messagix.Client

	log     zerolog.Logger
	cookies *cookies.Cookies
	login   *bridgev2.UserLogin

	incomingEvents   chan *metaEvent
	messageConverter *msgconv.MessageConverter
}

func cookiesFromMetadata(metadata map[string]interface{}) *cookies.Cookies {
	platform := types.Platform(metadata["platform"].(float64))

	m := make(map[string]string)
	for k, v := range metadata["cookies"].(map[string]interface{}) {
		m[k] = v.(string)
	}

	c := &cookies.Cookies{
		Platform: platform,
	}
	c.UpdateValues(m)

	return c
}

// Why are these separate?
func platformToMode(platform types.Platform) config.BridgeMode {
	switch platform {
	case types.Facebook:
		return config.ModeFacebook
	case types.Instagram:
		return config.ModeInstagram
	default:
		panic(fmt.Sprintf("unknown platform %d", platform))
	}
}

func NewMetaClient(ctx context.Context, main *MetaConnector, login *bridgev2.UserLogin) (*MetaClient, error) {
	log := zerolog.Ctx(ctx).With().Str("component", "meta_client").Logger()
	log.Debug().Any("metadata", login.Metadata.Extra).Msg("Creating new Meta client")

	var c *cookies.Cookies
	if _, ok := login.Metadata.Extra["cookies"].(map[string]interface{}); ok {
		c = cookiesFromMetadata(login.Metadata.Extra)
	} else {
		c = login.Metadata.Extra["cookies"].(*cookies.Cookies)
	}

	return &MetaClient{
		Main:           main,
		cookies:        c,
		log:            log,
		login:          login,
		incomingEvents: make(chan *metaEvent, 8),
		messageConverter: &msgconv.MessageConverter{
			BridgeMode: platformToMode(c.Platform),
		},
	}, nil
}

func (m *MetaClient) Update(ctx context.Context) error {
	m.login.Metadata.Extra["cookies"] = m.cookies
	err := m.login.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save updated cookies: %w", err)
	}
	zerolog.Ctx(ctx).Debug().Msg("Updated cookies")
	return nil
}

// We don't want to block while handling events, but they must be processed in order, so we use a channel to queue them.
func (m *MetaClient) metaEventHandler(rawEvt any) {
	ctx := m.log.WithContext(context.TODO())

	evt := metaEvent{
		context: ctx,
		event:   rawEvt,
	}

	m.incomingEvents <- &evt
}

func (m *MetaClient) handleMetaEventLoop() {
	for evt := range m.incomingEvents {
		if evt == nil {
			m.log.Debug().Msg("Received nil event, stopping event handling")
			return
		}
		m.handleMetaEvent(evt.context, evt.event)
	}
}

func (m *MetaClient) handleMetaEvent(ctx context.Context, evt any) {
	log := zerolog.Ctx(ctx)

	switch evt := evt.(type) {
	case *messagix.Event_PublishResponse:
		log.Trace().Any("table", &evt.Table).Msg("Got new event table")
		m.handleTable(ctx, evt.Table)
	case *messagix.Event_Ready:
		log.Trace().Msg("Initial connect to Meta socket completed, sending connected BridgeState")
		m.login.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
	default:
		log.Warn().Type("event_type", evt).Msg("Unrecognized event type from messagix")
	}
}

func (m *MetaClient) handleTable(ctx context.Context, tbl *table.LSTable) {
	log := zerolog.Ctx(ctx)

	for _, contact := range tbl.LSDeleteThenInsertContact {
		log.Warn().Int64("contact_id", contact.Id).Msg("LSDeleteThenInsertContact")
	}
	for _, contact := range tbl.LSVerifyContactRowExists {
		log.Warn().Int64("contact_id", contact.ContactId).Msg("LSVerifyContactRowExists")
		ghost, err := m.Main.Bridge.GetGhostByID(ctx, networkid.UserID(strconv.Itoa(int(contact.ContactId))))
		if err != nil {
			log.Err(err).Int64("contact_id", contact.ContactId).Msg("Failed to get ghost")
			continue
		}
		ghost.UpdateInfo(ctx, &bridgev2.UserInfo{
			Name: &contact.Name,
		})
	}
	for _, thread := range tbl.LSDeleteThenInsertThread {
		log.Warn().Int64("thread_id", thread.ThreadKey).Msg("LSDeleteThenInsertThread")
		portal, err := m.Main.Bridge.GetPortalByID(ctx, networkid.PortalKey{
			ID: networkid.PortalID(strconv.Itoa(int(thread.ThreadKey))),
		})
		if err != nil {
			log.Err(err).Int64("thread_id", thread.ThreadKey).Msg("Failed to get portal")
			continue
		}
		portal.CreateMatrixRoom(ctx, m.login, &bridgev2.ChatInfo{
			Name:         &thread.ThreadName,
			Topic:        &thread.ThreadDescription,
			IsSpace:      &[]bool{false}[0],
			IsDirectChat: &[]bool{true}[0],
		})
	}
	for _, participant := range tbl.LSAddParticipantIdToGroupThread {
		log.Warn().Int64("thread_id", participant.ThreadKey).Int64("contact_id", participant.ContactId).Msg("LSAddParticipantIdToGroupThread")
	}
	for _, participant := range tbl.LSRemoveParticipantFromThread {
		log.Warn().Int64("thread_id", participant.ThreadKey).Int64("contact_id", participant.ParticipantId).Msg("LSRemoveParticipantFromThread")
	}
	for _, thread := range tbl.LSVerifyThreadExists {
		log.Warn().Int64("thread_id", thread.ThreadKey).Msg("LSVerifyThreadExists")
	}
	for _, mute := range tbl.LSUpdateThreadMuteSetting {
		log.Warn().Int64("thread_id", mute.ThreadKey).Msg("LSUpdateThreadMuteSetting")
	}
	for _, thread := range tbl.LSSyncUpdateThreadName {
		log.Warn().Int64("thread_id", thread.ThreadKey).Msg("LSUpdateThreadName")
		portal, err := m.Main.Bridge.GetPortalByID(ctx, networkid.PortalKey{
			ID: networkid.PortalID(strconv.Itoa(int(thread.ThreadKey))),
		})
		if err != nil {
			log.Err(err).Int64("thread_id", thread.ThreadKey).Msg("Failed to get portal")
			continue
		}
		portal.UpdateName(ctx, thread.ThreadName, nil, time.Time{})
	}

	upsert, insert := tbl.WrapMessages()
	for _, upsert := range upsert {
		log.Trace().Int64("thread_id", upsert.Range.ThreadKey).Msg("UpsertMessages")
	}
	for _, msg := range insert {
		log.Trace().Int64("thread_id", msg.ThreadKey).Str("message_id", msg.MessageId).Msg("InsertMessage")
		//converted := m.messageConverter.ToMatrix(ctx, msg)
		//log.Trace().Any("converted", converted).Msg("Converted message")
		m.insertMessage(ctx, msg)
	}
}

func (m *MetaClient) insertMessage(ctx context.Context, msg *table.WrappedMessage) {
	log := zerolog.Ctx(ctx)

	//converted := m.messageConverter.ToMatrix(ctx, msg)
	//log.Trace().Any("converted", converted).Msg("Converted message")

	log.Warn().Str("sender_id", strconv.Itoa(int(msg.SenderId))).Str("login_id", string(m.login.ID)).Msg("Inserting message")

	sender := bridgev2.EventSender{
		IsFromMe:    strconv.Itoa(int(msg.SenderId)) == string(m.login.ID),
		Sender:      networkid.UserID(strconv.Itoa(int(msg.SenderId))),
		SenderLogin: networkid.UserLoginID(strconv.Itoa(int(msg.SenderId))),
	}

	log.Warn().Any("sender", sender).Msg("Sender")

	m.Main.Bridge.QueueRemoteEvent(m.login, &bridgev2.SimpleRemoteEvent[*table.WrappedMessage]{
		Type: bridgev2.RemoteEventMessage,
		LogContext: func(c zerolog.Context) zerolog.Context {
			return c.
				Str("message_id", msg.MessageId).
				Any("sender", sender)
		},
		ID:     networkid.MessageID(msg.MessageId),
		Sender: sender,
		PortalKey: networkid.PortalKey{
			ID: networkid.PortalID(strconv.Itoa(int(msg.ThreadKey))),
		},
		Data:         msg,
		CreatePortal: true,
		ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, msg *table.WrappedMessage) (*bridgev2.ConvertedMessage, error) {
			return m.messageConverter.ToMatrix(ctx, msg), nil
		},
	})
}

func (m *MetaClient) Connect(ctx context.Context) error {
	client := messagix.NewClient(m.cookies, m.log.With().Str("component", "messagix").Logger())
	m.client = client

	_, initialTable, err := m.client.LoadMessagesPage()
	if err != nil {
		return fmt.Errorf("failed to load messages page: %w", err)
	}

	m.handleTable(ctx, initialTable)

	m.client.SetEventHandler(m.metaEventHandler)

	err = m.client.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to messagix: %w", err)
	}

	err = m.Update(ctx)
	if err != nil {
		return err
	}

	go m.handleMetaEventLoop()

	return nil
}

func (m *MetaClient) Disconnect() {
	m.incomingEvents <- nil
	close(m.incomingEvents)
	if m.client != nil {
		m.client.Disconnect()
	}
	m.client = nil
}

// GetCapabilities implements bridgev2.NetworkAPI.
func (m *MetaClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *bridgev2.NetworkRoomCapabilities {
	return &bridgev2.NetworkRoomCapabilities{}
}

// GetChatInfo implements bridgev2.NetworkAPI.
func (m *MetaClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	panic("unimplemented")
}

// GetUserInfo implements bridgev2.NetworkAPI.
func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	panic("unimplemented")
}

type msgconvContextKey int

const (
	msgconvContextKeyIntent msgconvContextKey = iota
	msgconvContextKeyClient
	msgconvContextKeyE2EEClient
	msgconvContextKeyBackfill
)

// HandleMatrixMessage implements bridgev2.NetworkAPI.
func (m *MetaClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	log := zerolog.Ctx(ctx)

	content, ok := msg.Event.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		log.Error().Type("content_type", content).Msg("Unexpected parsed content type")
		return nil, fmt.Errorf("unexpected parsed content type: %T", content)
	}
	if content.MsgType == event.MsgNotice /*&& !portal.bridge.Config.Bridge.BridgeNotices*/ {
		log.Warn().Msg("Ignoring notice message")
		return nil, nil
	}

	ctx = context.WithValue(ctx, msgconvContextKeyClient, m.client)

	thread, err := strconv.Atoi(string(msg.Portal.ID))
	if err != nil {
		log.Err(err).Str("thread_id", string(msg.Portal.ID)).Msg("Failed to parse thread ID")
		return nil, fmt.Errorf("failed to parse thread ID: %w", err)
	}

	tasks, otid, err := m.messageConverter.ToMeta(ctx, msg.Event, content, false, int64(thread))
	if errors.Is(err, metaTypes.ErrPleaseReloadPage) {
		log.Err(err).Msg("Got please reload page error while converting message, reloading page in background")
		// go m.client.Disconnect()
		// err = errReloading
		panic("unimplemented")
	} else if errors.Is(err, messagix.ErrTokenInvalidated) {
		panic("unimplemented")
		// go sender.DisconnectFromError(status.BridgeState{
		// 	StateEvent: status.StateBadCredentials,
		// 	Error:      MetaCookieRemoved,
		// })
		// err = errLoggedOut
	}

	if err != nil {
		log.Err(err).Msg("Failed to convert message")
		//go ms.sendMessageMetrics(evt, err, "Error converting", true)
		return nil, err
	}

	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int64("otid", otid)
	})
	log.Debug().Msg("Sending Matrix message to Meta")

	otidStr := strconv.FormatInt(otid, 10)
	//portal.pendingMessages[otid] = evt.ID
	//messageTS := time.Now()
	var resp *table.LSTable

	retries := 0
	for retries < 5 {
		if err = m.client.WaitUntilCanSendMessages(15 * time.Second); err != nil {
			log.Err(err).Msg("Error waiting to be able to send messages, retrying")
		} else {
			resp, err = m.client.ExecuteTasks(tasks...)
			if err == nil {
				break
			}
			log.Err(err).Msg("Failed to send message to Meta, retrying")
		}
		retries++
	}

	log.Trace().Any("response", resp).Msg("Meta send response")
	var msgID string
	if resp != nil && err == nil {
		for _, replace := range resp.LSReplaceOptimsiticMessage {
			if replace.OfflineThreadingId == otidStr {
				msgID = replace.MessageId
			}
		}
		if len(msgID) == 0 {
			for _, failed := range resp.LSMarkOptimisticMessageFailed {
				if failed.OTID == otidStr {
					log.Warn().Str("message", failed.Message).Msg("Sending message failed")
					//go ms.sendMessageMetrics(evt, fmt.Errorf("%w: %s", errServerRejected, failed.Message), "Error sending", true)
					return nil, fmt.Errorf("sending message failed: %s", failed.Message)
				}
			}
			for _, failed := range resp.LSHandleFailedTask {
				if failed.OTID == otidStr {
					log.Warn().Str("message", failed.Message).Msg("Sending message failed")
					//go ms.sendMessageMetrics(evt, fmt.Errorf("%w: %s", errServerRejected, failed.Message), "Error sending", true)
					return nil, fmt.Errorf("sending message failed: %s", failed.Message)
				}
			}
			log.Warn().Msg("Message send response didn't include message ID")
		}
	}
	// if msgID != "" {
	// 	portal.pendingMessagesLock.Lock()
	// 	_, ok = portal.pendingMessages[otid]
	// 	if ok {
	// 		portal.storeMessageInDB(ctx, evt.ID, msgID, otid, sender.MetaID, messageTS, 0)
	// 		delete(portal.pendingMessages, otid)
	// 	} else {
	// 		log.Debug().Msg("Not storing message send response: pending message was already removed from map")
	// 	}
	// 	portal.pendingMessagesLock.Unlock()
	// }

	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        networkid.MessageID(msgID),
			MXID:      msg.Event.ID,
			Room:      networkid.PortalKey{ID: msg.Portal.ID},
			SenderID:  networkid.UserID(msg.Event.Sender),
			Timestamp: time.Time{},
		},
	}, nil

	// timings.totalSend = time.Since(start)
	// go ms.sendMessageMetrics(evt, err, "Error sending", true)
}

// IsLoggedIn implements bridgev2.NetworkAPI.
func (m *MetaClient) IsLoggedIn() bool {
	panic("unimplemented")
}

// IsThisUser implements bridgev2.NetworkAPI.
func (m *MetaClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	panic("unimplemented")
}

// LogoutRemote implements bridgev2.NetworkAPI.
func (m *MetaClient) LogoutRemote(ctx context.Context) {
	panic("unimplemented")
}

var (
	_ bridgev2.NetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.EditHandlingNetworkAPI        = (*MetaClient)(nil)
	// _ bridgev2.ReactionHandlingNetworkAPI    = (*MetaClient)(nil)
	// _ bridgev2.RedactionHandlingNetworkAPI   = (*MetaClient)(nil)
	// _ bridgev2.ReadReceiptHandlingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.ReadReceiptHandlingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.TypingHandlingNetworkAPI      = (*MetaClient)(nil)
	// _ bridgev2.IdentifierResolvingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.GroupCreatingNetworkAPI       = (*MetaClient)(nil)
	// _ bridgev2.ContactListingNetworkAPI      = (*MetaClient)(nil)
)
