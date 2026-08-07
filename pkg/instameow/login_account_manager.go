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

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
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

func newInstagramAccountManagerToken(session *types.InstagramMobileSession) instagramAccountManagerToken {
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
		c.buildAndroidHeaders(),
		[]byte(form.Encode()),
		types.FORM,
	)
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

func (c *Client) switchInstagramAccountManagerAccount(
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
		c.buildAndroidHeaders(),
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
