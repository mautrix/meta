package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type MetaClient struct {
	Main      *MetaConnector
	Client    *messagix.Client
	LoginMeta *metaid.UserLoginMetadata
	UserLogin *bridgev2.UserLogin
	Ghost     *bridgev2.Ghost

	stopHandlingTables atomic.Pointer[context.CancelFunc]
	initialTable       atomic.Pointer[table.LSTable]
	parsedTables       chan *parsedTable
	backfillCollectors map[int64]*BackfillCollector
	backfillLock       sync.Mutex
	connectLock        sync.Mutex
	stopConnectAttempt atomic.Pointer[context.CancelFunc]

	editChannels *exsync.Map[string, chan *FBEditEvent]

	connectBackgroundEvt           chan connectBackgroundEvent
	connectBackgroundWAOfflineSync *exsync.Event
	connectBackgroundWAEventCount  atomic.Uint32

	stopPeriodicReconnect atomic.Pointer[context.CancelFunc]
	lastFullReconnect     time.Time
	connectWaiter         *exsync.Event
	e2eeConnectWaiter     *exsync.Event
	firstE2EEConnectDone  bool

	E2EEClient      *whatsmeow.Client
	WADevice        *store.Device
	e2eeConnectLock sync.Mutex

	metaState status.BridgeState
	waState   status.BridgeState

	waLastPresence   waTypes.Presence
	igThreadIDs      map[string]int64
	igUserIDs        map[string]int64
	igUserIDsReverse map[int64]string
}

func (m *MetaConnector) getMessagixConfig() *messagix.Config {
	return &messagix.Config{
		MayConnectToDGW: m.Config.ReceiveInstagramTypingIndicators,
		ClientSettings:  m.Bridge.GetHTTPClientSettings(),
	}
}

func (m *MetaConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	loginMetadata := login.Metadata.(*metaid.UserLoginMetadata)
	c := &MetaClient{
		Main:      m,
		LoginMeta: loginMetadata,
		UserLogin: login,

		parsedTables:       make(chan *parsedTable, 16),
		backfillCollectors: make(map[int64]*BackfillCollector),

		connectBackgroundWAOfflineSync: exsync.NewEvent(),

		connectWaiter:     exsync.NewEvent(),
		e2eeConnectWaiter: exsync.NewEvent(),
		igThreadIDs:       map[string]int64{},
		igUserIDs:         map[string]int64{},
		igUserIDsReverse:  map[int64]string{},
	}
	c.editChannels = exsync.NewMap[string, chan *FBEditEvent]()
	login.Client = c
	return nil
}

var (
	_ bridgev2.NetworkAPI                    = (*MetaClient)(nil)
	_ bridgev2.CredentialExportingNetworkAPI = (*MetaClient)(nil)
	_ status.BridgeStateFiller               = (*MetaClient)(nil)
)

type respGetProxy struct {
	ProxyURL string `json:"proxy_url"`
}

// TODO this should be moved into mautrix-go

func (m *MetaConnector) getProxy(reason string) (string, error) {
	if m.Config.GetProxyFrom == "" {
		return m.Config.Proxy, nil
	}
	parsed, err := url.Parse(m.Config.GetProxyFrom)
	if err != nil {
		return "", fmt.Errorf("failed to parse address: %w", err)
	}
	q := parsed.Query()
	q.Set("reason", reason)
	parsed.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("User-Agent", mautrix.DefaultUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	} else if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	var respData respGetProxy
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return respData.ProxyURL, nil
}

func (m *MetaClient) ensureMessagixClient() {
	if m.LoginMeta.Cookies != nil && m.Client == nil {
		m.LoginMeta.Cookies.Platform = m.LoginMeta.Platform
		m.Client = messagix.NewClient(
			m.LoginMeta.Cookies,
			m.UserLogin.Log.With().Str("component", "messagix").Logger(),
			m.Main.getMessagixConfig(),
		)
		m.Client.SetEventHandler(m.handleMetaEvent)
	}
}

func (m *MetaClient) ExportCredentials(ctx context.Context) any {
	if m.Client == nil {
		return nil
	}
	return m.Client.GetCookies()
}

func (m *MetaClient) Connect(ctx context.Context) {
	if !m.connectLock.TryLock() {
		zerolog.Ctx(ctx).Error().Msg("Connect called multiple times in parallel")
		return
	}
	defer m.connectLock.Unlock()
	if m.metaState.StateEvent == "" && m.waState.StateEvent == "" {
		// Ensure both states start at CONNECTING now
		m.metaState.StateEvent = status.StateConnecting
		if m.LoginMeta.Platform.IsMessenger() || m.Main.Config.IGE2EE {
			m.waState.StateEvent = status.StateConnecting
		}
		m.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	}
	retryCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if oldCancel := m.stopConnectAttempt.Swap(&cancel); oldCancel != nil {
		(*oldCancel)()
	}
	m.connectWithRetry(retryCtx, ctx, 0)
}

const MaxConnectRetries = 10

func (m *MetaClient) connectWithRetry(retryCtx, ctx context.Context, attempts int) {
	m.ensureMessagixClient()
	cli := m.Client
	if cli == nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      MetaNotLoggedIn,
		})
		return
	}
	if attempts > 0 {
		retryIn := time.Duration(1<<attempts) * time.Second
		zerolog.Ctx(ctx).Debug().Stringer("retry_in", retryIn).Msg("Sleeping before retrying connection")
		select {
		case <-time.After(retryIn):
		case <-retryCtx.Done():
			zerolog.Ctx(ctx).Err(ctx.Err()).Msg("Connection cancelled during sleep")
			return
		}
	} else if state, err := m.Main.DB.PopReconnectionState(ctx, m.UserLogin.ID); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get reconnection state")
	} else if state != nil {
		if !m.Main.Config.CacheConnectionState {
			zerolog.Ctx(ctx).Debug().Msg("Not using saved reconnection state as it's disabled in the config")
		} else if err = cli.LoadState(state); err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to load reconnection state")
		} else {
			zerolog.Ctx(ctx).Debug().Msg("Reconnecting with cached state")
			m.connectWithCache(ctx)
			return
		}
	} else {
		zerolog.Ctx(ctx).Debug().Msg("No saved reconnection state")
	}
	if m.Main.Config.GetProxyFrom != "" || m.Main.Config.Proxy != "" {
		cli.GetNewProxy = m.Main.getProxy
		if !cli.UpdateProxy("connect") {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaProxyUpdateFail,
			})
			return
		}
	}
	currentUser, initialTable, err := cli.LoadMessagesPage(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to load messages page")
		if stopPeriodicReconnect := m.stopPeriodicReconnect.Swap(nil); stopPeriodicReconnect != nil {
			(*stopPeriodicReconnect)()
		}
		if errors.Is(err, messagix.ErrTokenInvalidated) {
			state := status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      MetaCookieRemoved,
			}
			if errors.Is(err, messagix.ErrTokenInvalidatedRedirect) {
				state.Error = MetaRedirectedToLoginPage
			} else if errors.Is(err, messagix.ErrUserIDIsZero) {
				state.Error = MetaUserIDIsZero
			}
			m.UserLogin.BridgeState.Send(state)
			m.Client = nil
			m.LoginMeta.Cookies = nil
			err = m.UserLogin.Save(ctx)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to save user login after clearing cookies")
			}
		} else if errors.Is(err, messagix.ErrChallengeRequired) {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGChallengeRequired,
				UserAction: status.UserActionRestart,
			})
		} else if errors.Is(err, messagix.ErrAccountSuspended) {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGAccountSuspended,
			})
		} else if errors.Is(err, messagix.ErrCheckpointRequired) {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      FBCheckpointRequired,
				UserAction: status.UserActionRestart,
			})
		} else if errors.Is(err, messagix.ErrConsentRequired) {
			code := IGConsentRequired
			if m.LoginMeta.Platform.IsMessenger() {
				code = FBConsentRequired
			}
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      code,
				UserAction: status.UserActionRestart,
			})
		} else if lsErr := (&types.ErrorResponse{}); errors.As(err, &lsErr) {
			stateEvt := status.StateUnknownError
			if lsErr.ErrorCode == 1357053 {
				stateEvt = status.StateBadCredentials
			} else if attempts < MaxConnectRetries {
				stateEvt = status.StateTransientDisconnect
			}
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: stateEvt,
				Error:      status.BridgeStateErrorCode(fmt.Sprintf("meta-lserror-%d", lsErr.ErrorCode)),
				Message:    lsErr.Error(),
			})
			if stateEvt == status.StateTransientDisconnect {
				m.connectWithRetry(retryCtx, ctx, attempts+1)
			}
		} else if gqlErr := (&types.GraphQLError{}); errors.As(err, &gqlErr) {
			// TODO determine if this should retry
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaGraphQLError,
				Message:    gqlErr.Message,
			})
		} else if attempts < MaxConnectRetries && !errors.Is(err, messagix.ErrVersionIDNotFound) {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateTransientDisconnect,
				Error:      MetaConnectError,
			})
			m.connectWithRetry(retryCtx, ctx, attempts+1)
		} else {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaConnectError,
				Info: map[string]any{
					"go_error": err.Error(),
				},
			})
		}
		return
	}
	if retryCtx.Err() != nil {
		zerolog.Ctx(ctx).Err(ctx.Err()).Msg("Connection cancelled")
		return
	}
	m.connectWithTable(ctx, initialTable, currentUser)
}

func (m *MetaClient) connectWithTable(ctx context.Context, initialTable *table.LSTable, currentUser types.UserInfo) {
	zerolog.Ctx(ctx).Debug().Msg("Loaded messages page, connecting to MQTT with initial table")
	go m.handleTableLoop(ctx)

	var err error
	m.Ghost, err = m.Main.Bridge.GetGhostByID(ctx, networkid.UserID(m.UserLogin.ID))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get own ghost")
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      MetaConnectError,
		})
		return
	}
	m.UserLogin.RemoteName = currentUser.GetName()
	m.UserLogin.RemoteProfile.Name = currentUser.GetName()
	if !m.LoginMeta.Platform.IsMessenger() {
		m.UserLogin.RemoteProfile.Username = currentUser.GetUsername()
	}
	m.UserLogin.RemoteProfile.Avatar = m.Ghost.AvatarMXC

	m.initialTable.Store(initialTable)

	err = m.Client.Connect(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to connect")
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      MetaConnectError,
		})
		return
	}

	go m.periodicReconnect()
}

func (m *MetaClient) connectWithCache(ctx context.Context) {
	go m.handleTableLoop(ctx)

	err := m.Client.Connect(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to connect")
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      MetaConnectError,
		})
		return
	}

	go m.periodicReconnect()
}

func (m *MetaClient) periodicReconnect() {
	if m.Main.Config.ForceRefreshIntervalSeconds <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if oldCancel := m.stopPeriodicReconnect.Swap(&cancel); oldCancel != nil {
		(*oldCancel)()
	}
	interval := time.Duration(m.Main.Config.ForceRefreshIntervalSeconds) * time.Second
	timer := time.NewTimer(interval)
	defer timer.Stop()
	m.UserLogin.Log.Info().Stringer("interval", interval).Msg("Starting periodic reconnect loop")
	for {
		select {
		case <-timer.C:
			m.UserLogin.Log.Info().Msg("Doing periodic reconnect")
			m.FullReconnect()
		case <-ctx.Done():
			return
		}
	}
}

func (m *MetaClient) tryConnectE2EE(fromConnectFailure bool) {
	err := m.connectE2EE()
	if err != nil {
		if m.waState.StateEvent != status.StateBadCredentials && m.waState.StateEvent != status.StateUnknownError {
			m.waState = status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      WAConnectError,
				Info: map[string]any{
					"go_error": err.Error(),
				},
			}
			m.UserLogin.BridgeState.Send(m.waState)
		}
		if fromConnectFailure {
			m.UserLogin.Log.Err(err).Msg("Failed to connect to e2ee after 415 error")
		} else {
			m.UserLogin.Log.Err(err).Msg("Failed to connect to e2ee")
		}
	}
}

func (m *MetaClient) connectE2EE() error {
	m.e2eeConnectLock.Lock()
	defer m.e2eeConnectLock.Unlock()
	if m.E2EEClient != nil {
		return fmt.Errorf("already connected to e2ee")
	}
	log := m.UserLogin.Log.With().Str("component", "e2ee").Logger()
	ctx := log.WithContext(m.Main.Bridge.BackgroundCtx)
	var err error
	if m.WADevice == nil && m.LoginMeta.WADeviceID != 0 {
		m.WADevice, err = m.Main.DeviceStore.GetDevice(ctx, waTypes.JID{User: string(m.UserLogin.ID), Device: m.LoginMeta.WADeviceID, Server: waTypes.MessengerServer})
		if err != nil {
			return fmt.Errorf("failed to get whatsmeow device: %w", err)
		} else if m.WADevice == nil {
			log.Warn().Uint16("device_id", m.LoginMeta.WADeviceID).Msg("Existing device not found in store")
		}
	}
	isNew := false
	if m.WADevice == nil {
		isNew = true
		m.WADevice = m.Main.DeviceStore.NewDevice()
	}
	if suggested := m.Client.MessengerLite.GetSuggestedDeviceID(); suggested != uuid.Nil {
		m.WADevice.FacebookUUID = suggested
	}
	m.Client.SetDevice(m.WADevice)

	if isNew {
		fbid := metaid.ParseUserLoginID(m.UserLogin.ID)
		log.Info().Msg("Registering new e2ee device")
		err = m.Client.RegisterE2EE(ctx, fbid)
		if err != nil {
			return fmt.Errorf("failed to register e2ee device: %w", err)
		}
		m.LoginMeta.WADeviceID = m.WADevice.ID.Device
		err = m.WADevice.Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save whatsmeow device store: %w", err)
		}
		err = m.UserLogin.Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save device ID to user login: %w", err)
		}
	}
	m.E2EEClient, err = m.Client.PrepareE2EEClient()
	if err != nil {
		return fmt.Errorf("failed to prepare e2ee client: %w", err)
	}
	if bridgev2.PortalEventBuffer == 0 {
		m.E2EEClient.SynchronousAck = true
		m.E2EEClient.EnableDecryptedEventBuffer = true
	}
	m.E2EEClient.AddEventHandlerWithSuccessStatus(m.e2eeEventHandler)
	err = m.E2EEClient.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to e2ee socket: %w", err)
	}
	return nil
}

func (m *MetaClient) Disconnect() {
	state := m.disconnect(true)
	if state != nil {
		err := m.Main.DB.PutReconnectionState(m.UserLogin.Log.WithContext(context.Background()), m.UserLogin.ID, state)
		if err != nil {
			m.UserLogin.Log.Err(err).Msg("Failed to save reconnection state")
		} else {
			m.UserLogin.Log.Debug().Msg("Saved reconnection state")
		}
	}
	m.metaState = status.BridgeState{}
	m.waState = status.BridgeState{}
}

func (m *MetaClient) disconnect(dumpState bool) (state json.RawMessage) {
	if stopConnectAttempt := m.stopConnectAttempt.Swap(nil); stopConnectAttempt != nil {
		(*stopConnectAttempt)()
	}
	if cli := m.Client; cli != nil {
		cli.SetEventHandler(nil)
		cli.Disconnect()
		if dumpState && m.Main.Config.CacheConnectionState {
			var err error
			state, err = cli.DumpState()
			if err != nil {
				m.UserLogin.Log.Err(err).Msg("Failed to dump state")
			}
		}
		m.Client = nil
	}
	if ecli := m.E2EEClient; ecli != nil {
		ecli.Disconnect()
		m.E2EEClient = nil
	}
	if stopTableLoop := m.stopHandlingTables.Swap(nil); stopTableLoop != nil {
		(*stopTableLoop)()
	}
	if stopPeriodicReconnect := m.stopPeriodicReconnect.Swap(nil); stopPeriodicReconnect != nil {
		(*stopPeriodicReconnect)()
	}
	return
}

func (m *MetaClient) IsLoggedIn() bool {
	return m.Client.IsAuthenticatedAndLoaded()
}

func (m *MetaClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return networkid.UserLoginID(userID) == m.UserLogin.ID
}

func (m *MetaClient) LogoutRemote(ctx context.Context) {
	m.disconnect(false)
	if dev := m.WADevice; dev != nil {
		err := dev.Delete(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to delete device from store")
		}
	}
	m.resetWADevice()
	m.metaState = status.BridgeState{}
	m.waState = status.BridgeState{}
	m.LoginMeta.Cookies = nil
	m.lastFullReconnect = time.Time{}
}

func (m *MetaClient) canReconnect() bool {
	return time.Since(m.lastFullReconnect) > time.Duration(m.Main.Config.MinFullReconnectIntervalSeconds)*time.Second && m.LoginMeta.Cookies != nil
}

func (m *MetaClient) FullReconnect() {
	if m.LoginMeta.Cookies == nil {
		return
	}
	ctx := m.UserLogin.Log.WithContext(m.Main.Bridge.BackgroundCtx)
	m.connectWaiter.Clear()
	m.e2eeConnectWaiter.Clear()
	m.disconnect(false)
	m.Connect(ctx)
	m.lastFullReconnect = time.Now()
}

func (m *MetaClient) resetWADevice() {
	m.WADevice = nil
	m.LoginMeta.WADeviceID = 0
}

func (m *MetaClient) FillBridgeState(state status.BridgeState) status.BridgeState {
	// The Meta bridge internally has two states - one for connection to meta and one for whatsapp;
	// as such we need to merge the two states when sending updates and pick the "worst case".
	copyFrom := m.pickWorstCaseBridgeState(state)
	if copyFrom != nil {
		state.StateEvent = copyFrom.StateEvent
		state.Error = copyFrom.Error
		state.Message = copyFrom.Message
		state.Info = copyFrom.Info
	}

	if state.Info == nil {
		state.Info = make(map[string]any)
	}
	state.Info["mode"] = m.LoginMeta.Platform.String()
	if m.LoginMeta.LoginUA != "" {
		state.Info["login_user_agent"] = m.LoginMeta.LoginUA
	}
	return state
}

func (m *MetaClient) pickWorstCaseBridgeState(state status.BridgeState) *status.BridgeState {
	if state.StateEvent == m.waState.StateEvent && state.StateEvent == m.metaState.StateEvent {
		// If both states are the same as the input, we can use as-is
		return nil
	}

	// Now find the worst case state in order (BAD_CREDENTIALS being worst), prefer the input state
	// if matches or fallback to either metaState or waState.
	for _, status := range []status.BridgeStateEvent{
		status.StateBadCredentials,
		status.StateUnknownError,
		status.StateTransientDisconnect,
		status.StateConnecting,
	} {
		if state.StateEvent == status {
			return nil
		} else if m.waState.StateEvent == status {
			return &m.waState
		} else if m.metaState.StateEvent == status {
			return &m.metaState
		}
	}

	return nil
}

func (m *MetaClient) updateWAPresence(ctx context.Context, presence waTypes.Presence) error {
	err := m.E2EEClient.SendPresence(ctx, presence)
	if err == nil {
		m.waLastPresence = presence
	}
	return err
}
