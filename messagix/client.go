package messagix

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"sync"

	"github.com/0xzer/messagix/cookies"
	"github.com/0xzer/messagix/crypto"
	"github.com/0xzer/messagix/data/endpoints"
	"github.com/0xzer/messagix/table"
	"github.com/0xzer/messagix/types"
	"github.com/google/go-querystring/query"
	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
)

var USER_AGENT = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36"
var ErrRedirectAttempted = errors.New("redirect attempted")

type EventHandler func(evt interface{})
type Client struct {
	Account *Account
	Threads *Threads
	Messages *Messages
	Instagram *InstagramMethods
	Facebook *FacebookMethods
	Logger zerolog.Logger

	http *http.Client
	socket *Socket
	eventHandler EventHandler
	configs *Configs
	SyncManager *SyncManager

	cookies cookies.Cookies
	proxy func(*http.Request) (*url.URL, error)

	lsRequests int
	graphQLRequests int
	platform types.Platform
	endpoints map[string]string
	taskMutex *sync.Mutex
	activeTasks []int
}

// pass an empty zerolog.Logger{} for no logging
func NewClient(platform types.Platform, cookies cookies.Cookies, logger zerolog.Logger, proxy string) (*Client, error) {
	cli := &Client{
		http: &http.Client{
			Transport: &http.Transport{},
		},
		cookies: cookies,
		Logger: logger,
		lsRequests: 0,
		graphQLRequests: 1,
		platform: platform,
		activeTasks: make([]int, 0),
		taskMutex: &sync.Mutex{},
	}

	cli.Account = &Account{client: cli}
	cli.Messages = &Messages{client: cli}
	cli.Threads = &Threads{client: cli}
	cli.configurePlatformClient()
	cli.configs = &Configs{
		client: cli,
		needSync: false,
		browserConfigTable: &types.SchedulerJSDefineConfig{},
		accountConfigTable: &table.LSTable{},
		Bitmap: crypto.NewBitmap(),
		CsrBitmap: crypto.NewBitmap(),
	}
	if proxy != "" {
		err := cli.SetProxy(proxy)
		if err != nil {
			return nil, fmt.Errorf("messagix-client: failed to set proxy (%e)", err)
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
		return fmt.Errorf("messagix-configs: failed to setup configs (%e)", err)
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

func (c *Client) SetProxy(proxy string) error {
	proxyParsed, err := url.Parse(proxy)
	if err != nil {
		return err
	}

	c.http.Transport = &http.Transport{
		Proxy: http.ProxyURL(proxyParsed),
	}
	c.Logger.Debug().Any("addr", proxyParsed.Host).Msg("Proxy Updated")
	return nil
}

func (c *Client) SetEventHandler(handler EventHandler) {
	c.eventHandler = handler
}

func (c *Client) Connect() error {
	if c.socket == nil {
		err := c.configureAfterLogin()
		if err != nil {
			return err
		}
	}
	return c.socket.Connect()
}

func (c *Client) Disconnect() {
	if c.socket.conn != nil {
		c.socket.conn.Close()
	}
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
	h.Add("sec-fetch-dest", "empty")
	h.Add("sec-fetch-mode", "cors")

	if c.platform == types.Facebook {
		h.Add("sec-fetch-site", "same-origin") // header is required
		h.Add("sec-fetch-user", "?1")
		h.Add("host", c.getEndpoint("host"))
		h.Add("upgrade-insecure-requests", "1")
		h.Add("origin", c.getEndpoint("base_url"))
		h.Add("cookie", "_js_datr="+jsDatr)
		h.Add("referer", c.getEndpoint("login_page"))
		q := c.NewHttpQuery()
		q.AcceptOnlyEssential = "false"
		payloadQuery = q
	} else {
		h.Add("sec-fetch-site", "same-site") // header is required
		h.Add("host", c.getEndpoint("host"))
		h.Add("origin", c.getEndpoint("base_url"))
		h.Add("referer", c.getEndpoint("base_url") + "/")
		h.Add("x-instagram-ajax", strconv.Itoa(int(c.configs.browserConfigTable.SiteData.ServerRevision)))
		variables, err := json.Marshal(&types.InstagramCookiesVariables{
			FirstPartyTrackingOptIn: true,
			IgDid: c.cookies.GetValue("ig_did"),
			ThirdPartyTrackingOptIn: true,
			Input: struct{ClientMutationID int "json:\"client_mutation_id,omitempty\""}{0},
		})
		h.Del("x-csrftoken")
		if err != nil {
			return fmt.Errorf("failed to marshal *types.InstagramCookiesVariables into bytes: %e", err)
		}
		q := &HttpQuery{
			DocID: "3810865872362889",
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

	if c.platform == types.Facebook {
		datr := c.findCookie(req.Cookies(), "datr")
		if datr == nil {
			return fmt.Errorf("consenting to facebook cookies failed, could not find datr cookie in set-cookie header")
		}
	
		c.cookies = &cookies.FacebookCookies{
			Datr: datr.Value,
			Wd: "2276x1156",
		}
	}
	return nil
}

func (c *Client) enableRedirects() {
	c.http.CheckRedirect = nil
}

func (c *Client) disableRedirects() {
	c.http.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return ErrRedirectAttempted
	}
}

func (c *Client) getEndpoint(name string) string {
    if endpoint, ok := c.endpoints[name]; ok {
        return endpoint
    }

    log.Fatalf("failed to find endpoint for name: %s (platform=%v)", name, c.platform)
    return ""
}

func (c *Client) IsAuthenticated() bool {
	var isAuthenticated bool
	if c.platform == types.Facebook {
		isAuthenticated = c.configs.browserConfigTable.CurrentUserInitialData.AccountID != "0"
	} else {
		c.Logger.Info().Any("data", c.configs.browserConfigTable.PolarisViewer).Msg("PolarisViewer")
		isAuthenticated = c.configs.browserConfigTable.PolarisViewer.ID != ""
	}
	return isAuthenticated
}

func (c *Client) CurrentPlatform() string {
	var s string
	if c.platform == types.Facebook {
		s = "Facebook"
	} else {
		s = "Instagram"
	}
	return s
}

func (c *Client) GetCurrentAccount() (types.AccountInfo, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("messagix-client: not yet authenticated")
	}

	if c.platform == types.Facebook {
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