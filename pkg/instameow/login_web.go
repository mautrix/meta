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

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type instagramWebLoginResponse struct {
	Authenticated     bool   `json:"authenticated"`
	TwoFactorRequired bool   `json:"two_factor_required"`
	Message           string `json:"message"`
}

// CreateInstagramWebSession turns a completed native login into a web session
// that can be used by Instagram's web messaging APIs and realtime stream.
func (c *Client) CreateInstagramWebSession(ctx context.Context, identifier, password string) error {
	if c == nil {
		return ErrClientIsNil
	} else if identifier == "" || password == "" {
		return errors.New("instagram web login is missing credentials")
	}

	c.cookies.UpdateValues(map[cookies.MetaCookieName]string{
		cookies.IGCookieMachineID: c.cookies.Get(cookies.IGCookieMachineID),
		cookies.IGCookieDeviceID:  c.cookies.Get(cookies.IGCookieDeviceID),
	})
	c.cookies.IGWWWClaim = ""

	c.configs = httpclient.NewConfigs(c)
	c.http.SetConfigs(c.configs)
	moduleLoader := httpclient.NewModuleParser(c, c.http, c.configs)
	moduleLoader.LS = nil
	baseURL := c.GetEndpoint("base_url")
	loginPageURL := baseURL + "/accounts/login/"
	if err := moduleLoader.Load(ctx, loginPageURL); err != nil {
		return fmt.Errorf("failed to load Instagram web login page: %w", err)
	}
	c.configs.Setup(false)

	encryption := c.configs.BrowserConfigTable.InstagramPasswordEncryption
	keyID, err := strconv.Atoi(encryption.KeyID)
	if err != nil || keyID <= 0 || encryption.PublicKey == "" {
		return errors.New("instagram web login page did not include password encryption keys")
	}
	encryptedPassword, err := crypto.EncryptInstagramWebPassword(
		keyID,
		encryption.PublicKey,
		password,
	)
	if err != nil {
		return fmt.Errorf("failed to encrypt Instagram web password: %w", err)
	}

	form := url.Values{
		"enc_password":                {encryptedPassword},
		"loginAttemptSubmissionCount": {"0"},
		"optIntoOneTap":               {"false"},
		"queryParams":                 {"{}"},
		"stopDeletionNonce":           {""},
		"trustedDeviceRecords":        {"{}"},
		"username":                    {identifier},
	}
	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", baseURL)
	headers.Set("referer", loginPageURL)
	headers.Set("x-requested-with", "XMLHttpRequest")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		baseURL+"/api/v1/web/accounts/login/ajax/",
		http.MethodPost,
		headers,
		[]byte(form.Encode()),
		types.FORM,
	)
	var result instagramWebLoginResponse
	parseErr := json.Unmarshal(body, &result)
	if requestErr != nil {
		if parseErr == nil && result.Message != "" {
			return fmt.Errorf("instagram web login failed: %s", result.Message)
		}
		return fmt.Errorf("instagram web login request failed: %w", requestErr)
	} else if response == nil {
		return errors.New("instagram web login returned no response")
	} else if parseErr != nil {
		return fmt.Errorf("failed to parse Instagram web login: %w", parseErr)
	} else if result.TwoFactorRequired {
		return errors.New("instagram web session requires an additional two-factor login")
	} else if !result.Authenticated {
		if result.Message != "" {
			return fmt.Errorf("instagram web login failed: %s", result.Message)
		}
		return errors.New("instagram web login did not authenticate")
	} else if missing := c.cookies.GetMissingCookieNames(); len(missing) > 0 {
		return fmt.Errorf("instagram web login succeeded without required cookies: %v", missing)
	}
	return nil
}
