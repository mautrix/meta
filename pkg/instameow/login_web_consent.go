// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
// SPDX-License-Identifier: AGPL-3.0-or-later

package instameow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
)

var ErrInstagramWebCookieConsentRequired = errors.New("instagram requires a cookie consent choice")

type instagramWebCookieConsentState struct {
	page    *instagramCAALoginPage
	values  map[cookies.MetaCookieName]string
	expires time.Time
}

func (c *Client) prepareInstagramWebCookieConsent(page *instagramCAALoginPage) error {
	if !page.consent.Get("shouldShowCookieBanner").Bool() && !page.polarisConsent.Get("should_show_consent_dialog").Bool() {
		return nil
	}
	// Only the observed granular, blocking, no-reload banner is supported. In particular,
	// noCookies is not permission to flush the server's deferred cookie queue.
	if page.consent.Get("noCookies").Type != gjson.False || page.consent.Get("nonBlockingBannerPage").Type != gjson.False ||
		!page.consent.Get("initialConsent").IsArray() || len(page.consent.Get("initialConsent").Array()) != 0 ||
		page.granularConsent.Type != gjson.True || page.skipConsentReload.Type != gjson.True ||
		page.cookieDomain.Get("domain").String() != "instagram.com" || !page.deferredCookies.IsObject() {
		return ErrInstagramWebCheckpointUnsupported
	}
	now := time.Now()
	state := &instagramWebCookieConsentState{page: page, values: make(map[cookies.MetaCookieName]string), expires: now.Add(5 * time.Minute)}
	valid := true
	page.deferredCookies.ForEach(func(name, spec gjson.Result) bool {
		switch name.String() {
		case "_js_ig_did", "_js_mid", "_js_datr":
		default:
			valid = false
			return false
		}
		expiry := spec.Get("expiration_for_js")
		cookie := &http.Cookie{Name: name.String(), Value: spec.Get("value").String(), Path: spec.Get("path").String(), Secure: spec.Get("secure").Bool()}
		domain := strings.TrimPrefix(spec.Get("domain").String(), ".")
		if !spec.IsObject() || spec.Get("value").Type != gjson.String || cookie.Value == "" || len(cookie.Value) > 4096 || cookie.Valid() != nil ||
			cookie.Path != "/" || spec.Get("secure").Type != gjson.True || (domain != "" && domain != "instagram.com") ||
			expiry.Type != gjson.Number || expiry.Float() != float64(expiry.Int()) || expiry.Int() <= 1 || expiry.Int() > 1<<53 {
			valid = false
			return false
		}
		// CookieCore accepts either an absolute future timestamp or a TTL in ms.
		ttl := expiry.Int()
		if ttl > now.UnixMilli() {
			ttl -= now.UnixMilli()
		}
		if ttl < state.expires.Sub(now).Milliseconds() {
			state.expires = now.Add(time.Duration(ttl) * time.Millisecond)
		}
		state.values[cookies.MetaCookieName(cookie.Name)] = cookie.Value
		return true
	})
	if !valid || len(state.values) != 3 || c.configs.BrowserConfigTable.PolarisSiteData.DeviceID == "" {
		return ErrInstagramWebCheckpointUnsupported
	}
	c.webCookieConsent = state
	return ErrInstagramWebCookieConsentRequired
}

// ContinueInstagramWebSessionAfterCookieConsent is called only after the user
// chooses to continue with optional cookies declined. It consumes the bootstrap
// once; an ambiguous consent response must never trigger a password submission.
func (c *Client) ContinueInstagramWebSessionAfterCookieConsent(ctx context.Context, identifier, password string) (*InstagramWebTwoFactorChallenge, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	state := c.webCookieConsent
	c.webCookieConsent = nil
	if state == nil || time.Now().After(state.expires) || identifier == "" || password == "" {
		return nil, ErrInstagramWebCheckpointRequestFailed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// This is the browser's decline-optional branch, not consent-to-everything.
	variables, err := json.Marshal(map[string]any{
		"consent_to_everything": false, "first_party_tracking_opt_in": true,
		"third_party_tracking_opt_in": false, "opted_in_categories": []string{},
		"opted_in_controls": []string{}, "ig_did": c.configs.BrowserConfigTable.PolarisSiteData.DeviceID,
	})
	if err != nil {
		return nil, ErrInstagramWebCheckpointRequestFailed
	}
	for name, value := range state.values {
		c.cookies.Set(name, value)
	}
	body, err := c.instagramCAAWebMutation(ctx, "PolarisCookieMutation", "26585026644496092", variables)
	if err != nil {
		return nil, err
	}
	body = bytes.TrimPrefix(bytes.TrimSpace(body), httpclient.AntiJSPrefix)
	root := gjson.ParseBytes(body)
	graphErrors := root.Get("errors")
	if !gjson.ValidBytes(body) || root.Get("data.ig_browser_terminal_consent_mutation.success").Type != gjson.True || root.Get("error").Exists() ||
		(graphErrors.Type != gjson.Null && (!graphErrors.IsArray() || len(graphErrors.Array()) != 0)) {
		return nil, ErrInstagramWebCheckpointRequestFailed
	}
	c.log.Debug().Msg("Instagram required-cookie consent completed with optional cookies declined")
	return c.createInstagramCAAWebSession(ctx, state.page, identifier, password)
}
