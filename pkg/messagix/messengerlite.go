package messagix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type MessengerLiteLoginState struct {
	DeviceID       string
	FamilyDeviceID string
	MachineID      string
}

type MessengerLiteMethods struct {
	client *Client

	browser *bloks.Browser

	loginState MessengerLiteLoginState
}

func (fb *MessengerLiteMethods) GetLoginMetadata() MessengerLiteLoginState {
	if fb == nil {
		return MessengerLiteLoginState{}
	}
	return fb.loginState
}

type NetworkTags struct {
	Product         string `json:"product"`
	Purpose         string `json:"purpose,omitempty"`
	RequestCategory string `json:"request_category,omitempty"`
	RetryAttempt    string `json:"retry_attempt"`
}

type RequestAnalytics struct {
	NetworkTags NetworkTags `json:"network_tags"`
}

func makeRequestAnalyticsHeader(includeCategory bool) (string, error) {
	anal := RequestAnalytics{
		NetworkTags: NetworkTags{
			Product:      useragent.MessengerLiteAppId,
			RetryAttempt: "0",
		},
	}
	if includeCategory {
		anal.NetworkTags.RequestCategory = "graphql"
		anal.NetworkTags.Purpose = "fetch"
	}
	hdr, err := json.Marshal(anal)
	if err != nil {
		return "", fmt.Errorf("make analytics header: %w", err)
	}
	return string(hdr), nil
}

type LightspeedKeyResponse struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
}

func (fb *MessengerLiteMethods) getBrowserConfig() *bloks.BrowserConfig {
	return &bloks.BrowserConfig{
		EncryptPassword: func(ctx context.Context, password string) (string, error) {
			key, err := fb.client.fetchLightspeedKey(ctx)
			if err != nil {
				return "", fmt.Errorf("fetching lightspeed key for messenger lite: %w", err)
			}

			encryptedPW, err := crypto.EncryptPassword(int(fb.client.Platform), key.KeyID, key.PublicKey, password)
			if err != nil {
				return "", fmt.Errorf("encrypting password for messenger lite: %w", err)
			}
			return encryptedPW, nil
		},
		MakeBloksRequest: fb.client.MakeBloksRequest,
	}
}

func (c *Client) fetchLightspeedKey(ctx context.Context) (*LightspeedKeyResponse, error) {
	endpoint := c.GetEndpoint("pwd_key")

	loginMeta := c.MessengerLite.GetLoginMetadata()

	params := map[string]any{
		"access_token": useragent.MessengerLiteAccessToken,
		"device_id":    loginMeta.DeviceID,
		"machine_id":   loginMeta.MachineID,
		"version":      "3",
	}

	query := url.Values{}
	for key, value := range params {
		query.Set(key, fmt.Sprintf("%v", value)) // Convert `any` to string
	}
	fullURL := endpoint + "?" + query.Encode()

	analHdr, err := makeRequestAnalyticsHeader(true)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"accept":                      "*/*",
		"x-fb-appid":                  useragent.MessengerLiteAppId,
		"x-fb-request-analytics-tags": analHdr,
		"user-agent":                  useragent.MessengerLiteUserAgent,
		"accept-language":             "en-US,en;q=0.9",
		"request_token":               uuid.New().String(),
	}

	httpHeaders := http.Header{}
	for k, v := range headers {
		httpHeaders.Set(k, v)
	}

	_, responseBytes, err := c.MakeRequest(ctx, fullURL, "GET", httpHeaders, nil, types.NONE)
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

func (payload *BloksLoginActionResponsePayload) GetCookies() *cookies.Cookies {
	newCookies := &cookies.Cookies{Platform: types.MessengerLite}
	newCookies.UpdateValues(make(map[cookies.MetaCookieName]string))
	for _, raw := range payload.SessionCookies {
		newCookies.Set(cookies.MetaCookieName(raw.Name), raw.Value)
	}
	return newCookies
}

func (m *MessengerLiteMethods) DoLoginSteps(ctx context.Context, userInput map[string]string) (*bridgev2.LoginStep, *BloksLoginActionResponsePayload, error) {
	if m.browser == nil {
		m.browser = bloks.NewBrowser(m.getBrowserConfig())

		m.loginState = MessengerLiteLoginState{
			DeviceID:       m.browser.Bridge.DeviceID,
			FamilyDeviceID: m.browser.Bridge.FamilyDeviceID,
			MachineID:      m.browser.Bridge.MachineID,
		}
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

	return nil, &loginRespPayload, nil
}

type apnsEncryptionParams struct {
	Algorithm string `json:"algorithm"`
	Key       []byte `json:"key"`
	KeyID     string `json:"key_id"`
}

type apnsExtraData struct {
	IPhoneSettingMask int `json:"iphone_setting_mask"`
}

type apnsProtocolParams struct {
	DeviceID               string               `json:"device_id"`
	Encryption             apnsEncryptionParams `json:"encryption"`
	ExtraData              apnsExtraData        `json:"extra_data"`
	FamilyDeviceID         string               `json:"family_device_id"`
	IOSIdentifierForVendor string               `json:"ios_identifier_for_vendor"`
	IsNonPhone             bool                 `json:"is_non_phone"`
	Token                  string               `json:"token"`
	URL                    string               `json:"url"`
}

func (m *MessengerLiteMethods) RegisterPushNotifications(ctx context.Context, endpoint string, keys PushKeys, loginMeta metaid.MessengerLiteLoginMetadata) error {
	protocol := apnsProtocolParams{
		DeviceID: loginMeta.DeviceID,
		Encryption: apnsEncryptionParams{
			Algorithm: "AES_GCM",
			Key:       []byte(keys.P256DH),
			KeyID:     fmt.Sprintf("%s", time.Now().Unix()),
		},
		ExtraData: apnsExtraData{
			IPhoneSettingMask: 640,
		},
		FamilyDeviceID:         loginMeta.FamilyDeviceID,
		IOSIdentifierForVendor: strings.ToUpper(uuid.New().String()),
		IsNonPhone:             false,
		Token:                  endpoint,
		URL:                    "http://push.apple.com/pushkit/voip",
	}

	protocolB, err := json.Marshal(protocol)
	if err != nil {
		return fmt.Errorf("marshal apns protocl params: %w", err)
	}

	form := url.Values{}
	form.Add("access_token", loginMeta.AccessToken)
	form.Add("protocol_params", string(protocolB))

	analHdr, err := makeRequestAnalyticsHeader(false)
	if err != nil {
		return err
	}
	headers := map[string]string{
		"accept":                      "*/*",
		"x-fb-http-engine":            "NSURL",
		"x-fb-request-analytics-tags": analHdr,
		"request_token":               strings.ToUpper(uuid.New().String()),
		"accept-language":             "en-US,en;q=0.9",
		"user-agent":                  useragent.MessengerLiteUserAgent,
		"x-fb-appid":                  useragent.MessengerLiteAppId,
	}

	httpHeaders := http.Header{}
	for k, v := range headers {
		httpHeaders.Set(k, v)
	}

	resp, _, err := m.client.MakeRequest(ctx, "https://graph.facebook.com/v2.10/me/register_push_tokens", "POST", httpHeaders, []byte(form.Encode()), types.FORM)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad http status on push registration: %d", resp.StatusCode)
	}

	return nil
}
