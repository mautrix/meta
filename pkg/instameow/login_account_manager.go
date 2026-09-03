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
	"sort"
	"strings"

	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const instagramAccountManagerField = "account"

type instagramAccountManagerAccount struct {
	UserID   string
	Username string
}

type instagramAccountManagerToken struct {
	AccountType string `json:"account_type"`
	TokenID     int    `json:"token_id"`
	Token       string `json:"token_str"`
	UserID      string `json:"user_fbid"`
	TokenType   string `json:"token_type"`
	TokenApp    string `json:"token_app"`
	TokenSource string `json:"token_source"`
}

type instagramAccountManagerResponse struct {
	Result []struct {
		Token struct {
			AccountType string `json:"account_type"`
		} `json:"token"`
		ConnectedAccounts []struct {
			SSOEnabled bool   `json:"is_sso_enabled"`
			UserID     string `json:"user_fbid"`
			User       struct {
				PK       json.RawMessage `json:"pk"`
				PKID     json.RawMessage `json:"pk_id"`
				ID       json.RawMessage `json:"id"`
				Username string          `json:"username"`
			} `json:"user"`
		} `json:"connected_accounts"`
	} `json:"result"`
}

type instagramAccountManagerError struct {
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
}

type instagramAccountManagerWebLoginResponse struct {
	Authenticated bool `json:"authenticated"`
}

type instagramWebAccountManagerResponse struct {
	IGAccounts []struct {
		Username string `json:"username"`
	} `json:"igAccounts"`
}

type instagramWebAccountManagerState struct {
	Accounts  []instagramAccountManagerAccount
	CSRFToken string
	Complete  bool
}

func newInstagramAccountManagerToken(session *instagramMobileSession) instagramAccountManagerToken {
	return instagramAccountManagerToken{
		AccountType: "Instagram",
		TokenID:     0,
		Token:       session.Authorization,
		UserID:      session.UserID,
		TokenType:   "first_party",
		TokenApp:    "Instagram",
		TokenSource: "active_account",
	}
}

func (c *Client) instagramAccountManagerHeaders() http.Header {
	session := c.mobileSession
	headers := http.Header{}
	headers.Set("authorization", session.Authorization)
	headers.Set("user-agent", instagramMobileUserAgent)
	headers.Set("ig-u-ds-user-id", session.UserID)
	headers.Set("ig-intended-user-id", session.UserID)
	headers.Set("ig-u-rur", session.RUR)
	headers.Set("ig-u-shbid", session.SHBID)
	headers.Set("ig-u-shbts", session.SHBTS)
	headers.Set("ig-u-ig-direct-region-hint", session.DirectRegionHint)
	headers.Set("x-ig-www-claim", session.WWWClaim)
	headers.Set("x-ig-device-id", session.Device.DeviceID)
	headers.Set("x-ig-family-device-id", session.Device.PhoneID)
	headers.Set("x-ig-android-id", session.Device.AndroidDeviceID)
	headers.Set("x-mid", c.cookies.Get(cookies.IGCookieMachineID))
	headers.Set("x-ig-app-id", useragent.IGAndroidAppID)
	return headers
}

func parseInstagramAccountManagerAccounts(
	body []byte,
	currentUserID string,
	currentUsername string,
) ([]instagramAccountManagerAccount, error) {
	var response instagramAccountManagerResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	accountsByID := make(map[string]instagramAccountManagerAccount)
	for _, result := range response.Result {
		if result.Token.AccountType != "Instagram" {
			continue
		}
		for _, connectedAccount := range result.ConnectedAccounts {
			if !connectedAccount.SSOEnabled || connectedAccount.User.Username == "" {
				continue
			}
			userID := rawJSONScalar(connectedAccount.User.PKID)
			if userID == "" {
				userID = rawJSONScalar(connectedAccount.User.PK)
			}
			if userID == "" {
				userID = rawJSONScalar(connectedAccount.User.ID)
			}
			if userID == "" {
				userID = connectedAccount.UserID
			}
			if userID == "" {
				continue
			}
			accountsByID[userID] = instagramAccountManagerAccount{
				UserID:   userID,
				Username: connectedAccount.User.Username,
			}
		}
	}
	if currentUserID != "" && currentUsername != "" {
		accountsByID[currentUserID] = instagramAccountManagerAccount{
			UserID:   currentUserID,
			Username: currentUsername,
		}
	}
	accounts := make([]instagramAccountManagerAccount, 0, len(accountsByID))
	for _, account := range accountsByID {
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		iCurrent := accounts[i].UserID == currentUserID
		jCurrent := accounts[j].UserID == currentUserID
		if iCurrent != jCurrent {
			return iCurrent
		}
		return strings.ToLower(accounts[i].Username) < strings.ToLower(accounts[j].Username)
	})
	return accounts, nil
}

func instagramAccountManagerRequestError(
	action string,
	response *http.Response,
	body []byte,
	requestErr error,
) error {
	if response == nil {
		return fmt.Errorf("%s: %w", action, requestErr)
	}
	if httpclient.IsPermanentRequestError(requestErr) {
		return fmt.Errorf("%s: %w", action, requestErr)
	}
	var failure instagramAccountManagerError
	_ = json.Unmarshal(body, &failure)
	switch {
	case failure.ErrorType != "" && failure.Message != "":
		return fmt.Errorf(
			"%s: Instagram returned %s (%s, HTTP %d)",
			action,
			failure.Message,
			failure.ErrorType,
			response.StatusCode,
		)
	case failure.ErrorType != "":
		return fmt.Errorf(
			"%s: Instagram returned %s (HTTP %d)",
			action,
			failure.ErrorType,
			response.StatusCode,
		)
	default:
		return fmt.Errorf("%s: Instagram returned HTTP %d", action, response.StatusCode)
	}
}

func (c *Client) getInstagramAccountManagerAccounts(
	ctx context.Context,
) ([]instagramAccountManagerAccount, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	session := c.mobileSession
	if session == nil || session.Authorization == "" || session.UserID == "" {
		return nil, errors.New("instagram Account Manager requires an authorized mobile session")
	}
	token, err := json.Marshal([]instagramAccountManagerToken{
		newInstagramAccountManagerToken(session),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode Instagram Account Manager token: %w", err)
	}
	form := url.Values{
		"surface":                {"account_switcher"},
		"include_social_context": {"false"},
		"tokens":                 {string(token)},
	}
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		instagramMobileAPIBase+"fxcal/get_sso_accounts/",
		http.MethodPost,
		c.instagramAccountManagerHeaders(),
		[]byte(form.Encode()),
		types.FORM,
	)
	if response != nil {
		c.cookies.UpdateFromResponse(response)
	}
	if requestErr != nil {
		return nil, instagramAccountManagerRequestError(
			"failed to load Instagram Account Manager profiles",
			response,
			body,
			requestErr,
		)
	}
	accounts, err := parseInstagramAccountManagerAccounts(
		body,
		session.UserID,
		session.Username,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Instagram Account Manager profiles: %w", err)
	}
	return accounts, nil
}

func (c *Client) switchInstagramAccountManagerProfile(
	ctx context.Context,
	state *mobileLoginState,
	account instagramAccountManagerAccount,
) error {
	if c == nil {
		return ErrClientIsNil
	}
	if state == nil {
		return errors.New("instagram Account Manager is missing the mobile login state")
	}

	if err := c.loadIndex(ctx); err != nil {
		return fmt.Errorf("failed to prepare the primary Instagram web session: %w", err)
	}
	primaryCookies := c.cookies.GetAll()
	primaryWWWClaim := c.cookies.IGWWWClaim

	if err := c.switchInstagramAccountManagerMobileAccount(ctx, state, account); err != nil {
		return err
	}

	// The mobile FXCAL login authorizes API calls, but the web realtime stream is
	// provisioned separately. Switch the already authenticated primary web session
	// through Instagram's matching FXCAL endpoint before persisting the selection.
	c.cookies.UpdateValues(primaryCookies)
	c.cookies.IGWWWClaim = primaryWWWClaim
	if err := c.switchInstagramAccountManagerWebAccount(ctx, account.Username); err != nil {
		return err
	}
	if err := c.loadIndex(ctx); err != nil {
		return fmt.Errorf("failed to load the selected Instagram web session: %w", err)
	}
	if c.cookies.Get(cookies.IGCookieCSRFToken) == "" {
		c.cookies.Set(cookies.IGCookieCSRFToken, state.CSRFToken)
	}
	if !strings.EqualFold(c.configs.BrowserConfigTable.PolarisViewer.GetUsername(), account.Username) {
		return errors.New("instagram Account Manager web login returned the wrong profile")
	}
	return nil
}

func (c *Client) switchInstagramAccountManagerMobileAccount(
	ctx context.Context,
	state *mobileLoginState,
	account instagramAccountManagerAccount,
) error {
	if c == nil {
		return ErrClientIsNil
	}
	session := c.mobileSession
	if session == nil || session.Authorization == "" || session.UserID == "" {
		return errors.New("instagram Account Manager requires an authorized mobile session")
	}
	if state == nil {
		return errors.New("instagram Account Manager is missing the mobile login state")
	}
	token, err := json.Marshal(newInstagramAccountManagerToken(session))
	if err != nil {
		return fmt.Errorf("failed to encode Instagram Account Manager token: %w", err)
	}
	form := url.Values{
		"pk":        {account.UserID},
		"adid":      {state.AdvertisingID},
		"device_id": {state.AndroidDeviceID},
		"guid":      {state.DeviceID},
		"phone_id":  {state.PhoneID},
		"surface":   {"account_switcher"},
		"token":     {string(token)},
	}
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		instagramMobileAPIBase+"fxcal/sso_login/",
		http.MethodPost,
		c.instagramAccountManagerHeaders(),
		[]byte(form.Encode()),
		types.FORM,
	)
	if requestErr != nil {
		return instagramAccountManagerRequestError(
			"failed to switch Instagram Account Manager profile",
			response,
			body,
			requestErr,
		)
	}
	if response == nil {
		return errors.New("instagram Account Manager login returned no response")
	}
	var loginResponse instagramMobileLoginResponse
	if err = json.Unmarshal(body, &loginResponse); err != nil {
		return fmt.Errorf("failed to parse Instagram Account Manager login: %w", err)
	}
	if err = c.updateMobileLoginResponseState(ctx, state, response); err != nil {
		return err
	}
	if err = c.applyMobileAuthorization(response, &loginResponse, state); err != nil {
		return err
	}
	if c.mobileSession == nil || c.mobileSession.UserID != account.UserID {
		return errors.New("instagram Account Manager returned the wrong profile")
	}
	if missing := c.cookies.GetMissingCookieNames(); len(missing) > 0 {
		return fmt.Errorf(
			"instagram Account Manager login succeeded without required cookies: %v",
			missing,
		)
	}
	return nil
}

func (c *Client) switchInstagramAccountManagerWebAccount(
	ctx context.Context,
	username string,
) error {
	queryParams := c.http.NewHTTPQuery()
	form := url.Values{
		"igUsername": {username},
	}
	if queryParams.FbDtsg != "" {
		form.Set("fb_dtsg", queryParams.FbDtsg)
	}
	if queryParams.Jazoest != "" {
		form.Set("jazoest", queryParams.Jazoest)
	}
	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", c.GetEndpoint("base_url"))
	headers.Set("referer", c.GetEndpoint("messages"))
	headers.Set("x-requested-with", "XMLHttpRequest")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		c.GetEndpoint("fxcal_sso_login"),
		http.MethodPost,
		headers,
		[]byte(form.Encode()),
		types.FORM,
	)
	if response != nil {
		c.cookies.UpdateFromResponse(response)
	}
	if requestErr != nil {
		return instagramAccountManagerRequestError(
			"failed to switch the Instagram Account Manager web session",
			response,
			body,
			requestErr,
		)
	}
	var result instagramAccountManagerWebLoginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse the Instagram Account Manager web login: %w", err)
	}
	if !result.Authenticated {
		return errors.New("instagram Account Manager did not authenticate the selected web profile")
	}
	return nil
}

func (c *Client) getInstagramWebAccountManagerAccounts(
	ctx context.Context,
	currentUsername string,
) ([]instagramAccountManagerAccount, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", c.GetEndpoint("base_url"))
	headers.Set("referer", c.GetEndpoint("messages"))
	headers.Set("x-requested-with", "XMLHttpRequest")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		c.GetEndpoint("fxcal_sso_users"),
		http.MethodPost,
		headers,
		nil,
		types.NONE,
	)
	if response != nil {
		c.cookies.UpdateFromResponse(response)
	}
	if requestErr != nil {
		return nil, instagramAccountManagerRequestError(
			"failed to load Instagram web Account Manager profiles",
			response,
			body,
			requestErr,
		)
	}
	var result instagramWebAccountManagerResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Instagram web Account Manager profiles: %w", err)
	}
	accounts := []instagramAccountManagerAccount{{Username: currentUsername}}
	seen := map[string]bool{strings.ToLower(currentUsername): true}
	for _, account := range result.IGAccounts {
		username := strings.TrimSpace(account.Username)
		key := strings.ToLower(username)
		if username != "" && !seen[key] {
			seen[key] = true
			accounts = append(accounts, instagramAccountManagerAccount{Username: username})
		}
	}
	return accounts, nil
}

func (c *Client) DoInstagramWebAccountManagerSteps(
	ctx context.Context,
	userInput map[string]string,
) (*bridgev2.LoginStep, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	state := c.webAccountManager
	if state == nil {
		if err := c.loadIndex(ctx); err != nil {
			return nil, fmt.Errorf("failed to load the current Instagram web session: %w", err)
		}
		currentUsername := strings.TrimSpace(c.configs.BrowserConfigTable.PolarisViewer.GetUsername())
		if currentUsername == "" {
			return nil, errors.New("instagram web login did not return the current profile")
		}
		accounts, err := c.getInstagramWebAccountManagerAccounts(ctx, currentUsername)
		if err != nil {
			return nil, err
		}
		state = &instagramWebAccountManagerState{
			Accounts:  accounts,
			CSRFToken: c.cookies.Get(cookies.IGCookieCSRFToken),
			Complete:  len(accounts) == 1,
		}
		c.webAccountManager = state
	}
	if state.Complete {
		return nil, nil
	}
	selectedUsername := strings.TrimSpace(userInput[instagramAccountManagerField])
	if selectedUsername == "" {
		return instagramAccountManagerSelectionStep(state.Accounts), nil
	}
	for i := range state.Accounts {
		selectedAccount := &state.Accounts[i]
		if !strings.EqualFold(selectedAccount.Username, selectedUsername) {
			continue
		}
		if i != 0 {
			if err := c.switchInstagramAccountManagerWebAccount(ctx, selectedAccount.Username); err != nil {
				return nil, err
			} else if err := c.loadIndex(ctx); err != nil {
				return nil, fmt.Errorf("failed to load the selected Instagram web session: %w", err)
			}
			if c.cookies.Get(cookies.IGCookieCSRFToken) == "" {
				c.cookies.Set(cookies.IGCookieCSRFToken, state.CSRFToken)
			}
			if !strings.EqualFold(c.configs.BrowserConfigTable.PolarisViewer.GetUsername(), selectedAccount.Username) {
				return nil, errors.New("instagram Account Manager web login returned the wrong profile")
			}
		}
		state.Complete = true
		return nil, nil
	}
	return nil, errors.New("invalid Instagram web Account Manager profile selection")
}

func instagramAccountManagerSelectionStep(
	accounts []instagramAccountManagerAccount,
) *bridgev2.LoginStep {
	options := make([]string, len(accounts))
	for i, account := range accounts {
		options[i] = account.Username
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "fi.mau.meta.instagram.account_manager",
		Instructions: "Choose the Instagram profile to connect",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					ID:      instagramAccountManagerField,
					Name:    "Instagram profile",
					Type:    bridgev2.LoginInputFieldTypeSelect,
					Options: options,
				},
			},
		},
	}
}
