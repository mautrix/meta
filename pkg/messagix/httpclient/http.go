package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/imroc/req/v3"
	utls "github.com/refraction-networking/utls"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type HTTPClient struct {
	parent          Client
	log             *zerolog.Logger
	graphQLRequests int
	lsRequests      int
	configs         *Configs

	HTTP            *http.Client
	HTTPSettings    exhttp.ClientSettings
	websocketClient *http.Client
	proxyAddr       string
	GetNewProxy     func(reason string) (string, error)

	LogRedactedBloksPayloads bool
}

type Client interface {
	GetPlatform() types.Platform
	GetLogger() *zerolog.Logger
	GetCookies() *cookies.Cookies
	GetEndpoint(name string) string
	IsAuthenticated() bool
}

func NewHTTPClient(cli Client, configs *Configs, settings exhttp.ClientSettings) *HTTPClient {
	c := &HTTPClient{
		parent:          cli,
		log:             cli.GetLogger(),
		graphQLRequests: 1,
		lsRequests:      0,
		configs:         configs,
	}
	c.SetConfig(settings)
	return c
}

func (c *HTTPClient) SetConfig(settings exhttp.ClientSettings) {
	if c == nil {
		return
	}
	c.HTTPSettings = settings.
		WithGlobalTimeout(60 * time.Second).
		WithResponseHeaderTimeout(20 * time.Second)
	if c.proxyAddr != "" {
		c.HTTPSettings, _ = c.HTTPSettings.WithProxy(c.proxyAddr)
	}
	reqClient := req.C().ImpersonateChrome()
	wsClient := req.C().ImpersonateChrome()
	forceHTTP1ChromeFingerprint(wsClient)
	if DisableTLSVerification {
		reqClient.SetTLSClientConfig(&tls.Config{
			InsecureSkipVerify: true,
		})
		wsClient.SetTLSClientConfig(&tls.Config{
			InsecureSkipVerify: true,
		})
	}

	oldHTTP := c.HTTP
	c.websocketClient = req.WithTransportOverride(c.HTTPSettings.WithGlobalTimeout(WebsocketHandshakeTimeout), wsClient).Compile()
	c.HTTP = req.WithTransportOverride(c.HTTPSettings, reqClient).Compile()
	c.HTTP.CheckRedirect = c.checkHTTPRedirect
	if oldHTTP != nil {
		oldHTTP.CloseIdleConnections()
	}

	if DisableTLSVerification {
		c.HTTP.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}
}

// TODO deduplicate this with mautrix-discord (and maybe shorten it, looks to be duplicating too much of req)
func forceHTTP1ChromeFingerprint(c *req.Client) {
	c.SetTLSHandshake(func(ctx context.Context, addr string, plainConn net.Conn) (net.Conn, *tls.ConnectionState, error) {
		hostname := addr
		if i := strings.LastIndex(addr, ":"); i != -1 {
			hostname = addr[:i]
		}

		spec, err := utls.UTLSIdToSpec(utls.HelloChrome_120)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build Chrome uTLS spec: %w", err)
		}

		// The actual changes we're making here:
		exts := spec.Extensions[:0]
		for _, ext := range spec.Extensions {
			switch e := ext.(type) {
			case *utls.ApplicationSettingsExtension:
				continue
			case *utls.ALPNExtension:
				e.AlpnProtocols = []string{"http/1.1"}
			}
			exts = append(exts, ext)
		}
		spec.Extensions = exts

		tlsConfig := c.GetTLSClientConfig()
		uconn := utls.UClient(plainConn, &utls.Config{
			ServerName:         hostname,
			NextProtos:         []string{"http/1.1"},
			RootCAs:            tlsConfig.RootCAs,
			InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
			KeyLogWriter:       tlsConfig.KeyLogWriter,
		}, utls.HelloCustom)
		if err := uconn.ApplyPreset(&spec); err != nil {
			return nil, nil, fmt.Errorf("failed to apply Chrome uTLS spec: %w", err)
		}
		if err := uconn.HandshakeContext(ctx); err != nil {
			return nil, nil, err
		}

		cs := uconn.ConnectionState()
		return uconn, &tls.ConnectionState{
			Version:            cs.Version,
			HandshakeComplete:  cs.HandshakeComplete,
			DidResume:          cs.DidResume,
			CipherSuite:        cs.CipherSuite,
			NegotiatedProtocol: cs.NegotiatedProtocol,
			ServerName:         cs.ServerName,
			PeerCertificates:   cs.PeerCertificates,
			VerifiedChains:     cs.VerifiedChains,
		}, nil
	})
}

func (c *HTTPClient) SetProxy(proxyAddr string) error {
	if c == nil {
		return nil
	}
	proxyParsed, err := url.Parse(proxyAddr)
	if err != nil {
		return err
	}

	c.proxyAddr = proxyAddr
	c.SetConfig(c.HTTPSettings)

	c.log.Debug().
		Str("scheme", proxyParsed.Scheme).
		Str("host", proxyParsed.Host).
		Msg("Using proxy")
	return nil
}

func (c *HTTPClient) UpdateProxy(reason string) bool {
	if c == nil || c.GetNewProxy == nil {
		return true
	}
	if proxyAddr, err := c.GetNewProxy(reason); err != nil {
		c.log.Err(err).Str("reason", reason).Msg("Failed to get new proxy")
		return false
	} else if err = c.SetProxy(proxyAddr); err != nil {
		c.log.Err(err).Str("reason", reason).Msg("Failed to set new proxy")
		return false
	}
	return true
}

var DisableTLSVerification = false
var WebsocketHandshakeTimeout = 20 * time.Second

func (c *HTTPClient) GetWebsocketDialer() *websocket.DialOptions {
	if c == nil {
		return nil
	}
	return &websocket.DialOptions{HTTPClient: c.websocketClient}
}

type HTTPQuery struct {
	AcceptOnlyEssential  string `url:"accept_only_essential,omitempty"`
	Av                   string `url:"av,omitempty"`          // not required
	User                 string `url:"__user,omitempty"`      // not required
	A                    string `url:"__a,omitempty"`         // 1 or 0 wether to include "suggestion_keys" or not in the response - no idea what this is
	Req                  string `url:"__req,omitempty"`       // not required
	Hs                   string `url:"__hs,omitempty"`        // not required
	Dpr                  string `url:"dpr,omitempty"`         // not required
	Ccg                  string `url:"__ccg,omitempty"`       // not required
	Rev                  string `url:"__rev,omitempty"`       // not required
	S                    string `url:"__s,omitempty"`         // not required
	Hsi                  string `url:"__hsi,omitempty"`       // not required
	Dyn                  string `url:"__dyn,omitempty"`       // not required
	Csr                  string `url:"__csr,omitempty"`       // not required
	CometReq             string `url:"__comet_req,omitempty"` // not required but idk what this is
	FbDtsg               string `url:"fb_dtsg,omitempty"`
	Jazoest              string `url:"jazoest,omitempty"`                  // not required
	Lsd                  string `url:"lsd,omitempty"`                      // required
	SpinR                string `url:"__spin_r,omitempty"`                 // not required
	SpinB                string `url:"__spin_b,omitempty"`                 // not required
	SpinT                string `url:"__spin_t,omitempty"`                 // not required
	Jssesw               string `url:"__jssesw,omitempty"`                 // not required
	FbAPICallerClass     string `url:"fb_api_caller_class,omitempty"`      // not required
	FbAPIReqFriendlyName string `url:"fb_api_req_friendly_name,omitempty"` // not required
	Variables            string `url:"variables,omitempty"`
	ServerTimestamps     string `url:"server_timestamps,omitempty"` // "true" or "false"
	DocID                string `url:"doc_id,omitempty"`
	D                    string `url:"__d,omitempty"`               // for insta
	AppID                string `url:"app_id,omitempty"`            // not required
	PushEndpoint         string `url:"push_endpoint,omitempty"`     // not required
	SubscriptionKeys     string `url:"subscription_keys,omitempty"` // not required
	DeviceToken          string `url:"device_token,omitempty"`      // not required
	DeviceType           string `url:"device_type,omitempty"`       // not required
	Mid                  string `url:"mid,omitempty"`               // not required
	Aaid                 string `url:"__aaid,omitempty"`

	ClientPreviousActorID string `url:"client_previous_actor_id,omitempty"` // not required
	RouteURL              string `url:"route_url,omitempty"`                // not required
	RoutingNamespace      string `url:"routing_namespace,omitempty"`        // not required
	Crn                   string `url:"__crn,omitempty"`                    // not required

	// The following keys are used for Messenger Lite:

	Method string `url:"method,omitempty"`
	Pretty string `url:"pretty,omitempty"` // "true" or "false"
	Format string `url:"format,omitempty"`
	// ServerTimestamps
	Locale  string `url:"locale,omitempty"`
	Purpose string `url:"purpose,omitempty"`
	// FbAPIReqFriendlyName
	ClientDocID                                 string `url:"client_doc_id,omitempty"`
	EnableCanonicalNaming                       string `url:"enable_canonical_naming,omitempty"`                          // "true" or "false"
	EnableCanonicalVariableOverrides            string `url:"enable_canonical_variable_overrides,omitempty"`              // "true" or "false"
	EnableCanonicalNamingAmbiguousTypePrefixing string `url:"enable_canonical_naming_ambiguous_type_prefixing,omitempty"` // "true" or "false"
	// Variables
}

func (c *HTTPClient) NewHTTPQuery() *HTTPQuery {
	c.graphQLRequests++
	siteConfig := c.configs.BrowserConfigTable.SiteData
	dpr := strconv.FormatFloat(siteConfig.Pr, 'g', 4, 64)
	query := &HTTPQuery{
		User:     c.configs.BrowserConfigTable.CurrentUserInitialData.UserID,
		A:        "1",
		Req:      strconv.FormatInt(int64(c.graphQLRequests), 36),
		Hs:       siteConfig.HasteSession,
		Dpr:      dpr,
		Ccg:      c.configs.BrowserConfigTable.WebConnectionClassServerGuess.ConnectionClass,
		Rev:      strconv.Itoa(siteConfig.SpinR),
		S:        c.configs.WebSessionID,
		Hsi:      siteConfig.Hsi,
		Dyn:      c.configs.Bitmap.CompressedStr,
		Csr:      c.configs.CSRBitmap.CompressedStr,
		CometReq: c.configs.CometReq,
		FbDtsg:   c.configs.BrowserConfigTable.DTSGInitData.Token,
		Jazoest:  c.configs.Jazoest,
		Lsd:      c.configs.LSDToken,
		SpinR:    strconv.Itoa(siteConfig.SpinR),
		SpinB:    siteConfig.SpinB,
		SpinT:    strconv.Itoa(siteConfig.SpinT),
	}
	/*if c.configs.BrowserConfigTable.CurrentUserInitialData.UserID != "0" {
		query.Av = c.configs.BrowserConfigTable.CurrentUserInitialData.UserID
	}*/
	if c.parent.GetPlatform() == types.Instagram {
		query.D = "www"
		query.Jssesw = "1"
	} else {
		query.Aaid = "0"
	}
	return query
}

const MaxHTTPRetries = 5

var (
	ErrTokenInvalidated         = errors.New("access token is no longer valid")
	ErrTokenInvalidatedRedirect = fmt.Errorf("%w: redirected", ErrTokenInvalidated)
	ErrChallengeRequired        = errors.New("challenge required")
	ErrCheckpointRequired       = errors.New("checkpoint required")
	ErrConsentRequired          = errors.New("consent required")
	ErrAccountSuspended         = errors.New("account suspended")
	ErrRequestFailed            = errors.New("failed to send request")
	ErrResponseReadFailed       = errors.New("failed to read response body")
	ErrUnexpectedError          = errors.New("server returned unexpected HTTP status")
	ErrMaxRetriesReached        = errors.New("maximum retries reached")
	ErrTooManyRedirects         = errors.New("too many redirects")
	ErrUserIDIsZero             = fmt.Errorf("%w: user id in initial data is zero", ErrTokenInvalidated)
	ErrVersionIDNotFound        = errors.New("version ID not found")
)

func IsPermanentRequestError(err error) bool {
	return errors.Is(err, ErrTokenInvalidated) ||
		errors.Is(err, ErrChallengeRequired) ||
		errors.Is(err, ErrCheckpointRequired) ||
		errors.Is(err, ErrConsentRequired) ||
		errors.Is(err, ErrAccountSuspended) ||
		errors.Is(err, ErrTooManyRedirects)
}

func (c *HTTPClient) checkHTTPRedirect(req *http.Request, via []*http.Request) error {
	if req.Response == nil {
		return nil
	}
	if len(via) > 5 {
		return ErrTooManyRedirects
	}
	if !strings.HasSuffix(req.URL.Hostname(), "fbcdn.net") && !strings.HasSuffix(req.URL.Hostname(), "facebookcooa4ldbat4g7iacswl3p2zrf5nuylvnhxn6kqolvojixwid.onion") {
		var prevURL string
		if len(via) > 0 {
			prevURL = via[len(via)-1].URL.String()
		}
		c.log.Warn().
			Stringer("url", req.URL).
			Str("prev_url", prevURL).
			Msg("HTTP request was redirected")
	}
	if strings.HasPrefix(req.URL.Path, "/challenge/") {
		return fmt.Errorf("%w: redirected to %s", ErrChallengeRequired, req.URL.String())
	} else if req.URL.Path == "/accounts/suspended/" {
		return fmt.Errorf("%w: redirected to %s", ErrAccountSuspended, req.URL.String())
	} else if req.URL.Path == "/consent/" || strings.HasPrefix(req.URL.Path, "/privacy/consent/") {
		return fmt.Errorf("%w: redirected to %s", ErrConsentRequired, req.URL.String())
	} else if strings.HasPrefix(req.URL.Path, "/checkpoint/") {
		return fmt.Errorf("%w: redirected to %s", ErrCheckpointRequired, req.URL.String())
	}
	respCookies := req.Response.Cookies()
	for _, cookie := range respCookies {
		if (cookie.Name == "xs" || cookie.Name == "sessionid") && cookie.MaxAge < 0 {
			return fmt.Errorf("%w: %s cookie was deleted", ErrTokenInvalidated, cookie.Name)
		}
	}
	if req.URL.Path == "/login.php" {
		return fmt.Errorf("%w to %s", ErrTokenInvalidatedRedirect, req.URL.String())
	}
	return nil
}

func (c *HTTPClient) MakeRequest(
	ctx context.Context,
	url string,
	method string,
	headers http.Header,
	payload []byte,
	contentType types.ContentType,
) (*http.Response, []byte, error) {
	return c.makeRequest(ctx, url, method, headers, payload, contentType, func(e *zerolog.Event) *zerolog.Event {
		return e
	})
}

func (c *HTTPClient) makeRequest(
	ctx context.Context,
	url string,
	method string,
	headers http.Header,
	payload []byte,
	contentType types.ContentType,
	logContext func(e *zerolog.Event) *zerolog.Event,
) (*http.Response, []byte, error) {
	var attempts int
	for {
		attempts++
		start := time.Now()
		resp, respDat, err := c.makeRequestDirect(ctx, url, method, headers, payload, contentType)
		dur := time.Since(start)
		if err == nil {
			logContext(c.log.Debug()).
				Str("url", url).
				Str("method", method).
				Dur("duration", dur).
				Int("status_code", resp.StatusCode).
				Msg("Request successful")
			return resp, respDat, nil
		} else if attempts > MaxHTTPRetries {
			logContext(c.log.Err(err)).
				Str("url", url).
				Str("method", method).
				Dur("duration", dur).
				Msg("Request failed, giving up")
			return nil, nil, fmt.Errorf("%w: %w", ErrMaxRetriesReached, err)
		} else if IsPermanentRequestError(err) || (resp != nil && resp.StatusCode < 500 && resp.StatusCode != 429) || ctx.Err() != nil {
			logContext(c.log.Err(err)).
				Str("url", url).
				Str("method", method).
				Dur("duration", dur).
				Msg("Request failed, cannot be retried")
			return nil, nil, err
		}
		backoff := time.Duration(attempts) * 3 * time.Second
		if resp.StatusCode == 429 {
			backoff *= 2
		}
		logContext(c.log.Err(err)).
			Str("url", url).
			Str("method", method).
			Dur("duration", dur).
			Dur("backoff", backoff).
			Msg("Request failed, retrying")
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
}

func (c *HTTPClient) makeRequestDirect(ctx context.Context, url string, method string, headers http.Header, payload []byte, contentType types.ContentType) (*http.Response, []byte, error) {
	newRequest, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, nil, err
	}

	if contentType != types.NONE {
		headers.Set("content-type", string(contentType))
	}

	newRequest.Header = headers

	response, err := c.HTTP.Do(newRequest)
	defer func() {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
	}()
	if err != nil {
		c.UpdateProxy(fmt.Sprintf("http request error: %v", err.Error()))
		return nil, nil, fmt.Errorf("%w: %w", ErrRequestFailed, err)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return response, nil, fmt.Errorf("%w: %w", ErrResponseReadFailed, err)
	}

	if response.StatusCode >= 400 {
		return response, nil, fmt.Errorf("%w %d", ErrUnexpectedError, response.StatusCode)
	}

	return response, responseBody, nil
}

func (c *HTTPClient) fetchPageData(ctx context.Context, page string) ([]byte, error) {
	headers := c.BuildHeaders(true, true)
	//headers.Set("host", m.client.getEndpoint("host"))
	_, responseBody, err := c.MakeRequest(ctx, page, "GET", headers, nil, types.NONE)
	return responseBody, err
}

func (c *HTTPClient) BuildHeaders(withCookies, isSecFetchDocument bool) http.Header {
	headers := http.Header{}
	headers.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	headers.Set("accept-language", "en-US,en;q=0.9")
	headers.Set("dpr", useragent.DPR)
	headers.Set("user-agent", useragent.UserAgent)
	headers.Set("sec-ch-ua", useragent.SecCHUserAgent)
	headers.Set("sec-ch-ua-platform", useragent.SecCHPlatform)
	headers.Set("sec-ch-prefers-color-scheme", useragent.SecCHPrefersColorScheme)
	headers.Set("sec-ch-ua-full-version-list", useragent.SecCHFullVersionList)
	headers.Set("sec-ch-ua-mobile", useragent.SecCHMobile)
	headers.Set("sec-ch-ua-model", useragent.SecCHModel)
	headers.Set("sec-ch-ua-platform-version", useragent.SecCHPlatformVersion)

	if isSecFetchDocument {
		headers.Set("sec-fetch-dest", "document")
		headers.Set("sec-fetch-mode", "navigate")
		headers.Set("sec-fetch-site", "none") // header is required, otherwise they dont send the csr bitmap data in the response. lets also include the other headers to be sure
		headers.Set("sec-fetch-user", "?1")
		headers.Set("upgrade-insecure-requests", "1")
	} else {
		c.addFacebookHeaders(&headers)
		if !c.parent.GetPlatform().IsMessenger() {
			c.addInstagramHeaders(&headers)
		}
	}

	if c.parent.GetCookies() != nil && withCookies {
		if cookieStr := c.parent.GetCookies().String(); cookieStr != "" {
			headers.Set("cookie", cookieStr)
		}
		w, _ := c.parent.GetCookies().GetViewports()
		headers.Set("viewport-width", w)
		headers.Set("x-asbd-id", "129477")
	}
	return headers
}

func (c *HTTPClient) buildMessengerLiteHeaders() (http.Header, error) {
	analyticsTags, err := MakeRequestAnalyticsHeader()
	if err != nil {
		return nil, err
	}

	// This isn't from a browser, so we don't include most of the usual headers
	headers := http.Header{}
	headers.Set("user-agent", useragent.MessengerLiteUserAgent)
	headers.Set("x-fb-http-engine", "Tigon+iOS")
	headers.Set("accept", "*/*")
	headers.Set("priority", "u=3, i")
	headers.Set("accept-language", "en-US,en;q=0.9")
	headers.Set("x-fb-request-analytics-tags", analyticsTags)

	return headers, nil
}

func (c *HTTPClient) addFacebookHeaders(h *http.Header) {
	if c != nil && c.configs != nil && c.configs.LSDToken != "" {
		h.Set("x-fb-lsd", c.configs.LSDToken)
	}
}

func (c *HTTPClient) addInstagramHeaders(h *http.Header) {
	if c == nil || c.configs == nil {
		return
	}
	if csrfToken := c.parent.GetCookies().Get(cookies.IGCookieCSRFToken); csrfToken != "" {
		h.Set("x-csrftoken", csrfToken)
	}

	if mid := c.parent.GetCookies().Get(cookies.IGCookieMachineID); mid != "" {
		h.Set("x-mid", mid)
	}

	if c.configs.BrowserConfigTable != nil {
		if c.parent.GetCookies().IGWWWClaim != "" {
			h.Set("x-ig-www-claim", c.parent.GetCookies().IGWWWClaim)
		}
		h.Set("x-ig-app-id", c.configs.BrowserConfigTable.CurrentUserInitialData.AppID)
	}
}
