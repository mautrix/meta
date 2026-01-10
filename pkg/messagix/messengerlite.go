package messagix

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	//"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type MessengerLiteMethods struct {
	client *Client
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

func makeRequestAnalyticsHeader() (string, error) {
	anal := RequestAnalytics{
		NetworkTags: NetworkTags{
			Product:         useragent.MessengerLiteAppId,
			RequestCategory: "graphql",
			Purpose:         "fetch",
			RetryAttempt:    "0",
		},
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

func (c *Client) fetchLightspeedKey(ctx context.Context) (*LightspeedKeyResponse, error) {
	endpoint := c.GetEndpoint("pwd_key")

	params := map[string]any{
		"access_token": useragent.MessengerLiteAccessToken,
		"device_id":    c.DeviceID,
		"machine_id":   c.machineId,
		"version":      "3",
	}

	query := url.Values{}
	for key, value := range params {
		query.Set(key, fmt.Sprintf("%v", value)) // Convert `any` to string
	}
	fullURL := endpoint + "?" + query.Encode()

	analHdr, err := makeRequestAnalyticsHeader()
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

func (c *Client) loadMessengerLiteLoginPage(ctx context.Context) (*bloks.BloksBundle, error) {
	if c.machineId != "" {
		randomBytes := make([]byte, 18)
		_, err := rand.Read(randomBytes)
		if err != nil {
			return nil, err
		}
		c.machineId = base64.StdEncoding.EncodeToString(randomBytes)
	}

	// WADevice doesn't exist yet, this should be copied to store.Device.FacebookUUID
	if c.DeviceID == uuid.Nil {
		c.DeviceID = uuid.New()
	}

	return c.makeDoublyWrappedBloksRequest(ctx, bloks.BloksDocLoginHome, map[string]any{
		"is_from_logged_out":                0,
		"flow_source":                       "aymh_single_profile_native_integration_point",
		"offline_experiment_group":          "caa_iteration_v2_perf_ls_ios_test_1",
		"family_device_id":                  c.DeviceID,
		"layered_homepage_experiment_group": "not_in_experiment",
		"is_caa_perf_enabled":               1,
		"waterfall_id":                      "0143cbfa4ec747949d67511836abe901",
		"should_show_logged_in_aymh_ui":     0,
		"INTERNAL_INFRA_screen_id":          "CAA_LOGIN_HOME_PAGE",
		"is_platform_login":                 0,
		"device_id":                         c.DeviceID,
		"left_nav_button_action":            "BACK",
		"is_from_aymh":                      1,
		"access_flow_version":               "F2_FLOW",
		"machine_id":                        c.machineId,
	}, map[string]any{
		"show_internal_settings":           0,
		"lois_settings":                    map[string]any{"lois_token": ""},
		"should_show_nested_nta_from_aymh": 1,
	})
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
	newCookies := &cookies.Cookies{}
	newCookies.UpdateValues(make(map[string]string))
	for _, raw := range payload.SessionCookies {
		newCookies.Set(cookies.MetaCookieName(raw.Name), raw.Value)
	}
	return newCookies
}

func (fb *MessengerLiteMethods) Login(ctx context.Context, username, password string) (*cookies.Cookies, error) {
	// TODO: Extract info from login page
	loginPage, err := fb.client.loadMessengerLiteLoginPage(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading messenger lite login page: %w", err)
	}

	unminifier, err := bloks.GetUnminifier(loginPage)
	if err != nil {
		return nil, err
	}
	loginPage.Unminify(unminifier)

	var loginParams map[string]string
	loginInterp := bloks.NewInterpreter(loginPage, &bloks.InterpBridge{
		DeviceID:  strings.ToUpper(fb.client.DeviceID.String()),
		MachineID: fb.client.machineId,
		EncryptPassword: func(password string) (string, error) {
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
		DoRPC: func(name string, params map[string]string) error {
			if name != "com.bloks.www.bloks.caa.login.async.send_login_request" {
				return fmt.Errorf("got unexpected rpc %s", name)
			}
			loginParams = params
			return nil
		},
	})

	fillTextInput := func(fieldName string, fillText string) error {
		input := loginPage.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			if comp.ComponentID != "bk.components.TextInput" {
				return false
			}
			name, ok := comp.Attributes["html_name"].BloksTreeNodeContent.(*bloks.BloksTreeLiteral)
			if !ok {
				return false
			}
			str, ok := name.BloksJavascriptValue.(string)
			if !ok {
				return false
			}
			return str == fieldName
		})
		if input == nil {
			return fmt.Errorf("couldn't find %s field", fieldName)
		}
		err := input.SetTextContent(fillText)
		if err != nil {
			return err
		}
		onChanged, ok := input.Attributes["on_text_change"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return fmt.Errorf("%s field doesn't have on_text_change script", fieldName)
		}
		_, err = loginInterp.Evaluate(bloks.InterpBindThis(ctx, input), &onChanged.AST)
		if err != nil {
			return fmt.Errorf("%s on_text_changed: %w", fieldName, err)
		}
		return nil
	}

	err = fillTextInput("email", username)
	if err != nil {
		return nil, err
	}
	err = fillTextInput("password", password)
	if err != nil {
		return nil, err
	}

	loginText := loginPage.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
		if comp.ComponentID != "bk.data.TextSpan" {
			return false
		}
		text, ok := comp.Attributes["text"].BloksTreeNodeContent.(*bloks.BloksTreeLiteral)
		if !ok {
			return false
		}
		str, ok := text.BloksJavascriptValue.(string)
		if !ok {
			return false
		}
		return str == "Log in"
	})
	if loginText == nil {
		return nil, fmt.Errorf("couldn't find login button")
	}

	var loginExtension *bloks.BloksTreeComponent
	loginText.FindAncestor(func(comp *bloks.BloksTreeComponent) bool {
		loginExtension = comp.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			return comp.ComponentID == "bk.components.FoaTouchExtension"
		})
		return loginExtension != nil
	})
	if loginExtension == nil {
		return nil, fmt.Errorf("couldn't find login extension")
	}
	onTouchDown, ok := loginExtension.Attributes["on_touch_down"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
	if !ok {
		return nil, fmt.Errorf("login button doesn't have on_touch_down script")
	}
	onTouchUp, ok := loginExtension.Attributes["on_touch_up"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
	if !ok {
		return nil, fmt.Errorf("login button doesn't have on_touch_up script")
	}

	_, err = loginInterp.Evaluate(bloks.InterpBindThis(ctx, loginExtension), &onTouchDown.AST)
	if err != nil {
		return nil, fmt.Errorf("on_touch_down: %w", err)
	}
	_, err = loginInterp.Evaluate(bloks.InterpBindThis(ctx, loginExtension), &onTouchUp.AST)
	if err != nil {
		return nil, fmt.Errorf("on_touch_up: %w", err)
	}

	if loginParams == nil {
		return nil, fmt.Errorf("bloks did not generate login rpc")
	}

	loginResp, err := fb.client.makeSinglyWrappedBloksRequest(
		ctx, bloks.BloksDocSendLogin, loginParams,
	)
	if err != nil {
		return nil, fmt.Errorf("sending bloks login request: %w", err)
	}

	unminifier, err = bloks.GetUnminifier(loginResp)
	if err != nil {
		return nil, err
	}
	loginResp.Unminify(unminifier)

	loginResp.Print("")
	return nil, fmt.Errorf("haven't gotten here yet")

	// newCookies := convertCookies(data)
	// return newCookies, nil
}
