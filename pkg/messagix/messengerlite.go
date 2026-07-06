package messagix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

var ErrTransientTokenLogin = errors.New("login approved but could not be completed automatically, please try logging in again")

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
		EncryptPassword: func(ctx context.Context, password string) (string, error) {
			key, err := fb.client.FetchLightspeedKey(ctx)
			if err != nil {
				return "", fmt.Errorf("fetching lightspeed key for messenger lite: %w", err)
			}

			encryptedPW, err := crypto.EncryptPassword(int(fb.client.Platform), key.KeyID, key.PublicKey, password)
			if err != nil {
				return "", fmt.Errorf("encrypting password for messenger lite: %w", err)
			}
			return encryptedPW, nil
		},
		MakeBloksRequest: fb.client.http.MakeBloksRequest,
	}
}

func (c *Client) FetchLightspeedKey(ctx context.Context) (*LightspeedKeyResponse, error) {
	endpoint := c.GetEndpoint("pwd_key")

	params := map[string]any{
		"access_token": useragent.MessengerLiteAccessToken,
		"device_id":    strings.ToUpper(c.MessengerLite.deviceID.String()),
		"machine_id":   c.MessengerLite.machineID,
		"version":      "3",
	}

	query := url.Values{}
	for key, value := range params {
		query.Set(key, fmt.Sprintf("%v", value)) // Convert `any` to string
	}
	fullURL := endpoint + "?" + query.Encode()

	analyticsTags, err := httpclient.MakeRequestAnalyticsHeader()
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"accept":                      "*/*",
		"x-fb-appid":                  useragent.MessengerLiteAppID,
		"x-fb-request-analytics-tags": analyticsTags,
		"user-agent":                  useragent.MessengerLiteUserAgent,
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

func convertCookies(payload *BloksLoginActionResponsePayload) *cookies.Cookies {
	return cookiesFromRaw(payload.SessionCookies)
}

func cookiesFromRaw(rawCookies []RawCookie) *cookies.Cookies {
	newCookies := &cookies.Cookies{Platform: types.MessengerLite}
	newCookies.UpdateValues(make(map[cookies.MetaCookieName]string))
	for _, raw := range rawCookies {
		newCookies.Set(cookies.MetaCookieName(raw.Name), raw.Value)
	}
	return newCookies
}

type getSessionForAppResponse struct {
	SessionCookies []RawCookie `json:"session_cookies"`
	UID            int64       `json:"uid"`

	// Present when the request fails.
	ErrorCode    int    `json:"error_code"`
	ErrorMsg     string `json:"error_msg"`
	ErrorSubcode int    `json:"error_subcode"`
}

func (c *Client) ExchangeTransientToken(ctx context.Context, transientToken string) (*cookies.Cookies, error) {
	endpoint := c.GetEndpoint("get_session_for_app")

	query := url.Values{}
	query.Set("access_token", transientToken)
	query.Set("new_app_id", useragent.MessengerLiteAppID)
	query.Set("generate_session_cookies", "1")
	query.Set("format", "json")
	fullURL := endpoint + "?" + query.Encode()

	analyticsTags, err := httpclient.MakeRequestAnalyticsHeader()
	if err != nil {
		return nil, err
	}
	httpHeaders := http.Header{}
	for k, v := range map[string]string{
		"accept":                      "*/*",
		"x-fb-appid":                  useragent.MessengerLiteAppID,
		"x-fb-request-analytics-tags": analyticsTags,
		"user-agent":                  useragent.MessengerLiteUserAgent,
		"accept-language":             "en-US,en;q=0.9",
		"request_token":               uuid.New().String(),
	} {
		httpHeaders.Set(k, v)
	}

	_, responseBytes, err := c.http.MakeRequest(ctx, fullURL, "GET", httpHeaders, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("requesting session for transient token: %w", err)
	}

	var response getSessionForAppResponse
	if err = json.Unmarshal(responseBytes, &response); err != nil {
		return nil, fmt.Errorf("parsing session-for-app response: %w", err)
	}
	if response.ErrorCode != 0 {
		return nil, fmt.Errorf("session-for-app error %d/%d: %s", response.ErrorCode, response.ErrorSubcode, response.ErrorMsg)
	}
	if len(response.SessionCookies) == 0 {
		return nil, fmt.Errorf("session-for-app returned no cookies for transient token")
	}
	return cookiesFromRaw(response.SessionCookies), nil
}

// For testing only
func (m *MessengerLiteMethods) SetDeviceIdentifiers(deviceID uuid.UUID) {
	m.deviceID = deviceID
}

func (m *MessengerLiteMethods) DoLoginSteps(ctx context.Context, userInput map[string]string) (*bridgev2.LoginStep, *cookies.Cookies, error) {
	if m.browser == nil {
		m.browser = bloks.NewBrowser(m.getBrowserConfig())

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

	if loginRespPayload.CredentialType == "transient_token" && len(loginRespPayload.SessionCookies) == 0 {
		if loginRespPayload.AccessToken == "" {
			return nil, nil, ErrTransientTokenLogin
		}
		m.client.Logger.Debug().Msg("Got credential_type transient_token, exchanging for session cookies")
		newCookies, err := m.client.ExchangeTransientToken(ctx, loginRespPayload.AccessToken)
		if err != nil {
			m.client.Logger.Warn().Err(err).Msg("Failed to exchange transient token for session cookies")
			return nil, nil, fmt.Errorf("%w: %w", ErrTransientTokenLogin, err)
		}
		return nil, newCookies, nil
	}

	if len(loginRespPayload.SessionCookies) == 0 {
		return nil, nil, fmt.Errorf(
			"messenger-lite login returned no cookies after login",
		)
	}

	return nil, convertCookies(&loginRespPayload), nil
}
