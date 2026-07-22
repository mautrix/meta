package messagix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"
	"go.mau.fi/util/exsync"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/endpoints"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

var ErrClientIsNil = whatsmeow.ErrClientIsNil

type EventHandler func(ctx context.Context, evt any)

type Config struct {
	ClientSettings           exhttp.ClientSettings
	LogRedactedBloksPayloads bool
}

type Client struct {
	http *httpclient.HTTPClient

	Instagram     *InstagramMethods
	Facebook      *FacebookMethods
	MessengerLite *MessengerLiteMethods
	Logger        zerolog.Logger
	Platform      types.Platform

	socket             *dgw.Socket
	socketWasSynced    atomic.Bool
	socketWasConnected atomic.Bool
	packetsSent        atomic.Uint32
	socketSyncWaiters  *exsync.Map[int64, chan *PublishResponseData]

	eventHandler EventHandler
	configs      *httpclient.Configs
	syncManager  *SyncManager

	cookies *cookies.Cookies

	device *store.Device

	endpoints  map[string]string
	nextTaskID atomic.Int64

	catRefreshLock         sync.Mutex
	unnecessaryCATRequests int

	stopCurrentConnections atomic.Pointer[context.CancelFunc]
	connectionLoopStopped  *exsync.Event
	canSendMessages        *exsync.Event
}

var MaxConnectBackoff = 5 * time.Minute

func NewClient(cookies *cookies.Cookies, logger zerolog.Logger, cfg *Config) *Client {
	if cookies.Platform == types.Unset {
		panic("messagix: platform must be set in cookies")
	}
	cli := &Client{
		cookies:               cookies,
		Logger:                logger,
		Platform:              cookies.Platform,
		connectionLoopStopped: exsync.NewEvent(),
		canSendMessages:       exsync.NewEvent(),
		socketSyncWaiters:     exsync.NewMap[int64, chan *PublishResponseData](),
	}
	cli.configs = httpclient.NewConfigs(cli)
	cli.http = httpclient.NewHTTPClient(cli, cli.configs, cfg.ClientSettings)
	cli.http.LogRedactedBloksPayloads = cfg.LogRedactedBloksPayloads
	cli.nextTaskID.Store(-1) // start from 0
	cli.connectionLoopStopped.Set()

	cli.configurePlatformClient()
	cli.socket = dgw.NewSocket(dgw.SocketOptions{
		GetCookies: func() string {
			return cli.GetCookies().String()
		},
		OnConnect: cli.onSocketConnect,
		Origin:    cli.GetEndpoint("base_url"),
		WSURL:     cli.GetEndpoint("dgw_lightspeed"),
		DialOpts:  *cli.http.GetWebsocketDialer(),
		Log:       logger.With().Str("socket", "main").Logger(),
		Facebook:  true,
		LoggingID: true,
	})

	return cli
}

func (c *Client) GetCookies() *cookies.Cookies {
	if c == nil {
		return nil
	}
	return c.cookies
}

func (c *Client) GetHTTP() *httpclient.HTTPClient {
	if c == nil {
		return nil
	}
	return c.http
}

type dumpedState struct {
	Configs     *httpclient.Configs
	SyncStore   map[int64]*socket.QueryMetadata
	PacketsSent uint16
	SessionID   int64
	Timestamp   time.Time
}

func (c *Client) DumpState() (json.RawMessage, error) {
	if c == nil || c.configs == nil || c.syncManager == nil || c.socket == nil || !c.socketWasSynced.Load() {
		return nil, nil
	}
	return json.Marshal(&dumpedState{
		Configs:     c.configs,
		SyncStore:   c.syncManager.store,
		PacketsSent: uint16(c.packetsSent.Load()),
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
	c.updateSocketIDs()
	c.packetsSent.Store(uint32(dumped.PacketsSent))
	c.socketWasSynced.Store(true)
	c.Logger.Info().Msg("Loaded state")
	return nil
}

func (c *Client) LoadMessagesPage(ctx context.Context) (types.UserInfo, *table.LSTable, error) {
	if c == nil {
		return nil, nil, ErrClientIsNil
	} else if !c.cookies.IsLoggedIn() {
		return nil, nil, httpclient.ErrTokenInvalidated
	}

	moduleLoader := httpclient.NewModuleParser(c, c.http, c.configs)
	err := moduleLoader.Load(ctx, c.GetEndpoint("messages"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load inbox: %w", err)
	}

	c.syncManager = c.newSyncManager()
	ls, err := c.setupConfigs(ctx, moduleLoader.LS)
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
	case types.MessengerLiteIOS, types.MessengerLiteAndroid:
		selectedEndpoints = endpoints.MessengerLiteEndpoints
		c.Facebook = &FacebookMethods{client: c}
		c.MessengerLite = &MessengerLiteMethods{client: c}
	case types.Instagram:
		selectedEndpoints = endpoints.InstagramEndpoints
		c.Instagram = &InstagramMethods{client: c}
	}

	c.endpoints = selectedEndpoints
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

func (c *Client) Connect(ctx context.Context) error {
	if c == nil {
		return ErrClientIsNil
	} else if !c.IsAuthenticated() {
		return fmt.Errorf("messagix-client: not yet authenticated, cannot connect")
	} else if c.socket.UserID == "0" || c.socket.UserID == "" {
		return fmt.Errorf("messagix-client: socket not configured with user ID")
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
			c.clearSocketSyncWaiters()
			c.canSendMessages.Clear()
			if ctx.Err() != nil {
				zerolog.Ctx(ctx).Warn().
					Err(ctx.Err()).
					AnErr("connect_err", err).
					Msg("Context canceled, stopping connection loop")
				return
			}
			closeCode := websocket.CloseStatus(err)
			// TODO determine if this is the correct thing to fail on
			if closeCode == dgw.CloseStatusUnauthorized {
				c.Logger.Err(err).Msg("Error in connection, exiting")
				c.HandleEvent(ctx, &PermanentErrorEvent{Err: err})
				return
			}
			c.HandleEvent(ctx, &TransientDisconnectEvent{Err: err, ConnectionAttempts: connectionAttempts})
			if time.Since(connectStart) > 2*time.Minute && c.socketWasConnected.Load() {
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
			c.http.UpdateProxy("reconnect")
		}
	}()
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
	if !c.connectionLoopStopped.WaitTimeout(5 * time.Second) {
		c.Logger.Warn().Msg("Connection loop didn't stop in time")
	}
}

func (c *Client) IsConnected() bool {
	return c != nil && c.socket.IsConnected()
}

func (c *Client) GetEndpoint(name string) string {
	if endpoint, ok := c.endpoints[name]; ok {
		return endpoint
	}
	panic(fmt.Sprintf("messagix-client: endpoint %s not found", name))
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

func (c *Client) getTaskID() int64 {
	return c.nextTaskID.Add(1)
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
}

func (c *Client) FetchMoreThreads(ctx context.Context, syncGroup int64) (*socket.KeyStoreData, *table.LSTable, error) {
	if c == nil {
		return nil, nil, ErrClientIsNil
	}
	keyStore := c.syncManager.getSyncGroupKeyStore(syncGroup)
	zerolog.Ctx(ctx).Debug().Any("key_store", keyStore).Msg("Current key store for thread sync")
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

	resp, err := c.makeLSRequest(ctx, payload, 3)
	if err != nil {
		return nil, nil, err
	}

	tbl, err := resp.Parse(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}
	c.PostHandlePublishResponse(tbl)

	return keyStore, tbl, nil
}
