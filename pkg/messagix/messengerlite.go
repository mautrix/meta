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
	log := fb.client.Logger
	log.Debug().Msg("Starting Messenger Lite login flow")

	fb.client.MessengerLite.deviceID = uuid.New()
	fb.client.MessengerLite.familyDeviceID = uuid.New()
	fb.client.MessengerLite.machineID = string(random.StringBytes(25))

	doc := &bloks.BloksDocProcessClientDataAndRedirect
	loginPage, err := fb.client.makeBloksRequest(ctx, doc, bloks.NewBloksRequest(doc, bloks.BloksParamsInner(map[string]any{
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

	unminifier, err := bloks.GetUnminifier(loginPage)
	if err != nil {
		return nil, err
	}
	loginPage.Unminify(unminifier)

	var newPage *bloks.BloksBundle
	var loginParams map[string]string
	bridge := bloks.InterpBridge{
		DeviceID:       strings.ToUpper(fb.client.MessengerLite.deviceID.String()),
		FamilyDeviceID: strings.ToUpper(fb.client.MessengerLite.familyDeviceID.String()),
		MachineID:      fb.client.MessengerLite.machineID,
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
		DisplayNewScreen: func(toDisplay *bloks.BloksBundle) error {
			newPage = toDisplay
			return nil
		},
	}
	loginInterp, err := bloks.NewInterpreter(ctx, loginPage, &bridge, nil)
	if err != nil {
		return nil, err
	}
	log.Debug().Msg("Handling redirect to login page")
	_, err = loginInterp.Evaluate(ctx, &loginPage.Layout.Payload.Action.AST)
	if err != nil {
		return nil, err
	}
	if newPage == nil {
		return nil, fmt.Errorf("wasn't redirected to login page")
	}

	log.Debug().Msg("Filling in email and password on login page")
	loginPage = newPage
	loginInterp, err = bloks.NewInterpreter(ctx, loginPage, &bridge, loginInterp)
	if err != nil {
		return nil, err
	}

	fillTextInput := func(page *bloks.BloksBundle, interp *bloks.Interpreter, fieldName string, fillText string) error {
		input := page.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
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
		_, err = interp.Evaluate(bloks.InterpBindThis(ctx, input), &onChanged.AST)
		if err != nil {
			return fmt.Errorf("%s on_text_changed: %w", fieldName, err)
		}
		return nil
	}

	tapButton := func(page *bloks.BloksBundle, interp *bloks.Interpreter, buttonText string) error {
		textComp := page.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			if comp.Attributes["text"] == nil {
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
			return str == buttonText
		})
		if textComp == nil {
			return fmt.Errorf("couldn't find %s button", buttonText)
		}
		var buttonExtension *bloks.BloksTreeComponent
		textComp.FindAncestor(func(comp *bloks.BloksTreeComponent) bool {
			buttonExtension = comp.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
				return comp.ComponentID == "bk.components.FoaTouchExtension"
			})
			return buttonExtension != nil
		})
		if buttonExtension == nil {
			return fmt.Errorf("couldn't find %s button extension", buttonText)
		}
		onTouchDown, ok := buttonExtension.Attributes["on_touch_down"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return fmt.Errorf("%s button doesn't have on_touch_down script", buttonText)
		}
		onTouchUp, ok := buttonExtension.Attributes["on_touch_up"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return fmt.Errorf("%s button doesn't have on_touch_up script", buttonText)
		}
		_, err = interp.Evaluate(bloks.InterpBindThis(ctx, buttonExtension), &onTouchDown.AST)
		if err != nil {
			return fmt.Errorf("%s on_touch_down: %w", buttonText, err)
		}
		_, err = interp.Evaluate(bloks.InterpBindThis(ctx, buttonExtension), &onTouchUp.AST)
		if err != nil {
			return fmt.Errorf("%s on_touch_up: %w", buttonText, err)
		}
		return nil
	}

	err = fillTextInput(loginPage, loginInterp, "email", username)
	if err != nil {
		return nil, err
	}
	err = fillTextInput(loginPage, loginInterp, "password", password)
	if err != nil {
		return nil, err
	}
	err = tapButton(loginPage, loginInterp, "Log in")
	if err != nil {
		return nil, err
	}

	if loginParams == nil {
		return nil, fmt.Errorf("bloks did not generate login rpc")
	}
	var loginParamsInner bloks.BloksParamsInner
	err = json.Unmarshal([]byte(loginParams["params"]), &loginParamsInner)
	if err != nil {
		return nil, err
	}

	doc = &bloks.BloksDocSendLoginRequest
	loginResp, err := fb.client.makeBloksRequest(
		ctx, doc, bloks.NewBloksRequest(doc, loginParamsInner),
	)
	if err != nil {
		return nil, fmt.Errorf("sending bloks login request: %w", err)
	}

	log.Debug().Msg("Handling login page response")
	unminifier, err = bloks.GetUnminifier(loginResp)
	if err != nil {
		return nil, err
	}
	loginResp.Unminify(unminifier)

	var mfaParams map[string]string
	var loginRespData string
	loginRespInterp, err := bloks.NewInterpreter(ctx, loginResp, &bloks.InterpBridge{
		DeviceID:       strings.ToUpper(fb.client.MessengerLite.deviceID.String()),
		FamilyDeviceID: strings.ToUpper(fb.client.MessengerLite.familyDeviceID.String()),
		MachineID:      fb.client.MessengerLite.machineID,
		DoRPC: func(name string, params map[string]string) error {
			switch name {
			case "com.bloks.www.two_step_verification.entrypoint":
				mfaParams = params
			default:
				return fmt.Errorf("got unexpected rpc %s from login resp", name)
			}
			return nil
		},
		HandleLoginResponse: func(data string) error {
			loginRespData = data
			return nil
		},
	}, loginInterp)
	if err != nil {
		return nil, err
	}
	_, err = loginRespInterp.Evaluate(ctx, &loginResp.Layout.Payload.Action.AST)
	if err != nil {
		return nil, err
	}
	if mfaParams != nil {
		var mfaParamsInner bloks.BloksParamsInner
		err = json.Unmarshal([]byte(mfaParams["params"]), &mfaParamsInner)
		if err != nil {
			return nil, err
		}

		doc := &bloks.BloksDocTwoStepVerificationEntrypoint
		mfaPage, err := fb.client.makeBloksRequest(
			ctx, doc, bloks.NewBloksRequest(doc, mfaParamsInner),
		)
		if err != nil {
			return nil, fmt.Errorf("sending bloks mfa entrypoint request: %w", err)
		}

		log.Debug().Msg("Filling in MFA code")
		unminifier, err = bloks.GetUnminifier(mfaPage)
		if err != nil {
			return nil, err
		}
		mfaPage.Unminify(unminifier)

		var mfaVerifyParams map[string]string
		mfaPageInterp, err := bloks.NewInterpreter(ctx, mfaPage, &bloks.InterpBridge{
			DeviceID:       strings.ToUpper(fb.client.MessengerLite.deviceID.String()),
			FamilyDeviceID: strings.ToUpper(fb.client.MessengerLite.familyDeviceID.String()),
			MachineID:      fb.client.MessengerLite.machineID,
			DoRPC: func(name string, params map[string]string) error {
				switch name {
				case "com.bloks.www.two_step_verification.verify_code.async":
					mfaVerifyParams = params
				default:
					return fmt.Errorf("got unexpected rpc %s from login resp", name)
				}
				return nil
			},
		}, loginRespInterp)
		if err != nil {
			return nil, err
		}

		codeInput := mfaPage.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
			if comp.ComponentID != "bk.components.TextInput" {
				return false
			}
			return comp.FindDescendant(func(comp *bloks.BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.AccessibilityExtension" {
					return false
				}
				label, ok := comp.Attributes["label"].BloksTreeNodeContent.(*bloks.BloksTreeLiteral)
				if !ok {
					return false
				}
				str, ok := label.BloksJavascriptValue.(string)
				if !ok {
					return false
				}
				return str == "Code"
			}) != nil
		})

		code, err := getMFACode()
		if err != nil {
			return nil, err
		}
		err = codeInput.SetTextContent(code)
		if err != nil {
			return nil, err
		}
		onChanged, ok := codeInput.Attributes["on_text_change"].BloksTreeNodeContent.(*bloks.BloksTreeScript)
		if !ok {
			return nil, fmt.Errorf("code field doesn't have on_text_change script")
		}
		_, err = mfaPageInterp.Evaluate(bloks.InterpBindThis(ctx, codeInput), &onChanged.AST)
		if err != nil {
			return nil, fmt.Errorf("code on_text_changed: %w", err)
		}

		err = tapButton(mfaPage, mfaPageInterp, "Continue")
		if err != nil {
			return nil, err
		}
		if mfaVerifyParams == nil {
			return nil, fmt.Errorf("mfa screen didn't trigger verify rpc")
		}

		var mfaVerifyParamsInner bloks.BloksParamsInner
		err = json.Unmarshal([]byte(mfaVerifyParams["params"]), &mfaVerifyParamsInner)
		if err != nil {
			return nil, err
		}

		doc = &bloks.BloksDocVerifyCode
		mfaVerified, err := fb.client.makeBloksRequest(
			ctx, doc, bloks.NewBloksRequest(doc, mfaVerifyParamsInner),
		)
		if err != nil {
			return nil, err
		}

		log.Debug().Msg("Handling MFA code response")
		unminifier, err = bloks.GetUnminifier(mfaVerified)
		if err != nil {
			return nil, err
		}
		mfaVerified.Unminify(unminifier)

		mfaVerifiedInterp, err := bloks.NewInterpreter(ctx, mfaVerified, &bloks.InterpBridge{
			DeviceID:       strings.ToUpper(fb.client.MessengerLite.deviceID.String()),
			FamilyDeviceID: strings.ToUpper(fb.client.MessengerLite.familyDeviceID.String()),
			MachineID:      fb.client.MessengerLite.machineID,
			HandleLoginResponse: func(data string) error {
				loginRespData = data
				return nil
			},
		}, mfaPageInterp)
		if err != nil {
			return nil, err
		}
		_, err = mfaVerifiedInterp.Evaluate(ctx, &mfaVerified.Layout.Payload.Action.AST)
		if err != nil {
			return nil, err
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
		return nil, err
	}

	newCookies := convertCookies(&loginRespPayload)
	return newCookies, nil
}
