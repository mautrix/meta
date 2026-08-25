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
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/loginerrors"
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
	userID id.UserID,
	useProxy bool,
	transport http.RoundTripper,
) (*instameow.Client, error) {
	var loginDevice *types.InstagramLoginDevice
	if conn.DB != nil {
		var err error
		loginDevice, err = conn.DB.GetInstagramLoginDevice(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to load Instagram app installation identity: %w", err)
		}
	}
	client := instameow.NewClient(instameow.ClientParams{
		Cookies:                  c,
		Log:                      log,
		Settings:                 conn.Bridge.GetHTTPClientSettings(),
		DisableTyping:            conn.Config.DisableTyping,
		LogRedactedBloksPayloads: conn.Config.LogRedactedBloksPayloads,
		MobileLoginDevice:        loginDevice,
		SaveMobileLoginDevice: func(ctx context.Context, device types.InstagramLoginDevice) error {
			if conn.DB == nil {
				return nil
			}
			return conn.DB.PutInstagramLoginDevice(ctx, userID, device)
		},
	})
	if transport != nil {
		client.GetHTTP().GetNewProxy = nil
		client.GetHTTP().HTTP.Transport = transport
	} else if useProxy && (conn.Config.GetProxyFrom != "" || conn.Config.Proxy != "") {
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
	transport  http.RoundTripper
	identifier string
	password   string
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessWithParams = (*MetaNativeLogin)(nil)

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.StartWithParams(ctx, bridgev2.LoginStartParams{})
}

func (m *MetaNativeLogin) StartWithParams(
	ctx context.Context,
	params bridgev2.LoginStartParams,
) (*bridgev2.LoginStep, error) {
	m.transport = params.HTTP
	return m.start(ctx, "Enter your Instagram email or username and password.")
}

func (m *MetaNativeLogin) start(ctx context.Context, instructions string) (*bridgev2.LoginStep, error) {
	m.client = nil
	m.caaStarted = false
	m.clearCredentials()
	if m.User == nil || m.Main == nil {
		return nil, errors.New("instagram login is not initialized")
	}
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	log := m.User.Log.With().Str("component", "instagram_login").Logger()
	log.Debug().Bool("client_http", m.transport != nil).Msg("Starting Instagram native login flow")
	var userID id.UserID
	if m.User.User != nil {
		userID = m.User.MXID
	} else if m.Main.DB != nil {
		return nil, errors.New("instagram login user is missing database state")
	}
	client, err := getInstaNativeClient(
		ctx,
		log,
		m.Main,
		loginCookies,
		userID,
		m.Main.Config.ProxyOther,
		m.transport,
	)
	if err != nil {
		return nil, err
	}
	m.client = client
	return instagramCredentialsStep(instructions), nil
}

func (m *MetaNativeLogin) Cancel() {
	m.client = nil
	m.caaStarted = false
	m.transport = nil
	m.clearCredentials()
}

func (m *MetaNativeLogin) clearCredentials() {
	m.identifier = ""
	m.password = ""
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
		m.identifier = identifier
		m.password = password
	}
	m.caaStarted = true

	step, err := m.client.DoInstagramCAALoginSteps(ctx, input)
	if err != nil {
		if isClientHTTPError(err) {
			m.User.Log.Warn().Err(err).Msg("Instagram login request failed on the client")
			return m.start(ctx, "The request did not complete on this device. Please try again.")
		}
		m.clearCredentials()
		return nil, fmt.Errorf("failed to log in to Instagram through CAA: %w", err)
	}
	if step != nil {
		return step, nil
	}
	m.clearCredentials()
	return m.complete(ctx)
}

func (m *MetaNativeLogin) complete(ctx context.Context) (*bridgev2.LoginStep, error) {
	log := m.User.Log.With().Str("component", "instameow").Logger()
	loginCookies := m.client.GetCookies()
	if missingCookies := loginCookies.GetMissingCookieNames(); len(missingCookies) > 0 {
		return nil, loginerrors.MissingCookies.AppendMessage(": %v", missingCookies)
	}
	client, err := getInstaClient(log, m.Main, loginCookies, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	loginTransport := m.transport
	m.transport = nil
	var restoreTransport func()
	if loginTransport != nil {
		originalTransport := client.GetHTTP().HTTP.Transport
		client.GetHTTP().HTTP.Transport = loginTransport
		restoreTransport = func() {
			if loginTransport != nil {
				client.GetHTTP().HTTP.Transport = originalTransport
				loginTransport = nil
			}
		}
		defer restoreTransport()
	}
	return loginWithCookies(ctx, log, client, m.User, m.Main, loginCookies, restoreTransport)
}

func isClientHTTPError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "error from client: ")
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
