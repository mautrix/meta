// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
// SPDX-License-Identifier: AGPL-3.0-or-later

package instameow

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"golang.org/x/net/html"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

var (
	instagramAPCaptchaSubmit = instagramAuthPlatformOperation{"AuthPlatformReCaptchaSubmitCaptchaMutation", "9960725864052044", "xfb_auth_platform_user_passed_google_recaptcha"}
	instagramAPCaptchaRender = instagramAuthPlatformOperation{"AuthPlatformReCaptchaRenderOutcomeMutation", "28372617359013204", "xfb_auth_platform_recaptcha_render_outcome"}
)

const InstagramWebCaptchaStepID = "fi.mau.meta.instagram.web_captcha"

//go:embed login_web_captcha.js
var instagramCaptchaJS string

type instagramAuthPlatformCaptcha struct {
	iframeURL    string
	instance     string
	expires      time.Time
	instrumented bool
	lastToken    [32]byte
}

// The provider's AuthPlatformReCaptcha/ReCaptcha components use the fbsbx
// iframe and submit its human-solved token with the original encrypted context.
// No password, APC or session cookie is passed into the separate webview.
func (c *Client) prepareInstagramAuthPlatformCaptcha(body []byte) error {
	params := url.Values{"referer": {url.QueryEscape("https://www.instagram.com")}, "locale": {"en_US"}}
	state := &instagramAuthPlatformCaptcha{instance: uuid.NewString(), expires: time.Now().Add(10 * time.Minute)}
	seen := map[string]string{}
	invalid := false
	set := func(key, value string) {
		if len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
			invalid = true
		}
		if previous, exists := seen[key]; exists && previous != value {
			invalid = true
		}
		seen[key] = value
		if value != "" {
			params.Set(key, value)
		}
	}
	var visit func(gjson.Result, int)
	visit = func(value gjson.Result, depth int) {
		if depth > 64 {
			invalid = true
			return
		}
		if value.IsObject() {
			if name := value.Get("recaptcha_config_name"); name.Exists() && name.Type != gjson.Null {
				if name.Type != gjson.String {
					invalid = true
				} else {
					set("captcha_client_config_name", name.String())
				}
			}
			if value.Get("use_instrumented_renderer").Bool() {
				state.instrumented = true
			}
		} else if value.IsArray() {
			items := value.Array()
			if len(items) >= 3 && items[0].String() == "CookieConsentIFrameConfig" {
				if consent := items[2].Get("consent_param"); consent.Type == gjson.String {
					set("__cci", consent.String())
				}
			}
		}
		if value.IsArray() || value.IsObject() {
			value.ForEach(func(_, child gjson.Result) bool { visit(child, depth+1); return !invalid })
		}
	}
	tokens := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			if tokens.Err() != io.EOF || invalid {
				return ErrInstagramWebCheckpointUnsupported
			}
			state.iframeURL = "https://www.fbsbx.com/captcha/recaptcha/iframe/?" + params.Encode()
			c.webAuthPlatform.captcha = state
			return nil
		case html.StartTagToken:
			tag := tokens.Token()
			if tag.Data == "script" {
				for _, attr := range tag.Attr {
					if attr.Key == "type" && attr.Val == "application/json" && tokens.Next() == html.TextToken {
						text := tokens.Text()
						if gjson.ValidBytes(text) {
							visit(gjson.ParseBytes(text), 0)
						}
						break
					}
				}
			}
		}
	}
}

func (c *Client) HasInstagramWebCaptcha() bool {
	return c.webAuthPlatform != nil && c.webAuthPlatform.captcha != nil
}

func (c *Client) instagramAuthPlatformCaptchaStep(notice string) (*bridgev2.LoginStep, error) {
	if !c.HasInstagramWebCaptcha() || time.Now().After(c.webAuthPlatform.captcha.expires) {
		c.webAuthPlatform = nil
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	state := c.webAuthPlatform.captcha
	config, err := json.Marshal(map[string]string{"iframeURL": state.iframeURL, "instance": state.instance})
	if err != nil {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	if notice == "" {
		notice = "Complete Instagram's CAPTCHA in the embedded browser. Your login will continue here afterwards."
	}
	return &bridgev2.LoginStep{
		Type: bridgev2.LoginStepTypeCookies, StepID: InstagramWebCaptchaStepID, Instructions: notice,
		CookiesParams: &bridgev2.LoginCookiesParams{
			URL:       "https://www.instagram.com/accounts/login/",
			UserAgent: useragent.UserAgent,
			ExtractJS: "(" + instagramCaptchaJS + ")(" + string(config) + ")",
			Fields: []bridgev2.LoginCookieField{{ID: "captcha_token", Required: true,
				Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeSpecial, Name: "captcha_token"}}}},
		},
	}, nil
}

func (c *Client) SubmitInstagramWebCaptcha(ctx context.Context, token string) (*bridgev2.LoginStep, error) {
	if _, err := c.instagramAuthPlatformCaptchaStep(""); err != nil {
		return nil, err
	}
	s := c.webAuthPlatform
	pending := s.captcha
	token = strings.TrimSpace(token)
	fingerprint := sha256.Sum256([]byte(token))
	if token == "" || len(token) > 16384 || strings.ContainsAny(token, "\r\n\x00") || fingerprint == pending.lastToken {
		return c.instagramAuthPlatformCaptchaStep("Complete a fresh CAPTCHA to continue this login.")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pending.lastToken = fingerprint
	// Consume before transport: an ambiguous response must not replay a token.
	s.captcha = nil
	if pending.instrumented {
		_, err := c.instagramAuthPlatformRequest(ctx, instagramAPCaptchaRender, map[string]any{})
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// The browser treats render-outcome telemetry errors as non-fatal.
	}
	data, err := c.instagramAuthPlatformRequest(ctx, instagramAPCaptchaSubmit, map[string]any{"captcha_response": token})
	if err != nil {
		return nil, err
	}
	if data.Get("is_success").Type == gjson.False {
		pending.instance = uuid.NewString()
		s.captcha = pending
		return c.instagramAuthPlatformCaptchaStep("Instagram rejected or expired that CAPTCHA. Complete a fresh challenge.")
	}
	if data.Get("is_success").Type != gjson.True || data.Get("redirect_uri").Type != gjson.String || data.Get("redirect_uri").String() == "" {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	if err = c.advanceInstagramAuthPlatform(ctx, data.Get("redirect_uri").String()); err != nil {
		return nil, err
	}
	if c.webAuthPlatform == nil {
		return nil, nil
	}
	if c.HasInstagramWebCaptcha() {
		return c.instagramAuthPlatformCaptchaStep("")
	}
	return c.webAuthPlatform.step(""), nil
}
