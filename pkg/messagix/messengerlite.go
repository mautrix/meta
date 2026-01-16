package messagix

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"go.mau.fi/util/random"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type MessengerLiteMethods struct {
	client *Client

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
		"device_id":    c.MessengerLite.deviceID,
		"machine_id":   c.MessengerLite.machineID,
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
	newCookies := &cookies.Cookies{Platform: types.MessengerLite}
	newCookies.UpdateValues(make(map[cookies.MetaCookieName]string))
	for _, raw := range payload.SessionCookies {
		newCookies.Set(cookies.MetaCookieName(raw.Name), raw.Value)
	}
	return newCookies
}

func (fb *MessengerLiteMethods) Login(ctx context.Context, username, password string, getMFACode func() (string, error)) (*cookies.Cookies, error) {
	log := fb.client.Logger.With().Str("component", "messenger_lite_login").Logger()
	log.Debug().Msg("Starting Messenger Lite login flow")

	fb.client.MessengerLite.deviceID = uuid.New()
	fb.client.MessengerLite.familyDeviceID = uuid.New()
	fb.client.MessengerLite.machineID = string(random.StringBytes(25))

	makeBridge := func(bri *bloks.InterpBridge) *bloks.InterpBridge {
		bri.DeviceID = strings.ToUpper(fb.client.MessengerLite.deviceID.String())
		bri.FamilyDeviceID = strings.ToUpper(fb.client.MessengerLite.familyDeviceID.String())
		bri.MachineID = fb.client.MessengerLite.machineID
		return bri
	}

	log.Debug().Msg("Requesting redirect to login page")

	doc := &bloks.BloksDocProcessClientDataAndRedirect
	loginRedirectAction, err := fb.client.makeBloksRequest(ctx, doc, bloks.NewBloksRequest(doc, bloks.BloksParamsInner(map[string]any{
		"blocked_uid":                               []any{},
		"offline_experiment_group":                  "caa_iteration_v2_perf_ls_ios_test_1",
		"family_device_id":                          strings.ToUpper(fb.client.MessengerLite.familyDeviceID.String()),
		"use_auto_login_interstitial":               true,
		"layered_homepage_experiment_group":         "not_in_experiment",
		"disable_recursive_auto_login_interstitial": true,
		"show_internal_settings":                    false,
		"waterfall_id":                              hex.EncodeToString(random.Bytes(16)),
		"account_list":                              []any{},
		"disable_auto_login":                        false,
		"is_from_logged_in_switcher":                false,
		"auto_login_interstitial_experiment_group":  "",
		"device_id":                                 strings.ToUpper(fb.client.MessengerLite.deviceID.String()),
		"machine_id":                                fb.client.MessengerLite.machineID,
	})))
	if err != nil {
		return nil, fmt.Errorf("loading messenger lite login page: %w", err)
	}

	log.Debug().Msg("Processing redirect to login page")

	var loginPage *bloks.BloksBundle
	loginRedirectInterp, err := bloks.NewInterpreter(ctx, loginRedirectAction, makeBridge(&bloks.InterpBridge{
		DisplayNewScreen: func(name string, toDisplay *bloks.BloksBundle) error {
			switch name {
			case "com.bloks.www.caa.login.login_homepage":
				loginPage = toDisplay
				return nil
			default:
				return fmt.Errorf("unexpected login screen %q", name)
			}
		},
	}), nil)
	if err != nil {
		return nil, fmt.Errorf("creating login redirect interpreter: %w", err)
	}

	_, err = loginRedirectInterp.Evaluate(ctx, loginRedirectAction.Action())
	if err != nil {
		return nil, fmt.Errorf("running login redirect action: %w", err)
	}
	if loginPage == nil {
		return nil, fmt.Errorf("wasn't redirected to login page")
	}

	log.Debug().Msg("Filling in email and password on login page")

	var loginParams map[string]string
	loginInterp, err := bloks.NewInterpreter(ctx, loginPage, makeBridge(&bloks.InterpBridge{
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
			switch name {
			case "com.bloks.www.bloks.caa.login.async.send_login_request":
				loginParams = params
			default:
				return fmt.Errorf("got unexpected rpc %s from login page", name)
			}
			return nil
		},
	}), loginRedirectInterp)
	if err != nil {
		return nil, fmt.Errorf("creating login interpreter: %w", err)
	}

	err = loginPage.
		FindDescendant(bloks.FilterByAttribute("bk.components.TextInput", "html_name", "email")).
		FillInput(ctx, loginInterp, username)
	if err != nil {
		return nil, fmt.Errorf("filling email input: %w", err)
	}

	err = loginPage.
		FindDescendant(bloks.FilterByAttribute("bk.components.TextInput", "html_name", "password")).
		FillInput(ctx, loginInterp, username)
	if err != nil {
		return nil, fmt.Errorf("filling password input: %w", err)
	}

	err = loginPage.
		FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "Log in")).
		FindContainingButton().
		TapButton(ctx, loginInterp)
	if err != nil {
		return nil, fmt.Errorf("tapping login button: %w", err)
	}

	if loginParams == nil {
		return nil, fmt.Errorf("bloks did not generate login rpc")
	}
	var loginParamsInner bloks.BloksParamsInner
	err = json.Unmarshal([]byte(loginParams["params"]), &loginParamsInner)
	if err != nil {
		return nil, fmt.Errorf("parsing login params: %w", err)
	}

	log.Debug().Msg("Sending login request")

	doc = &bloks.BloksDocSendLoginRequest
	loginResp, err := fb.client.makeBloksRequest(
		ctx, doc, bloks.NewBloksRequest(doc, loginParamsInner),
	)
	if err != nil {
		return nil, fmt.Errorf("sending bloks login request: %w", err)
	}

	log.Debug().Msg("Processing login response")

	var mfaLandingPage *bloks.BloksBundle
	var mfaParams map[string]string
	var loginRespData string
	loginRespInterp, err := bloks.NewInterpreter(ctx, loginResp, makeBridge(&bloks.InterpBridge{
		DoRPC: func(name string, params map[string]string) error {
			switch name {
			case "com.bloks.www.two_step_verification.entrypoint":
				mfaParams = params
				return nil
			default:
				return fmt.Errorf("got unexpected rpc %s from login resp", name)
			}
		},
		DisplayNewScreen: func(name string, toDisplay *bloks.BloksBundle) error {
			switch name {
			case "com.bloks.www.caa.ar.code_entry":
				mfaLandingPage = toDisplay
				return nil
			default:
				return fmt.Errorf("got unexpected page %s from login resp", name)
			}
		},
		HandleLoginResponse: func(data string) error {
			loginRespData = data
			return nil
		},
	}), loginInterp)
	if err != nil {
		return nil, fmt.Errorf("creating login response interpreter: %w", err)
	}

	_, err = loginRespInterp.Evaluate(ctx, &loginResp.Layout.Payload.Action.AST)
	if err != nil {
		return nil, fmt.Errorf("running login response action: %w", err)
	}

	if mfaParams != nil {
		var mfaParamsInner bloks.BloksParamsInner
		err = json.Unmarshal([]byte(mfaParams["params"]), &mfaParamsInner)
		if err != nil {
			return nil, fmt.Errorf("parsing mfa params: %w", err)
		}

		log.Debug().Msg("Requesting MFA entrypoint page")

		doc := &bloks.BloksDocTwoStepVerificationEntrypoint
		mfaLandingPage, err = fb.client.makeBloksRequest(
			ctx, doc, bloks.NewBloksRequest(doc, mfaParamsInner),
		)
		if err != nil {
			return nil, fmt.Errorf("sending bloks mfa entrypoint request: %w", err)
		}
	}
	if mfaLandingPage != nil {
		log.Debug().Msg("Pushing MFA method selection button")

		var mfaMethodsPage *bloks.BloksBundle
		mfaLandingInterp, err := bloks.NewInterpreter(ctx, mfaLandingPage, makeBridge(&bloks.InterpBridge{
			DisplayNewScreen: func(name string, toDisplay *bloks.BloksBundle) error {
				switch name {
				default:
					return fmt.Errorf("unexpected mfa screen %s", name)
				}
				mfaMethodsPage = toDisplay
				return nil
			},
		}), loginRespInterp)
		if err != nil {
			return nil, fmt.Errorf("creating mfa landing page interpreter: %w", err)
		}

		err = mfaLandingPage.
			FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "Try another way")).
			FindContainingButton().
			TapButton(ctx, mfaLandingInterp)
		if err != nil {
			return nil, fmt.Errorf("tapping method selection button: %w", err)
		}

		if mfaMethodsPage == nil {
			return nil, fmt.Errorf("mfa methods screen didn't display")
		}

		// TODO what happens now?

		if true {
			return nil, fmt.Errorf("not implemented yet")
		}
		mfaCodePage := mfaLandingPage // FIXME

		log.Debug().Msg("Filling in MFA code")
		var mfaVerifyParams map[string]string
		mfaInterp, err := bloks.NewInterpreter(ctx, mfaCodePage, makeBridge(&bloks.InterpBridge{
			DoRPC: func(name string, params map[string]string) error {
				switch name {
				case "com.bloks.www.two_step_verification.verify_code.async":
					mfaVerifyParams = params
				default:
					return fmt.Errorf("got unexpected rpc %s from login resp", name)
				}
				return nil
			},
		}), loginRespInterp)
		if err != nil {
			return nil, fmt.Errorf("creating mfa interpreter: %w", err)
		}

		code, err := getMFACode()
		if err != nil {
			return nil, fmt.Errorf("getting mfa code from user: %w", err)
		}

		err = mfaCodePage.
			FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(bloks.FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, mfaInterp, code)
		if err != nil {
			return nil, fmt.Errorf("filling mfa code input: %w", err)
		}

		err = mfaCodePage.
			FindDescendant(bloks.FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, mfaInterp)
		if err != nil {
			return nil, fmt.Errorf("tapping login button: %w", err)
		}

		if mfaVerifyParams == nil {
			return nil, fmt.Errorf("mfa screen didn't trigger verify rpc")
		}

		var mfaVerifyParamsInner bloks.BloksParamsInner
		err = json.Unmarshal([]byte(mfaVerifyParams["params"]), &mfaVerifyParamsInner)
		if err != nil {
			return nil, fmt.Errorf("parsing mfa verification params: %w", err)
		}

		doc = &bloks.BloksDocVerifyCode
		mfaVerified, err := fb.client.makeBloksRequest(
			ctx, doc, bloks.NewBloksRequest(doc, mfaVerifyParamsInner),
		)
		if err != nil {
			return nil, fmt.Errorf("verifying mfa code: %w", err)
		}

		log.Debug().Msg("Handling MFA code response")
		mfaVerifiedInterp, err := bloks.NewInterpreter(ctx, mfaVerified, makeBridge(&bloks.InterpBridge{
			HandleLoginResponse: func(data string) error {
				loginRespData = data
				return nil
			},
		}), mfaInterp)
		if err != nil {
			return nil, fmt.Errorf("creating mfa verification response interpreter: %w", err)
		}
		_, err = mfaVerifiedInterp.Evaluate(ctx, &mfaVerified.Layout.Payload.Action.AST)
		if err != nil {
			return nil, fmt.Errorf("running mfa verification response action: %w", err)
		}
		if loginRespData == "" {
			return nil, fmt.Errorf("mfa verify response didn't trigger callback")
		}
	} else if loginRespData == "" {
		return nil, fmt.Errorf("login response didn't trigger callback")
	}

	log.Debug().Msg("Extracting credentials from login response")

	var loginRespPayload BloksLoginActionResponsePayload
	err = json.Unmarshal([]byte(loginRespData), &loginRespPayload)
	if err != nil {
		return nil, fmt.Errorf("parsing login response data: %w", err)
	}

	newCookies := convertCookies(&loginRespPayload)
	return newCookies, nil
}
