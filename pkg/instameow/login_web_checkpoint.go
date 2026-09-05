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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type instagramWebCheckpointNavigation struct {
	Forward string `json:"forward"`
}

type instagramWebCheckpointStep struct {
	completed     bool
	continuation  bool
	ChallengeType string                           `json:"challengeType"`
	Type          string                           `json:"type"`
	Status        string                           `json:"status"`
	Errors        []string                         `json:"errors"`
	Navigation    instagramWebCheckpointNavigation `json:"navigation"`
}

type instagramWebCheckpointResponse struct {
	Challenge *instagramWebCheckpointStep `json:"challenge"`
	instagramWebCheckpointStep
}

const instagramWebCheckpointChoiceDelay = 3 * time.Second

func (r instagramWebCheckpointResponse) step() instagramWebCheckpointStep {
	if r.Challenge != nil {
		step := *r.Challenge
		if step.Status == "" || (r.Status != "" && !strings.EqualFold(r.Status, "ok")) {
			step.Status = r.Status
		}
		if step.Type == "" {
			step.Type = r.Type
		}
		step.Errors = append(step.Errors, r.Errors...)
		return step
	}
	return r.instagramWebCheckpointStep
}

func parseInstagramWebCheckpointResponse(body []byte) (instagramWebCheckpointStep, error) {
	body = bytes.TrimPrefix(bytes.TrimSpace(body), httpclient.AntiJSPrefix)
	var response instagramWebCheckpointResponse
	err := json.Unmarshal(body, &response)
	var pending struct {
		instagramWebLoginResponse
		Challenge struct {
			URL string `json:"url"`
			instagramWebLoginResponse
		} `json:"challenge"`
	}
	if pendingErr := json.Unmarshal(body, &pending); err == nil {
		err = pendingErr
	}
	step := response.step()
	step.continuation = pending.TwoFactorRequired || pending.Challenge.TwoFactorRequired || response.ChallengeType != "" || strings.EqualFold(response.Type, "CHALLENGE") ||
		instagramWebChallengeRequired(pending.instagramWebLoginResponse) || pending.RedirectURL != "" ||
		instagramWebChallengeRequired(pending.Challenge.instagramWebLoginResponse) || pending.Challenge.RedirectURL != "" || pending.Challenge.URL != ""
	return step, err
}

func instagramWebCheckpointMethod(step instagramWebCheckpointStep) string {
	switch step.ChallengeType {
	case "VerifyEmailCodeForm":
		return "EMAIL"
	case "VerifySMSCodeForm", "VerifySMSCodeFormForSMSCaptcha":
		return "SMS"
	default:
		return ""
	}
}

func (c *Client) startInstagramWebCheckpoint(
	ctx context.Context,
	result instagramWebLoginResponse,
) (*InstagramWebTwoFactorChallenge, error) {
	rawURL := result.CheckpointURL
	if strings.TrimSpace(rawURL) == "" {
		rawURL = result.RedirectURL
	}
	if instagramWebCheckpointURLKind(rawURL) == "auth_platform" {
		return c.startInstagramAuthPlatform(ctx, rawURL, "")
	}
	checkpointURL, ok := normalizeInstagramWebChallengeURL(rawURL)
	if !ok {
		if strings.TrimSpace(result.CheckpointURL) != "" || strings.TrimSpace(result.RedirectURL) != "" {
			c.log.Debug().Str("checkpoint_url_kind", instagramWebCheckpointURLKind(rawURL)).Msg("Unsupported Instagram web checkpoint URL")
			return nil, ErrInstagramWebCheckpointUnsupported
		}
		return nil, httpclient.ErrChallengeRequired
	}
	expectedUserID := instagramWebCheckpointUserID(checkpointURL)
	checkpointURL, step, err := c.renderInstagramWebCheckpoint(ctx, checkpointURL)
	if err != nil {
		return nil, err
	}
	if instagramWebCheckpointURLKind(checkpointURL) == "auth_platform" {
		return c.startInstagramAuthPlatform(ctx, checkpointURL, expectedUserID)
	}
	if step.completed {
		return nil, nil
	}
	method := instagramWebCheckpointMethod(step)
	if method == "" {
		if step.ChallengeType != "" && step.ChallengeType != "SelectContactPointRecoveryForm" {
			return nil, ErrInstagramWebCheckpointUnsupported
		}
		if err = waitInstagramWebCheckpoint(ctx); err != nil {
			return nil, err
		}
		var trySMS bool
		method, trySMS, err = c.selectInstagramWebCheckpointMethod(ctx, checkpointURL, "1")
		if err != nil {
			return nil, err
		}
		if method == "" && !trySMS {
			return nil, nil
		} else if trySMS {
			if err = waitInstagramWebCheckpoint(ctx); err != nil {
				return nil, err
			}
			method, _, err = c.selectInstagramWebCheckpointMethod(ctx, checkpointURL, "0")
			if err != nil {
				return nil, err
			}
			if method == "" {
				return nil, nil
			}
		}
	}
	if method == "" {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	csrfToken := c.cookies.Get(cookies.IGCookieCSRFToken)
	if csrfToken == "" {
		return nil, errInstagramWebTwoFactorMissingCSRF
	}
	c.webTwoFactor = &instagramWebTwoFactorState{
		method:        method,
		csrfToken:     csrfToken,
		checkpointURL: checkpointURL,
	}
	c.log.Debug().Str("challenge_type", method).Msg("Started Instagram web checkpoint")
	return &InstagramWebTwoFactorChallenge{Email: method == "EMAIL", SMS: method == "SMS"}, nil
}

func (c *Client) renderInstagramWebCheckpoint(
	ctx context.Context,
	checkpointURL string,
) (string, instagramWebCheckpointStep, error) {
	for range 3 {
		response, body, err := c.instagramWebCheckpointRequest(
			ctx, checkpointURL, http.MethodGet, nil, true, "render",
		)
		if err != nil {
			return "", instagramWebCheckpointStep{}, err
		} else if response != nil && response.StatusCode < 400 && response.Request != nil && response.Request.URL != nil &&
			instagramWebCheckpointURLKind(response.Request.URL.String()) == "auth_platform" {
			return response.Request.URL.String(), instagramWebCheckpointStep{}, nil
		} else if c.instagramWebCheckpointResponseComplete(checkpointURL, response, body) {
			return checkpointURL, instagramWebCheckpointStep{completed: true}, nil
		}
		if response != nil && response.StatusCode >= 300 && response.StatusCode < 400 {
			if target, ok := resolveInstagramAuthPlatformURL(checkpointURL, response.Header.Get("Location")); ok && strings.HasPrefix(target.Path, "/auth_platform/") {
				return target.String(), instagramWebCheckpointStep{}, nil
			}
			var ok bool
			checkpointURL, ok = resolveInstagramWebCheckpointURL(checkpointURL, response.Header.Get("Location"))
			if !ok {
				return "", instagramWebCheckpointStep{}, ErrInstagramWebCheckpointUnsupported
			}
			continue
		}
		step, parseErr := parseInstagramWebCheckpointResponse(body)
		if parseErr == nil {
			if response != nil && response.StatusCode >= 400 && step.ChallengeType == "" {
				return "", instagramWebCheckpointStep{}, ErrInstagramWebCheckpointUnsupported
			}
			return checkpointURL, step, nil
		}
		if response != nil && response.StatusCode >= 400 {
			return "", instagramWebCheckpointStep{}, ErrInstagramWebCheckpointUnsupported
		}
		// The normal render response is HTML. The next POST returns the
		// machine-readable challenge step.
		return checkpointURL, instagramWebCheckpointStep{}, nil
	}
	return "", instagramWebCheckpointStep{}, ErrInstagramWebCheckpointUnsupported
}

func waitInstagramWebCheckpoint(ctx context.Context) error {
	timer := time.NewTimer(instagramWebCheckpointChoiceDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func mustParseURL(raw string) *url.URL {
	parsed, _ := url.Parse(raw)
	return parsed
}

func (c *Client) selectInstagramWebCheckpointMethod(
	ctx context.Context,
	checkpointURL,
	choice string,
) (method string, tryAnotherMethod bool, err error) {
	response, body, requestErr := c.instagramWebCheckpointRequest(
		ctx,
		checkpointURL,
		http.MethodPost,
		url.Values{"choice": {choice}},
		false,
		"select_method",
	)
	if requestErr != nil {
		return "", false, requestErr
	} else if c.instagramWebCheckpointResponseComplete(checkpointURL, response, body) {
		return "", false, nil
	}
	step, parseErr := parseInstagramWebCheckpointResponse(body)
	if parseErr != nil {
		return "", false, ErrInstagramWebCheckpointUnsupported
	} else if method = instagramWebCheckpointMethod(step); method != "" {
		return method, false, nil
	} else if step.ChallengeType == "SelectContactPointRecoveryForm" && choice == "1" {
		return "", true, nil
	}
	return "", false, ErrInstagramWebCheckpointUnsupported
}

func (c *Client) completeInstagramWebCheckpoint(
	ctx context.Context,
	state *instagramWebTwoFactorState,
	verificationCode string,
) error {
	response, body, requestErr := c.instagramWebCheckpointRequest(
		ctx,
		state.checkpointURL,
		http.MethodPost,
		url.Values{"security_code": {verificationCode}},
		false,
		"submit_code",
	)
	c.refreshInstagramWebCheckpointCSRF(state)
	if requestErr != nil {
		return requestErr
	} else if c.instagramWebCheckpointResponseComplete(state.checkpointURL, response, body) {
		return nil
	}
	step, parseErr := parseInstagramWebCheckpointResponse(body)
	if parseErr != nil {
		return ErrInstagramWebCheckpointUnsupported
	} else if instagramWebCheckpointMethod(step) != "" {
		return ErrInstagramWebTwoFactorCodeRejected
	} else if step.ChallengeType == "ReviewContactPointChangeForm" && len(step.Errors) == 0 {
		return c.confirmInstagramWebCheckpointContact(ctx, state, step.Navigation.Forward)
	} else if len(step.Errors) > 0 {
		return ErrInstagramWebCheckpointUnsupported
	}
	return ErrInstagramWebCheckpointUnsupported
}

func (c *Client) confirmInstagramWebCheckpointContact(
	ctx context.Context,
	state *instagramWebTwoFactorState,
	forward string,
) error {
	forwardURL, ok := resolveInstagramWebCheckpointURL(state.checkpointURL, forward)
	if !ok {
		return ErrInstagramWebCheckpointUnsupported
	}
	if err := waitInstagramWebCheckpoint(ctx); err != nil {
		return err
	}
	seed := "#PWD_INSTAGRAM_BROWSER:0:" + strconv.FormatInt(time.Now().Unix(), 10) + ":"
	form := url.Values{
		"choice":            {"0"},
		"enc_new_password1": {seed},
		"new_password1":     {""},
		"enc_new_password2": {seed},
		"new_password2":     {""},
	}
	response, body, requestErr := c.instagramWebCheckpointRequest(
		ctx, forwardURL, http.MethodPost, form, false, "confirm_contact",
	)
	c.refreshInstagramWebCheckpointCSRF(state)
	if requestErr != nil {
		return requestErr
	} else if c.instagramWebCheckpointResponseComplete(forwardURL, response, body) {
		return nil
	}
	return ErrInstagramWebCheckpointUnsupported
}

func resolveInstagramWebCheckpointURL(baseURL, raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", false
	}
	base, baseErr := url.Parse(baseURL)
	reference, referenceErr := url.Parse(strings.TrimSpace(raw))
	if baseErr != nil || referenceErr != nil {
		return "", false
	}
	resolved := base.ResolveReference(reference)
	normalized, ok := normalizeInstagramWebChallengeURL(resolved.String())
	if !ok {
		return "", false
	}
	baseUserID, resolvedUserID := instagramWebCheckpointUserID(baseURL), instagramWebCheckpointUserID(normalized)
	if baseUserID != "" && baseUserID != resolvedUserID {
		return "", false
	}
	return normalized, true
}

func (c *Client) instagramWebCheckpointResponseComplete(checkpointURL string, response *http.Response, body []byte) bool {
	if response == nil || response.StatusCode < 200 || response.StatusCode >= 400 {
		return false
	}
	step, parseErr := parseInstagramWebCheckpointResponse(body)
	if parseErr == nil && (step.continuation || step.ChallengeType != "" || len(step.Errors) != 0 || strings.EqualFold(step.Type, "CHALLENGE") ||
		(step.Status != "" && !strings.EqualFold(step.Status, "ok"))) {
		return false
	}
	if response.StatusCode >= 300 {
		location, err := url.Parse(response.Header.Get("Location"))
		if err != nil || response.Header.Get("Location") == "" {
			return false
		}
		target := mustParseURL(checkpointURL).ResolveReference(location)
		// Only a terminal web destination may finish a checkpoint on a redirect.
		if target.Scheme != "https" || target.User != nil || (target.Port() != "" && target.Port() != "443") ||
			!strings.EqualFold(strings.TrimSuffix(target.Hostname(), "."), "www.instagram.com") {
			return false
		}
		switch target.Path {
		case "/", "/accounts/edit/", "/direct/inbox/":
		default:
			return false
		}
	} else if parseErr != nil || !strings.EqualFold(step.Status, "ok") {
		return false
	}
	return c.instagramWebCheckpointSessionReady(checkpointURL)
}

func (c *Client) instagramWebCheckpointSessionReady(checkpointURL string) bool {
	c.ensureInstagramWebUserID()
	if len(c.cookies.GetMissingCookieNames()) != 0 {
		return false
	}
	expectedUserID := instagramWebCheckpointUserID(checkpointURL)
	return expectedUserID == "" || expectedUserID == c.cookies.Get(cookies.IGCookieDSUserID)
}

func instagramWebCheckpointUserID(rawURL string) string {
	parsed := mustParseURL(rawURL)
	if parsed == nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "challenge" || parts[i] == "checkpoint" {
			if accountID, err := strconv.ParseUint(parts[i+1], 10, 64); err == nil && accountID > 0 {
				return parts[i+1]
			}
		}
	}
	return ""
}

func instagramWebCheckpointURLKind(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "invalid"
	}
	if parsed.Scheme == "" {
		parsed = mustParseURL("https://www.instagram.com").ResolveReference(parsed)
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || (parsed.Port() != "" && parsed.Port() != "443") ||
		(host != "instagram.com" && !strings.HasSuffix(host, ".instagram.com")) {
		return "untrusted"
	}
	path := "/" + strings.Trim(parsed.Path, "/") + "/"
	switch {
	case strings.Contains(path, "/auth_platform/"):
		return "auth_platform"
	case path == "/web/unsupported_version/":
		return "unsupported_version"
	case strings.Contains(path, "/challenge/") || strings.Contains(path, "/checkpoint/"):
		return "legacy_checkpoint"
	default:
		return "other_instagram"
	}
}

func (c *Client) refreshInstagramWebCheckpointCSRF(state *instagramWebTwoFactorState) {
	if csrfToken := c.cookies.Get(cookies.IGCookieCSRFToken); csrfToken != "" {
		state.csrfToken = csrfToken
	}
}

func (c *Client) instagramWebCheckpointRequest(
	ctx context.Context,
	requestURL,
	method string,
	form url.Values,
	document bool,
	phase string,
) (*http.Response, []byte, error) {
	headers := c.http.BuildHeaders(true, document)
	if !document {
		parsed := mustParseURL(requestURL)
		headers.Set("origin", parsed.Scheme+"://"+parsed.Host)
		headers.Set("referer", requestURL)
		headers.Set("x-requested-with", "XMLHttpRequest")
		headers.Set("sec-fetch-dest", "empty")
		headers.Set("sec-fetch-mode", "cors")
		headers.Set("sec-fetch-site", "same-origin")
		if err := c.addInstagramWebLoginHeaders(headers); err != nil {
			return nil, nil, err
		}
	}
	var payload []byte
	contentType := types.NONE
	if form != nil {
		payload = []byte(form.Encode())
		contentType = types.FORM
	}
	response, body, requestErr := c.http.MakeRequestOnceNoRedirect(ctx, requestURL, method, headers, payload, contentType)
	if response != nil {
		if (phase == "render" || phase == "auth_platform_render") && response.Request != nil && response.Request.URL != nil {
			if _, ok := resolveInstagramAuthPlatformURL(requestURL, response.Request.URL.String()); !ok {
				if _, legacyOK := resolveInstagramWebCheckpointURL(requestURL, response.Request.URL.String()); phase != "render" || !legacyOK {
					return response, nil, ErrInstagramWebCheckpointUnsupported
				}
			}
		}
		c.cookies.UpdateFromResponse(response)
	}
	statusCode := 0
	if response != nil {
		statusCode = response.StatusCode
	}
	c.log.Debug().Str("checkpoint_phase", phase).Int("status_code", statusCode).
		Str("response_kind", instagramWebLoginResponseKind(body)).Msg("Instagram web checkpoint request completed")
	if requestErr == nil || (response != nil && response.StatusCode == http.StatusBadRequest && errors.Is(requestErr, httpclient.ErrUnexpectedError)) {
		return response, body, nil
	} else if errors.Is(requestErr, httpclient.ErrRateLimited) ||
		errors.Is(requestErr, httpclient.ErrAccountSuspended) ||
		errors.Is(requestErr, context.Canceled) || errors.Is(requestErr, context.DeadlineExceeded) {
		return response, body, requestErr
	}
	return response, body, ErrInstagramWebCheckpointRequestFailed
}
