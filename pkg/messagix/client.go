package messagix

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"
	"go.mau.fi/util/exsync"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"golang.org/x/net/proxy"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/data/endpoints"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

var ErrClientIsNil = whatsmeow.ErrClientIsNil

type EventHandler func(ctx context.Context, evt any)

type Config struct {
	MayConnectToDGW bool
	ClientSettings  exhttp.ClientSettings
}

type Client struct {
	Instagram     *InstagramMethods
	Facebook      *FacebookMethods
	MessengerLite *MessengerLiteMethods
	Logger        zerolog.Logger
	Platform      types.Platform

	http         *http.Client
	httpSettings exhttp.ClientSettings
	proxyAddr    string
	socket       *Socket
	dgwSocket    *dgw.Socket
	eventHandler EventHandler
	configs      *Configs
	syncManager  *SyncManager

	cookies         *cookies.Cookies
	httpProxy       func(*http.Request) (*url.URL, error)
	socksProxy      proxy.Dialer
	GetNewProxy     func(reason string) (string, error)
	mayConnectToDGW bool

	device *store.Device

	lsRequests      int
	graphQLRequests int
	endpoints       map[string]string
	taskMutex       *sync.Mutex
	activeTasks     []int

	catRefreshLock         sync.Mutex
	unnecessaryCATRequests int

	stopCurrentConnections atomic.Pointer[context.CancelFunc]
	connectionLoopStopped  *exsync.Event
	canSendMessages        *exsync.Event
}

var DisableTLSVerification = false
var MaxConnectBackoff = 5 * time.Minute

func NewClient(cookies *cookies.Cookies, logger zerolog.Logger, cfg *Config) *Client {
	if cookies.Platform == types.Unset {
		panic("messagix: platform must be set in cookies")
	}
	cli := &Client{
		cookies:               cookies,
		Logger:                logger,
		lsRequests:            0,
		graphQLRequests:       1,
		Platform:              cookies.Platform,
		activeTasks:           make([]int, 0),
		taskMutex:             &sync.Mutex{},
		connectionLoopStopped: exsync.NewEvent(),
		canSendMessages:       exsync.NewEvent(),
	}
	cli.SetHTTP(cfg.ClientSettings)
	cli.connectionLoopStopped.Set()
	if DisableTLSVerification {
		cli.http.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}
	cli.http.CheckRedirect = cli.checkHTTPRedirect

	cli.configurePlatformClient()
	cli.configs = &Configs{
		client:             cli,
		BrowserConfigTable: &types.SchedulerJSDefineConfig{},
		Bitmap:             crypto.NewBitmap(),
		CSRBitmap:          crypto.NewBitmap(),
	}
	cli.socket = cli.newSocketClient()
	cli.mayConnectToDGW = cfg.MayConnectToDGW

	return cli
}

func (c *Client) GetCookies() *cookies.Cookies {
	return c.cookies
}

type dumpedState struct {
	Configs     *Configs
	SyncStore   map[int64]*socket.QueryMetadata
	PacketsSent uint16
	SessionID   int64
	Timestamp   time.Time
}

func (c *Client) DumpState() (json.RawMessage, error) {
	if c == nil || c.configs == nil || c.syncManager == nil || c.socket == nil || c.socket.packetsSent == 0 || !c.socket.previouslyConnected {
		return nil, nil
	}
	return json.Marshal(&dumpedState{
		Configs:     c.configs,
		SyncStore:   c.syncManager.store,
		PacketsSent: c.socket.packetsSent,
		SessionID:   c.socket.sessionID,
		Timestamp:   time.Now(),
	})
}

const MaxCachedStateAge = 24 * time.Hour

var ErrCachedStateTooOld = errors.New("cached state is too old")

func (c *Client) LoadState(state json.RawMessage) error {
	if c == nil {
		return ErrClientIsNil
	}
	var dumped dumpedState
	if err := json.Unmarshal(state, &dumped); err != nil {
		return err
	} else if !dumped.Timestamp.IsZero() && time.Since(dumped.Timestamp) > MaxCachedStateAge {
		return fmt.Errorf("%w (created at %s)", ErrCachedStateTooOld, dumped.Timestamp)
	}

	c.configs = dumped.Configs
	c.syncManager = c.newSyncManager()
	c.syncManager.store = dumped.SyncStore
	c.socket.packetsSent = dumped.PacketsSent
	c.socket.sessionID = dumped.SessionID
	if c.Platform == types.Instagram {
		c.socket.broker = "wss://edge-chat.instagram.com/chat?"
	} else {
		c.socket.broker = c.configs.BrowserConfigTable.MqttWebConfig.Endpoint
	}
	c.socket.previouslyConnected = true
	c.Logger.Info().Int64("session_id", c.socket.sessionID).Msg("Loaded state")
	return nil
}

func (c *Client) LoadMessagesPage(ctx context.Context) (types.UserInfo, *table.LSTable, error) {
	if c == nil {
		return nil, nil, ErrClientIsNil
	} else if !c.cookies.IsLoggedIn() {
		return nil, nil, ErrTokenInvalidated
	}

	moduleLoader := &ModuleParser{client: c, LS: &table.LSTable{}}
	err := moduleLoader.Load(ctx, c.GetEndpoint("messages"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load inbox: %w", err)
	}

	c.syncManager = c.newSyncManager()
	ls, err := c.configs.SetupConfigs(ctx, moduleLoader.LS)
	if err != nil {
		return nil, nil, err
	}
	var currentUser types.UserInfo
	if c.Platform.IsMessenger() {
		currentUser = &c.configs.BrowserConfigTable.CurrentUserInitialData
	} else {
		currentUser = &c.configs.BrowserConfigTable.PolarisViewer
	}
	return currentUser, ls, nil
}

func (c *Client) GetPlatform() types.Platform {
	if c == nil {
		return types.Unset
	}
	return c.Platform
}

func (c *Client) configurePlatformClient() {
	var selectedEndpoints map[string]string
	switch c.Platform {
	case types.Facebook:
		selectedEndpoints = endpoints.FacebookEndpoints
		c.Facebook = &FacebookMethods{client: c}
	case types.FacebookTor:
		selectedEndpoints = endpoints.FacebookTorEndpoints
		c.Facebook = &FacebookMethods{client: c}
	case types.Messenger:
		selectedEndpoints = endpoints.MessengerEndpoints
		c.Facebook = &FacebookMethods{client: c}
	case types.MessengerLite:
		selectedEndpoints = endpoints.MessengerLiteEndpoints
		c.MessengerLite = &MessengerLiteMethods{client: c}
	case types.Instagram:
		selectedEndpoints = endpoints.InstagramEndpoints
		c.Instagram = &InstagramMethods{client: c}
	}

	c.endpoints = selectedEndpoints
}

func (c *Client) SetProxy(proxyAddr string) error {
	if c == nil {
		return ErrClientIsNil
	}
	proxyParsed, err := url.Parse(proxyAddr)
	if err != nil {
		return err
	}

	if proxyParsed.Scheme == "http" || proxyParsed.Scheme == "https" {
		c.httpProxy = http.ProxyURL(proxyParsed)
		c.proxyAddr = proxyAddr
	} else if proxyParsed.Scheme == "socks5" {
		c.socksProxy, err = proxy.FromURL(proxyParsed, &net.Dialer{Timeout: 20 * time.Second})
		if err != nil {
			return err
		}
		c.proxyAddr = proxyAddr
	}
	c.SetHTTP(c.httpSettings)

	c.Logger.Debug().
		Str("scheme", proxyParsed.Scheme).
		Str("host", proxyParsed.Host).
		Msg("Using proxy")
	return nil
}

func (c *Client) SetEventHandler(handler EventHandler) {
	if c == nil {
		return
	}
	c.eventHandler = handler
}

func (c *Client) HandleEvent(ctx context.Context, evt any) {
	if c.eventHandler != nil {
		c.eventHandler(ctx, evt)
	}
}

func (c *Client) SetHTTP(settings exhttp.ClientSettings) {
	if c == nil {
		return
	}
	c.httpSettings = settings.WithGlobalTimeout(60 * time.Second)
	if c.proxyAddr != "" {
		c.httpSettings, _ = c.httpSettings.WithProxy(c.proxyAddr)
	}
	oldHTTP := c.http
	c.http = c.httpSettings.Compile()
	c.http.CheckRedirect = c.checkHTTPRedirect
	if oldHTTP != nil {
		oldHTTP.CloseIdleConnections()
	}
}

func (c *Client) UpdateProxy(reason string) bool {
	if c == nil || c.GetNewProxy == nil {
		return true
	}
	if proxyAddr, err := c.GetNewProxy(reason); err != nil {
		c.Logger.Err(err).Str("reason", reason).Msg("Failed to get new proxy")
		return false
	} else if err = c.SetProxy(proxyAddr); err != nil {
		c.Logger.Err(err).Str("reason", reason).Msg("Failed to set new proxy")
		return false
	}
	return true
}

func (c *Client) Connect(ctx context.Context) error {
	if c == nil {
		return ErrClientIsNil
	} else if err := c.socket.CanConnect(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	oldCancel := c.stopCurrentConnections.Swap(&cancel)
	if oldCancel != nil {
		(*oldCancel)()
	}
	c.connectionLoopStopped.Clear()
	go func() {
		defer func() {
			if c.stopCurrentConnections.Load() == &cancel {
				c.connectionLoopStopped.Set()
			}
		}()
		connectionAttempts := 1
		var reconnectIn time.Duration
		for {
			c.canSendMessages.Clear() // In case we're reconnecting from a normal network error
			connectStart := time.Now()
			err := c.socket.Connect(ctx)
			c.canSendMessages.Clear()
			if ctx.Err() != nil {
				zerolog.Ctx(ctx).Warn().
					Err(ctx.Err()).
					AnErr("connect_err", err).
					Msg("Context canceled, stopping connection loop")
				return
			}
			if errors.Is(err, CONNECTION_REFUSED_UNAUTHORIZED) ||
				// TODO server unavailable may mean a challenge state, should be checked somehow
				errors.Is(err, CONNECTION_REFUSED_SERVER_UNAVAILABLE) {
				c.HandleEvent(ctx, &Event_PermanentError{Err: err})
				return
			} else if errors.Is(err, CONNECTION_REFUSED_BAD_USERNAME_OR_PASSWORD) && connectionAttempts > 5 {
				// Allow up to 5 reconnects for this error as it does not seem permanent
				c.HandleEvent(ctx, &Event_PermanentError{Err: err})
				return
			}
			c.HandleEvent(ctx, &Event_SocketError{Err: err, ConnectionAttempts: connectionAttempts})
			if time.Since(connectStart) > 2*time.Minute && (err == nil || errors.Is(err, socket.ErrInReadLoop)) {
				// Reconnect immediately after a long successful connection
				reconnectIn = 0
				connectionAttempts = 0
			} else {
				if reconnectIn == 0 {
					reconnectIn = 1 * time.Second
				}
				connectionAttempts += 1
				reconnectIn *= 2
				if reconnectIn > MaxConnectBackoff {
					reconnectIn = MaxConnectBackoff
				}
			}
			if err != nil {
				c.Logger.Err(err).Dur("reconnect_in", reconnectIn).Msg("Error in connection, reconnecting")
			} else {
				c.Logger.Warn().Dur("reconnect_in", reconnectIn).Msg("Connection closed without error, reconnecting")
			}
			select {
			case <-time.After(reconnectIn):
			case <-ctx.Done():
				return
			}
			c.UpdateProxy("reconnect")
		}
	}()
	if c.Platform == types.Instagram && c.mayConnectToDGW {
		go c.connectDGW(ctx)
	}
	return nil
}

func (c *Client) connectDGW(ctx context.Context) error {
	reconnectIn := 2 * time.Second
	for {
		connectStart := time.Now()
		err := c.connectDGWOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Since(connectStart) > 2*time.Minute {
			reconnectIn = 2 * time.Second
		} else {
			reconnectIn *= 2
			if reconnectIn > MaxConnectBackoff {
				reconnectIn = MaxConnectBackoff
			}
		}
		if err != nil {
			c.Logger.Err(err).Dur("reconnect_in", reconnectIn).Msg("Error in DGW connection, reconnecting")
		} else {
			c.Logger.Warn().Dur("reconnect_in", reconnectIn).Msg("DGW connection closed without error, reconnecting")
		}
		select {
		case <-time.After(reconnectIn):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Client) connectDGWOnce(ctx context.Context) error {
	c.dgwSocket = dgw.NewSocketClient(c)
	err := c.dgwSocket.CanConnect()
	if err != nil {
		return err
	}
	err = c.dgwSocket.Connect(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) Disconnect() {
	if c == nil {
		return
	}
	if fn := c.stopCurrentConnections.Load(); fn != nil {
		(*fn)()
	}
	c.socket.Disconnect()
	if c.dgwSocket != nil {
		c.dgwSocket.Disconnect()
	}
	if !c.connectionLoopStopped.WaitTimeout(5 * time.Second) {
		c.Logger.Warn().Msg("Connection loop didn't stop in time")
	}
}

func (c *Client) IsConnected() bool {
	return c != nil && c.socket.conn != nil
}

func (c *Client) GetEndpoint(name string) string {
	if endpoint, ok := c.endpoints[name]; ok {
		return endpoint
	}
	panic(fmt.Sprintf("messagix-client: endpoint %s not found", name))
}

func (c *Client) getEndpointForThreadID(threadID int64) string {
	return c.GetEndpoint("thread") + strconv.FormatInt(threadID, 10) + "/"
}

func (c *Client) IsAuthenticated() bool {
	if c == nil {
		return false
	}
	var isAuthenticated bool
	if c.Platform.IsMessenger() {
		isAuthenticated = c.configs.BrowserConfigTable.CurrentUserInitialData.AccountID != "0"
	} else {
		isAuthenticated = c.configs.BrowserConfigTable.PolarisViewer.ID != ""
	}
	return isAuthenticated
}

func (c *Client) IsAuthenticatedAndLoaded() bool {
	return c.IsAuthenticated() && c.syncManager != nil
}

func (c *Client) GetCurrentAccount() (types.UserInfo, error) {
	if c == nil {
		return nil, ErrClientIsNil
	} else if !c.IsAuthenticated() {
		return nil, fmt.Errorf("messagix-client: not yet authenticated")
	}

	if c.Platform.IsMessenger() {
		return &c.configs.BrowserConfigTable.CurrentUserInitialData, nil
	} else {
		return &c.configs.BrowserConfigTable.PolarisViewer, nil
	}
}

func (c *Client) getTaskID() int {
	c.taskMutex.Lock()
	defer c.taskMutex.Unlock()
	id := 0
	for slices.Contains[[]int, int](c.activeTasks, id) {
		id++
	}

	c.activeTasks = append(c.activeTasks, id)
	return id
}

func (c *Client) WaitUntilCanSendMessages(ctx context.Context, timeout time.Duration) error {
	if c == nil {
		return ErrClientIsNil
	}
	select {
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for canSendMessages")
	case <-ctx.Done():
		return ctx.Err()
	case <-c.canSendMessages.GetChan():
		return nil
	}
}

func (c *Client) GetLogger() *zerolog.Logger {
	return &c.Logger
}

func (c *Client) ForceReconnect() {
	if c == nil {
		return
	}
	c.socket.Disconnect()
	c.dgwSocket.Disconnect()
}

func (c *Client) FetchMoreThreads(ctx context.Context, syncGroup int64) (*socket.KeyStoreData, *table.LSTable, error) {
	if c == nil {
		return nil, nil, ErrClientIsNil
	}
	keyStore := c.syncManager.getSyncGroupKeyStore(syncGroup)
	if keyStore == nil || !keyStore.HasMoreBefore {
		return nil, nil, nil // No more threads
	}

	tskm := c.newTaskManager()
	tskm.AddNewTask(&socket.FetchThreadsTask{
		IsAfter:                    0,
		ParentThreadKey:            keyStore.ParentThreadKey,
		ReferenceThreadKey:         keyStore.MinThreadKey,
		ReferenceActivityTimestamp: keyStore.MinLastActivityTimestampMs,
		AdditionalPagesToFetch:     0,
		Cursor:                     c.syncManager.GetCursor(syncGroup),
		SyncGroup:                  int(syncGroup),
	})

	payload, err := tskm.FinalizePayload()
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.socket.makeLSRequest(ctx, payload, 3)
	if err != nil {
		return nil, nil, err
	}

	resp.Finish()
	c.socket.postHandlePublishResponse(resp.Table)

	return keyStore, resp.Table, nil
}
