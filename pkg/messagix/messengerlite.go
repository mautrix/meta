package messagix

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"

	//"go.mau.fi/mautrix-meta/pkg/messagix"
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

func (c *Client) fetchLightspeedKey(ctx context.Context) (int, string, error) {
	// pwd_key
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
		return 0, "", err
	}

	headers := map[string]string{
		"accept":                      "*/*",
		"x-fb-appid":                  useragent.MessengerLiteAppId,
		"x-fb-request-analytics-tags": analHdr,
		"user-agent":                  useragent.UserAgent,
		"accept-language":             "en-US,en;q=0.9",
		"request_token":               uuid.New().String(),
	}

	httpHeaders := http.Header{}
	for k, v := range headers {
		httpHeaders.Set(k, v)
	}

	_, responseBytes, err := c.MakeRequest(ctx, fullURL, "GET", httpHeaders, nil, types.NONE)
	if err != nil {
		return 0, "", err
	}

	var response map[string]any
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		return 0, "", err
	}

	key_id := response["key_id"].(float64)
	public_key := response["public_key"].(string)
	return int(key_id), public_key, nil
}

func (c *Client) loadMessengerLiteLoginPage(ctx context.Context) error {
	if c.machineId != "" {
		randomBytes := make([]byte, 18)
		_, err := rand.Read(randomBytes)
		if err != nil {
			return err
		}
		c.machineId = base64.StdEncoding.EncodeToString(randomBytes)
	}

	// WADevice doesn't exist yet, this should be copied to store.Device.FacebookUUID
	if c.DeviceID == uuid.Nil {
		c.DeviceID = uuid.New()
	}

	_, err := c.makeWrappedBloksRequest(ctx, "CAA_LOGIN_HOME_PAGE", map[string]any{
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

	return err
}

func (c *Client) sendAsyncLoginRequest(ctx context.Context, username, password string) (*BloksLoginActionResponsePayload, error) {
	deviceId := strings.ToUpper(c.DeviceID.String())
	payload, err := c.makeWrappedBloksRequest(ctx, "CAA_LOGIN_ASYNC_SEND_LOGIN_REQUEST", map[string]any{
		"family_device_id": deviceId,
		"is_from_aymh":     0,
		"should_trigger_override_login_success_action": 0,
		"INTERNAL__latency_qpl_marker_id":              36707139,
		"is_from_assistive_id":                         0,
		"login_source":                                 "Login",
		"is_from_logged_in_switcher":                   0,
		"should_trigger_override_login_2fa_action":     0,
		"is_from_landing_page":                         0,
		"device_id":                                    deviceId,
		"INTERNAL__latency_qpl_instance_id":            44728029400434,
		"is_platform_login":                            0,
		"is_from_msplit_fallback":                      0,
		"ar_event_source":                              "login_home_page",
		"is_from_password_entry_page":                  0,
		"waterfall_id":                                 "0143cbfa4ec747949d67511836abe901",
		"access_flow_version":                          "pre_mt_behavior",
		"credential_type":                              "password",
		"username_text_input_id":                       "7earom:118",
		"password_text_input_id":                       "7earom:119",
		"is_from_logged_out":                           0,
		"caller":                                       "gslr",
		"server_login_source":                          "login",
		"layered_homepage_experiment_group":            "not_in_experiment",
		"reg_flow_source":                              "aymh_single_profile_native_integration_point",
		"is_caa_perf_enabled":                          1,
		"is_vanilla_password_page_empty_password":      0,
		"is_from_empty_password":                       0,
		"offline_experiment_group":                     "caa_iteration_v2_perf_ls_ios_test_1",
		"login_credential_type":                        "none",
		"two_step_login_type":                          "one_step_login",
	}, map[string]any{
		"try_num":                               1,
		"block_store_machine_id":                "",
		"contact_point":                         username,
		"has_granted_read_phone_permissions":    0,
		"lois_settings":                         map[string]any{"lois_token": ""},
		"event_step":                            "home_page",
		"event_flow":                            "login_manual",
		"should_show_nested_nta_from_aymh":      1,
		"cloud_trust_token":                     nil,
		"has_granted_read_contacts_permissions": 0,
		"device_id":                             deviceId,
		"sso_token_map_json_string":             `{"": []}`,
		"auth_secure_device_id":                 "",
		"openid_tokens":                         map[string]any{},
		"login_attempt_count":                   1,
		"app_manager_id":                        "",
		"has_whatsapp_installed":                0,
		"aymh_accounts": []any{
			map[string]any{
				"id":       "",
				"profiles": map[string]any{},
			},
		},
		"client_known_key_hash":       "",
		"fb_ig_device_id":             []string{},
		"encrypted_msisdn":            "",
		"secure_family_device_id":     "",
		"machine_id":                  c.machineId,
		"password":                    password,
		"password_contains_non_ascii": "false",
		"accounts_list": []any{
			map[string]any{
				"metadata":        map[string]any{},
				"uid":             "",
				"credential_type": "none",
				"token":           "none",
			},
		},
		"headers_infra_flow_id": "",
		"family_device_id":      c.DeviceID,
	})
	if err != nil {
		return nil, err
	}

	c.Logger.Debug().Any("actionPayload", payload.Action).Msg("Processed Messenger Lite login response")

	payloadData, err := c.parseBloksActionPayload(payload.Action)
	if err != nil {
		return nil, err
	}

	c.Logger.Debug().Any("payloadData", payloadData).Msg("Parsed Bloks action payload")

	return payloadData, nil
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

func (c *Client) parseBloksActionPayload(actionPayload string) (*BloksLoginActionResponsePayload, error) {
	re := regexp.MustCompile(`\{\\.*?\}"`)
	match := re.FindString(actionPayload)

	if match == "" {
		return nil, fmt.Errorf("no json found in action string")
	}

	match = strings.TrimSuffix(match, `"`)
	// Unescape double backslashes to single ones
	unescaped := strings.ReplaceAll(match, `\\`, `\`)
	unescaped = strings.ReplaceAll(unescaped, `\"`, `"`)

	// Decode into BloksLoginActionResponsePayload
	var data BloksLoginActionResponsePayload
	err := json.Unmarshal([]byte(unescaped), &data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return &data, nil
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
	fb.client.Logger.Debug().Msg("Loading Messenger Lite login page")
	err := fb.client.loadMessengerLiteLoginPage(ctx)
	if err != nil {
		return nil, err
	}

	fb.client.Logger.Debug().Msg("Fetching Lightspeed key for Messenger Lite")
	keyId, pubKey, err := fb.client.fetchLightspeedKey(ctx)
	if err != nil {
		return nil, err
	}

	fb.client.Logger.Debug().Msg("Encrypting password for Messenger Lite")
	encryptedPW, err := crypto.EncryptPassword(int(fb.client.Platform), keyId, pubKey, password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password for facebook: %w", err)
	}

	data, err := fb.client.sendAsyncLoginRequest(ctx, username, encryptedPW)
	if err != nil {
		return nil, err
	}

	newCookies := convertCookies(data)

	fb.client.Logger.Debug().Any("data", data).Msg("Processed Messenger Lite login response")

	return newCookies, nil
}
