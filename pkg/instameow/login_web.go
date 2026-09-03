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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/google/go-querystring/query"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

const instagramWebTwoFactorValidateCodeDocID = "26264014419868193"

var ErrInstagramWebCredentialsRejected = errors.New("instagram web credentials were rejected")
var ErrInstagramWebChallengeRequired = errors.New("instagram web login requires a challenge")
var ErrInstagramWebTwoFactorCodeRejected = errors.New("instagram web two-factor code was rejected")
var ErrInstagramWebTwoFactorCodeResent = fmt.Errorf("%w: replacement SMS requested", ErrInstagramWebTwoFactorCodeRejected)
var errInstagramWebTwoFactorSMSRejected = fmt.Errorf("%w: SMS code validation failed", ErrInstagramWebTwoFactorCodeRejected)
var errInstagramWebTwoFactorSMSNotSent = errors.New("instagram could not send a replacement SMS code")
var errInstagramWebTwoFactorMissingCSRF = errors.New("instagram web two-factor challenge is missing a CSRF token")

type InstagramWebTwoFactorChallenge struct {
	TOTP     bool
	SMS      bool
	WhatsApp bool
}

type instagramWebTwoFactorInfo struct {
	Identifier        string `json:"two_factor_identifier"`
	Username          string `json:"username"`
	TOTP              bool   `json:"totp_two_factor_on"`
	SMS               bool   `json:"sms_two_factor_on"`
	WhatsApp          bool   `json:"whatsapp_two_factor_on"`
	EncryptedContext  string `json:"encrypted_context"`
	MaskedPhoneNumber string `json:"obfuscated_phone_number_2"`
}

type instagramWebTwoFactorState struct {
	identifier         string
	username           string
	encryptedContext   string
	maskedContactPoint string
	method             string
	csrfToken          string
	smsReplacementSent bool
}

type instagramWebLoginResponse struct {
	Authenticated     bool                      `json:"authenticated"`
	TwoFactorRequired bool                      `json:"two_factor_required"`
	Message           string                    `json:"message"`
	Status            string                    `json:"status"`
	ErrorType         string                    `json:"error_type"`
	CheckpointURL     string                    `json:"checkpoint_url"`
	RedirectURL       string                    `json:"redirect_url"`
	TwoFactorInfo     instagramWebTwoFactorInfo `json:"two_factor_info"`
}

func instagramWebSprinkleToken(csrfToken string, config types.SprinkleConfig) (string, error) {
	if csrfToken == "" {
		return "", errors.New("instagram web login is missing a CSRF token")
	} else if config.ParamName == "" {
		return "", errors.New("instagram web login page did not include sprinkle configuration")
	} else if !config.ShouldRandomize && config.Version <= 0 {
		return "", errors.New("instagram web login page included an invalid sprinkle version")
	}
	sum := 0
	for _, character := range csrfToken {
		sum += int(character)
	}
	token := strconv.Itoa(sum)
	if !config.ShouldRandomize {
		token = strconv.Itoa(config.Version) + token
	}
	return token, nil
}

func newInstagramWebLoginForm(
	identifier,
	encryptedPassword,
	csrfToken string,
	sprinkleConfig types.SprinkleConfig,
) (url.Values, error) {
	sprinkleToken, err := instagramWebSprinkleToken(csrfToken, sprinkleConfig)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"caaF2DebugGroup":             {"-1"},
		"enc_password":                {encryptedPassword},
		"isPrivacyPortalReq":          {"false"},
		"loginAttemptSubmissionCount": {"0"},
		"optIntoOneTap":               {"false"},
		"queryParams":                 {"{}"},
		"stopDeletionNonce":           {""},
		"trustedDeviceRecords":        {"{}"},
		"username":                    {identifier},
	}
	form.Set(sprinkleConfig.ParamName, sprinkleToken)
	return form, nil
}

func newInstagramWebTwoFactorForm(
	state *instagramWebTwoFactorState,
	verificationCode,
	csrfToken string,
	sprinkleConfig types.SprinkleConfig,
) (url.Values, error) {
	if state == nil || state.identifier == "" || state.username == "" {
		return nil, errors.New("instagram web two-factor challenge is missing state")
	}
	verificationMethod := map[string]string{"SMS": "1", "TOTP": "3"}[state.method]
	if verificationMethod == "" {
		return nil, errors.New("instagram web two-factor challenge has no verification method")
	}
	sprinkleToken, err := instagramWebSprinkleToken(csrfToken, sprinkleConfig)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"identifier":          {state.identifier},
		"queryParams":         {"{}"},
		"trust_signal":        {"true"},
		"username":            {state.username},
		"verificationCode":    {verificationCode},
		"verification_method": {verificationMethod},
	}
	form.Set(sprinkleConfig.ParamName, sprinkleToken)
	return form, nil
}

func newInstagramWebTwoFactorSMSForm(state *instagramWebTwoFactorState, csrfToken string, sprinkleConfig types.SprinkleConfig) (url.Values, error) {
	if state == nil || state.identifier == "" || state.username == "" || state.method != "SMS" {
		return nil, errors.New("instagram SMS two-factor challenge is missing state")
	}
	sprinkleToken, err := instagramWebSprinkleToken(csrfToken, sprinkleConfig)
	if err != nil {
		return nil, err
	}
	form := url.Values{"identifier": {state.identifier}, "username": {state.username}}
	form.Set(sprinkleConfig.ParamName, sprinkleToken)
	return form, nil
}

func (c *Client) addInstagramWebLoginHeaders(headers http.Header) error {
	if c.configs == nil || c.configs.BrowserConfigTable == nil {
		return errors.New("instagram web login page configuration is missing")
	}
	config := c.configs.BrowserConfigTable
	if config.InstagramWebPushInfo.RolloutHash == "" {
		return errors.New("instagram web login page did not include a rollout hash")
	} else if c.configs.WebSessionID == "" {
		return errors.New("instagram web login is missing a web session ID")
	} else if headers.Get("x-csrftoken") == "" {
		return errors.New("instagram web login is missing the CSRF header")
	} else if headers.Get("x-ig-app-id") == "" {
		return errors.New("instagram web login page did not include an app ID")
	}
	headers.Set("x-instagram-ajax", config.InstagramWebPushInfo.RolloutHash)
	headers.Set("x-web-session-id", c.configs.WebSessionID)
	if c.cookies.IGWWWClaim == "" {
		headers.Set("x-ig-www-claim", "0")
	} else {
		headers.Set("x-ig-www-claim", c.cookies.IGWWWClaim)
	}
	if config.PolarisSiteData.SendDeviceIDHeader {
		if config.PolarisSiteData.DeviceID == "" {
			return errors.New("instagram web login page requested an empty web device ID")
		}
		headers.Set("x-web-device-id", config.PolarisSiteData.DeviceID)
	}
	return nil
}

func instagramWebLoginResponseKind(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	switch {
	case len(trimmed) == 0:
		return "empty"
	case json.Valid(trimmed):
		return "json"
	case bytes.HasPrefix(trimmed, []byte("for (;;);")):
		return "prefixed_json"
	case trimmed[0] == '<':
		return "html"
	default:
		return "other"
	}
}

func instagramWebLoginResponseKeys(body []byte) []string {
	var response map[string]json.RawMessage
	if json.Unmarshal(body, &response) != nil {
		return nil
	}
	keys := make([]string, 0, len(response))
	for key := range response {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func instagramWebLoginResponseClass(result instagramWebLoginResponse) string {
	detail := strings.ToLower(result.Message + " " + result.ErrorType)
	switch {
	case result.TwoFactorRequired:
		return "two_factor_required"
	case result.CheckpointURL != "" || result.RedirectURL != "" ||
		strings.Contains(detail, "checkpoint") || strings.Contains(detail, "challenge"):
		return "challenge_required"
	case strings.Contains(detail, "feedback_required") || strings.Contains(detail, "rate") ||
		strings.Contains(detail, "wait") || strings.Contains(detail, "try again later"):
		return "temporarily_blocked"
	case strings.Contains(detail, "password") || strings.Contains(detail, "credential") ||
		strings.Contains(detail, "username") || strings.Contains(detail, "user_not_found"):
		return "credentials_rejected"
	case strings.Contains(detail, "consent"):
		return "consent_required"
	case strings.Contains(detail, "disabled") || strings.Contains(detail, "suspended"):
		return "account_restricted"
	case result.Message != "" || result.ErrorType != "":
		return "provider_error"
	default:
		return "unclassified"
	}
}

func instagramWebCredentialsRejected(result instagramWebLoginResponse) bool {
	return strings.EqualFold(strings.TrimSpace(result.ErrorType), "bad_password") &&
		instagramWebLoginResponseClass(result) == "credentials_rejected"
}

func instagramWebChallengeRequired(result instagramWebLoginResponse) bool {
	return instagramWebChallengeURL(result.CheckpointURL) || instagramWebChallengeURL(result.RedirectURL) ||
		instagramWebChallengeReason(result.ErrorType) || instagramWebChallengeReason(result.Message)
}

func instagramWebChallengeReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	return strings.EqualFold(reason, "checkpoint_required") || strings.EqualFold(reason, "challenge_required")
}

func instagramWebChallengeURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	trusted := parsed.Scheme == "" && parsed.Host == "" || parsed.Scheme == "https" &&
		(host == "instagram.com" || strings.HasSuffix(host, ".instagram.com") ||
			host == "facebook.com" || strings.HasSuffix(host, ".facebook.com"))
	if !trusted {
		return false
	}
	return strings.HasPrefix(parsed.Path, "/challenge/") || strings.HasPrefix(parsed.Path, "/checkpoint/")
}

func instagramWebFormFields(form url.Values) []string {
	fields := make([]string, 0, len(form))
	for field := range form {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func (c *Client) logInstagramWebRequestRejection(
	message string,
	requestErr error,
	response *http.Response,
	body []byte,
	headers http.Header,
	form url.Values,
	responseClass string,
) {
	logEvent := c.log.Warn().
		Err(requestErr).
		Int("response_bytes", len(body)).
		Str("response_kind", instagramWebLoginResponseKind(body)).
		Bool("csrf_header_present", headers.Get("x-csrftoken") != "").
		Bool("cookie_header_present", headers.Get("cookie") != "").
		Bool("instagram_ajax_header_present", headers.Get("x-instagram-ajax") != "").
		Bool("web_session_header_present", headers.Get("x-web-session-id") != "").
		Bool("web_device_header_present", headers.Get("x-web-device-id") != "").
		Str("response_class", responseClass).
		Strs("response_keys", instagramWebLoginResponseKeys(body)).
		Strs("form_fields", instagramWebFormFields(form))
	if response != nil {
		logEvent = logEvent.
			Int("status_code", response.StatusCode).
			Str("content_type", response.Header.Get("content-type"))
	}
	logEvent.Msg(message)
}

func (c *Client) captureInstagramWebTwoFactor(
	result instagramWebLoginResponse,
	fallbackUsername string,
	preResponseCSRFToken string,
	statusCode int,
) (*InstagramWebTwoFactorChallenge, error) {
	info := result.TwoFactorInfo
	username := info.Username
	if username == "" {
		username = fallbackUsername
	}
	if info.EncryptedContext == "" && (info.Identifier == "" || username == "") {
		return nil, errors.New("instagram web two-factor response is missing its challenge identifier")
	}
	method := ""
	switch {
	case info.TOTP:
		method = "TOTP"
	case info.SMS:
		method = "SMS"
	case info.WhatsApp:
		method = "WHATSAPP"
	}
	if info.EncryptedContext != "" && method == "" {
		return nil, errors.New("instagram web two-factor response uses an unsupported verification method")
	}
	maskedContactPoint := ""
	if method == "SMS" || method == "WHATSAPP" {
		maskedContactPoint = info.MaskedPhoneNumber
	}
	responseCSRFToken := c.cookies.Get(cookies.IGCookieCSRFToken)
	csrfToken := responseCSRFToken
	retainedCSRFToken := false
	if csrfToken == "" {
		csrfToken = preResponseCSRFToken
		retainedCSRFToken = csrfToken != ""
	}
	if csrfToken == "" {
		return nil, errInstagramWebTwoFactorMissingCSRF
	}
	c.webTwoFactor = &instagramWebTwoFactorState{
		identifier:         info.Identifier,
		username:           username,
		encryptedContext:   info.EncryptedContext,
		maskedContactPoint: maskedContactPoint,
		method:             method,
		csrfToken:          csrfToken,
	}
	c.log.Debug().
		Int("status_code", statusCode).
		Str("challenge_type", method).
		Bool("csrf_pre_response_present", preResponseCSRFToken != "").
		Bool("csrf_rotated", responseCSRFToken != "" && responseCSRFToken != preResponseCSRFToken).
		Bool("csrf_retained", retainedCSRFToken).
		Msg("Captured Instagram web two-factor challenge")
	return &InstagramWebTwoFactorChallenge{
		TOTP:     info.TOTP,
		SMS:      info.SMS,
		WhatsApp: info.WhatsApp,
	}, nil
}

// CreateInstagramWebSession creates a session that can be used by Instagram's
// web messaging APIs and realtime stream. A returned challenge must be
// completed with CompleteInstagramWebSessionTwoFactor.
func (c *Client) CreateInstagramWebSession(
	ctx context.Context,
	identifier,
	password string,
) (*InstagramWebTwoFactorChallenge, error) {
	if c == nil {
		return nil, ErrClientIsNil
	} else if identifier == "" || password == "" {
		return nil, errors.New("instagram web login is missing credentials")
	}
	c.webTwoFactor = nil
	c.webAccountManager = nil

	// Keep only stable device cookies between attempts. Loading the login page
	// below refreshes the remaining unauthenticated web-session state.
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
	loginPageURL := c.GetEndpoint("login")
	if err := moduleLoader.Load(ctx, loginPageURL); err != nil {
		return nil, fmt.Errorf("failed to load Instagram web login page: %w", err)
	}
	c.configs.Setup(false)

	encryption := c.configs.BrowserConfigTable.InstagramPasswordEncryption
	keyID, err := strconv.Atoi(encryption.KeyID)
	if err != nil || keyID <= 0 || encryption.PublicKey == "" {
		return nil, errors.New("instagram web login page did not include password encryption keys")
	}
	encryptedPassword, err := crypto.EncryptInstagramWebPassword(
		keyID,
		encryption.PublicKey,
		password,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt Instagram web password: %w", err)
	}

	form, err := newInstagramWebLoginForm(
		identifier,
		encryptedPassword,
		c.cookies.Get(cookies.IGCookieCSRFToken),
		c.configs.BrowserConfigTable.SprinkleConfig,
	)
	if err != nil {
		return nil, err
	}
	preResponseCSRFToken := c.cookies.Get(cookies.IGCookieCSRFToken)
	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", baseURL)
	headers.Set("referer", loginPageURL)
	headers.Set("x-requested-with", "XMLHttpRequest")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	if err = c.addInstagramWebLoginHeaders(headers); err != nil {
		return nil, err
	}
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		c.GetEndpoint("login_ajax"),
		http.MethodPost,
		headers,
		[]byte(form.Encode()),
		types.FORM,
	)
	if response != nil {
		c.cookies.UpdateFromResponse(response)
	}
	var result instagramWebLoginResponse
	parseErr := json.Unmarshal(body, &result)
	if requestErr != nil {
		c.logInstagramWebRequestRejection(
			"Instagram web login request was rejected",
			requestErr,
			response,
			body,
			headers,
			form,
			instagramWebLoginResponseClass(result),
		)
		if parseErr == nil && instagramWebCredentialsRejected(result) {
			return nil, ErrInstagramWebCredentialsRejected
		} else if parseErr == nil && result.TwoFactorRequired {
			statusCode := 0
			if response != nil {
				statusCode = response.StatusCode
			}
			return c.captureInstagramWebTwoFactor(result, identifier, preResponseCSRFToken, statusCode)
		} else if parseErr == nil && instagramWebChallengeRequired(result) {
			return nil, ErrInstagramWebChallengeRequired
		} else if parseErr == nil && result.Message != "" {
			return nil, fmt.Errorf("instagram web login failed: %s", result.Message)
		}
		return nil, fmt.Errorf("instagram web login request failed: %w", requestErr)
	} else if response == nil {
		return nil, errors.New("instagram web login returned no response")
	} else if parseErr != nil {
		return nil, fmt.Errorf("failed to parse Instagram web login: %w", parseErr)
	} else if instagramWebCredentialsRejected(result) {
		return nil, ErrInstagramWebCredentialsRejected
	} else if result.TwoFactorRequired {
		return c.captureInstagramWebTwoFactor(result, identifier, preResponseCSRFToken, response.StatusCode)
	} else if instagramWebChallengeRequired(result) {
		return nil, ErrInstagramWebChallengeRequired
	} else if !result.Authenticated {
		if result.Message != "" {
			return nil, fmt.Errorf("instagram web login failed: %s", result.Message)
		}
		return nil, errors.New("instagram web login did not authenticate")
	}
	c.ensureInstagramWebUserID()
	if missing := c.cookies.GetMissingCookieNames(); len(missing) > 0 {
		return nil, fmt.Errorf("instagram web login succeeded without required cookies: %v", missing)
	}
	return nil, nil
}

type instagramWebTwoFactorSensitiveCode struct {
	Value string `json:"sensitive_string_value"`
}

type instagramWebTwoFactorGraphQLVariables struct {
	Code               instagramWebTwoFactorSensitiveCode `json:"code"`
	EncryptedContext   string                             `json:"encryptedContext"`
	Flow               string                             `json:"flow"`
	MaskedContactPoint *string                            `json:"maskedContactPoint"`
	Method             string                             `json:"method"`
	NextURI            *string                            `json:"next_uri"`
	SharedPrefsData    *string                            `json:"shared_prefs_data"`
	TrustThisDevice    bool                               `json:"trust_this_device"`
}

type instagramWebTwoFactorGraphQLResponse struct {
	Data *struct {
		ValidateCode *struct {
			IsCodeValid bool `json:"is_code_valid"`
		} `json:"xfb_two_factor_login_validate_code"`
	} `json:"data"`
}

func (c *Client) newInstagramWebTwoFactorGraphQLForm(
	state *instagramWebTwoFactorState,
	verificationCode string,
) (url.Values, error) {
	if state == nil || state.encryptedContext == "" || state.method == "" {
		return nil, errors.New("instagram encrypted web two-factor challenge is missing state")
	}
	var maskedContactPoint *string
	if state.maskedContactPoint != "" {
		maskedContactPoint = &state.maskedContactPoint
	}
	variables, err := json.Marshal(instagramWebTwoFactorGraphQLVariables{
		Code: instagramWebTwoFactorSensitiveCode{
			Value: verificationCode,
		},
		EncryptedContext:   state.encryptedContext,
		Flow:               "TWO_FACTOR_LOGIN",
		MaskedContactPoint: maskedContactPoint,
		Method:             state.method,
		TrustThisDevice:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Instagram web two-factor request: %w", err)
	}
	requestQuery := c.http.NewHTTPQuery()
	requestQuery.FbAPICallerClass = "RelayModern"
	requestQuery.FbAPIReqFriendlyName = "useTwoFactorLoginValidateCodeMutation"
	requestQuery.Variables = string(variables)
	requestQuery.ServerTimestamps = "true"
	requestQuery.DocID = instagramWebTwoFactorValidateCodeDocID
	form, err := query.Values(requestQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to build Instagram web two-factor request: %w", err)
	}
	sprinkleToken, err := instagramWebSprinkleToken(
		c.cookies.Get(cookies.IGCookieCSRFToken),
		c.configs.BrowserConfigTable.SprinkleConfig,
	)
	if err != nil {
		return nil, err
	}
	form.Set(c.configs.BrowserConfigTable.SprinkleConfig.ParamName, sprinkleToken)
	return form, nil
}

func (c *Client) instagramWebTwoFactorHeaders(referer string) (http.Header, error) {
	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", c.GetEndpoint("base_url"))
	headers.Set("referer", referer)
	headers.Set("x-requested-with", "XMLHttpRequest")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	return headers, c.addInstagramWebLoginHeaders(headers)
}

func (c *Client) resendInstagramWebTwoFactorSMS(ctx context.Context, state *instagramWebTwoFactorState) error {
	form, err := newInstagramWebTwoFactorSMSForm(
		state,
		c.cookies.Get(cookies.IGCookieCSRFToken),
		c.configs.BrowserConfigTable.SprinkleConfig,
	)
	if err != nil {
		return err
	}
	headers, err := c.instagramWebTwoFactorHeaders(c.GetEndpoint("login_two_factor"))
	if err != nil {
		return err
	}
	response, body, requestErr := c.http.MakeRequestOnce(
		ctx,
		c.GetEndpoint("login_two_factor_sms"),
		http.MethodPost,
		headers,
		[]byte(form.Encode()),
		types.FORM,
	)
	if response != nil {
		c.cookies.UpdateFromResponse(response)
		if csrfToken := c.cookies.Get(cookies.IGCookieCSRFToken); csrfToken != "" {
			state.csrfToken = csrfToken
		}
	}
	var result instagramWebLoginResponse
	parseErr := json.Unmarshal(body, &result)
	if requestErr != nil {
		c.logInstagramWebRequestRejection(
			"Instagram web two-factor SMS request was rejected",
			requestErr,
			response,
			body,
			headers,
			form,
			instagramWebLoginResponseClass(result),
		)
		return fmt.Errorf("instagram web two-factor SMS request failed: %w", requestErr)
	} else if response == nil {
		return errors.New("instagram web two-factor SMS request returned no response")
	} else if parseErr != nil {
		return fmt.Errorf("failed to parse Instagram web two-factor SMS response: %w", parseErr)
	} else if !strings.EqualFold(result.Status, "ok") || result.TwoFactorInfo.Identifier == "" {
		return errInstagramWebTwoFactorSMSNotSent
	}
	state.identifier = result.TwoFactorInfo.Identifier
	state.smsReplacementSent = true
	return nil
}

func instagramWebTwoFactorSMSWasRejected(result instagramWebLoginResponse) bool {
	return strings.EqualFold(result.ErrorType, "sms_code_validation_failed") || strings.EqualFold(result.ErrorType, "invalid_verficaition_code")
}

func (c *Client) completeInstagramWebTwoFactorLegacy(
	ctx context.Context,
	state *instagramWebTwoFactorState,
	verificationCode string,
) error {
	form, err := newInstagramWebTwoFactorForm(
		state,
		verificationCode,
		c.cookies.Get(cookies.IGCookieCSRFToken),
		c.configs.BrowserConfigTable.SprinkleConfig,
	)
	if err != nil {
		return err
	}
	headers, err := c.instagramWebTwoFactorHeaders(c.GetEndpoint("login_two_factor"))
	if err != nil {
		return err
	}
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		c.GetEndpoint("login_two_factor_ajax"),
		http.MethodPost,
		headers,
		[]byte(form.Encode()),
		types.FORM,
	)
	if response != nil {
		c.cookies.UpdateFromResponse(response)
		if csrfToken := c.cookies.Get(cookies.IGCookieCSRFToken); csrfToken != "" {
			state.csrfToken = csrfToken
		}
	}
	var result instagramWebLoginResponse
	parseErr := json.Unmarshal(body, &result)
	if requestErr != nil {
		c.logInstagramWebRequestRejection(
			"Instagram web two-factor request was rejected",
			requestErr,
			response,
			body,
			headers,
			form,
			instagramWebLoginResponseClass(result),
		)
		if response != nil && response.StatusCode >= 400 && response.StatusCode < 500 && parseErr == nil {
			if instagramWebTwoFactorSMSWasRejected(result) {
				return errInstagramWebTwoFactorSMSRejected
			}
			return ErrInstagramWebTwoFactorCodeRejected
		}
		return fmt.Errorf("instagram web two-factor request failed: %w", requestErr)
	} else if response == nil {
		return errors.New("instagram web two-factor login returned no response")
	} else if parseErr != nil {
		return fmt.Errorf("failed to parse Instagram web two-factor login: %w", parseErr)
	} else if !result.Authenticated && !strings.EqualFold(result.Status, "ok") {
		if instagramWebTwoFactorSMSWasRejected(result) {
			return errInstagramWebTwoFactorSMSRejected
		}
		return ErrInstagramWebTwoFactorCodeRejected
	}
	return nil
}

func (c *Client) completeInstagramWebTwoFactorEncrypted(
	ctx context.Context,
	state *instagramWebTwoFactorState,
	verificationCode string,
) error {
	form, err := c.newInstagramWebTwoFactorGraphQLForm(state, verificationCode)
	if err != nil {
		return err
	}
	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", c.GetEndpoint("base_url"))
	headers.Set("referer", c.GetEndpoint("login_two_step_verification"))
	headers.Set("x-fb-friendly-name", "useTwoFactorLoginValidateCodeMutation")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	if err = c.addInstagramWebLoginHeaders(headers); err != nil {
		return err
	}
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		c.GetEndpoint("graphql"),
		http.MethodPost,
		headers,
		[]byte(form.Encode()),
		types.FORM,
	)
	if response != nil {
		c.cookies.UpdateFromResponse(response)
		if csrfToken := c.cookies.Get(cookies.IGCookieCSRFToken); csrfToken != "" {
			state.csrfToken = csrfToken
		}
	}
	body = bytes.TrimPrefix(body, httpclient.AntiJSPrefix)
	var result instagramWebTwoFactorGraphQLResponse
	parseErr := json.Unmarshal(body, &result)
	if requestErr != nil {
		c.logInstagramWebRequestRejection(
			"Instagram encrypted web two-factor request was rejected",
			requestErr,
			response,
			body,
			headers,
			form,
			"two_factor_code_rejected",
		)
		if response != nil && response.StatusCode >= 400 && response.StatusCode < 500 && parseErr == nil {
			return ErrInstagramWebTwoFactorCodeRejected
		}
		return fmt.Errorf("instagram encrypted web two-factor request failed: %w", requestErr)
	} else if response == nil {
		return errors.New("instagram encrypted web two-factor login returned no response")
	} else if parseErr != nil {
		return fmt.Errorf("failed to parse Instagram encrypted web two-factor login: %w", parseErr)
	} else if result.Data == nil || result.Data.ValidateCode == nil || !result.Data.ValidateCode.IsCodeValid {
		return ErrInstagramWebTwoFactorCodeRejected
	}
	return nil
}

// CompleteInstagramWebSessionTwoFactor finishes a challenge returned by
// CreateInstagramWebSession. Challenge identifiers and codes remain in memory
// for the lifetime of the Bridgev2 login process and are never logged.
func (c *Client) CompleteInstagramWebSessionTwoFactor(
	ctx context.Context,
	verificationCode string,
) error {
	if c == nil {
		return ErrClientIsNil
	} else if c.webTwoFactor == nil {
		return errors.New("instagram web two-factor challenge is not active")
	}
	verificationCode = strings.TrimSpace(verificationCode)
	if verificationCode == "" {
		return errors.New("instagram web two-factor code is empty")
	}
	state := c.webTwoFactor
	if state.csrfToken == "" {
		return errInstagramWebTwoFactorMissingCSRF
	}
	c.cookies.Set(cookies.IGCookieCSRFToken, state.csrfToken)
	headers := c.http.BuildHeaders(true, false)
	c.log.Debug().
		Bool("csrf_cookie_present", c.cookies.Get(cookies.IGCookieCSRFToken) != "").
		Bool("cookie_header_present", headers.Get("cookie") != "").
		Bool("csrf_header_present", headers.Get("x-csrftoken") != "").
		Msg("Prepared Instagram web two-factor CSRF state")
	var err error
	if state.encryptedContext == "" {
		err = c.completeInstagramWebTwoFactorLegacy(ctx, state, verificationCode)
	} else {
		err = c.completeInstagramWebTwoFactorEncrypted(ctx, state, verificationCode)
	}
	if err != nil {
		if errors.Is(err, errInstagramWebTwoFactorSMSRejected) && !state.smsReplacementSent &&
			state.encryptedContext == "" && state.method == "SMS" {
			if resendErr := c.resendInstagramWebTwoFactorSMS(ctx, state); resendErr != nil {
				return fmt.Errorf("failed to request a replacement Instagram SMS code: %w", resendErr)
			}
			return ErrInstagramWebTwoFactorCodeResent
		}
		return err
	}
	c.ensureInstagramWebUserID()
	if missing := c.cookies.GetMissingCookieNames(); len(missing) > 0 {
		return fmt.Errorf("instagram web two-factor login succeeded without required cookies: %v", missing)
	}
	c.webTwoFactor = nil
	return nil
}

// ensureInstagramWebUserID derives the ds_user_id cookie from the sessionid when
// Instagram authenticates without returning it as its own cookie, which happens
// on the encrypted two-factor GraphQL path.
func (c *Client) ensureInstagramWebUserID() {
	if c.cookies.Get(cookies.IGCookieDSUserID) != "" {
		return
	}
	if userID := instagramWebUserIDFromSessionID(c.cookies.Get(cookies.IGCookieSessionID)); userID != "" {
		c.cookies.Set(cookies.IGCookieDSUserID, userID)
	}
}

// instagramWebUserIDFromSessionID extracts the numeric account ID from the
// sessionid cookie, whose value is "<ds_user_id>:<token>:<...>" (percent-encoded).
func instagramWebUserIDFromSessionID(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	decoded, err := url.QueryUnescape(sessionID)
	if err != nil {
		decoded = sessionID
	}
	userID, _, _ := strings.Cut(decoded, ":")
	if _, err := strconv.ParseInt(userID, 10, 64); err != nil {
		return ""
	}
	return userID
}
