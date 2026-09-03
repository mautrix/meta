// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package instameow

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const (
	instagramCAALoginEntrypoint    = "com.bloks.www.bloks.caa.login.process_client_data_and_redirect"
	instagramCAAOAuthEntrypoint    = "com.bloks.www.caa.login.oauth.token.fetch.async"
	instagramCAASendEntrypoint     = "com.bloks.www.bloks.caa.login.async.send_login_request"
	instagramCAALegacyHomepage     = "com.bloks.www.caa.login.login_homepage"
	instagramCAAAutomaticStepLimit = 16
	instagramCAAAPIBase            = "https://b.i.instagram.com/api/v1/"
	instagramCAAGraphQLURL         = "https://b.i.instagram.com/graphql_www"
	instagramUSDIDRegistrationDoc  = "124930351917786857261002920888"
)

func instagramDeviceNetworkInfo() map[string]any {
	return map[string]any{
		"active_subscriptions_info":  []any{},
		"default_subscription_info":  nil,
		"is_airplane_mode":           false,
		"is_active_network_cellular": false,
		"is_device_sms_capable":      true,
		"sim_count":                  2,
		"is_wifi":                    true,
	}
}

type instagramCAALoginState struct {
	Browser                *bloks.Browser
	Mobile                 *mobileLoginState
	AccountManagerAccounts []instagramAccountManagerAccount
	Complete               bool
	AccountManagerChecked  bool
	AccountManagerComplete bool
	AAC                    string
	WaterfallID            string
	AttestationNonce       string
}

var ErrInstagramCAAUnsafeAccountStep = errors.New("instagram returned a sign-in step this bridge cannot safely complete")

type instagramCAALoginResponse struct {
	Headers       string `json:"headers"`
	LoginResponse string `json:"login_response"`
}

func (c *Client) ClearInstagramCAALoginState() {
	if c != nil {
		c.caaLogin = nil
		c.mobileLogin = nil
		c.mobileSession = nil
	}
}

func parseInstagramCAAResponseHeaders(rawHeaders string) (http.Header, error) {
	decoder := json.NewDecoder(strings.NewReader(rawHeaders))
	decoder.UseNumber()
	var headerValues map[string]any
	if err := decoder.Decode(&headerValues); err != nil {
		return nil, err
	}
	headers := make(http.Header)
	for key, rawValue := range headerValues {
		values := []any{rawValue}
		if arrayValue, ok := rawValue.([]any); ok {
			values = arrayValue
		}
		for _, value := range values {
			switch typedValue := value.(type) {
			case string:
				headers.Add(key, typedValue)
			case json.Number:
				headers.Add(key, typedValue.String())
			case bool:
				headers.Add(key, strconv.FormatBool(typedValue))
			case nil:
				continue
			default:
				return nil, fmt.Errorf("unsupported header value type %T", value)
			}
		}
	}
	return headers, nil
}

func makeInstagramUSDIDRegistrationToken(state *mobileLoginState) (string, error) {
	publicKey, err := x509.MarshalPKIXPublicKey(&state.USDIDKey.PublicKey)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	encode := func(value any) (string, error) {
		data, marshalErr := json.Marshal(value)
		return base64.RawURLEncoding.EncodeToString(data), marshalErr
	}
	payload, err := encode(map[string]any{
		"sub": state.USDID, "iat": now, "aud": useragent.IGAndroidAppID, "exp": now + 3600,
		"pub": base64.StdEncoding.EncodeToString(publicKey), "alg": "ES256",
	})
	if err != nil {
		return "", err
	}
	protected, err := encode(map[string]any{"typ": "JWT", "alg": "ES256", "kid": state.USDIDKeyID, "aid": useragent.IGAndroidAppID, "ver": "1"})
	if err != nil {
		return "", err
	}
	signature, err := signUSDID(state.USDIDKey, protected+"."+payload)
	if err != nil {
		return "", err
	}
	return encode(map[string]any{"payload": payload, "signatures": []any{map[string]any{"protected": protected, "signature": signature}}})
}

func (c *Client) registerInstagramUSDID(ctx context.Context, state *mobileLoginState) error {
	if state.USDIDRegistered {
		return nil
	}
	token, err := makeInstagramUSDIDRegistrationToken(state)
	if err != nil {
		return fmt.Errorf("build USDID registration token: %w", err)
	}
	variables, _ := json.Marshal(map[string]any{"input": map[string]any{
		"usdid_token": map[string]string{"sensitive_string_value": token},
		"fdid":        map[string]string{"sensitive_string_value": state.PhoneID},
	}})
	form := url.Values{
		"method": {"post"}, "pretty": {"false"}, "format": {"json"},
		"server_timestamps": {"true"}, "locale": {"user"}, "purpose": {"fetch"},
		"fb_api_req_friendly_name": {"IGUSDIDRegistrationMutation"},
		"enable_canonical_naming":  {"true"}, "enable_canonical_variable_overrides": {"true"},
		"enable_canonical_naming_ambiguous_type_prefixing": {"true"},
		"client_doc_id": {instagramUSDIDRegistrationDoc}, "variables": {string(variables)},
	}
	headers := c.mobileLoginHeaders(state)
	headers.Set("x-fb-friendly-name", "IGUSDIDRegistrationMutation")
	headers.Set("x-root-field-name", "usdid_registration")
	headers.Set("x-graphql-client-library", "pando")
	headers.Set("x-client-doc-id", instagramUSDIDRegistrationDoc)
	body, err := c.makeMobileLoginRequest(ctx, state, instagramCAAGraphQLURL, headers, []byte(form.Encode()))
	if err != nil {
		return err
	}
	var result struct {
		Data map[string]struct {
			Success bool `json:"success"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return err
	}
	for key, registration := range result.Data {
		if strings.Contains(key, "usdid_registration") && registration.Success {
			state.USDIDRegistered = true
			return c.persistMobileLoginDevice(ctx, state)
		}
	}
	return errors.New("Instagram rejected USDID registration")
}

func instagramCAAProcessParams(state *instagramCAALoginState) bloks.BloksParamsInner {
	return bloks.BloksParamsInner{
		"is_from_logged_out": false, "logged_out_user": "", "qpl_join_id": nil, "family_device_id": state.Mobile.PhoneID,
		"device_id":                state.Mobile.AndroidDeviceID,
		"offline_experiment_group": "caa_iteration_v3_perf_ig_4", "waterfall_id": state.WaterfallID,
		"logout_source": "", "show_internal_settings": false, "last_auto_login_time": 0, "disable_auto_login": false,
		"qe_device_id":                state.Mobile.DeviceID,
		"use_auto_login_interstitial": true, "disable_recursive_auto_login_interstitial": true,
		"auto_login_interstitial_experiment_group_name": "", "is_from_logged_in_switcher": false,
		"switcher_logged_in_uid": "", "account_list": []any{}, "blocked_uid": []any{},
		"INTERNAL_INFRA_THEME": "THREE_NEUTRAL_GRAY", "layered_homepage_experiment_group": "Deploy: Not in Experiment",
		"launched_url": "", "sim_phone_numbers": []any{}, "is_from_registration_reminder": false,
	}
}

func instagramCAAValue(bundle *bloks.BloksBundle, replacement string) string {
	if bundle == nil {
		return ""
	}
	for _, variable := range bundle.Layout.Payload.Variables {
		if variable.Info.Name != "CAA_ACCOUNT_ACCESS_CONTEXT:aac" {
			continue
		}
		if replacement != "" {
			variable.Info.Initial, variable.Info.InitialScript = replacement, nil
			return replacement
		}
		if value, ok := variable.Info.Initial.(string); ok {
			return value
		}
		if script := variable.Info.InitialScript; script != nil {
			if call, ok := script.AST.Content.(*bloks.BloksScriptFuncall); ok && len(call.Args) > 0 {
				if value, ok := call.Args[0].Content.(*bloks.BloksScriptLiteral); ok {
					result, _ := value.Value().(string)
					return result
				}
			}
		}
	}
	return ""
}

func (c *Client) prepareInstagramCAAPreflight(ctx context.Context, state *instagramCAALoginState, username string) error {
	if err := c.registerInstagramUSDID(ctx, state.Mobile); err != nil {
		return err
	}
	process, err := c.makeInstagramBloksRequest(ctx, &bloks.BloksActionDocInstagram,
		instagramCAALoginEntrypoint, instagramCAAProcessParams(state), "", "")
	if err != nil {
		return err
	}
	if state.AAC = instagramCAAValue(process, ""); state.AAC == "" {
		return errors.New("Instagram CAA preflight did not return account access context")
	}
	headers := c.mobileLoginHeaders(state.Mobile)
	headers.Set("x-fb-friendly-name", "IgApi: attestation/create_android_keystore/")
	form := url.Values{"app_scoped_device_id": {state.Mobile.DeviceID}, "key_hash": {""}}
	body, err := c.makeMobileLoginRequest(ctx, state.Mobile, instagramCAAAPIBase+"attestation/create_android_keystore/", headers, []byte(form.Encode()))
	if err != nil {
		return err
	}
	var attestation struct {
		ChallengeNonce string `json:"challenge_nonce"`
	}
	if err = json.Unmarshal(body, &attestation); err != nil || attestation.ChallengeNonce == "" {
		return errors.New("Instagram CAA preflight did not return an attestation nonce")
	}
	state.AttestationNonce = attestation.ChallengeNonce
	_, err = c.makeInstagramBloksRequest(ctx, &bloks.BloksActionDocInstagram, instagramCAAOAuthEntrypoint,
		bloks.BloksParamsInner{
			"client_input_params": map[string]any{
				"username_input": username, "si_device_param_network_info": instagramDeviceNetworkInfo(), "aac": state.AAC,
				"lois_settings": map[string]string{"lois_token": ""}, "cloud_trust_token": nil, "zero_balance_state": "", "network_bssid": nil},
			"server_params": map[string]any{
				"is_from_logged_out": 0, "layered_homepage_experiment_group": "Deploy: Not in Experiment", "device_id": state.Mobile.AndroidDeviceID,
				"login_surface": "login_home", "waterfall_id": state.WaterfallID, "INTERNAL__latency_qpl_instance_id": time.Now().UnixMilli(),
				"is_platform_login": 0, "login_entry_point": "logged_out", "INTERNAL__latency_qpl_marker_id": 36707139,
				"family_device_id": state.Mobile.PhoneID, "offline_experiment_group": "caa_iteration_v3_perf_ig_4", "access_flow_version": "pre_mt_behavior",
				"is_from_logged_in_switcher": 0, "qe_device_id": state.Mobile.DeviceID}},
		"", "")
	return err
}

func (c *Client) prepareInstagramCAALogin(ctx context.Context, username string) (*instagramCAALoginState, error) {
	if c.caaLogin != nil {
		return c.caaLogin, nil
	}
	mobile, err := c.prepareMobilePasswordLogin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare Instagram CAA login: %w", err)
	}
	browser, err := bloks.NewBrowser(&bloks.BrowserConfig{
		Platform: types.Instagram,
		EncryptPassword: func(ctx context.Context, password string) (string, error) {
			encrypted, encryptErr := crypto.EncryptInstagramAppPassword(
				mobile.PasswordKeyID,
				mobile.PasswordPublicKey,
				password,
			)
			if encryptErr != nil {
				return "", fmt.Errorf("encrypting Instagram CAA password: %w", encryptErr)
			}
			return encrypted, nil
		},
		MakeBloksRequest: c.makeInstagramBloksRequest,
		FetchAsset: func(ctx context.Context, assetURL string) ([]byte, string, error) {
			response, body, requestErr := c.http.MakeRequest(
				ctx,
				assetURL,
				http.MethodGet,
				c.mobileLoginHeaders(mobile),
				nil,
				types.NONE,
			)
			contentType := ""
			if response != nil {
				contentType = response.Header.Get("Content-Type")
			}
			return body, contentType, requestErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to construct Instagram CAA browser: %w", err)
	}
	browser.Bridge.DeviceID = mobile.DeviceID
	browser.Bridge.FamilyDeviceID = mobile.PhoneID
	browser.Bridge.AndroidDeviceID = mobile.AndroidDeviceID
	browser.Bridge.MachineID = mobile.MachineID
	waterfallID := uuid.NewString()
	browser.Bridge.DeviceNetworkInfo = instagramDeviceNetworkInfo()
	browser.Bridge.GetSecureNoncesForUser = func(string) any {
		return nil
	}
	c.caaLogin = &instagramCAALoginState{
		Browser:     browser,
		Mobile:      mobile,
		WaterfallID: waterfallID,
	}
	if err = c.prepareInstagramCAAPreflight(ctx, c.caaLogin, username); err != nil {
		c.caaLogin = nil
		return nil, fmt.Errorf("failed to prepare current Instagram CAA login: %w", err)
	}
	return c.caaLogin, nil
}

func (c *Client) DoInstagramCAALoginSteps(
	ctx context.Context,
	userInput map[string]string,
) (*bridgev2.LoginStep, error) {
	return c.doInstagramCAALoginSteps(ctx, userInput, false)
}

func (c *Client) DoInstagramCAALoginStepsExactAccount(
	ctx context.Context,
	userInput map[string]string,
	expectedIdentifier,
	expectedUserID string,
) (*bridgev2.LoginStep, error) {
	step, err := c.doInstagramCAALoginSteps(ctx, userInput, true)
	if step == nil && err == nil && !instagramCAAAccountMatches(c.mobileSession, expectedIdentifier, expectedUserID) {
		return nil, ErrInstagramCAAUnsafeAccountStep
	}
	return step, err
}

func instagramCAAAccountMatches(session *instagramMobileSession, expectedIdentifier, expectedUserID string) bool {
	if session == nil {
		return false
	} else if expectedUserID = strings.TrimSpace(expectedUserID); expectedUserID != "" {
		return session.UserID == expectedUserID
	}
	expectedIdentifier = strings.TrimPrefix(strings.TrimSpace(expectedIdentifier), "@")
	return expectedIdentifier != "" && !strings.Contains(expectedIdentifier, "@") &&
		strings.EqualFold(session.Username, expectedIdentifier)
}

func (c *Client) doInstagramCAALoginSteps(ctx context.Context, userInput map[string]string, exactAccount bool) (*bridgev2.LoginStep, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	if c.log != nil {
		ctx = c.log.WithContext(ctx)
	}
	state, err := c.prepareInstagramCAALogin(ctx, userInput["username"])
	if err != nil {
		return nil, err
	}
	for automaticSteps := 0; state.Browser.State != bloks.StateSuccess; automaticSteps++ {
		if automaticSteps >= instagramCAAAutomaticStepLimit {
			return nil, errors.New("instagram CAA login exceeded automatic step limit")
		}
		if exactAccount {
			switch state.Browser.State {
			case bloks.StateAuthenticationConfirm, bloks.StateAccountSelectionPage, bloks.StateSuggestedAccountPage,
				bloks.StateCaptchaPage, bloks.StateReCaptchaPage, bloks.StateOAuthPage,
				bloks.StateChooseContactPointPage:
				return nil, ErrInstagramCAAUnsafeAccountStep
			}
		}
		step, stepErr := state.Browser.DoLoginStep(ctx, userInput)
		if stepErr != nil {
			return nil, stepErr
		}
		if step != nil {
			return step, nil
		}
	}
	if !state.Complete {
		if err = c.applyInstagramCAALogin(ctx, state); err != nil {
			return nil, err
		}
		state.Complete = true
	}
	if exactAccount {
		return nil, nil
	}
	if !state.AccountManagerChecked {
		state.AccountManagerAccounts, err = c.getInstagramAccountManagerAccounts(ctx)
		if err != nil {
			return nil, err
		}
		state.AccountManagerChecked = true
		if len(state.AccountManagerAccounts) <= 1 {
			state.AccountManagerComplete = true
		}
	}
	if !state.AccountManagerComplete {
		selectedUsername := userInput[instagramAccountManagerField]
		if selectedUsername == "" {
			return instagramAccountManagerSelectionStep(state.AccountManagerAccounts), nil
		}
		var selectedAccount *instagramAccountManagerAccount
		for i := range state.AccountManagerAccounts {
			if state.AccountManagerAccounts[i].Username == selectedUsername {
				selectedAccount = &state.AccountManagerAccounts[i]
				break
			}
		}
		if selectedAccount == nil {
			return nil, errors.New("invalid Instagram Account Manager profile selection")
		}
		if selectedAccount.UserID != c.mobileSession.UserID {
			if err = c.switchInstagramAccountManagerProfile(ctx, state.Mobile, *selectedAccount); err != nil {
				return nil, err
			}
		}
		state.AccountManagerComplete = true
	}
	return nil, nil
}

func (c *Client) makeInstagramBloksRequest(
	ctx context.Context,
	doc *bloks.BloksDoc,
	appID string,
	inner bloks.BloksParamsInner,
	_ string,
	_ string,
) (*bloks.BloksBundle, error) {
	if doc == nil {
		return nil, errors.New("instagram Bloks request is missing its document type")
	}
	if c.mobileLogin == nil {
		return nil, errors.New("instagram Bloks request is missing its mobile session")
	}
	state := c.caaLogin
	if appID == instagramCAASendEntrypoint {
		if state == nil || state.AAC == "" || state.AttestationNonce == "" {
			return nil, errors.New("instagram credential request is missing CAA preflight state")
		}
		clientParams, clientOK := inner["client_input_params"].(map[string]any)
		serverParams, serverOK := inner["server_params"].(map[string]any)
		if !clientOK || !serverOK {
			return nil, errors.New("instagram credential request has invalid CAA parameters")
		}
		clientParams["aac"] = state.AAC
		serverParams["waterfall_id"] = state.WaterfallID
	}
	params, err := json.Marshal(inner)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Instagram Bloks parameters: %w", err)
	}
	clientContext, err := json.Marshal(map[string]string{
		"bloks_version": bloks.BloksVersionInstagramAndroid,
		"styles_id":     "instagram",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Instagram Bloks client context: %w", err)
	}
	form := url.Values{
		"_uuid":               {c.mobileLogin.DeviceID},
		"bk_client_context":   {string(clientContext)},
		"bloks_versioning_id": {bloks.BloksVersionInstagramAndroid},
		"params":              {string(params)},
	}
	route := "bloks/apps/"
	if doc.RootField == "bloks_action" {
		route = "bloks/async_action/"
	}
	headers := c.mobileLoginHeaders(c.mobileLogin)
	headers.Set("x-fb-friendly-name", "IgApi: "+route+appID+"/")
	requestBase := instagramMobileAPIBase
	if state != nil && appID != instagramCAALegacyHomepage {
		requestBase = instagramCAAAPIBase
	}
	if appID == instagramCAASendEntrypoint {
		attestation, _ := json.Marshal(map[string]any{"attestation": []any{map[string]any{
			"version": 2, "type": "keystore", "errors": []int{-1013},
			"challenge_nonce": state.AttestationNonce, "signed_nonce": "", "key_hash": "",
		}}})
		headers.Set("x-ig-attest-params", string(attestation))
	}
	body, requestErr := c.makeMobileLoginRequest(ctx, c.mobileLogin,
		requestBase+route+url.PathEscape(appID)+"/", headers, []byte(form.Encode()))
	var bundle bloks.BloksBundle
	if err = json.Unmarshal(body, &bundle); err != nil {
		if requestErr != nil {
			return nil, fmt.Errorf("instagram Bloks request failed: %w", requestErr)
		}
		return nil, fmt.Errorf("failed to parse Instagram Bloks response: %w", err)
	}
	if appID == instagramCAALegacyHomepage && state != nil && instagramCAAValue(&bundle, state.AAC) == "" {
		return nil, errors.New("Instagram login page did not expose its CAA account access context")
	}
	if c.logRedactedBloksPayloads {
		if err = bloks.LogRedactedBundle(c.log, appID, body); err != nil {
			return nil, err
		}
	}
	if requestErr != nil {
		return nil, fmt.Errorf("instagram Bloks request failed: %w", requestErr)
	}
	return &bundle, nil
}

func (c *Client) applyInstagramCAALogin(ctx context.Context, state *instagramCAALoginState) error {
	var caaResponse instagramCAALoginResponse
	if err := json.Unmarshal([]byte(state.Browser.LoginData), &caaResponse); err != nil {
		return fmt.Errorf("failed to parse Instagram CAA login result: %w", err)
	}
	if caaResponse.Headers == "" || caaResponse.LoginResponse == "" {
		return errors.New("instagram CAA login result is incomplete")
	}
	responseHeaders, err := parseInstagramCAAResponseHeaders(caaResponse.Headers)
	if err != nil {
		return fmt.Errorf("failed to parse Instagram CAA response headers: %w", err)
	}
	var loginResponse instagramMobileLoginResponse
	if err := json.Unmarshal([]byte(caaResponse.LoginResponse), &loginResponse); err != nil {
		return fmt.Errorf("failed to parse Instagram CAA authorization: %w", err)
	}
	response := &http.Response{Header: responseHeaders}
	if err := c.updateMobileLoginResponseState(ctx, state.Mobile, response); err != nil {
		return err
	}
	if err := c.applyMobileAuthorization(response, &loginResponse, state.Mobile); err != nil {
		return err
	}
	if missing := c.cookies.GetMissingCookieNames(); len(missing) > 0 {
		return fmt.Errorf("instagram CAA login succeeded without required cookies: %v", missing)
	}
	return nil
}
