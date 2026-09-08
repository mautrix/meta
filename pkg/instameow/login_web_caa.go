// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
// SPDX-License-Identifier: AGPL-3.0-or-later

package instameow

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.mau.fi/util/random"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
)

const instagramCAAWebLoginDocID = "27972648395719857"
const instagramCAAWebLoginOperation = "useCDSWebLoginMutation"
const instagramCAAWebLoginRoute = "comet.igweb.PolarisCAAIGLoginHomepageRoute"

type instagramCAALoginPage struct {
	form, encryption, props            gjson.Result
	consent, polarisConsent            gjson.Result
	cookieDomain, deferredCookies      gjson.Result
	granularConsent, skipConsentReload gjson.Result
}

// Only inert JSON is inspected. A CAA page with incomplete or unsupported state
// must not fall through to AJAX and submit a password to a different protocol.
func parseInstagramCAALoginPage(body []byte) (*instagramCAALoginPage, error) {
	if len(body) > 4<<20 {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	page := &instagramCAALoginPage{}
	selected, indicated, invalid := false, false, false
	set := func(dst *gjson.Result, value gjson.Result) {
		if dst.Exists() && dst.Raw != value.Raw {
			invalid = true
		}
		*dst = value
	}
	var visit func(gjson.Result, int)
	visit = func(value gjson.Result, depth int) {
		if depth > 64 {
			invalid = true
			return
		}
		if value.IsObject() {
			if deferred := value.Get("deferredCookies"); deferred.Exists() {
				set(&page.deferredCookies, deferred)
			}
			if gate := value.Get("gkxData.3123.result"); gate.Exists() {
				set(&page.granularConsent, gate)
			}
			if gate := value.Get("qexData.4809.r"); gate.Exists() {
				set(&page.skipConsentReload, gate)
			}
			if value.Get("ef_page").String() == "PolarisCAAIGLoginHomepageRoute" {
				indicated = true
			}
			if route := value.Get("initialRouteInfo.route"); route.Exists() {
				if route.Get("canonicalRouteName").String() == instagramCAAWebLoginRoute {
					selected = true
					set(&page.props, route.Get("rootView.props"))
				}
			}
			if form := value.Get("caa_login_form_data"); form.Exists() {
				indicated = true
				set(&page.form, form)
			}
			if encryption := value.Get("caa_password_encryption_data.encryption_data"); encryption.Exists() {
				set(&page.encryption, encryption)
			}
		}
		if value.IsArray() {
			module := value.Array()
			if len(module) >= 3 && module[1].IsArray() && module[2].IsObject() {
				switch module[0].String() {
				case "InitialCookieConsent":
					set(&page.consent, module[2])
				case "PolarisCookieConsent":
					set(&page.polarisConsent, module[2])
				case "CookieDomain":
					set(&page.cookieDomain, module[2])
				}
			}
		}
		if value.IsObject() || value.IsArray() {
			value.ForEach(func(_, child gjson.Result) bool { visit(child, depth+1); return !invalid })
		}
	}
	err := visitInstagramJSONScripts(body, func(data []byte) error {
		if !gjson.ValidBytes(data) {
			return ErrInstagramWebCheckpointUnsupported
		}
		visit(gjson.ParseBytes(data), 0)
		return nil
	})
	if err != nil || invalid {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	if !selected && !indicated {
		return nil, nil
	}
	if !selected || !page.form.IsObject() || !page.encryption.IsObject() || page.props.Get("appId").Int() != 1217981644879628 {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	return page, nil
}

func (c *Client) instagramCAAWebLoginVariables(page *instagramCAALoginPage, identifier, password string) ([]byte, error) {
	form := page.form
	// Do not fabricate browser-collected signals or solve an unobserved challenge.
	if form.Get("shared_prefs").Bool() || form.Get("ab_testing_enabled").Bool() || form.Get("send_screen_dimensions").Bool() ||
		form.Get("sketch_seed1").Type != gjson.Null || form.Get("sketch_seed2").Type != gjson.Null || form.Get("timestamp").Type != gjson.Null ||
		page.props.Get("next").Type != gjson.Null || page.props.Get("authDomainDataKey").Type != gjson.Null || page.props.Get("forceAuthentication").Bool() {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	keyID := page.encryption.Get("key_id")
	publicKey := page.encryption.Get("public_key").String()
	if keyID.Type != gjson.Number || keyID.Int() < 1 || keyID.Int() > 255 || keyID.Float() != float64(keyID.Int()) || len(publicKey) != 64 {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	encrypted, err := crypto.EncryptInstagramWebPassword(int(keyID.Int()), publicKey, password)
	if err != nil {
		return nil, ErrInstagramWebCheckpointRequestFailed
	}
	extra := map[string]string{
		"ab_test_data": "", "shared_prefs_data": "", "cuid": form.Get("idd_user_crypted_uid").String(),
		"guid": "f" + hex.EncodeToString(random.Bytes(8)), "jazoest": form.Get("jazoest.value").String(),
		"lgndim": "", "lgnjs": strconv.FormatInt(time.Now().Unix(), 10), "lgnrnd": form.Get("lgnrnd").String(),
		"locale": form.Get("locale").String(), "login_source": strings.ToLower(form.Get("login_source").String()),
		"lsd": form.Get("lsd.value").String(), "next": "", "prefill_contact_point": form.Get("prefill_contactpoint").String(),
		"prefill_source": form.Get("prefill_source").String(), "prefill_type": "", "skstamp": "", "timezone": "",
	}
	if form.Get("prefill_contactpoint").Type != gjson.Null {
		extra["prefill_type"] = "contact_point"
	}
	deviceID := c.configs.BrowserConfigTable.PolarisSiteData.DeviceID
	if deviceID == "" {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	sensitive := instagramWebTwoFactorSensitiveCode{Value: encrypted}
	return json.Marshal(map[string]any{"scale": c.configs.BrowserConfigTable.SiteData.Pr, "input": map[string]any{
		"actor_id": "0", "client_mutation_id": "1", "access_flow_version": "pre_mt_behavior",
		"account_recovery_entry_point": nil, "app": "instagram", "auth_domain_data_key": nil,
		"caa_login_request_extra_info": extra, "credential_type": "password", "dyi_job_id": "",
		"enc_password": sensitive, "password": sensitive, "event_request_id": uuid.NewString(),
		"identifier": identifier, "ig_web_device_id": deviceID, "initial_request_id": "1", "lids": nil,
		"login_source": "COMET_HEADERLESS_LOGIN", "next": nil, "passkey_payload": nil, "persistent": true,
		"query_params": "{}", "trusted_device_records": "{}", "use_uid_to_login": false, "waterfall_id": uuid.NewString(),
	}})
}

func (c *Client) createInstagramCAAWebSession(ctx context.Context, page *instagramCAALoginPage, identifier, password string) (*InstagramWebTwoFactorChallenge, error) {
	variables, err := c.instagramCAAWebLoginVariables(page, identifier, password)
	if err != nil {
		return nil, err
	}
	csrf := c.cookies.Get(cookies.IGCookieCSRFToken)
	body, err := c.instagramCAAWebMutation(ctx, instagramCAAWebLoginOperation, instagramCAAWebLoginDocID, variables)
	if err != nil {
		return nil, err
	}
	return c.handleInstagramCAAWebLoginResponse(ctx, body, identifier, csrf)
}

func (c *Client) instagramCAAWebMutation(ctx context.Context, operation, docID string, variables []byte) ([]byte, error) {
	config := c.configs.BrowserConfigTable
	rq := c.http.NewHTTPQuery()
	rq.FbAPIReqFriendlyName, rq.DocID = operation, docID
	rq.Variables, rq.Crn = string(variables), instagramCAAWebLoginRoute
	rq.CometReq = strconv.FormatInt(config.SiteData.CometEnv, 10)
	csrf := c.cookies.Get(cookies.IGCookieCSRFToken)
	sprinkle := config.SprinkleConfig
	if rq.Lsd == "" || csrf == "" || sprinkle.ParamName != "jazoest" || (!sprinkle.ShouldRandomize && sprinkle.Version <= 0) {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	// A rejected or lost response is terminal for this submission, never a reason
	// to try AJAX or the mobile password endpoint with the same credentials.
	return c.instagramWebGraphQLRequest(ctx, rq, c.GetEndpoint("login"), csrf)
}

func (c *Client) handleInstagramCAAWebLoginResponse(ctx context.Context, body []byte, identifier, csrf string) (*InstagramWebTwoFactorChallenge, error) {
	body = bytes.TrimPrefix(bytes.TrimSpace(body), httpclient.AntiJSPrefix)
	if !gjson.ValidBytes(body) {
		return nil, ErrInstagramWebCheckpointRequestFailed
	}
	root := gjson.ParseBytes(body)
	data := root.Get("data.caa_login_web")
	graphErrors := root.Get("errors")
	if !data.IsObject() || (graphErrors.Type != gjson.Null && (!graphErrors.IsArray() || len(graphErrors.Array()) != 0)) || root.Get("error").Exists() {
		return nil, ErrInstagramWebCheckpointRequestFailed
	}
	c.log.Debug().Int64("error_code", data.Get("error_code").Int()).
		Bool("authenticated", data.Get("ig_authenticated").Bool()).
		Bool("has_redirect", data.Get("redirect_uri").String() != "").
		Bool("has_two_factor", data.Get("two_factor_result").Type != gjson.Null).
		Msg("Instagram CAA login response")
	// GraphQL also returns this object with null fields when no deletion is pending.
	deletion := data.Get("stop_deletion_payload")
	if deletion.Get("stop_deletion_date").Type != gjson.Null && deletion.Get("stop_deletion_nonce").Type != gjson.Null {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	if twoFactor := data.Get("two_factor_result"); twoFactor.Type != gjson.Null && twoFactor.String() != "" {
		var result instagramWebLoginResponse
		if twoFactor.Type != gjson.String || json.Unmarshal([]byte(twoFactor.String()), &result) != nil || !result.TwoFactorRequired {
			return nil, ErrInstagramWebCheckpointUnsupported
		}
		return c.captureInstagramWebTwoFactor(result, identifier, csrf, http.StatusOK)
	}
	if data.Get("is_ig_login_recaptcha").Bool() {
		return nil, ErrInstagramWebCheckpointCAPTCHA
	}
	if redirect := data.Get("redirect_uri").String(); redirect != "" {
		if target, valid := resolveInstagramAuthPlatformURL("https://www.instagram.com/", redirect); valid {
			if strings.HasPrefix(target.Path, "/auth_platform/") {
				challenge, err := c.startInstagramAuthPlatform(ctx, redirect, "")
				// A credential rejection may also carry an AuthPlatform redirect. Give
				// verification priority, but retain the rejection if it only logs us out.
				if errors.Is(err, errInstagramAuthPlatformLoggedOut) && data.Get("error_code").Int() == 1348009 &&
					data.Get("error_style").String() == "GENERIC_BANNER" && !data.Get("ig_authenticated").Bool() {
					return nil, ErrInstagramWebCredentialsRejected
				}
				return challenge, err
			}
			// A redirect alone is not successful authentication. The existing terminal
			// verifier requires a complete session and matching session/user cookies.
			c.webAuthPlatform = &instagramAuthPlatformState{url: mustParseURL(c.GetEndpoint("login"))}
			if err := c.advanceInstagramAuthPlatform(ctx, target.String()); err != nil {
				c.webAuthPlatform = nil
				return nil, err
			}
			if c.webAuthPlatform != nil {
				return &InstagramWebTwoFactorChallenge{AuthPlatform: true}, nil
			}
			return nil, nil
		}
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	// The frontend applies generic error flags only after two-factor/redirect
	// dispatch. recaptcha_needed can accompany an ordinary AuthPlatform redirect;
	// only is_ig_login_recaptcha selects its interactive CAPTCHA dialog.
	if data.Get("reg_nta_context").Type != gjson.Null {
		return nil, ErrInstagramWebCheckpointUnsupported
	} else if data.Get("recaptcha_needed").Bool() {
		return nil, ErrInstagramWebCheckpointCAPTCHA
	} else if data.Get("error_style").String() == "RATE_LIMIT_BANNER" {
		return nil, httpclient.ErrRateLimited
	}
	if data.Get("ig_authenticated").Bool() {
		if code := data.Get("error_code"); code.Type != gjson.Null && code.String() != "0" {
			return nil, ErrInstagramWebCheckpointUnsupported
		}
		c.ensureInstagramWebUserID()
		userID := c.cookies.Get(cookies.IGCookieDSUserID)
		if len(c.cookies.GetMissingCookieNames()) == 0 && userID != "" && instagramWebUserIDFromSessionID(c.cookies.Get(cookies.IGCookieSessionID)) == userID {
			return nil, nil
		}
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	// INLINE is the frontend's recoverable input-error style, not a server failure.
	if data.Get("error_style").String() == "INLINE" && data.Get("error_message.text").String() != "" {
		return nil, ErrInstagramWebCredentialsRejected
	}
	return nil, ErrInstagramWebCheckpointUnsupported
}
