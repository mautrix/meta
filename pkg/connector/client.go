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

	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"
	"go.mau.fi/util/ptr"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

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
	incomingTables     chan *table.LSTable
	backfillCollectors map[int64]*BackfillCollector
	backfillLock       sync.Mutex

	stopPeriodicReconnect atomic.Pointer[context.CancelFunc]
	lastFullReconnect     time.Time
	connectWaiter         *exsync.Event
	e2eeConnectWaiter     *exsync.Event

	E2EEClient      *whatsmeow.Client
	WADevice        *store.Device
	e2eeConnectLock sync.Mutex

	metaState status.BridgeState
	waState   status.BridgeState
}

func (m *MetaConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	loginMetadata := login.Metadata.(*metaid.UserLoginMetadata)
	var messagixClient *messagix.Client
	if loginMetadata.Cookies != nil {
		loginMetadata.Cookies.Platform = loginMetadata.Platform
		messagixClient = messagix.NewClient(loginMetadata.Cookies, login.Log.With().Str("component", "messagix").Logger())
	}
	c := &MetaClient{
		Main:      m,
		Client:    messagixClient,
		LoginMeta: loginMetadata,
		UserLogin: login,

		incomingTables:     make(chan *table.LSTable, 16),
		backfillCollectors: make(map[int64]*BackfillCollector),

		connectWaiter:     exsync.NewEvent(),
		e2eeConnectWaiter: exsync.NewEvent(),
	}
	if messagixClient != nil {
		messagixClient.SetEventHandler(c.handleMetaEvent)
	}
	login.Client = c
	return nil
}

var (
	_ bridgev2.NetworkAPI                      = (*MetaClient)(nil)
	_ status.StandaloneCustomBridgeStateFiller = (*MetaClient)(nil)
)

type respGetProxy struct {
	ProxyURL string `json:"proxy_url"`
}

// TODO this should be moved into mautrix-go

func (m *MetaClient) getProxy(reason string) (string, error) {
	if m.Main.Config.GetProxyFrom == "" {
		return m.Main.Config.Proxy, nil
	}
	parsed, err := url.Parse(m.Main.Config.GetProxyFrom)
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

func (m *MetaClient) Connect(ctx context.Context) error {
	if m.Client == nil {
		m.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      MetaNotLoggedIn,
		})
		return nil
	}
	if m.Main.Config.GetProxyFrom != "" || m.Main.Config.Proxy != "" {
		m.Client.GetNewProxy = m.getProxy
		if !m.Client.UpdateProxy("connect") {
			return fmt.Errorf("failed to update proxy")
		}
	}
	currentUser, initialTable, err := m.Client.LoadMessagesPage()
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to load messages page")
		if stopPeriodicReconnect := m.stopPeriodicReconnect.Swap(nil); stopPeriodicReconnect != nil {
			(*stopPeriodicReconnect)()
		}
		if errors.Is(err, messagix.ErrTokenInvalidated) {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      MetaCookieRemoved,
			})
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
			})
		} else if errors.Is(err, messagix.ErrAccountSuspended) {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGAccountSuspended,
			})
		} else if errors.Is(err, messagix.ErrConsentRequired) {
			code := IGConsentRequired
			if m.LoginMeta.Platform.IsMessenger() {
				code = FBConsentRequired
			}
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      code,
			})
		} else if lsErr := (&types.ErrorResponse{}); errors.As(err, &lsErr) {
			stateEvt := status.StateUnknownError
			if lsErr.ErrorCode == 1357053 {
				stateEvt = status.StateBadCredentials
			}
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: stateEvt,
				Error:      status.BridgeStateErrorCode(fmt.Sprintf("meta-lserror-%d", lsErr.ErrorCode)),
				Message:    lsErr.Error(),
			})
		} else {
			m.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaConnectError,
			})
		}
		return nil
	}
	return m.connectWithTable(ctx, initialTable, currentUser)
}

func (m *MetaClient) connectWithTable(ctx context.Context, initialTable *table.LSTable, currentUser types.UserInfo) error {
	go m.handleTableLoop()

	var err error
	m.Ghost, err = m.Main.Bridge.GetGhostByID(ctx, networkid.UserID(m.UserLogin.ID))
	if err != nil {
		return fmt.Errorf("failed to get own ghost: %w", err)
	}
	m.UserLogin.RemoteName = currentUser.GetName()
	m.UserLogin.RemoteProfile.Name = currentUser.GetName()
	if !m.LoginMeta.Platform.IsMessenger() {
		m.UserLogin.RemoteProfile.Username = currentUser.GetUsername()
	}
	m.UserLogin.RemoteProfile.Avatar = m.Ghost.AvatarMXC

	m.initialTable.Store(initialTable)

	err = m.Client.Connect()
	if err != nil {
		return err
	}

	go m.periodicReconnect()

	return nil
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

func (m *MetaClient) connectE2EE() error {
	m.e2eeConnectLock.Lock()
	defer m.e2eeConnectLock.Unlock()
	if m.E2EEClient != nil {
		return fmt.Errorf("already connected to e2ee")
	}
	log := m.UserLogin.Log.With().Str("component", "e2ee").Logger()
	ctx := log.WithContext(context.TODO())
	var err error
	if m.WADevice == nil && m.LoginMeta.WADeviceID != 0 {
		m.WADevice, err = m.Main.DeviceStore.GetDevice(waTypes.JID{User: string(m.UserLogin.ID), Device: m.LoginMeta.WADeviceID, Server: waTypes.MessengerServer})
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
	m.Client.SetDevice(m.WADevice)

	if isNew {
		fbid := metaid.ParseUserLoginID(m.UserLogin.ID)
		log.Info().Msg("Registering new e2ee device")
		err = m.Client.RegisterE2EE(ctx, fbid)
		if err != nil {
			return fmt.Errorf("failed to register e2ee device: %w", err)
		}
		m.LoginMeta.WADeviceID = m.WADevice.ID.Device
		err = m.WADevice.Save()
		if err != nil {
			return fmt.Errorf("failed to save whatsmeow device store: %w", err)
		}
		err = m.UserLogin.Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save device ID to user login: %w", err)
		}
	}
	m.E2EEClient = m.Client.PrepareE2EEClient()
	m.E2EEClient.AddEventHandler(m.e2eeEventHandler)
	err = m.E2EEClient.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to e2ee socket: %w", err)
	}
	return nil
}

func (m *MetaClient) Disconnect() {
	if cli := m.Client; cli != nil {
		cli.Disconnect()
		m.Client = nil
	}
	if ecli := m.E2EEClient; ecli != nil {
		ecli.Disconnect()
		m.E2EEClient = nil
	}
	m.metaState = status.BridgeState{}
	m.waState = status.BridgeState{}
	if stopTableLoop := m.stopHandlingTables.Swap(nil); stopTableLoop != nil {
		(*stopTableLoop)()
	}
	if stopPeriodicReconnect := m.stopPeriodicReconnect.Swap(nil); stopPeriodicReconnect != nil {
		(*stopPeriodicReconnect)()
	}
}

var metaCaps = &bridgev2.NetworkRoomCapabilities{
	FormattedText:    true,
	UserMentions:     true,
	Replies:          true,
	Edits:            true,
	EditMaxCount:     10,
	EditMaxAge:       24 * time.Hour,
	Reactions:        true,
	ReactionCount:    1,
	LocationMessages: true,
	Captions:         true,
}

var metaCapsWithThreads *bridgev2.NetworkRoomCapabilities

func init() {
	metaCapsWithThreads = ptr.Clone(metaCaps)
	metaCapsWithThreads.Threads = true
}

func (m *MetaClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *bridgev2.NetworkRoomCapabilities {
	switch portal.Metadata.(*metaid.PortalMetadata).ThreadType {
	case table.COMMUNITY_GROUP:
		return metaCapsWithThreads
	}
	return metaCaps
}

func (m *MetaClient) IsLoggedIn() bool {
	return m.Client != nil && m.Client.SyncManager != nil
}

func (m *MetaClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return networkid.UserLoginID(userID) == m.UserLogin.ID
}

func (m *MetaClient) LogoutRemote(ctx context.Context) {
	m.Disconnect()
	if dev := m.WADevice; dev != nil {
		err := dev.Delete()
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to delete device from store")
		}
	}
	m.resetWADevice()
}

func (m *MetaClient) canReconnect() bool {
	return time.Since(m.lastFullReconnect) > time.Duration(m.Main.Config.MinFullReconnectIntervalSeconds)*time.Second && m.LoginMeta.Cookies != nil
}

func (m *MetaClient) FullReconnect() {
	if m.LoginMeta.Cookies == nil {
		return
	}
	ctx := m.UserLogin.Log.WithContext(context.TODO())
	m.connectWaiter.Clear()
	m.e2eeConnectWaiter.Clear()
	m.Disconnect()
	m.Client = messagix.NewClient(m.LoginMeta.Cookies, m.UserLogin.Log.With().Str("component", "messagix").Logger())
	m.Client.SetEventHandler(m.handleMetaEvent)
	err := m.Connect(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to reconnect")
	}
}

func (m *MetaClient) resetWADevice() {
	m.WADevice = nil
	m.LoginMeta.WADeviceID = 0
}

func (m *MetaClient) FillBridgeState(state status.BridgeState) status.BridgeState {
	if state.StateEvent == status.StateConnected {
		var copyFrom *status.BridgeState
		if m.waState.StateEvent != "" && m.waState.StateEvent != status.StateConnected {
			copyFrom = &m.waState
		}
		if m.metaState.StateEvent != "" && m.metaState.StateEvent != status.StateConnected {
			copyFrom = &m.metaState
		}
		if copyFrom != nil {
			state.StateEvent = copyFrom.StateEvent
			state.Error = copyFrom.Error
			state.Message = copyFrom.Message
		}
	}
	return state
}
