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
		if step.Status == "" {
			step.Status = r.Status
		}
		return step
	}
	return r.instagramWebCheckpointStep
}

func parseInstagramWebCheckpointResponse(body []byte) (instagramWebCheckpointStep, error) {
	body = bytes.TrimPrefix(bytes.TrimSpace(body), httpclient.AntiJSPrefix)
	var response instagramWebCheckpointResponse
	err := json.Unmarshal(body, &response)
	return response.step(), err
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
	checkpointURL, ok := normalizeInstagramWebChallengeURL(result.CheckpointURL)
	if !ok {
		checkpointURL, ok = normalizeInstagramWebChallengeURL(result.RedirectURL)
	}
	if !ok {
		if strings.TrimSpace(result.CheckpointURL) != "" || strings.TrimSpace(result.RedirectURL) != "" {
			return nil, ErrInstagramWebCheckpointUnsupported
		}
		return nil, httpclient.ErrChallengeRequired
	}
	checkpointURL, step, err := c.renderInstagramWebCheckpoint(ctx, checkpointURL)
	if err != nil {
		return nil, err
	}
	if c.instagramWebCheckpointSessionReady(checkpointURL) {
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
		if c.instagramWebCheckpointSessionReady(checkpointURL) {
			return nil, nil
		} else if trySMS {
			if err = waitInstagramWebCheckpoint(ctx); err != nil {
				return nil, err
			}
			method, _, err = c.selectInstagramWebCheckpointMethod(ctx, checkpointURL, "0")
			if err != nil {
				return nil, err
			}
			if c.instagramWebCheckpointSessionReady(checkpointURL) {
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
		} else if c.instagramWebCheckpointSessionReady(checkpointURL) {
			return checkpointURL, instagramWebCheckpointStep{}, nil
		}
		if response != nil && response.StatusCode >= 300 && response.StatusCode < 400 {
			var ok bool
			checkpointURL, ok = resolveInstagramWebCheckpointURL(checkpointURL, response.Header.Get("Location"))
			if !ok {
				return "", instagramWebCheckpointStep{}, ErrInstagramWebCheckpointUnsupported
			}
			continue
		}
		step, parseErr := parseInstagramWebCheckpointResponse(body)
		if parseErr == nil {
			return checkpointURL, step, nil
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
	_, body, requestErr := c.instagramWebCheckpointRequest(
		ctx,
		checkpointURL,
		http.MethodPost,
		url.Values{"choice": {choice}},
		false,
		"select_method",
	)
	if requestErr != nil {
		return "", false, requestErr
	} else if c.instagramWebCheckpointSessionReady(checkpointURL) {
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
	_, body, requestErr := c.instagramWebCheckpointRequest(
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
	} else if c.instagramWebCheckpointSessionReady(state.checkpointURL) {
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
	return c.finishInstagramWebCheckpoint(state.checkpointURL, step)
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
	_, body, requestErr := c.instagramWebCheckpointRequest(
		ctx, forwardURL, http.MethodPost, form, false, "confirm_contact",
	)
	c.refreshInstagramWebCheckpointCSRF(state)
	if requestErr != nil {
		return requestErr
	} else if c.instagramWebCheckpointSessionReady(state.checkpointURL) {
		return nil
	}
	step, parseErr := parseInstagramWebCheckpointResponse(body)
	if parseErr != nil {
		return ErrInstagramWebCheckpointUnsupported
	}
	return c.finishInstagramWebCheckpoint(state.checkpointURL, step)
}

func resolveInstagramWebCheckpointURL(baseURL, raw string) (string, bool) {
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
	if baseUserID != "" && resolvedUserID != "" && baseUserID != resolvedUserID {
		return "", false
	}
	return normalized, true
}

func (c *Client) finishInstagramWebCheckpoint(checkpointURL string, step instagramWebCheckpointStep) error {
	if !c.instagramWebCheckpointSessionReady(checkpointURL) ||
		(step.Status != "" && !strings.EqualFold(step.Status, "ok")) {
		return ErrInstagramWebCheckpointUnsupported
	}
	return nil
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
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 3 || (parts[0] != "challenge" && parts[0] != "checkpoint") {
		return ""
	}
	if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
		return ""
	}
	return parts[1]
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
		c.cookies.UpdateFromResponse(response)
	}
	statusCode := 0
	if response != nil {
		statusCode = response.StatusCode
	}
	c.log.Debug().Str("checkpoint_phase", phase).Int("status_code", statusCode).
		Str("response_kind", instagramWebLoginResponseKind(body)).Msg("Instagram web checkpoint request completed")
	if requestErr == nil || (response != nil && errors.Is(requestErr, httpclient.ErrUnexpectedError)) {
		return response, body, nil
	} else if errors.Is(requestErr, httpclient.ErrRateLimited) ||
		errors.Is(requestErr, httpclient.ErrAccountSuspended) ||
		errors.Is(requestErr, context.Canceled) || errors.Is(requestErr, context.DeadlineExceeded) {
		return response, body, requestErr
	}
	return response, body, ErrInstagramWebCheckpointRequestFailed
}
