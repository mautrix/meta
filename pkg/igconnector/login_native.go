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

	LoginStepIDCredentials  = "fi.mau.meta.instagram.credentials"
	LoginStepIDWebTwoFactor = "fi.mau.meta.instagram.web_two_factor"

	loginFieldIdentifier       = "username"
	loginFieldPassword         = "password"
	loginFieldWebTwoFactorCode = "verification_code"
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

	client          *instameow.Client
	transport       http.RoundTripper
	webTwoFactor    *instameow.InstagramWebTwoFactorChallenge
	webSessionReady bool
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
	m.webTwoFactor = nil
	m.webSessionReady = false
	if m.User == nil || m.Main == nil {
		return nil, errors.New("instagram login is not initialized")
	}
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	log := m.User.Log.With().Str("component", "instagram_login").Logger()
	log.Debug().Bool("client_http", m.transport != nil).Msg("Starting Instagram web password login flow")
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
	m.webTwoFactor = nil
	m.webSessionReady = false
	m.transport = nil
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
	if m.webSessionReady {
		return m.continueWebAccountManager(ctx, input)
	}
	if m.webTwoFactor != nil {
		verificationCode := strings.TrimSpace(input[loginFieldWebTwoFactorCode])
		if verificationCode == "" {
			return instagramWebTwoFactorStep(
				m.webTwoFactor,
				"Enter the verification code to continue creating the Instagram messaging session.",
			), nil
		}
		err := m.client.CompleteInstagramWebSessionTwoFactor(ctx, verificationCode)
		if err != nil {
			if isClientHTTPError(err) {
				m.User.Log.Warn().Err(err).Msg("Instagram web two-factor request failed on the client")
				return instagramWebTwoFactorStep(
					m.webTwoFactor,
					"The request did not complete on this device. Enter a fresh verification code and try again.",
				), nil
			} else if errors.Is(err, instameow.ErrInstagramWebTwoFactorCodeRejected) {
				return instagramWebTwoFactorStep(
					m.webTwoFactor,
					"Instagram did not accept that code. Enter a new code and try again.",
				), nil
			}
			m.Cancel()
			return nil, fmt.Errorf("failed to complete Instagram web two-factor login: %w", err)
		}
		m.webTwoFactor = nil
		m.webSessionReady = true
		return m.continueWebAccountManager(ctx, input)
	}

	identifier := strings.TrimSpace(input[loginFieldIdentifier])
	password := input[loginFieldPassword]
	if identifier == "" || password == "" {
		return instagramCredentialsStep(
			"Enter both your Instagram email or username and password.",
		), nil
	}
	challenge, err := m.client.CreateInstagramWebSession(ctx, identifier, password)
	if err != nil {
		if isClientHTTPError(err) {
			m.User.Log.Warn().Err(err).Msg("Instagram web login request failed on the client")
			return m.start(ctx, "The request did not complete on this device. Please try again.")
		} else if isMissingInstagramWebTwoFactorCSRF(err) {
			return m.start(ctx, "Instagram did not return the security state needed to continue. Please try again.")
		}
		m.Cancel()
		return nil, fmt.Errorf("failed to create Instagram web session: %w", err)
	}
	if challenge != nil {
		m.webTwoFactor = challenge
		return instagramWebTwoFactorStep(challenge, ""), nil
	}
	m.webSessionReady = true
	return m.continueWebAccountManager(ctx, input)
}

func (m *MetaNativeLogin) continueWebAccountManager(
	ctx context.Context,
	input map[string]string,
) (*bridgev2.LoginStep, error) {
	step, err := m.client.DoInstagramWebAccountManagerSteps(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to select Instagram web Account Manager profile: %w", err)
	} else if step != nil {
		return step, nil
	}
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

func isMissingInstagramWebTwoFactorCSRF(err error) bool {
	return err != nil && strings.Contains(err.Error(), "instagram web two-factor challenge is missing a CSRF token")
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

func instagramWebTwoFactorStep(
	challenge *instameow.InstagramWebTwoFactorChallenge,
	instructions string,
) *bridgev2.LoginStep {
	if instructions == "" {
		switch {
		case challenge != nil && challenge.TOTP:
			instructions = "Enter the verification code from your authenticator app."
		case challenge != nil && (challenge.SMS || challenge.WhatsApp):
			instructions = "Enter the verification code Instagram sent to you."
		default:
			instructions = "Enter your Instagram verification code."
		}
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       LoginStepIDWebTwoFactor,
		Instructions: instructions,
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:        bridgev2.LoginInputFieldType2FACode,
					ID:          loginFieldWebTwoFactorCode,
					Name:        "Verification code",
					Description: "The verification code for your Instagram account.",
				},
			},
		},
	}
}
