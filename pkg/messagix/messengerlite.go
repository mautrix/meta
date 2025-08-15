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
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type MessengerLiteMethods struct {
	client *Client
}

func (c *Client) fetchLightspeedKey(ctx context.Context) (int, string, error) {
	// pwd_key
	endpoint := c.getEndpoint("pwd_key")

	params := map[string]any{
		"access_token":  MessengerLiteAccessToken,
		"device_id":     c.DeviceID,
		"machine_id":    c.machineId,
		"version":       "3",
	}

	query := url.Values{}
	for key, value := range params {
		query.Set(key, fmt.Sprintf("%v", value)) // Convert `any` to string
	}
	fullURL := endpoint + "?" + query.Encode()

	headers := map[string]string{
		"accept": "*/*",
		// TODO: Analytics
		"x-fb-appid": MessengerLiteAppId,
		"user-agent": UserAgent,
		"accept-language": "en-US,en;q=0.9",
		"request_token": uuid.New().String(),
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

	_ , respBytes, err := c.makeWrappedBloksRequest(ctx, "CAA_LOGIN_HOME_PAGE", map[string]any{
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

	c.Logger.Debug().Any("respBytes", respBytes).Msg("Loaded Messenger Lite login page")
	return err
}

func (c *Client) sendAsyncLoginRequest(ctx context.Context, username, password string) (*http.Response, []byte, error) {
	deviceId := strings.ToUpper(c.DeviceID.String())
	return c.makeWrappedBloksRequest(ctx, "CAA_LOGIN_ASYNC_SEND_LOGIN_REQUEST", map[string]any{
		"family_device_id": deviceId,
        "is_from_aymh": 0,
        "should_trigger_override_login_success_action": 0,
        "INTERNAL__latency_qpl_marker_id": 36707139,
        "is_from_assistive_id": 0,
        "login_source": "Login",
        "is_from_logged_in_switcher": 0,
        "should_trigger_override_login_2fa_action": 0,
        "is_from_landing_page": 0,
        "device_id": deviceId,
        "INTERNAL__latency_qpl_instance_id": 44728029400434,
        "is_platform_login": 0,
        "is_from_msplit_fallback": 0,
        "ar_event_source": "login_home_page",
        "is_from_password_entry_page": 0,
        "waterfall_id": "0143cbfa4ec747949d67511836abe901",
        "access_flow_version": "pre_mt_behavior",
        "credential_type": "password",
        "username_text_input_id": "7earom:118",
        "password_text_input_id": "7earom:119",
        "is_from_logged_out": 0,
        "caller": "gslr",
        "server_login_source": "login",
        "layered_homepage_experiment_group": "not_in_experiment",
        "reg_flow_source": "aymh_single_profile_native_integration_point",
        "is_caa_perf_enabled": 1,
        "is_vanilla_password_page_empty_password": 0,
        "is_from_empty_password": 0,
        "offline_experiment_group": "caa_iteration_v2_perf_ls_ios_test_1",
        "login_credential_type": "none",
        "two_step_login_type": "one_step_login",
	}, map[string]any{
		"try_num": 1,
        "block_store_machine_id": "",
        "contact_point": username,
        "has_granted_read_phone_permissions": 0,
        "lois_settings": map[string]any{"lois_token": ""},
        "event_step": "home_page",
        "event_flow": "login_manual",
        "should_show_nested_nta_from_aymh": 1,
        "cloud_trust_token": nil,
        "has_granted_read_contacts_permissions": 0,
        "device_id": deviceId,
        "sso_token_map_json_string": `{"": []}`,
        "auth_secure_device_id": "",
        "openid_tokens": map[string]any{},
        "login_attempt_count": 1,
        "app_manager_id": "",
        "has_whatsapp_installed": 0,
		"aymh_accounts": []any{
			map[string]any{
				"id":       "",
				"profiles": map[string]any{},
			},
		},
        "client_known_key_hash": "",
        "fb_ig_device_id": []string{},
        "encrypted_msisdn": "",
        "secure_family_device_id": "",
        "machine_id": c.machineId,
        "password": password,
        "password_contains_non_ascii": "false",
		"accounts_list": []any{
			map[string]any{
				"metadata":     map[string]any{},
				"uid":          "",
				"credential_type": "none",
				"token":       "none",
			},
		},
        "headers_infra_flow_id": "",
        "family_device_id": c.DeviceID,
	})
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

	resp, respBytes, err := fb.client.sendAsyncLoginRequest(ctx, username, encryptedPW)
	if err != nil {
		return nil, err
	}

	fb.client.Logger.Debug().Any("resp", resp).Any("respBytes", string(respBytes)).Msg("Processing Messenger Lite login response")

	return nil, nil
	// moduleLoader := fb.client.loadLoginPage(ctx)
	// loginFormTags := moduleLoader.FormTags[0]
	// loginPath, ok := loginFormTags.Attributes["action"]
	// if !ok {
	// 	return nil, fmt.Errorf("failed to resolve login path / action from html form tags for facebook login")
	// }

	// loginInputs := append(loginFormTags.Inputs, moduleLoader.LoginInputs...)
	// loginForm := types.LoginForm{}
	// v := reflect.ValueOf(&loginForm).Elem()
	// fb.client.configs.ParseFormInputs(loginInputs, v)

	// fb.client.configs.Jazoest = loginForm.Jazoest

	// needsCookieConsent := len(fb.client.configs.BrowserConfigTable.InitialCookieConsent.InitialConsent) == 0
	// if needsCookieConsent {
	// 	err := fb.client.sendCookieConsent(ctx, moduleLoader.JSDatr)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	// testDataSimulator := crypto.NewABTestData()
	// data := testDataSimulator.GenerateAbTestData([]string{identifier, password})

	// encryptedPW, err := crypto.EncryptPassword(int(types.Facebook), crypto.FacebookPubKeyId, crypto.FacebookPubKey, password)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to encrypt password for facebook: %w", err)
	// }

	// loginForm.Email = identifier
	// loginForm.EncPass = encryptedPW
	// loginForm.AbTestData = data
	// loginForm.Lgndim = "eyJ3IjoyMjc1LCJoIjoxMjgwLCJhdyI6MjI3NiwiYWgiOjEyMzIsImMiOjI0fQ==" // irrelevant
	// loginForm.Lgnjs = strconv.Itoa(fb.client.configs.BrowserConfigTable.SiteData.SpinT)
	// loginForm.Timezone = "-120"

	// form, err := query.Values(&loginForm)
	// if err != nil {
	// 	return nil, err
	// }

	// loginUrl := fb.client.getEndpoint("base_url") + loginPath
	// loginResp, loginBody, err := fb.client.sendLoginRequest(ctx, form, loginUrl)
	// if err != nil {
	// 	return nil, err
	// }

	// loginResult := fb.client.processLogin(loginResp, loginBody)
	// if loginResult != nil {
	// 	return nil, loginResult
	// }

	// return fb.client.cookies, nil
}
