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
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
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

type instagramCAASubmitter func(
	context.Context,
	*instameow.Client,
	map[string]string,
) (*bridgev2.LoginStep, error)

type instagramLoginCompleter func(
	context.Context,
	zerolog.Logger,
	*instameow.Client,
	*bridgev2.User,
	*IGConnector,
	*cookies.Cookies,
) (*bridgev2.LoginStep, error)

type instagramLoginClientFactory func(
	context.Context,
	zerolog.Logger,
	*IGConnector,
	*cookies.Cookies,
	bool,
) (*instameow.Client, error)

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

	submitCAA instagramCAASubmitter
	complete  instagramLoginCompleter
	newClient instagramLoginClientFactory
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	if m.User == nil || m.Main == nil {
		return nil, errors.New("instagram login is not initialized")
	}
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	log := m.User.Log.With().Str("component", "instagram_login").Logger()
	factory := m.newClient
	if factory == nil {
		factory = getInstaNativeClient
	}
	client, err := factory(ctx, log, m.Main, loginCookies, m.Main.Config.ProxyOther)
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

	submit := m.submitCAA
	if submit == nil {
		submit = func(
			ctx context.Context,
			client *instameow.Client,
			userInput map[string]string,
		) (*bridgev2.LoginStep, error) {
			return client.DoInstagramCAALoginSteps(ctx, userInput)
		}
	}
	step, err := submit(ctx, m.client, input)
	if err != nil {
		return nil, fmt.Errorf("failed to log in to Instagram through CAA: %w", err)
	}
	if step != nil {
		return step, nil
	}
	return m.completeLogin(ctx)
}

func (m *MetaNativeLogin) completeLogin(ctx context.Context) (*bridgev2.LoginStep, error) {
	complete := m.complete
	if complete == nil {
		complete = completeInstagramNativeLogin
	}
	log := m.User.Log.With().Str("component", "instameow").Logger()
	return complete(ctx, log, m.client, m.User, m.Main, m.client.GetCookies())
}

func completeInstagramNativeLogin(
	ctx context.Context,
	log zerolog.Logger,
	client *instameow.Client,
	bridgeUser *bridgev2.User,
	_ *IGConnector,
	c *cookies.Cookies,
) (*bridgev2.LoginStep, error) {
	mobileSession := client.GetMobileSession()
	if mobileSession == nil {
		return nil, errors.New("instagram native login did not return a mobile session")
	}
	userID, err := strconv.ParseInt(mobileSession.UserID, 10, 64)
	if err != nil || userID <= 0 {
		return nil, errors.New("instagram mobile login returned an invalid user ID")
	}
	remoteName := mobileSession.Username
	if remoteName == "" {
		remoteName = "Instagram"
	}
	loginID := metaid.MakeUserLoginID(userID)
	ul, err := bridgeUser.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: remoteName,
		RemoteProfile: status.RemoteProfile{
			Name:     remoteName,
			Username: mobileSession.Username,
		},
		Metadata: &metaid.UserLoginMetadata{
			Platform:      c.Platform,
			Cookies:       c,
			MobileSession: mobileSession,
			IGID:          mobileSession.UserID,
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to save Instagram mobile login: %w", err)
	}

	igClient := ul.Client.(*IGClient)
	client.SetLogger(ul.Log.With().Str("component", "instameow").Logger())
	client.SetEventHandler(igClient.handleIGEvent)
	igClient.Client = client

	backgroundCtx := ul.Log.WithContext(igClient.Main.Bridge.BackgroundCtx)
	go igClient.Connect(backgroundCtx)
	log.Info().Msg("Completed native Instagram mobile login")
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepIDComplete,
		Instructions: fmt.Sprintf("Logged in as %s (%s)", remoteName, ul.ID),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
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
