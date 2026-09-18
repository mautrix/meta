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
	"net/url"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

const (
	FlowIDInstagramPassword = "instagram-password"

	LoginStepIDCredentials  = "fi.mau.meta.instagram.credentials"
	LoginStepIDWebTwoFactor = "fi.mau.meta.instagram.web_two_factor"
	LoginStepIDWebChallenge = "fi.mau.meta.instagram.web_challenge"

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
		Cookies:                   c,
		Log:                       log,
		Settings:                  conn.Bridge.GetHTTPClientSettings(),
		DisableTyping:             conn.Config.DisableTyping,
		LogRedactedLoginResponses: conn.Config.LogRedactedLoginResponses,
		MobileLoginDevice:         loginDevice,
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

	client                 *instameow.Client
	caaClient              *instameow.Client
	transport              http.RoundTripper
	nativePush             bool
	caaIdentifier          string
	caaPassword            string
	caaUserID              string
	webTwoFactor           *instameow.InstagramWebTwoFactorChallenge
	webSessionReady        bool
	pendingWebChallengeURL string
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessCookies = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessWithParams = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessDisplayAndWait = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessStepCancel = (*MetaNativeLogin)(nil)

var errInstagramCAAUnsupportedStep = bridgev2.RespError{ErrCode: "FI.MAU.META_UNSUPPORTED_CAA_STEP", Err: "Instagram returned a sign-in step this bridge cannot safely complete", StatusCode: http.StatusBadRequest}
var errInstagramCAAFlowFailed = bridgev2.RespError{ErrCode: "FI.MAU.META_CAA_FAILED", Err: "Instagram couldn't complete this sign-in step. Try again.", StatusCode: http.StatusBadGateway, CanRetry: true}
var errInstagramWebCheckpointUnsupported = bridgev2.RespError{ErrCode: "FI.MAU.META_UNSUPPORTED_WEB_CHECKPOINT", Err: "Instagram returned a verification step this bridge cannot safely complete. Finish it in Instagram, then start a new login.", StatusCode: http.StatusBadRequest}
var errInstagramWebCheckpointCAPTCHA = bridgev2.RespError{ErrCode: "FI.MAU.META_WEB_CHECKPOINT_CAPTCHA", Err: "Instagram requires an interactive CAPTCHA check. This login flow cannot display that check yet.", StatusCode: http.StatusBadRequest}

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.StartWithParams(ctx, bridgev2.LoginStartParams{})
}

func (m *MetaNativeLogin) StartWithParams(
	ctx context.Context,
	params bridgev2.LoginStartParams,
) (*bridgev2.LoginStep, error) {
	m.transport = params.HTTP
	if m.Main != nil && m.Main.Bridge != nil {
		_, m.nativePush = m.Main.Bridge.Matrix.(bridgev2.MatrixConnectorWithNotifications)
	}
	return m.start(ctx, "Enter your Instagram email or username and password.")
}

func (m *MetaNativeLogin) start(ctx context.Context, instructions string) (*bridgev2.LoginStep, error) {
	m.clearCAAFallback()
	m.client = nil
	m.webTwoFactor = nil
	m.webSessionReady = false
	m.pendingWebChallengeURL = ""
	if m.User == nil || m.Main == nil {
		return nil, errors.New("instagram login is not initialized")
	}
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	log := m.User.Log.With().Str("component", "instagram_login").Logger()
	log.Debug().Bool("client_http", m.transport != nil).Bool("native_push", m.nativePush).Msg("Starting Instagram password login flow")
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
	m.clearCAAFallback()
	m.client = nil
	m.webTwoFactor = nil
	m.webSessionReady = false
	m.pendingWebChallengeURL = ""
	m.transport = nil
}

func (m *MetaNativeLogin) clearCAAFallback() {
	if m.caaClient != nil {
		m.caaClient.ClearInstagramCAALoginState()
		m.caaClient = nil
	}
	m.caaIdentifier, m.caaPassword, m.caaUserID = "", "", ""
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
	if m.caaClient != nil {
		return m.continueCAAFallback(ctx, input)
	}
	if m.webSessionReady {
		return m.continueWebAccountManager(ctx, input)
	}
	if m.webTwoFactor != nil && m.webTwoFactor.AuthPlatform {
		return m.continueWebAuthPlatform(ctx, input)
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
			if isClientHTTPError(err) || errors.Is(err, instameow.ErrInstagramWebCheckpointRequestFailed) {
				m.User.Log.Warn().Err(err).Msg("Instagram web two-factor request failed on the client")
				return instagramWebTwoFactorStep(
					m.webTwoFactor,
					"The request did not complete on this device. Enter a fresh verification code and try again.",
				), nil
			} else if errors.Is(err, instameow.ErrInstagramWebCheckpointUnsupported) {
				return nil, errInstagramWebCheckpointUnsupported
			} else if errors.Is(err, instameow.ErrInstagramWebTwoFactorCodeResent) {
				return instagramWebTwoFactorStep(
					m.webTwoFactor,
					"Instagram rejected that code. A fresh SMS code was requested. Enter it when it arrives.",
				), nil
			} else if errors.Is(err, instameow.ErrInstagramWebTwoFactorCodeRejected) {
				return instagramWebTwoFactorStep(
					m.webTwoFactor,
					"Instagram did not accept that code. Check that it is the latest code from Instagram, then try again.",
				), nil
			} else if errors.Is(err, httpclient.ErrRateLimited) {
				return nil, loginerrors.WithMessage(loginerrors.RateLimited, "Instagram is temporarily limiting verification attempts. Wait a while before starting a new login.")
			} else if errors.Is(err, httpclient.ErrAccountSuspended) {
				return nil, loginerrors.AccountSuspended
			}
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
	m.clearCAAFallback()
	if m.nativePush {
		m.caaIdentifier, m.caaPassword = identifier, password
	}
	return m.submitWebCredentials(ctx, identifier, password, true)
}

func (m *MetaNativeLogin) submitWebCredentials(
	ctx context.Context,
	identifier, password string,
	allowCAAFallback bool,
) (*bridgev2.LoginStep, error) {
	if allowCAAFallback {
		m.client.SetInstagramNativeSession(nil)
	}
	challenge, err := m.client.CreateInstagramWebSession(ctx, identifier, password)
	if errors.Is(err, instameow.ErrInstagramWebCookieConsentRequired) {
		challenge, err = m.client.ContinueInstagramWebSessionAfterCookieConsent(ctx, identifier, password)
	}
	if err != nil {
		if isClientHTTPError(err) || errors.Is(err, instameow.ErrInstagramWebCheckpointRequestFailed) {
			m.User.Log.Warn().Err(err).Msg("Instagram web login request failed on the client")
			return m.start(ctx, "The request did not complete on this device. Please try again.")
		} else if errors.Is(err, instameow.ErrInstagramWebCheckpointUnsupported) {
			return nil, errInstagramWebCheckpointUnsupported
		} else if errors.Is(err, instameow.ErrInstagramWebCheckpointCAPTCHA) {
			return nil, errInstagramWebCheckpointCAPTCHA
		} else if errors.Is(err, instameow.ErrInstagramWebLoginRejected) {
			m.clearCAAFallback()
			return instagramCredentialsStep("Instagram couldn't sign you in. Check your account in Instagram before trying again."), nil
		} else if errors.Is(err, instameow.ErrInstagramWebCredentialsRejected) {
			m.clearCAAFallback()
			return instagramCredentialsStep(
				"Instagram didn't accept that username or password. Check your credentials and try again.",
			), nil
		} else if errors.Is(err, httpclient.ErrRateLimited) {
			return nil, loginerrors.WithMessage(loginerrors.RateLimited, "Instagram is temporarily limiting login attempts. Wait a while before starting a new login.")
		} else if errors.Is(err, httpclient.ErrAccountSuspended) {
			return nil, loginerrors.AccountSuspended
		} else if errors.Is(err, instameow.ErrInstagramWebAccountPendingDeletion) {
			return nil, bridgev2.RespError{ErrCode: "FI.MAU.META_ACCOUNT_PENDING_DELETION", Err: "Instagram reports that this account is scheduled for deletion. Open Instagram to review the deletion request before starting a new login.", StatusCode: http.StatusForbidden}
		} else if errors.Is(err, httpclient.ErrChallengeRequired) || errors.Is(err, httpclient.ErrCheckpointRequired) {
			if !allowCAAFallback {
				return nil, errInstagramCAAFlowFailed
			}
			m.caaClient = m.client
			m.caaIdentifier = identifier
			m.caaPassword = password
			m.caaUserID = m.client.GetCookies().Get(cookies.IGCookieDSUserID)
			return m.continueCAAFallback(ctx, map[string]string{
				loginFieldIdentifier: identifier,
				loginFieldPassword:   password,
			})
		} else if isMissingInstagramWebTwoFactorCSRF(err) {
			return m.start(ctx, "Instagram did not return the security state needed to continue. Please try again.")
		}
		return nil, fmt.Errorf("failed to create Instagram web session: %w", err)
	}
	if challenge != nil {
		m.webTwoFactor = challenge
		if challenge.ChallengeURL != "" {
			return m.instagramWebChallengeStep(challenge.ChallengeURL), nil
		}
		if challenge.AuthPlatform {
			return m.continueWebAuthPlatform(ctx, nil)
		}
		return instagramWebTwoFactorStep(challenge, ""), nil
	}
	m.webSessionReady = true
	return m.continueWebAccountManager(ctx, map[string]string{})
}

func (m *MetaNativeLogin) CancelStep(ctx context.Context) (*bridgev2.LoginStep, error) {
	if m.caaClient != nil {
		if err := m.caaClient.CancelInstagramCAALoginStep(ctx); err != nil {
			return nil, err
		}
		return m.continueCAAFallback(ctx, nil)
	}
	if m.client != nil && m.client.HasInstagramWebCaptcha() {
		m.Cancel()
		return nil, bridgev2.ErrLoginStepCancelled
	}
	if m.client == nil || m.webTwoFactor == nil || !m.webTwoFactor.AuthPlatform {
		return nil, bridgev2.ErrLoginStepCancelled
	}
	return m.continueWebAuthPlatform(ctx, map[string]string{"back": "true"})
}

func (m *MetaNativeLogin) continueWebAuthPlatform(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	step, err := m.client.DoInstagramWebAuthPlatformSteps(ctx, input)
	return m.handleWebAuthPlatformResult(ctx, step, err)
}

// instagramWebChallengeStep hands a verification the bridge cannot drive itself to
// a client webview. The client loads the trusted instagram.com challenge URL,
// carries the session forward, and submits the resulting cookies once it lands on
// a logged-in URL.
func (m *MetaNativeLogin) instagramWebChallengeStep(challengeURL string) *bridgev2.LoginStep {
	m.pendingWebChallengeURL = challengeURL
	if m.Main.Config.LogRedactedLoginResponses {
		// The URL carries verification tokens, so it is logged only under the
		// redacted-login-response debug flag, for reproduction.
		m.User.Log.Debug().Str("challenge_url", challengeURL).Msg("Handing Instagram web challenge to client webview")
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeCookies,
		StepID:       LoginStepIDWebChallenge,
		Instructions: "Instagram needs you to finish a verification step. Complete it in the browser (you may need to sign in again), then your login will continue here.",
		CookiesParams: &bridgev2.LoginCookiesParams{
			URL: challengeURL,
			Fields: append(
				cookieListToFields(cookies.IGRequiredCookies, "instagram.com", true),
				cookieListToFields(cookies.IGOptionalCookies, "instagram.com", false)...,
			),
			WaitForURLPattern: instagramWebLoggedInURLPattern,
		},
	}
}

// SubmitCookies handles the two webview handoffs of the native login: the
// web-challenge step submits the session cookies collected after the user
// completed the verification, and the CAPTCHA step submits a solved token.
func (m *MetaNativeLogin) SubmitCookies(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if m.pendingWebChallengeURL != "" {
		if !m.nativePush {
			step, err := submitInstagramCookies(ctx, m.Main, m.User, input, m.client.GetInstagramNativeSession(), false)
			if err == nil {
				m.pendingWebChallengeURL = ""
			}
			return step, err
		}
		if m.client == nil {
			return nil, errInstagramCAAFlowFailed
		}
		values := make(map[cookies.MetaCookieName]string, len(input))
		for key, value := range input {
			values[cookies.MetaCookieName(key)] = value
		}
		m.client.GetCookies().UpdateValues(values)
		m.client.GetCookies().IGWWWClaim = ""
		m.pendingWebChallengeURL = ""
		m.webTwoFactor = nil
		m.webSessionReady = true
		return m.complete(ctx)
	}
	if m.client == nil || !m.client.HasInstagramWebCaptcha() {
		return nil, errInstagramWebCheckpointUnsupported
	}
	step, err := m.client.SubmitInstagramWebCaptcha(ctx, input["captcha_token"])
	return m.handleWebAuthPlatformResult(ctx, step, err)
}

func (m *MetaNativeLogin) handleWebAuthPlatformResult(ctx context.Context, step *bridgev2.LoginStep, err error) (*bridgev2.LoginStep, error) {
	if errors.Is(err, instameow.ErrInstagramWebLoginRejected) {
		m.clearCAAFallback()
		m.webTwoFactor = nil
		return instagramCredentialsStep("Instagram couldn't sign you in. Check your account in Instagram before trying again."), nil
	} else if errors.Is(err, instameow.ErrInstagramWebCheckpointCAPTCHA) {
		return nil, errInstagramWebCheckpointCAPTCHA
	} else if errors.Is(err, httpclient.ErrRateLimited) {
		return nil, loginerrors.WithMessage(loginerrors.RateLimited, "Instagram is temporarily limiting verification attempts. Wait a while before starting a new login.")
	} else if errors.Is(err, httpclient.ErrAccountSuspended) {
		return nil, loginerrors.AccountSuspended
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	} else if errors.Is(err, instameow.ErrInstagramWebCheckpointRequestFailed) {
		return nil, bridgev2.RespError{ErrCode: "FI.MAU.META_WEB_CHECKPOINT_FAILED", Err: "The Instagram verification request did not complete. Start a new login when your connection is stable.", StatusCode: http.StatusBadGateway}
	} else if err != nil {
		return nil, errInstagramWebCheckpointUnsupported
	} else if step != nil {
		return step, nil
	}
	m.webTwoFactor, m.webSessionReady = nil, true
	return m.continueWebAccountManager(ctx, nil)
}

func (m *MetaNativeLogin) continueCAAFallback(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if password := input[loginFieldPassword]; password != "" {
		m.caaPassword = password
	}
	step, err := m.caaClient.DoInstagramCAALoginStepsExactAccount(ctx, input, m.caaIdentifier, m.caaUserID)
	if errors.Is(err, bridgev2.ErrLoginStepCancelled) {
		return nil, err
	} else if err != nil {
		m.clearCAAFallback()
		if errors.Is(err, httpclient.ErrRateLimited) {
			return nil, loginerrors.RateLimited
		} else if errors.Is(err, httpclient.ErrAccountSuspended) {
			return nil, loginerrors.AccountSuspended
		} else if isClientHTTPError(err) {
			m.User.Log.Warn().Msg("Instagram CAA login request failed on the client")
			if !m.webSessionReady {
				return m.start(ctx, "The request did not complete on this device. Please try again.")
			}
			return nil, errInstagramCAAFlowFailed
		} else if errors.Is(err, instameow.ErrInstagramCAAUnsafeAccountStep) {
			return nil, errInstagramCAAUnsupportedStep
		}
		var responseError bridgev2.RespError
		if errors.As(err, &responseError) ||
			errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errInstagramCAAFlowFailed
	} else if step != nil {
		return step, nil
	}
	identifier, password := m.caaIdentifier, m.caaPassword
	nativeSession := m.caaClient.GetInstagramNativeSession()
	m.clearCAAFallback()
	if m.nativePush && (nativeSession == nil || nativeSession.Authorization == "" || nativeSession.UserID == "") {
		return nil, errInstagramCAAFlowFailed
	}
	m.client.SetInstagramNativeSession(nativeSession)
	if m.webSessionReady {
		return m.complete(ctx)
	}
	return m.submitWebCredentials(ctx, identifier, password, false)
}

func (m *MetaNativeLogin) Wait(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.SubmitUserInput(ctx, map[string]string{})
}

func (m *MetaNativeLogin) continueWebAccountManager(
	ctx context.Context,
	input map[string]string,
) (*bridgev2.LoginStep, error) {
	step, err := m.client.DoInstagramWebAccountManagerSteps(ctx, input)
	if errors.Is(err, httpclient.ErrConsentRequired) {
		return nil, loginerrors.Consent
	} else if errors.Is(err, httpclient.ErrRateLimited) {
		return nil, loginerrors.RateLimited
	} else if errors.Is(err, httpclient.ErrAccountSuspended) {
		return nil, loginerrors.AccountSuspended
	} else if err != nil {
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
	if m.nativePush && m.client.GetInstagramNativeSession() == nil {
		user, _, err := m.client.LoadIndex(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to verify Instagram messaging profile before notification setup: %w", err)
		}
		if user.ID == "" || user.ID != loginCookies.Get(cookies.IGCookieDSUserID) || user.GetUsername() == "" || m.caaPassword == "" {
			return nil, errInstagramCAAFlowFailed
		}
		nativeCookies := &cookies.Cookies{Platform: types.Instagram}
		nativeCookies.UpdateValues(nil)
		var userID id.UserID
		if m.User.User != nil {
			userID = m.User.MXID
		}
		m.caaClient, err = getInstaNativeClient(ctx, log, m.Main, nativeCookies, userID, m.Main.Config.ProxyOther, m.transport)
		if err != nil {
			return nil, err
		}
		m.caaIdentifier, m.caaUserID = user.GetUsername(), user.ID
		m.webSessionReady = true
		return m.continueCAAFallback(ctx, map[string]string{
			loginFieldIdentifier: m.caaIdentifier,
			loginFieldPassword:   m.caaPassword,
		})
	}
	m.clearCAAFallback()
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
	step, err := loginWithCookies(ctx, log, client, m.User, m.Main, loginCookies, m.client.GetInstagramNativeSession(), m.nativePush, restoreTransport)
	var requestErr *url.Error
	if ctx.Err() == nil && isClientHTTPError(err) && errors.As(err, &requestErr) &&
		requestErr.Op == "Get" && requestErr.URL == client.GetEndpoint("messages") {
		m.transport = loginTransport
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeUserInput,
			StepID:       "fi.mau.meta.instagram.inbox_retry",
			Instructions: "Your device couldn't load the Instagram inbox. Check your connection and retry to finish signing in.",
			UserInputParams: &bridgev2.LoginUserInputParams{Fields: []bridgev2.LoginInputDataField{{
				Type: bridgev2.LoginInputFieldTypeSelect, ID: "retry", Name: "Finish signing in",
				Options: []string{"Retry loading inbox"},
			}}},
		}, nil
	}
	return step, err
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
		case challenge != nil && challenge.Email:
			instructions = "Enter the verification code Instagram sent to your email."
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
