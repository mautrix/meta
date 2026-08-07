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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

const instagramCAALoginEntrypoint = "com.bloks.www.bloks.caa.login.process_client_data_and_redirect"

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
}

type instagramCAALoginResponse struct {
	Headers       string `json:"headers"`
	LoginResponse string `json:"login_response"`
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

func (c *Client) prepareInstagramCAALogin(ctx context.Context) (*instagramCAALoginState, error) {
	if c.caaLogin != nil {
		return c.caaLogin, nil
	}
	if c.enableMobileTLSFingerprint {
		c.http.SetMobileTLSFingerprint(true)
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
	browser.Bridge.DeviceNetworkInfo = instagramDeviceNetworkInfo()
	browser.Bridge.GetSecureNoncesForUser = func(string) any {
		return nil
	}
	c.caaLogin = &instagramCAALoginState{
		Browser: browser,
		Mobile:  mobile,
	}
	return c.caaLogin, nil
}

func (c *Client) DoInstagramCAALoginSteps(
	ctx context.Context,
	userInput map[string]string,
) (*bridgev2.LoginStep, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	if c.log != nil {
		// Provisioning requests do not always carry the network client's logger.
		// Attach it here so privacy-safe CAA state, screen, and field identifiers
		// are available when diagnosing a credentialed login.
		ctx = c.log.WithContext(ctx)
	}
	state, err := c.prepareInstagramCAALogin(ctx)
	if err != nil {
		return nil, err
	}
	for state.Browser.State != bloks.StateSuccess {
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
			if err = c.switchInstagramAccountManagerAccount(ctx, state.Mobile, *selectedAccount); err != nil {
				return nil, err
			}
		}
		state.AccountManagerComplete = true
	}
	if c.enableMobileTLSFingerprint {
		c.http.SetMobileTLSFingerprint(false)
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
	params, err := json.Marshal(inner)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Instagram Bloks parameters: %w", err)
	}
	clientContext, err := json.Marshal(map[string]string{
		"bloks_version": bloks.BloksVersionInstagram,
		"styles_id":     "instagram",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Instagram Bloks client context: %w", err)
	}
	form := url.Values{
		"_uuid":               {c.mobileLogin.DeviceID},
		"bk_client_context":   {string(clientContext)},
		"bloks_versioning_id": {bloks.BloksVersionInstagram},
		"params":              {string(params)},
	}
	route := "bloks/apps/"
	if doc.RootField == "bloks_action" {
		route = "bloks/async_action/"
	}
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		instagramMobileAPIBase+route+url.PathEscape(appID)+"/",
		http.MethodPost,
		c.mobileLoginHeaders(c.mobileLogin),
		[]byte(form.Encode()),
		types.FORM,
	)
	if response != nil {
		if err = c.updateMobileLoginResponseState(ctx, c.mobileLogin, response); err != nil {
			return nil, err
		}
	}
	var bundle bloks.BloksBundle
	if err = json.Unmarshal(body, &bundle); err != nil {
		if requestErr != nil {
			return nil, fmt.Errorf("instagram Bloks request failed: %w", requestErr)
		}
		return nil, fmt.Errorf("failed to parse Instagram Bloks response: %w", err)
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
