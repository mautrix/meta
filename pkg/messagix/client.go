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

	"github.com/google/go-querystring/query"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"golang.org/x/net/proxy"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/data/endpoints"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

const DPR = "1"
const BrowserName = "Chrome"
const ChromeVersion = "136"
const ChromeVersionFull = ChromeVersion + ".0.7103.48"
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + ChromeVersion + ".0.0.0 Safari/537.36"
const SecCHUserAgent = `"Chromium";v="` + ChromeVersion + `", "Google Chrome";v="` + ChromeVersion + `", "Not-A.Brand";v="99"`
const SecCHFullVersionList = `"Chromium";v="` + ChromeVersionFull + `", "Google Chrome";v="` + ChromeVersionFull + `", "Not-A.Brand";v="99.0.0.0"`
const OSName = "Linux"
const OSVersion = "6.8.0"
const SecCHPlatform = `"` + OSName + `"`
const SecCHPlatformVersion = `"` + OSVersion + `"`
const SecCHMobile = "?0"
const SecCHModel = `""`
const SecCHPrefersColorScheme = "light"

var ErrClientIsNil = whatsmeow.ErrClientIsNil

type EventHandler func(ctx context.Context, evt any)

type Client struct {
	Instagram *InstagramMethods
	Facebook  *FacebookMethods
	Logger    zerolog.Logger
	Platform  types.Platform

	http         *http.Client
	socket       *Socket
	eventHandler EventHandler
	configs      *Configs
	syncManager  *SyncManager

	cookies     *cookies.Cookies
	httpProxy   func(*http.Request) (*url.URL, error)
	socksProxy  proxy.Dialer
	GetNewProxy func(reason string) (string, error)

	device *store.Device

	lsRequests      int
	graphQLRequests int
	endpoints       map[string]string
	taskMutex       *sync.Mutex
	activeTasks     []int

	catRefreshLock         sync.Mutex
	unnecessaryCATRequests int

	stopCurrentConnection atomic.Pointer[context.CancelFunc]

	canSendMessages  bool
	sendMessagesCond *sync.Cond
}

var DisableTLSVerification = false

func NewClient(cookies *cookies.Cookies, logger zerolog.Logger) *Client {
	if cookies.Platform == types.Unset {
		panic("messagix: platform must be set in cookies")
	}
	cli := &Client{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 40 * time.Second,
				ForceAttemptHTTP2:     true,
			},
			Timeout: 60 * time.Second,
		},
		cookies:          cookies,
		Logger:           logger,
		lsRequests:       0,
		graphQLRequests:  1,
		Platform:         cookies.Platform,
		activeTasks:      make([]int, 0),
		taskMutex:        &sync.Mutex{},
		canSendMessages:  false,
		sendMessagesCond: sync.NewCond(&sync.Mutex{}),
	}
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
}

func (c *Client) DumpState() (json.RawMessage, error) {
	if c.configs == nil || c.syncManager == nil || c.socket == nil || c.socket.packetsSent == 0 || !c.socket.previouslyConnected {
		return nil, nil
	}
	return json.Marshal(&dumpedState{
		Configs:     c.configs,
		SyncStore:   c.syncManager.store,
		PacketsSent: c.socket.packetsSent,
		SessionID:   c.socket.sessionID,
	})
}

func (c *Client) LoadState(state json.RawMessage) error {
	var dumped dumpedState
	if err := json.Unmarshal(state, &dumped); err != nil {
		return err
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
		return nil, nil, fmt.Errorf("can't load messages page without being authenticated")
	}

	moduleLoader := &ModuleParser{client: c, LS: &table.LSTable{}}
	err := moduleLoader.Load(ctx, c.getEndpoint("messages"))
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

func (c *Client) loadLoginPage(ctx context.Context) *ModuleParser {
	moduleLoader := &ModuleParser{client: c}
	moduleLoader.Load(ctx, c.getEndpoint("login_page"))
	return moduleLoader
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
		c.http.Transport.(*http.Transport).Proxy = c.httpProxy
	} else if proxyParsed.Scheme == "socks5" {
		c.socksProxy, err = proxy.FromURL(proxyParsed, &net.Dialer{Timeout: 20 * time.Second})
		if err != nil {
			return err
		}
		contextDialer := c.socksProxy.(proxy.ContextDialer)
		c.http.Transport.(*http.Transport).DialContext = contextDialer.DialContext
	}

	c.Logger.Debug().
		Str("scheme", proxyParsed.Scheme).
		Str("host", proxyParsed.Host).
		Msg("Using proxy")
	return nil
}

func (c *Client) SetEventHandler(handler EventHandler) {
	c.eventHandler = handler
}

func (c *Client) handleEvent(ctx context.Context, evt any) {
	if c.eventHandler != nil {
		c.eventHandler(ctx, evt)
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
	oldCancel := c.stopCurrentConnection.Swap(&cancel)
	if oldCancel != nil {
		(*oldCancel)()
	}
	go func() {
		connectionAttempts := 1
		reconnectIn := 2 * time.Second
		for {
			c.disableSendingMessages() // In case we're reconnecting from a normal network error
			connectStart := time.Now()
			err := c.socket.Connect(ctx)
			c.disableSendingMessages()
			if ctx.Err() != nil {
				zerolog.Ctx(ctx).Warn().
					Err(ctx.Err()).
					AnErr("connect_err", err).
					Msg("Context canceled, stopping connection loop")
				return
			}
			if errors.Is(err, CONNECTION_REFUSED_UNAUTHORIZED) ||
				errors.Is(err, CONNECTION_REFUSED_BAD_USERNAME_OR_PASSWORD) ||
				// TODO server unavailable may mean a challenge state, should be checked somehow
				errors.Is(err, CONNECTION_REFUSED_SERVER_UNAVAILABLE) {
				c.handleEvent(ctx, &Event_PermanentError{Err: err})
				return
			}
			connectionAttempts += 1
			c.handleEvent(ctx, &Event_SocketError{Err: err, ConnectionAttempts: connectionAttempts})
			if time.Since(connectStart) > 2*time.Minute {
				reconnectIn = 2 * time.Second
			} else {
				reconnectIn *= 2
				if reconnectIn > 5*time.Minute {
					reconnectIn = 5 * time.Minute
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
	return nil
}

func (c *Client) Disconnect() {
	if c == nil {
		return
	}
	if fn := c.stopCurrentConnection.Load(); fn != nil {
		(*fn)()
	}
	c.socket.Disconnect()
}

func (c *Client) IsConnected() bool {
	return c != nil && c.socket.conn != nil
}

func (c *Client) sendCookieConsent(ctx context.Context, jsDatr string) error {

	var payloadQuery interface{}
	h := c.buildHeaders(false, false)
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")

	if c.Platform.IsMessenger() {
		h.Set("sec-fetch-site", "same-origin") // header is required
		h.Set("sec-fetch-user", "?1")
		h.Set("host", c.getEndpoint("host"))
		h.Set("upgrade-insecure-requests", "1")
		h.Set("origin", c.getEndpoint("base_url"))
		h.Set("cookie", "_js_datr="+jsDatr)
		h.Set("referer", c.getEndpoint("login_page"))
		q := c.newHTTPQuery()
		q.AcceptOnlyEssential = "false"
		payloadQuery = q
	} else {
		h.Set("sec-fetch-site", "same-site") // header is required
		h.Set("host", c.getEndpoint("host"))
		h.Set("origin", c.getEndpoint("base_url"))
		h.Set("referer", c.getEndpoint("base_url")+"/")
		h.Set("x-instagram-ajax", strconv.FormatInt(c.configs.BrowserConfigTable.SiteData.ServerRevision, 10))
		variables, err := json.Marshal(&types.InstagramCookiesVariables{
			FirstPartyTrackingOptIn: true,
			IgDid:                   c.cookies.Get("ig_did"),
			ThirdPartyTrackingOptIn: true,
			Input: struct {
				ClientMutationID int `json:"client_mutation_id,omitempty"`
			}{0},
		})
		h.Del("x-csrftoken")
		if err != nil {
			return fmt.Errorf("failed to marshal *types.InstagramCookiesVariables into bytes: %w", err)
		}
		q := &HttpQuery{
			DocID:     "3810865872362889",
			Variables: string(variables),
		}
		payloadQuery = q
	}

	form, err := query.Values(payloadQuery)
	if err != nil {
		return err
	}

	payload := []byte(form.Encode())
	req, _, err := c.MakeRequest(ctx, c.getEndpoint("cookie_consent"), "POST", h, payload, types.FORM)
	if err != nil {
		return err
	}

	if c.Platform.IsMessenger() {
		datr := c.findCookie(req.Cookies(), "datr")
		if datr == nil {
			return fmt.Errorf("consenting to facebook cookies failed, could not find datr cookie in set-cookie header")
		}

		c.cookies.Set(cookies.MetaCookieDatr, datr.Value)
		c.cookies.Set(cookies.FBCookieWindowDimensions, "1920x1003")
	}
	return nil
}

func (c *Client) getEndpoint(name string) string {
	if endpoint, ok := c.endpoints[name]; ok {
		return endpoint
	}
	panic(fmt.Sprintf("messagix-client: endpoint %s not found", name))
}

func (c *Client) getEndpointForThreadID(threadID int64) string {
	return c.getEndpoint("thread") + strconv.FormatInt(threadID, 10) + "/"
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

func (c *Client) EnableSendingMessages() {
	if c == nil {
		return
	}
	c.sendMessagesCond.L.Lock()
	c.canSendMessages = true
	c.sendMessagesCond.Broadcast()
	c.sendMessagesCond.L.Unlock()
}

func (c *Client) disableSendingMessages() {
	if c == nil {
		return
	}
	c.sendMessagesCond.L.Lock()
	c.canSendMessages = false
	c.sendMessagesCond.L.Unlock()
}

func (c *Client) WaitUntilCanSendMessages(ctx context.Context, timeout time.Duration) error {
	if c == nil {
		return ErrClientIsNil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	done := make(chan struct{})
	go func() {
		c.sendMessagesCond.L.Lock()
		defer c.sendMessagesCond.L.Unlock()
		c.sendMessagesCond.Wait()
		close(done)
	}()

	for !c.canSendMessages {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for canSendMessages")
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			if c.canSendMessages {
				return nil
			}
		}
	}
	return nil
}
