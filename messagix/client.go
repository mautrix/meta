package messagix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-querystring/query"
	"github.com/rs/zerolog"
	"golang.org/x/net/proxy"

	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/crypto"
	"go.mau.fi/mautrix-meta/messagix/data/endpoints"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

const DPR = "1"
const ChromeVersion = "118"
const ChromeVersionFull = ChromeVersion + ".0.5993.89"
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + ChromeVersion + ".0.0.0 Safari/537.36"
const SecCHUserAgent = `"Chromium";v="` + ChromeVersion + `", "Google Chrome";v="` + ChromeVersion + `", "Not-A.Brand";v="99"`
const SecCHFullVersionList = `"Chromium";v="` + ChromeVersionFull + `", "Google Chrome";v="` + ChromeVersionFull + `", "Not-A.Brand";v="99.0.0.0"`
const SecCHPlatform = `"Linux"`
const SecCHPlatformVersion = `"6.5.0"`
const SecCHMobile = "?0"
const SecCHModel = ""
const SecCHPrefersColorScheme = "light"

var ErrTokenInvalidated = errors.New("access token is no longer valid")

type EventHandler func(evt interface{})
type Client struct {
	Instagram *InstagramMethods
	Facebook  *FacebookMethods
	Logger    zerolog.Logger

	http         *http.Client
	socket       *Socket
	eventHandler EventHandler
	configs      *Configs
	SyncManager  *SyncManager

	cookies     cookies.Cookies
	httpProxy   func(*http.Request) (*url.URL, error)
	socksProxy  proxy.Dialer
	GetNewProxy func(reason string) (string, error)

	lsRequests      int
	graphQLRequests int
	platform        types.Platform
	endpoints       map[string]string
	taskMutex       *sync.Mutex
	activeTasks     []int

	stopCurrentConnection atomic.Pointer[context.CancelFunc]
}

func NewClient(platform types.Platform, cookies cookies.Cookies, logger zerolog.Logger, proxy string) (*Client, error) {
	cli := &Client{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ForceAttemptHTTP2:     true,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.Response == nil {
					return nil
				}
				respCookies := req.Response.Cookies()
				for _, cookie := range respCookies {
					if (cookie.Name == "xs" || cookie.Name == "sessionid") && cookie.MaxAge < 0 {
						return fmt.Errorf("%w: %s cookie was deleted", ErrTokenInvalidated, cookie.Name)
					}
				}
				return nil
			},
			Timeout: 60 * time.Second,
		},
		cookies:         cookies,
		Logger:          logger,
		lsRequests:      0,
		graphQLRequests: 1,
		platform:        platform,
		activeTasks:     make([]int, 0),
		taskMutex:       &sync.Mutex{},
	}

	cli.configurePlatformClient()
	cli.configs = &Configs{
		client:             cli,
		needSync:           false,
		browserConfigTable: &types.SchedulerJSDefineConfig{},
		accountConfigTable: &table.LSTable{},
		Bitmap:             crypto.NewBitmap(),
		CsrBitmap:          crypto.NewBitmap(),
	}
	if proxy != "" {
		err := cli.SetProxy(proxy)
		if err != nil {
			return nil, fmt.Errorf("messagix-client: failed to set proxy (%v)", err)
		}
	}

	if !cli.cookies.IsLoggedIn() {
		return cli, nil
	}

	err := cli.configureAfterLogin()
	if err != nil {
		return nil, err
	}

	return cli, nil
}

func (c *Client) configureAfterLogin() error {
	if !c.cookies.IsLoggedIn() {
		return fmt.Errorf("messagix-client: can't configure client after login, not authenticated yet")
	}
	socket := c.NewSocketClient()
	c.socket = socket

	moduleLoader := &ModuleParser{client: c}
	err := moduleLoader.Load(c.getEndpoint("messages"))
	if err != nil {
		return err
	}

	c.SyncManager = c.NewSyncManager()
	err = c.configs.SetupConfigs()
	if err != nil {
		return fmt.Errorf("messagix-configs: failed to setup configs (%v)", err)
	}

	return nil
}

func (c *Client) loadLoginPage() *ModuleParser {
	moduleLoader := &ModuleParser{client: c}
	moduleLoader.Load(c.getEndpoint("login_page"))
	return moduleLoader
}

func (c *Client) configurePlatformClient() {
	var selectedEndpoints map[string]string
	var cookieStruct cookies.Cookies
	switch c.platform {
	case types.Facebook:
		selectedEndpoints = endpoints.FacebookEndpoints
		cookieStruct = &cookies.FacebookCookies{}
		c.Facebook = &FacebookMethods{client: c}
	case types.FacebookTor:
		selectedEndpoints = endpoints.FacebookTorEndpoints
		cookieStruct = &cookies.FacebookCookies{}
		c.Facebook = &FacebookMethods{client: c}
	case types.Messenger:
		selectedEndpoints = endpoints.MessengerEndpoints
		cookieStruct = &cookies.FacebookCookies{}
		c.Facebook = &FacebookMethods{client: c}
	case types.Instagram:
		selectedEndpoints = endpoints.InstagramEndpoints
		cookieStruct = &cookies.InstagramCookies{}
		c.Instagram = &InstagramMethods{client: c}
	}

	c.endpoints = selectedEndpoints
	if reflect.ValueOf(c.cookies).IsNil() {
		c.cookies = cookieStruct
	}
}

func (c *Client) SetProxy(proxyAddr string) error {
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
		c.http.Transport.(*http.Transport).Dial = c.socksProxy.Dial
		contextDialer, ok := c.socksProxy.(proxy.ContextDialer)
		if ok {
			c.http.Transport.(*http.Transport).DialContext = contextDialer.DialContext
		}
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

func (c *Client) UpdateProxy(reason string) {
	if c.GetNewProxy == nil {
		return
	}
	if proxyAddr, err := c.GetNewProxy(reason); err != nil {
		c.Logger.Err(err).Str("reason", reason).Msg("Failed to get new proxy")
	} else if err = c.SetProxy(proxyAddr); err != nil {
		c.Logger.Err(err).Str("reason", reason).Msg("Failed to set new proxy")
	}
}

func (c *Client) Connect() error {
	if c.socket == nil {
		err := c.configureAfterLogin()
		if err != nil {
			return err
		}
	} else if err := c.socket.CanConnect(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.TODO())
	oldCancel := c.stopCurrentConnection.Swap(&cancel)
	if oldCancel != nil {
		(*oldCancel)()
	}
	go func() {
		reconnectIn := 2 * time.Second
		for {
			err := c.socket.Connect()
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, CONNECTION_REFUSED_UNAUTHORIZED) || errors.Is(err, CONNECTION_REFUSED_BAD_USERNAME_OR_PASSWORD) {
				c.eventHandler(&Event_PermanentError{Err: err})
				return
			}
			c.eventHandler(&Event_SocketError{Err: err})
			if errors.Is(err, ErrInReadLoop) {
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
	if fn := c.stopCurrentConnection.Load(); fn != nil {
		(*fn)()
	}
	c.socket.Disconnect()
}

func (c *Client) SaveSession(path string) error {
	jsonBytes, err := json.Marshal(c.cookies)
	if err != nil {
		return err
	}

	return os.WriteFile(path, jsonBytes, os.ModePerm)
}

func (c *Client) IsConnected() bool {
	return c.socket.conn != nil
}

func (c *Client) sendCookieConsent(jsDatr string) error {

	var payloadQuery interface{}
	h := c.buildHeaders(false)
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")

	if c.platform.IsMessenger() {
		h.Set("sec-fetch-site", "same-origin") // header is required
		h.Set("sec-fetch-user", "?1")
		h.Set("host", c.getEndpoint("host"))
		h.Set("upgrade-insecure-requests", "1")
		h.Set("origin", c.getEndpoint("base_url"))
		h.Set("cookie", "_js_datr="+jsDatr)
		h.Set("referer", c.getEndpoint("login_page"))
		q := c.NewHttpQuery()
		q.AcceptOnlyEssential = "false"
		payloadQuery = q
	} else {
		h.Set("sec-fetch-site", "same-site") // header is required
		h.Set("host", c.getEndpoint("host"))
		h.Set("origin", c.getEndpoint("base_url"))
		h.Set("referer", c.getEndpoint("base_url")+"/")
		h.Set("x-instagram-ajax", strconv.Itoa(int(c.configs.browserConfigTable.SiteData.ServerRevision)))
		variables, err := json.Marshal(&types.InstagramCookiesVariables{
			FirstPartyTrackingOptIn: true,
			IgDid:                   c.cookies.GetValue("ig_did"),
			ThirdPartyTrackingOptIn: true,
			Input: struct {
				ClientMutationID int "json:\"client_mutation_id,omitempty\""
			}{0},
		})
		h.Del("x-csrftoken")
		if err != nil {
			return fmt.Errorf("failed to marshal *types.InstagramCookiesVariables into bytes: %v", err)
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
	req, _, err := c.MakeRequest(c.getEndpoint("cookie_consent"), "POST", h, payload, types.FORM)
	if err != nil {
		return err
	}

	if c.platform.IsMessenger() {
		datr := c.findCookie(req.Cookies(), "datr")
		if datr == nil {
			return fmt.Errorf("consenting to facebook cookies failed, could not find datr cookie in set-cookie header")
		}

		c.cookies = &cookies.FacebookCookies{
			Datr: datr.Value,
			Wd:   "2276x1156",
		}
	}
	return nil
}

func (c *Client) getEndpoint(name string) string {
	if endpoint, ok := c.endpoints[name]; ok {
		return endpoint
	}
	panic(fmt.Sprintf("messagix-client: endpoint %s not found", name))
}

func (c *Client) IsAuthenticated() bool {
	var isAuthenticated bool
	if c.platform.IsMessenger() {
		isAuthenticated = c.configs.browserConfigTable.CurrentUserInitialData.AccountID != "0"
	} else {
		isAuthenticated = c.configs.browserConfigTable.PolarisViewer.ID != ""
	}
	return isAuthenticated
}

func (c *Client) GetCurrentAccount() (types.UserInfo, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("messagix-client: not yet authenticated")
	}

	if c.platform.IsMessenger() {
		return &c.configs.browserConfigTable.CurrentUserInitialData, nil
	} else {
		return &c.configs.browserConfigTable.PolarisViewer, nil
	}
}

func (c *Client) GetTaskId() int {
	c.taskMutex.Lock()
	defer c.taskMutex.Unlock()
	id := 0
	for slices.Contains[[]int, int](c.activeTasks, id) {
		id++
	}

	c.activeTasks = append(c.activeTasks, id)
	return id
}
