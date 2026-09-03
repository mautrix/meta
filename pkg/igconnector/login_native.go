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
	"sync"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
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
	caaFallback     bool
	caaIdentifier   string
	caaPassword     string
	caaUserID       string
	webTwoFactor    *instameow.InstagramWebTwoFactorChallenge
	webSessionReady bool

	stateMu       sync.Mutex
	cancelled     bool
	processCtx    context.Context
	processCancel context.CancelCauseFunc
	opDone        chan struct{}
	commitMu      sync.Mutex
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessWithParams = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessDisplayAndWait = (*MetaNativeLogin)(nil)

var errInstagramCAAUnsupportedStep = bridgev2.RespError{ErrCode: "FI.MAU.META_UNSUPPORTED_CAA_STEP", Err: "Instagram returned a sign-in step this bridge cannot safely complete", StatusCode: http.StatusBadRequest}
var errInstagramCAAFlowFailed = bridgev2.RespError{ErrCode: "FI.MAU.META_CAA_FAILED", Err: "Instagram couldn't complete this sign-in step. Try again.", StatusCode: http.StatusBadGateway, CanRetry: true}

func (m *MetaNativeLogin) runOperation(ctx context.Context, operation func(context.Context) (*bridgev2.LoginStep, error)) (*bridgev2.LoginStep, error) {
	return m.runOperationContext(ctx, false, operation)
}

func (m *MetaNativeLogin) runCancelableOperation(ctx context.Context, operation func(context.Context) (*bridgev2.LoginStep, error)) (*bridgev2.LoginStep, error) {
	return m.runOperationContext(ctx, true, operation)
}

func (m *MetaNativeLogin) runOperationContext(ctx context.Context, cancelWithCaller bool, operation func(context.Context) (*bridgev2.LoginStep, error)) (*bridgev2.LoginStep, error) {
	m.stateMu.Lock()
	if m.cancelled {
		m.stateMu.Unlock()
		return nil, bridgev2.ErrLoginStepCancelled
	} else if m.opDone != nil {
		m.stateMu.Unlock()
		return nil, errors.New("another Instagram login operation is already running")
	}
	if m.processCtx == nil {
		m.processCtx, m.processCancel = context.WithCancelCause(ctx)
	}
	processCtx := m.processCtx
	opCtx := processCtx
	var opCancel context.CancelCauseFunc
	var stopProcessCancel func() bool
	if cancelWithCaller {
		opCtx, opCancel = context.WithCancelCause(ctx)
		stopProcessCancel = context.AfterFunc(processCtx, func() {
			opCancel(context.Cause(processCtx))
		})
	}
	done := make(chan struct{})
	m.opDone = done
	m.stateMu.Unlock()
	defer func() {
		if stopProcessCancel != nil {
			stopProcessCancel()
			opCancel(nil)
		}
		m.stateMu.Lock()
		m.opDone = nil
		close(done)
		m.stateMu.Unlock()
	}()
	return operation(opCtx)
}

type instagramCAAStepSpec struct {
	instructions, fieldID, fieldName string
	fieldType                        bridgev2.LoginInputFieldType
}

type instagramWebSessionCreator func(context.Context, string, string) (*instameow.InstagramWebTwoFactorChallenge, error)
type instagramCAAStepper func(context.Context, map[string]string, string, string) (*bridgev2.LoginStep, error)

var instagramCAASafeSteps = map[string]instagramCAAStepSpec{
	"fi.mau.meta.instagram.caa.password":    {"Re-enter your Instagram password.", "password", "Password", bridgev2.LoginInputFieldTypePassword},
	"fi.mau.meta.instagram.caa.otp_code":    {"Enter the Instagram verification code.", "otp_code", "Verification code", bridgev2.LoginInputFieldType2FACode},
	"fi.mau.meta.instagram.caa.backup_code": {"Enter one of your Instagram backup codes.", "backup_code", "Backup code", bridgev2.LoginInputFieldType2FACode},
	"fi.mau.meta.instagram.caa.totp":        {"Enter the six-digit code from your authenticator app.", "totp_code", "Six-digit code", bridgev2.LoginInputFieldType2FACode},
	"fi.mau.meta.instagram.caa.sms":         {"Enter the SMS code Instagram sent.", "sms_code", "Six-digit code", bridgev2.LoginInputFieldType2FACode},
	"fi.mau.meta.instagram.caa.whatsapp":    {"Enter the code Instagram sent on WhatsApp.", "whatsapp_code", "Six-digit code", bridgev2.LoginInputFieldType2FACode},
}

func sanitizeInstagramCAALoginStep(step *bridgev2.LoginStep) error {
	if step == nil {
		return nil
	}
	if step.CookiesParams != nil || step.ClientHTTPParams != nil || step.WebAuthnParams != nil || step.CompleteParams != nil {
		return errInstagramCAAUnsupportedStep
	}
	if step.Type == bridgev2.LoginStepTypeDisplayAndWait {
		if step.StepID != "fi.mau.meta.instagram.caa.afad_wait" || step.UserInputParams != nil ||
			step.DisplayAndWaitParams == nil || step.DisplayAndWaitParams.Type != bridgev2.LoginDisplayTypeNothing ||
			step.DisplayAndWaitParams.Data != "" || step.DisplayAndWaitParams.ImageURL != "" {
			return errInstagramCAAUnsupportedStep
		}
		step.Instructions, step.DisplayAndWaitParams.CanCancel = "Approve this sign-in from an Instagram notification.", false
		return nil
	}
	spec, ok := instagramCAASafeSteps[step.StepID]
	if !ok || step.Type != bridgev2.LoginStepTypeUserInput || step.DisplayAndWaitParams != nil ||
		step.UserInputParams == nil || len(step.UserInputParams.Attachments) != 0 || len(step.UserInputParams.Fields) != 1 {
		return errInstagramCAAUnsupportedStep
	}
	field := &step.UserInputParams.Fields[0]
	if field.ID != spec.fieldID || field.Type != spec.fieldType || field.DefaultValue != "" || len(field.Options) != 0 {
		return errInstagramCAAUnsupportedStep
	}
	step.Instructions, field.Name, field.Description, field.Pattern = spec.instructions, spec.fieldName, "", ""
	step.UserInputParams.CanCancel = false
	return nil
}

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.StartWithParams(ctx, bridgev2.LoginStartParams{})
}

func (m *MetaNativeLogin) StartWithParams(
	ctx context.Context,
	params bridgev2.LoginStartParams,
) (*bridgev2.LoginStep, error) {
	return m.runOperation(ctx, func(ctx context.Context) (*bridgev2.LoginStep, error) {
		m.transport = params.HTTP
		return m.start(ctx, "Enter your Instagram email or username and password.")
	})
}

func (m *MetaNativeLogin) start(ctx context.Context, instructions string) (*bridgev2.LoginStep, error) {
	m.clearCAAFallback()
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
	m.commitMu.Lock()
	m.stateMu.Lock()
	m.cancelled = true
	cancel, done := m.processCancel, m.opDone
	m.stateMu.Unlock()
	if cancel != nil {
		cancel(bridgev2.ErrLoginStepCancelled)
	}
	m.commitMu.Unlock()
	if done != nil {
		<-done
	}
	m.stateMu.Lock()
	m.clearCAAFallback()
	m.client = nil
	m.webTwoFactor = nil
	m.webSessionReady = false
	m.transport = nil
	m.processCtx, m.processCancel = nil, nil
	m.stateMu.Unlock()
}

func (m *MetaNativeLogin) clearCAAFallback() {
	if m.client != nil {
		m.client.ClearInstagramCAALoginState()
	}
	m.caaFallback = false
	m.caaIdentifier, m.caaPassword, m.caaUserID = "", "", ""
}

func (m *MetaNativeLogin) beginLoginCommit() (func(), error) {
	m.commitMu.Lock()
	m.stateMu.Lock()
	cancelled := m.cancelled || m.processCtx != nil && m.processCtx.Err() != nil
	m.stateMu.Unlock()
	if cancelled {
		m.commitMu.Unlock()
		return nil, bridgev2.ErrLoginStepCancelled
	}
	return m.commitMu.Unlock, nil
}

func (m *MetaNativeLogin) SubmitUserInput(
	ctx context.Context,
	input map[string]string,
) (*bridgev2.LoginStep, error) {
	return m.runOperation(ctx, func(ctx context.Context) (*bridgev2.LoginStep, error) {
		return m.submitUserInput(ctx, input)
	})
}

func (m *MetaNativeLogin) submitUserInput(
	ctx context.Context,
	input map[string]string,
) (*bridgev2.LoginStep, error) {
	if m.client == nil {
		return instagramCredentialsStep(
			"This Instagram login session expired. Start the login again.",
		), nil
	}
	if m.caaFallback {
		return m.continueCAAFallback(ctx, input)
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
	return m.submitWebCredentials(ctx, identifier, password, true, m.client.CreateInstagramWebSession)
}

func (m *MetaNativeLogin) submitWebCredentials(
	ctx context.Context,
	identifier, password string,
	allowCAAFallback bool,
	createWebSession instagramWebSessionCreator,
) (*bridgev2.LoginStep, error) {
	challenge, err := createWebSession(ctx, identifier, password)
	if err != nil {
		if isClientHTTPError(err) {
			m.User.Log.Warn().Err(err).Msg("Instagram web login request failed on the client")
			return m.start(ctx, "The request did not complete on this device. Please try again.")
		} else if errors.Is(err, instameow.ErrInstagramWebCredentialsRejected) {
			return instagramCredentialsStep(
				"Instagram didn't accept that username or password. Check your credentials and try again.",
			), nil
		} else if errors.Is(err, httpclient.ErrRateLimited) {
			return nil, loginerrors.WithMessage(loginerrors.RateLimited, "Instagram is temporarily limiting login attempts. Wait a while before starting a new login.")
		} else if errors.Is(err, httpclient.ErrAccountSuspended) {
			return nil, loginerrors.AccountSuspended
		} else if errors.Is(err, instameow.ErrInstagramWebChallengeRequired) ||
			errors.Is(err, httpclient.ErrChallengeRequired) || errors.Is(err, httpclient.ErrCheckpointRequired) {
			if !allowCAAFallback {
				return nil, errInstagramCAAFlowFailed
			}
			m.caaFallback = true
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
		return instagramWebTwoFactorStep(challenge, ""), nil
	}
	m.webSessionReady = true
	return m.continueWebAccountManager(ctx, map[string]string{})
}

func (m *MetaNativeLogin) continueCAAFallback(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	return m.continueCAAFallbackWith(ctx, input, m.client.DoInstagramCAALoginStepsExactAccount, m.client.CreateInstagramWebSession)
}

func (m *MetaNativeLogin) continueCAAFallbackWith(
	ctx context.Context,
	input map[string]string,
	doCAASteps instagramCAAStepper,
	createWebSession instagramWebSessionCreator,
) (*bridgev2.LoginStep, error) {
	if password := input[loginFieldPassword]; password != "" {
		m.caaPassword = password
	}
	step, err := doCAASteps(ctx, input, m.caaIdentifier, m.caaUserID)
	if err != nil {
		m.clearCAAFallback()
		if isClientHTTPError(err) {
			m.User.Log.Warn().Msg("Instagram CAA fallback request failed on the client")
			return m.start(ctx, "The request did not complete on this device. Please try again.")
		} else if errors.Is(err, instameow.ErrInstagramCAAUnsafeAccountStep) {
			return nil, errInstagramCAAUnsupportedStep
		}
		var responseError bridgev2.RespError
		if errors.As(err, &responseError) || errors.Is(err, bridgev2.ErrLoginStepCancelled) ||
			errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errInstagramCAAFlowFailed
	} else if err = sanitizeInstagramCAALoginStep(step); err != nil {
		m.clearCAAFallback()
		return nil, err
	} else if step != nil {
		return step, nil
	}
	return m.resumeWebLoginAfterCAA(ctx, createWebSession)
}

func (m *MetaNativeLogin) resumeWebLoginAfterCAA(
	ctx context.Context,
	createWebSession instagramWebSessionCreator,
) (*bridgev2.LoginStep, error) {
	identifier, password := m.caaIdentifier, m.caaPassword
	m.clearCAAFallback()
	return m.submitWebCredentials(ctx, identifier, password, false, createWebSession)
}

func (m *MetaNativeLogin) Wait(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.runCancelableOperation(ctx, func(ctx context.Context) (*bridgev2.LoginStep, error) {
		return m.submitUserInput(ctx, map[string]string{})
	})
}

func (m *MetaNativeLogin) continueWebAccountManager(
	ctx context.Context,
	input map[string]string,
) (*bridgev2.LoginStep, error) {
	step, err := m.client.DoInstagramWebAccountManagerSteps(ctx, input)
	if errors.Is(err, httpclient.ErrRateLimited) {
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
	m.stateMu.Lock()
	cancelled := m.cancelled
	m.stateMu.Unlock()
	if cancelled {
		return nil, bridgev2.ErrLoginStepCancelled
	}
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
	return loginWithCookies(ctx, log, client, m.User, m.Main, loginCookies, m.beginLoginCommit, restoreTransport)
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
