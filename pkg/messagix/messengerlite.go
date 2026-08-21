package messagix

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type MessengerLiteMethods struct {
	client *Client

	browser *bloks.Browser

	deviceID       uuid.UUID
	familyDeviceID uuid.UUID
	machineID      string
}

func (fb *MessengerLiteMethods) GetSuggestedDeviceID() uuid.UUID {
	if fb == nil {
		return uuid.Nil
	}
	return fb.deviceID
}

type LightspeedKeyResponse struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
}

func (fb *MessengerLiteMethods) getBrowserConfig() *bloks.BrowserConfig {
	return &bloks.BrowserConfig{
		Platform: fb.client.Platform,
		EncryptPassword: func(ctx context.Context, password string) (string, error) {
			key, err := fb.client.FetchLightspeedKey(ctx)
			if err != nil {
				return "", fmt.Errorf("fetching lightspeed key for messenger lite: %w", err)
			}

			encryptedPW, err := crypto.EncryptPassword(fb.client.Platform, key.KeyID, key.PublicKey, password)
			if err != nil {
				return "", fmt.Errorf("encrypting password for messenger lite: %w", err)
			}
			return encryptedPW, nil
		},
		MakeBloksRequest: fb.client.http.MakeBloksRequest,
		FetchAsset:       fb.fetchAsset,
	}
}

// fetch arbitrary assets (CAPTCHA text/audio) using the same HTTP client as the main Client instance
func (fb *MessengerLiteMethods) fetchAsset(ctx context.Context, url string) ([]byte, string, error) {
	app, err := fb.client.getMessengerLiteApp()
	if err != nil {
		return nil, "", err
	}
	headers := http.Header{}
	headers.Set("accept", "*/*")
	headers.Set("accept-language", "en-US,en;q=0.9")
	headers.Set("user-agent", app.UserAgent)

	resp, body, err := fb.client.http.MakeRequest(ctx, url, http.MethodGet, headers, nil, types.NONE)
	if err != nil {
		return nil, "", err
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// messengerLiteApp is the first-party app that we pretend to be when making
// non-Bloks messenger lite requests.
type messengerLiteApp struct {
	AppID       string
	AccessToken string
	UserAgent   string
}

func (c *Client) getMessengerLiteApp() (*messengerLiteApp, error) {
	switch c.Platform {
	case types.MessengerLiteIOS:
		return &messengerLiteApp{
			AppID:       useragent.MessengerLiteIOSAppID,
			AccessToken: useragent.MessengerLiteIOSAccessToken,
			UserAgent:   useragent.MessengerLiteIOSUserAgent,
		}, nil
	case types.MessengerLiteAndroid:
		return &messengerLiteApp{
			AppID:       useragent.MessengerLiteAndroidAppID,
			AccessToken: useragent.MessengerLiteAndroidAccessToken,
			UserAgent:   useragent.MessengerLiteAndroidUserAgent,
		}, nil
	default:
		return nil, fmt.Errorf("platform %s does not support messenger lite auth requests", c.Platform.String())
	}
}

func (c *Client) FetchLightspeedKey(ctx context.Context) (*LightspeedKeyResponse, error) {
	endpoint := c.GetEndpoint("pwd_key")

	app, err := c.getMessengerLiteApp()
	if err != nil {
		return nil, err
	}
	params := map[string]any{
		"access_token": app.AccessToken,
		"device_id":    strings.ToUpper(c.MessengerLite.deviceID.String()),
		"machine_id":   c.MessengerLite.machineID,
		"version":      "3",
	}

	query := url.Values{}
	for key, value := range params {
		query.Set(key, fmt.Sprintf("%v", value)) // Convert `any` to string
	}
	fullURL := endpoint + "?" + query.Encode()

	analyticsTags, err := httpclient.MakeRequestAnalyticsHeader(app.AppID)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"accept":                      "*/*",
		"x-fb-appid":                  app.AppID,
		"x-fb-request-analytics-tags": analyticsTags,
		"user-agent":                  app.UserAgent,
		"accept-language":             "en-US,en;q=0.9",
		"request_token":               uuid.New().String(),
	}

	httpHeaders := http.Header{}
	for k, v := range headers {
		httpHeaders.Set(k, v)
	}

	_, responseBytes, err := c.http.MakeRequest(ctx, fullURL, "GET", httpHeaders, nil, types.NONE)
	if err != nil {
		return nil, err
	}

	var response LightspeedKeyResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

type RawCookie struct {
	Domain           string `json:"domain"`
	Expires          string `json:"expires"`
	ExpiresTimestamp int64  `json:"expires_timestamp"`
	HttpOnly         *bool  `json:"httponly"` // nullable
	Name             string `json:"name"`
	Path             string `json:"path"`
	SameSite         string `json:"samesite"`
	Secure           bool   `json:"secure"`
	Value            string `json:"value"`
}

type BloksLoginActionResponsePayload struct {
	AccessToken                   string `json:"access_token"`
	AnalyticsClaim                string `json:"analytics_claim"`
	AutoLoginSSO                  bool   `json:"auto_login_sso"`
	Confirmed                     bool   `json:"confirmed"`
	CredentialType                string `json:"credential_type"`
	HasEncryptedBackup            bool   `json:"has_encrypted_backup"`
	Identifier                    string `json:"identifier"`
	IsAccountConfirmed            bool   `json:"is_account_confirmed"`
	IsAymhSurveyEligible          bool   `json:"is_aymh_survey_eligible"`
	IsFBOnlyNotAllowedInMessenger bool   `json:"is_fb_only_not_allowed_in_msgr"`
	IsGamingConsented             bool   `json:"is_gaming_consented"`
	IsLisaSSOLogin                bool   `json:"is_lisa_sso_login"`
	IsMarketplaceConsented        bool   `json:"is_marketplace_consented"`
	IsMSplitAccount               bool   `json:"is_msplit_account"`
	IsSpectraAccount              bool   `json:"is_spectra_account"`
	MachineID                     string `json:"machine_id"`
	RefreshNonce                  bool   `json:"refresh_nonce"`
	Secret                        string `json:"secret"`
	SessionKey                    string `json:"session_key"`
	UID                           int64  `json:"uid"`
	UserStorageKey                string `json:"user_storage_key"`

	SessionCookies []RawCookie `json:"session_cookies"`
}

func (m *MessengerLiteMethods) convertCookies(rawCookies []RawCookie) *cookies.Cookies {
	newCookies := &cookies.Cookies{Platform: m.client.Platform}
	newCookies.UpdateValues(make(map[cookies.MetaCookieName]string))
	for _, raw := range rawCookies {
		newCookies.Set(cookies.MetaCookieName(raw.Name), raw.Value)
	}
	return newCookies
}

// SessionForAppOptions overrides the parameters of the create_session_for_app
// request. The zero value makes the same request the official app does.
type SessionForAppOptions struct {
	Endpoint string
	NewAppID string
	AppAuth  bool
}

type SessionForAppResponse struct {
	AccessToken    string      `json:"access_token"`
	SessionKey     string      `json:"session_key"`
	Secret         string      `json:"secret"`
	MachineID      string      `json:"machine_id"`
	AnalyticsClaim string      `json:"analytics_claim"`
	UID            json.Number `json:"uid"`
	SessionCookies []RawCookie `json:"session_cookies"`

	// The legacy REST API returns errors in the body with a 200 status
	ErrorCode    int    `json:"error_code"`
	ErrorSubcode int    `json:"error_subcode"`
	ErrorMsg     string `json:"error_msg"`
	// ...while the graph API uses the usual graph error object
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
		Subcode int    `json:"error_subcode"`
	} `json:"error"`
}

func (resp *SessionForAppResponse) Err() error {
	if resp.Error != nil {
		return fmt.Errorf("session-for-app error %d/%d: %s", resp.Error.Code, resp.Error.Subcode, resp.Error.Message)
	} else if resp.ErrorCode != 0 {
		return fmt.Errorf("session-for-app error %d/%d: %s", resp.ErrorCode, resp.ErrorSubcode, resp.ErrorMsg)
	}
	return nil
}

func (m *MessengerLiteMethods) GetSessionForApp(ctx context.Context, accessToken string, opts SessionForAppOptions) (*SessionForAppResponse, error) {
	app, err := m.client.getMessengerLiteApp()
	if err != nil {
		return nil, err
	}

	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = m.client.GetEndpoint("session_for_app")
	}
	newAppID := opts.NewAppID
	if newAppID == "" {
		newAppID = app.AppID
	}

	params := url.Values{}
	params.Set("format", "json")
	params.Set("access_token", accessToken)
	params.Set("new_app_id", newAppID)
	params.Set("generate_session_cookies", "1")
	params.Set("generate_analytics_claim", "1")
	if m.deviceID != uuid.Nil {
		params.Set("device_id", strings.ToUpper(m.deviceID.String()))
	}
	if m.machineID != "" {
		params.Set("machine_id", m.machineID)
	} else {
		params.Set("generate_machine_id", "1")
	}

	analyticsTags, err := httpclient.MakeRequestAnalyticsHeader(app.AppID)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	for k, v := range map[string]string{
		"accept":                      "*/*",
		"x-fb-appid":                  app.AppID,
		"x-fb-request-analytics-tags": analyticsTags,
		"user-agent":                  app.UserAgent,
		"accept-language":             "en-US,en;q=0.9",
		"request_token":               uuid.New().String(),
	} {
		headers.Set(k, v)
	}
	if opts.AppAuth {
		headers.Set("authorization", "OAuth "+app.AccessToken)
	}

	// The legacy REST host answers with a 200 and the error in the body, while
	// the graph hosts use a normal HTTP error status, so parse the body first
	// either way and only fall back to the HTTP error if it isn't usable.
	_, responseBytes, reqErr := m.client.http.MakeRequest(ctx, endpoint, "POST", headers, []byte(params.Encode()), types.FORM)

	var response SessionForAppResponse
	err = json.Unmarshal(responseBytes, &response)
	switch {
	case err == nil && response.Err() != nil:
		return nil, response.Err()
	case reqErr != nil:
		return nil, fmt.Errorf("requesting session for app: %w", reqErr)
	case err != nil:
		return nil, fmt.Errorf("parsing session-for-app response: %w", err)
	}
	return &response, nil
}

func (m *MessengerLiteMethods) ExchangeTransientToken(ctx context.Context, accessToken string) (*cookies.Cookies, error) {
	resp, err := m.GetSessionForApp(ctx, accessToken, SessionForAppOptions{})
	if err != nil {
		return nil, err
	}
	newCookies := m.convertCookies(resp.SessionCookies)
	if !newCookies.IsLoggedIn() {
		return nil, fmt.Errorf("session-for-app response is missing cookies: %v", newCookies.GetMissingCookieNames())
	}
	return newCookies, nil
}

// For testing only
func (m *MessengerLiteMethods) SetDeviceIdentifiers(deviceID uuid.UUID) {
	m.deviceID = deviceID
}

// For testing only
func (m *MessengerLiteMethods) SetMachineID(machineID string) {
	m.machineID = machineID
}

func (m *MessengerLiteMethods) DoLoginSteps(ctx context.Context, userInput map[string]string) (*bridgev2.LoginStep, *cookies.Cookies, error) {
	if m.browser == nil {
		b, err := bloks.NewBrowser(m.getBrowserConfig())
		if err != nil {
			return nil, nil, fmt.Errorf("construct browser: %w", err)
		}
		m.browser = b

		// these values are generated by us, they are known safe
		m.deviceID = uuid.MustParse(m.browser.Bridge.DeviceID)
		m.familyDeviceID = uuid.MustParse(m.browser.Bridge.FamilyDeviceID)
		m.machineID = m.browser.Bridge.MachineID
	}

	for m.browser.State != bloks.StateSuccess {
		step, err := m.browser.DoLoginStep(ctx, userInput)
		if err != nil {
			return nil, nil, err
		}
		if step != nil {
			if step.UserInputParams != nil {
				inputs := []string{}
				for _, input := range step.UserInputParams.Fields {
					inputs = append(inputs, input.ID)
				}
				m.client.Logger.Debug().Strs("inputs", inputs).Msg("Requesting user input")
			}
			return step, nil, nil
		}
	}

	var loginRespPayload BloksLoginActionResponsePayload
	err := json.Unmarshal([]byte(m.browser.LoginData), &loginRespPayload)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing login response data: %w", err)
	}

	if len(loginRespPayload.SessionCookies) == 0 {
		if loginRespPayload.AccessToken == "" {
			return nil, nil, loginerrors.MissingCookies.AppendMessage(". Meta returned incomplete credentials after login. If you don't have MFA enabled, please turn it on for your Facebook account. Otherwise, it may help to try again, log in from the official app/website first, or change the MFA settings for your Facebook account")
		}
		m.client.Logger.Debug().
			Str("credential_type", loginRespPayload.CredentialType).
			Msg("Login response didn't include session cookies, exchanging access token for a session")
		newCookies, err := m.ExchangeTransientToken(ctx, loginRespPayload.AccessToken)
		if err != nil {
			m.client.Logger.Warn().Err(err).
				Str("credential_type", loginRespPayload.CredentialType).
				Msg("Failed to exchange access token for session cookies")
			return nil, nil, loginerrors.TokenExchange
		}
		m.client.Logger.Debug().
			Str("credential_type", loginRespPayload.CredentialType).
			Any("cookie_names", slices.Collect(maps.Keys(newCookies.GetAll()))).
			Msg("Exchanged access token for session cookies")
		return nil, newCookies, nil
	}

	return nil, m.convertCookies(loginRespPayload.SessionCookies), nil
}
