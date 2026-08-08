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

package igconnector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

const (
	FlowIDInstagramPassword = "instagram-password"

	LoginStepIDCredentials = "fi.mau.meta.instagram.credentials"

	loginFieldIdentifier = "username"
	loginFieldPassword   = "password"
)

var loginFlowInstagramPassword = bridgev2.LoginFlow{
	Name:        "Instagram",
	Description: "Log in with your Instagram email or username and password",
	ID:          FlowIDInstagramPassword,
}

func getInstaNativeClient(
	ctx context.Context,
	log zerolog.Logger,
	conn *IGConnector,
	c *cookies.Cookies,
	useProxy bool,
) (*instameow.Client, error) {
	var loginDevice *types.InstagramLoginDevice
	if conn.DB != nil {
		var err error
		loginDevice, err = conn.DB.GetInstagramLoginDevice(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load Instagram app installation identity: %w", err)
		}
	}
	client := instameow.NewClient(instameow.ClientParams{
		Cookies:                    c,
		Log:                        log,
		Settings:                   conn.Bridge.GetHTTPClientSettings(),
		DisableTyping:              conn.Config.DisableTyping,
		EnableMobileTLSFingerprint: true,
		MobileLoginDevice:          loginDevice,
		SaveMobileLoginDevice: func(ctx context.Context, device types.InstagramLoginDevice) error {
			if conn.DB == nil {
				return nil
			}
			return conn.DB.PutInstagramLoginDevice(ctx, device)
		},
	})
	if useProxy && (conn.Config.GetProxyFrom != "" || conn.Config.Proxy != "") {
		client.GetHTTP().GetNewProxy = conn.getProxy
		if !client.GetHTTP().UpdateProxy("login") {
			return nil, errors.New("failed to update proxy")
		}
	}
	return client, nil
}

type MetaNativeLogin struct {
	User *bridgev2.User
	Main *IGConnector

	client     *instameow.Client
	caaStarted bool
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	if m.User == nil || m.Main == nil {
		return nil, errors.New("instagram login is not initialized")
	}
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	log := m.User.Log.With().Str("component", "instagram_login").Logger()
	client, err := getInstaNativeClient(ctx, log, m.Main, loginCookies, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	m.client = client
	m.caaStarted = false
	return instagramCredentialsStep("Enter your Instagram email or username and password."), nil
}

func (m *MetaNativeLogin) Cancel() {
	m.client = nil
	m.caaStarted = false
}

func (m *MetaNativeLogin) SubmitUserInput(
	ctx context.Context,
	input map[string]string,
) (*bridgev2.LoginStep, error) {
	if m.client == nil {
		return instagramCredentialsStep(
			"This Instagram login session expired. Start the login again.",
		), nil
	}
	if !m.caaStarted {
		identifier := strings.TrimSpace(input[loginFieldIdentifier])
		password := input[loginFieldPassword]
		if identifier == "" || password == "" {
			return instagramCredentialsStep(
				"Enter both your Instagram email or username and password.",
			), nil
		}
	}
	m.caaStarted = true

	step, err := m.client.DoInstagramCAALoginSteps(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to log in to Instagram through CAA: %w", err)
	}
	if step != nil {
		return step, nil
	}
	log := m.User.Log.With().Str("component", "instameow").Logger()
	loginCookies := m.client.GetCookies()
	if missingCookies := loginCookies.GetMissingCookieNames(); len(missingCookies) > 0 {
		return nil, ErrLoginMissingCookies.AppendMessage(": %v", missingCookies)
	}
	client, err := getInstaClient(log, m.Main, loginCookies, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	return loginWithCookies(ctx, log, client, m.User, m.Main, loginCookies)
}

func instagramCredentialsStep(instructions string) *bridgev2.LoginStep {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       LoginStepIDCredentials,
		Instructions: instructions,
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:        bridgev2.LoginInputFieldTypeUsername,
					ID:          loginFieldIdentifier,
					Name:        "Email or username",
					Description: "The email address or username for your Instagram account.",
				},
				{
					Type:        bridgev2.LoginInputFieldTypePassword,
					ID:          loginFieldPassword,
					Name:        "Password",
					Description: "Your Instagram password.",
				},
			},
		},
	}
}
