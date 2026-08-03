package messagix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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
	BusinessSuite            *BusinessSuiteContext
}

type BusinessSuiteContext struct {
	BusinessID string
	AssetID    string
	PageID     string
	PageName   string
}

type Client struct {
	http *httpclient.HTTPClient

	Facebook      *FacebookMethods
	MessengerLite *MessengerLiteMethods
	Logger        zerolog.Logger
	Platform      types.Platform
	BusinessSuite *BusinessSuiteContext

	socket             *dgw.Socket
	businessSocket     *businessSuiteSocket
	socketWasSynced    atomic.Bool
	socketWasConnected atomic.Bool
	packetsSent        atomic.Uint32
	socketSyncWaiters  *exsync.Map[int64, chan *PublishResponseData]

	eventHandler       EventHandler
	configs            *httpclient.Configs
	syncManager        *SyncManager
	switchableProfiles []types.SwitchableProfile

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
		BusinessSuite:         cfg.BusinessSuite,
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
		AppStreamGroup: func() string {
			if cli.Platform == types.BusinessSuite {
				return "group1"
			}
			return ""
		}(),
	})
	if cli.Platform == types.BusinessSuite {
		cli.businessSocket = newBusinessSuiteSocket(cli)
	}

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
	c.switchableProfiles = append(c.switchableProfiles[:0], moduleLoader.SwitchableProfiles...)
	if c.Platform == types.BusinessSuite && c.BusinessSuite != nil {
		c.BusinessSuite.BusinessID = fmt.Sprint(c.configs.BrowserConfigTable.CurrentBusinessUser.BusinessID)
		if c.BusinessSuite.PageID == "" {
			c.BusinessSuite.PageID = c.BusinessSuite.AssetID
		}
	}

	c.syncManager = c.newSyncManager()
	ls, err := c.setupConfigs(ctx, moduleLoader.LS)
	if err != nil {
		return nil, nil, err
	}
	currentUser := &c.configs.BrowserConfigTable.CurrentUserInitialData
	return currentUser, ls, nil
}

func (c *Client) GetBusinessSuiteContext() *BusinessSuiteContext {
	if c == nil || c.BusinessSuite == nil {
		return nil
	}
	copy := *c.BusinessSuite
	return &copy
}

// GetRequestActorID returns the selected Page actor used by Business Suite's
// GraphQL requests. Authentication remains tied to the human account in
// __user; av scopes the request to the Page mailbox.
func (c *Client) GetRequestActorID() string {
	if c == nil || c.Platform != types.BusinessSuite || c.BusinessSuite == nil {
		return ""
	}
	if c.BusinessSuite.PageID != "" {
		return c.BusinessSuite.PageID
	}
	return c.BusinessSuite.AssetID
}

// ResolveBusinessSuitePage follows Facebook's own Page inbox redirect to map
// a profile-switcher actor ID to the asset ID used by Business Suite.
func (c *Client) ResolveBusinessSuitePage(ctx context.Context, profile types.SwitchableProfile) (*BusinessSuiteContext, error) {
	if c == nil || profile.ID == "" {
		return nil, fmt.Errorf("messagix: a Page profile is required")
	}
	// The profile-switcher ID is a persona actor, not necessarily the public
	// Page ID. The public Page inbox URL performs the authoritative mapping.
	var resp *http.Response
	var err error
	if profile.Username != "" {
		headers := c.http.BuildHeaders(true, true)
		var pageBody []byte
		resp, pageBody, err = c.http.MakeRequest(ctx, "https://www.facebook.com/"+url.PathEscape(profile.Username), http.MethodGet, headers, nil, types.NONE)
		if err == nil {
			for _, pattern := range pageAssetPatterns {
				if match := pattern.FindSubmatch(pageBody); len(match) > 1 {
					assetID := string(match[1])
					return &BusinessSuiteContext{AssetID: assetID, PageID: assetID, PageName: profile.Name}, nil
				}
			}
		}
	}
	// Some switcher responses omit both username and avatar metadata. Business
	// Suite still exposes the account's active Page asset in its bootstrap.
	// This is authoritative when the login flow offered a single Page, and is a
	// safe fallback until the multi-asset selector is queried directly.
	if resp == nil || err != nil {
		headers := c.http.BuildHeaders(true, true)
		var suiteBody []byte
		resp, suiteBody, err = c.http.MakeRequest(ctx, "https://business.facebook.com/latest/inbox/all", http.MethodGet, headers, nil, types.NONE)
		if err == nil {
			for _, pattern := range pageAssetPatterns {
				if match := pattern.FindSubmatch(suiteBody); len(match) > 1 {
					assetID := string(match[1])
					return &BusinessSuiteContext{AssetID: assetID, PageID: assetID, PageName: profile.Name}, nil
				}
			}
		}
	}
	if err != nil || resp == nil {
		previousActor := c.cookies.Get(cookies.FBCookieIUser)
		c.cookies.Set(cookies.FBCookieIUser, profile.ID)
		headers := c.http.BuildHeaders(true, true)
		resp, _, err = c.http.MakeRequest(ctx, "https://www.facebook.com/messages", http.MethodGet, headers, nil, types.NONE)
		if previousActor == "" {
			c.cookies.Delete(cookies.FBCookieIUser)
		} else {
			c.cookies.Set(cookies.FBCookieIUser, previousActor)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Business Suite Page: %w", err)
	}
	q := resp.Request.URL.Query()
	assetID := q.Get("asset_id")
	if assetID == "" {
		return nil, fmt.Errorf("Business Suite redirect did not include a Page asset ID (final host %s path %s)", resp.Request.URL.Hostname(), resp.Request.URL.Path)
	}
	if _, err := strconv.ParseInt(assetID, 10, 64); err != nil {
		return nil, fmt.Errorf("Business Suite returned an invalid Page asset ID")
	}
	businessID := q.Get("business_id")
	if businessID == "" {
		businessID = q.Get("bpn_id")
	}
	return &BusinessSuiteContext{
		BusinessID: strings.TrimSpace(businessID),
		AssetID:    assetID,
		PageID:     assetID,
		PageName:   profile.Name,
	}, nil
}

var pageAssetPatterns = []*regexp.Regexp{
	regexp.MustCompile(`"asset_id":"(\d{6,})"`),
	regexp.MustCompile(`"pageID":"(\d{6,})"`),
	regexp.MustCompile(`"page_id":"(\d{6,})"`),
	regexp.MustCompile(`"delegate_page_id":"(\d{6,})"`),
}

// GetSwitchableProfiles returns Facebook Pages exposed by the current web
// session's profile switcher. The data comes from the page preload and does not
// require a Graph API access token.
func (c *Client) GetSwitchableProfiles() []types.SwitchableProfile {
	if c == nil {
		return nil
	}
	return append([]types.SwitchableProfile(nil), c.switchableProfiles...)
}

// DiscoverSwitchableProfiles loads Facebook's main page, where the account
// switcher is consistently preloaded even when it is absent from /messages.
func (c *Client) DiscoverSwitchableProfiles(ctx context.Context) error {
	if c == nil {
		return ErrClientIsNil
	}
	moduleLoader := httpclient.NewModuleParser(c, c.http, c.configs)
	if err := moduleLoader.DiscoverSwitchableProfiles(ctx, c.GetEndpoint("base_url")); err != nil {
		return fmt.Errorf("failed to load Facebook profile switcher: %w", err)
	}
	c.switchableProfiles = append(c.switchableProfiles[:0], moduleLoader.SwitchableProfiles...)
	c.Logger.Info().Int("switchable_profile_count", len(c.switchableProfiles)).Msg("Discovered Facebook switchable profiles")
	return nil
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
	case types.MessengerLiteIOS:
		selectedEndpoints = endpoints.MessengerLiteIOSEndpoints
		c.Facebook = &FacebookMethods{client: c}
		c.MessengerLite = &MessengerLiteMethods{client: c}
	case types.MessengerLiteAndroid:
		selectedEndpoints = endpoints.MessengerLiteAndroidEndpoints
		c.Facebook = &FacebookMethods{client: c}
		c.MessengerLite = &MessengerLiteMethods{client: c}
	case types.BusinessSuite:
		if c.BusinessSuite == nil || c.BusinessSuite.AssetID == "" {
			panic("messagix: Business Suite platform requires an asset ID")
		}
		selectedEndpoints = endpoints.MakeBusinessSuiteEndpoints(c.BusinessSuite.AssetID)
		c.Facebook = &FacebookMethods{client: c}
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
			var err error
			if c.Platform == types.BusinessSuite {
				err = c.businessSocket.Connect(ctx)
			} else {
				err = c.socket.Connect(ctx)
			}
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
	if c.Platform == types.BusinessSuite {
		c.businessSocket.Disconnect()
	} else {
		c.socket.Disconnect()
	}
	if !c.connectionLoopStopped.WaitTimeout(5 * time.Second) {
		c.Logger.Warn().Msg("Connection loop didn't stop in time")
	}
}

func (c *Client) IsConnected() bool {
	if c == nil {
		return false
	}
	if c.Platform == types.BusinessSuite {
		return c.businessSocket.isConnected()
	}
	return c.socket.IsConnected()
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
	return c.configs.BrowserConfigTable.CurrentUserInitialData.AccountID != "0"
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

	return &c.configs.BrowserConfigTable.CurrentUserInitialData, nil
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
	if c.Platform == types.BusinessSuite {
		c.businessSocket.ForceReconnect()
	} else {
		c.socket.ForceReconnect()
	}
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
