// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
// SPDX-License-Identifier: AGPL-3.0-or-later

package instameow

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/go-querystring/query"
	"github.com/tidwall/gjson"
	"golang.org/x/net/html"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type instagramAuthPlatformOperation struct{ name, docID, field string }

var (
	instagramAPCode                  = instagramAuthPlatformOperation{"AuthPlatformCodeEntryViewQuery", "34414353874878894", "xfb_auth_platform_enter_code_content"}
	instagramAPPicker                = instagramAuthPlatformOperation{"AuthPlatformChallengePickerViewQuery", "26777000331897324", "xfb_auth_platform_challenges"}
	instagramAPAnother               = instagramAuthPlatformOperation{"useAuthPlatformTryAnotherWayMutation", "9378248908953318", "xfb_auth_platform_try_another_way"}
	instagramAPSelect                = instagramAuthPlatformOperation{"useAuthPlatformSelectChallengeMutation", "9771607989592788", "xfb_auth_platform_select_challenge"}
	instagramAPSubmit                = instagramAuthPlatformOperation{"useAuthPlatformSubmitCodeMutation", "25017097917894476", "xfb_auth_platform_submit_code"}
	ErrInstagramWebCheckpointCAPTCHA = errors.New("instagram web checkpoint requires an interactive CAPTCHA")
)

type instagramAuthPlatformChoice struct {
	label              string
	challenge, contact int
}

type instagramAuthPlatformState struct {
	referrer                 string
	url                      *url.URL
	expectedUserID, channel  string
	notice                   string
	tryAnotherWay, enterCode bool
	mutationID               int
	choices                  []instagramAuthPlatformChoice
}

func resolveInstagramAuthPlatformURL(base, raw string) (*url.URL, bool) {
	reference, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	target := mustParseURL(base).ResolveReference(reference)
	params, err := url.ParseQuery(target.RawQuery)
	if err != nil || target.Scheme != "https" || target.User != nil ||
		(target.Port() != "" && target.Port() != "443") || !strings.EqualFold(target.Hostname(), "www.instagram.com") || target.RawPath != "" {
		return nil, false
	}
	switch target.Path {
	case "/auth_platform/", "/auth_platform/codeentry/", "/auth_platform/challengepicker/", "/auth_platform/recaptcha/":
		if len(params["apc"]) != 1 || params.Get("apc") == "" || len(params["device_id"]) > 1 {
			return nil, false
		}
	case "/", "/accounts/onetap/", "/accounts/edit/", "/direct/inbox/":
	default:
		return nil, false
	}
	target.Fragment = ""
	return target, true
}

func (c *Client) startInstagramAuthPlatform(ctx context.Context, rawURL, expectedUserID string) (*InstagramWebTwoFactorChallenge, error) {
	target, ok := resolveInstagramAuthPlatformURL("https://www.instagram.com/", rawURL)
	if !ok || !strings.HasPrefix(target.Path, "/auth_platform/") {
		c.log.Debug().Str("checkpoint_url_kind", instagramWebCheckpointURLKind(rawURL)).Msg("Unsupported Instagram web checkpoint URL")
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	expectedUserID = cmp.Or(expectedUserID, c.cookies.Get(cookies.IGCookieDSUserID))
	if expectedUserID != "" {
		if userID, err := strconv.ParseUint(expectedUserID, 10, 64); err != nil || userID == 0 {
			return nil, ErrInstagramWebCheckpointUnsupported
		}
	}
	c.webAuthPlatform = &instagramAuthPlatformState{url: mustParseURL(c.GetEndpoint("login")), expectedUserID: expectedUserID}
	if err := c.advanceInstagramAuthPlatform(ctx, target.String()); err != nil {
		c.webAuthPlatform = nil
		return nil, err
	} else if c.webAuthPlatform == nil {
		return nil, nil
	}
	return &InstagramWebTwoFactorChallenge{AuthPlatform: true}, nil
}

func (c *Client) advanceInstagramAuthPlatform(ctx context.Context, rawURL string) error {
	s := c.webAuthPlatform
	// Redirects keep the initiating document's referrer until a new page loads.
	s.referrer = s.url.String()
	for range 5 {
		target, ok := resolveInstagramAuthPlatformURL(s.url.String(), rawURL)
		if !ok {
			return ErrInstagramWebCheckpointUnsupported
		} else if target.Path == "/auth_platform/recaptcha/" {
			return ErrInstagramWebCheckpointCAPTCHA
		}
		s.url = target
		response, body, err := c.instagramWebCheckpointRequest(ctx, target.String(), http.MethodGet, nil, true, "auth_platform_render")
		if err != nil {
			return err
		} else if response == nil {
			return ErrInstagramWebCheckpointRequestFailed
		}
		// Some client transports follow redirects themselves. Validate their final URL too.
		if response.Request != nil && response.Request.URL != nil {
			target, ok = resolveInstagramAuthPlatformURL(target.String(), response.Request.URL.String())
			if !ok {
				return ErrInstagramWebCheckpointUnsupported
			} else if target.Path == "/auth_platform/recaptcha/" {
				return ErrInstagramWebCheckpointCAPTCHA
			}
			s.url = target
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			rawURL = response.Header.Get("Location")
			continue
		} else if response.StatusCode != http.StatusOK {
			return ErrInstagramWebCheckpointUnsupported
		}
		if !strings.HasPrefix(target.Path, "/auth_platform/") {
			c.ensureInstagramWebUserID()
			userID := c.cookies.Get(cookies.IGCookieDSUserID)
			if instagramWebLoginResponseKind(body) != "html" || len(c.cookies.GetMissingCookieNames()) != 0 ||
				instagramWebUserIDFromSessionID(c.cookies.Get(cookies.IGCookieSessionID)) != userID || (s.expectedUserID != "" && s.expectedUserID != userID) {
				return ErrInstagramWebCheckpointUnsupported
			}
			c.webAuthPlatform = nil
			return nil
		}
		if err = c.refreshInstagramAuthPlatformConfig(body); err != nil {
			return err
		}
		s.channel, s.choices, s.enterCode, s.tryAnotherWay = "", nil, false, false
		s.notice = ""
		op := instagramAPCode
		if target.Path == "/auth_platform/challengepicker/" {
			op = instagramAPPicker
		} else if target.Path != "/auth_platform/codeentry/" {
			return ErrInstagramWebCheckpointUnsupported
		}
		data, err := c.instagramAuthPlatformRequest(ctx, op, nil)
		if err != nil {
			return err
		}
		if op == instagramAPCode {
			s.channel = instagramAuthPlatformChannel(data.Get("challenge_name").String())
			s.tryAnotherWay = data.Get("should_show_try_another_way").Bool()
			if s.channel == "" || (data.Get("error_message").String() != "" && data.Get("error_style").String() != "INLINE") {
				return ErrInstagramWebCheckpointUnsupported
			}
			if data.Get("error_message").String() != "" {
				s.notice = "Instagram reported a problem with verification. Check the code or choose another available method."
			}
		} else {
			for _, option := range data.Get("challenge_options").Array() {
				channel, index := instagramAuthPlatformChannel(option.Get("challenge_name").String()), option.Get("index")
				contacts := option.Get("obfuscated_contact_points")
				if channel == "" || !contacts.IsArray() || index.Type != gjson.Number || index.Int() < 0 || index.Int() > 1<<31-1 || index.Float() != float64(index.Int()) {
					continue
				}
				for contact := range max(1, len(contacts.Array())) {
					if len(s.choices) >= 32 {
						return ErrInstagramWebCheckpointUnsupported
					}
					// Provisioning logs login steps: never copy contact details or provider text here.
					label := channel + " (option " + strconv.Itoa(len(s.choices)+1) + ")"
					s.choices = append(s.choices, instagramAuthPlatformChoice{label, int(index.Int()), contact})
				}
			}
			if len(s.choices) == 0 {
				return ErrInstagramWebCheckpointUnsupported
			}
		}
		return nil
	}
	return ErrInstagramWebCheckpointUnsupported
}

func instagramAuthPlatformChannel(method string) string {
	return map[string]string{"EMAIL": "email", "SMS": "SMS", "SOWA": "WhatsApp"}[method]
}

func (s *instagramAuthPlatformState) step(instructions string) *bridgev2.LoginStep {
	instructions = cmp.Or(instructions, s.notice)
	field := bridgev2.LoginInputDataField{Type: bridgev2.LoginInputFieldTypeSelect, ID: "method", Name: "Verification method"}
	if len(s.choices) != 0 {
		instructions = cmp.Or(instructions, "Choose one of the verification methods Instagram offered.")
		for _, choice := range s.choices {
			field.Options = append(field.Options, choice.label)
		}
	} else if s.tryAnotherWay && !s.enterCode {
		instructions = cmp.Or(instructions, "Instagram requires a code via "+s.channel+". You can enter it or choose another available method.")
		field.ID, field.Options = "action", []string{"Enter code", "Try another method"}
	} else {
		instructions = cmp.Or(instructions, "Enter the verification code Instagram sent via "+s.channel+".")
		field = bridgev2.LoginInputDataField{Type: bridgev2.LoginInputFieldType2FACode, ID: "verification_code", Name: "Verification code", Pattern: "^.{5,8}$"}
	}
	return &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeUserInput, StepID: "fi.mau.meta.instagram.auth_platform." + field.ID,
		Instructions: instructions, UserInputParams: &bridgev2.LoginUserInputParams{Fields: []bridgev2.LoginInputDataField{field}, CanCancel: s.tryAnotherWay && s.enterCode}}
}

// DoInstagramWebAuthPlatformSteps continues the provider's checkpoint, never a new password login.
func (c *Client) DoInstagramWebAuthPlatformSteps(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	s := c.webAuthPlatform
	if s == nil {
		return nil, ErrInstagramWebCheckpointUnsupported
	}
	op, values := instagramAPSubmit, map[string]any{}
	if len(s.choices) != 0 {
		for _, choice := range s.choices {
			if input["method"] == choice.label {
				values["challenge_index"], values["contact_point_index"] = choice.challenge, choice.contact
				break
			}
		}
		if len(values) == 0 {
			return s.step(""), nil
		}
		op = instagramAPSelect
	} else if s.tryAnotherWay && !s.enterCode {
		if input["action"] == "Enter code" {
			s.enterCode = true
		} else if input["action"] == "Try another method" {
			op = instagramAPAnother
		}
		if op != instagramAPAnother {
			return s.step(""), nil
		}
	} else {
		if input["back"] == "true" && s.tryAnotherWay {
			s.enterCode = false
			return s.step(""), nil
		}
		code := strings.TrimSpace(input["verification_code"])
		if n := utf8.RuneCountInString(code); n < 5 || n > 8 {
			return s.step(""), nil
		}
		values["code"] = code
	}
	data, err := c.instagramAuthPlatformRequest(ctx, op, values)
	if err != nil {
		return nil, err
	}
	redirect := data.Get("redirect_uri").String()
	if data.Get("error_style").String() == "INLINE" && redirect == "" && op == instagramAPSubmit {
		return s.step("Instagram did not accept that code. Check the latest code and try again, or go back to choose another method."), nil
	} else if redirect == "" || data.Get("error_message").String() != "" || data.Get("ap_error_code").Type != gjson.Null {
		return nil, ErrInstagramWebCheckpointUnsupported
	} else if err = c.advanceInstagramAuthPlatform(ctx, redirect); err != nil {
		return nil, err
	} else if c.webAuthPlatform == nil {
		return nil, nil
	}
	return c.webAuthPlatform.step(""), nil
}

func (c *Client) instagramAuthPlatformRequest(ctx context.Context, op instagramAuthPlatformOperation, input map[string]any) (gjson.Result, error) {
	s := c.webAuthPlatform
	params := s.url.Query()
	rq := c.http.NewHTTPQuery()
	variables := map[string]any{"apc": params.Get("apc")}
	if input != nil {
		s.mutationID++
		input["actor_id"], input["client_mutation_id"] = "0", strconv.Itoa(s.mutationID)
		input["encrypted_ap_context"] = params.Get("apc")
		variables = input
		rq.Crn = "comet.igweb.PolarisAuthPlatformCodeEntryRoute"
		if op == instagramAPSelect {
			rq.Crn = "comet.igweb.PolarisAuthPlatformChallengePickerRoute"
		}
	}
	if deviceID := params.Get("device_id"); deviceID != "" {
		variables["device_id"] = deviceID
	}
	if input != nil {
		variables = map[string]any{"input": variables}
	}
	encoded, err := json.Marshal(variables)
	if err != nil {
		return gjson.Result{}, ErrInstagramWebCheckpointRequestFailed
	}
	rq.Av, rq.User = "0", "0"
	rq.FbAPICallerClass, rq.FbAPIReqFriendlyName, rq.DocID = "RelayModern", op.name, op.docID
	rq.ServerTimestamps, rq.Variables, rq.Jssesw = "true", string(encoded), ""
	config := c.configs.BrowserConfigTable
	rq.Rev = strconv.FormatInt(config.SiteData.ClientRevision, 10)
	rq.FbDtsg = cmp.Or(config.DTSGInitialData.Token, config.DTSGInitData.Token)
	// Relay signs DTSG, or LSD for logged-out requests; the legacy login signs CSRF instead.
	sprinkle := config.SprinkleConfig
	csrfToken := cmp.Or(c.cookies.Get(cookies.IGCookieCSRFToken), config.InstagramSecurityConfig.CSRFToken)
	if sprinkle.ParamName != "jazoest" || (!sprinkle.ShouldRandomize && sprinkle.Version <= 0) || rq.Lsd == "" || csrfToken == "" {
		return gjson.Result{}, ErrInstagramWebCheckpointRequestFailed
	}
	sum := 0
	for _, character := range utf16.Encode([]rune(cmp.Or(rq.FbDtsg, rq.Lsd))) {
		sum += int(character)
	}
	rq.Jazoest = ""
	form, _ := query.Values(rq)
	token := strconv.Itoa(sum)
	if !sprinkle.ShouldRandomize {
		token = strconv.Itoa(sprinkle.Version) + token
	}
	form.Set(sprinkle.ParamName, token)
	headers := c.http.BuildHeaders(true, false)
	headers.Set("x-csrftoken", csrfToken)
	headers.Set("origin", "https://www.instagram.com")
	headers.Set("referer", s.url.String())
	headers.Set("x-fb-friendly-name", op.name)
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	c.log.Debug().Str("auth_platform_operation", op.name).Msg("Submitting Instagram verification request")
	response, body, err := c.http.MakeRequestOnceNoRedirect(ctx, "https://www.instagram.com/api/graphql", http.MethodPost, headers, []byte(form.Encode()), types.FORM)
	if response != nil {
		if response.Request != nil && response.Request.URL != nil && response.Request.URL.String() != "https://www.instagram.com/api/graphql" {
			return gjson.Result{}, ErrInstagramWebCheckpointRequestFailed
		}
		c.cookies.UpdateFromResponse(response)
	}
	if errors.Is(err, httpclient.ErrRateLimited) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return gjson.Result{}, err
	} else if err != nil || response == nil || response.StatusCode != http.StatusOK {
		return gjson.Result{}, ErrInstagramWebCheckpointRequestFailed
	}
	body = bytes.TrimPrefix(bytes.TrimSpace(body), httpclient.AntiJSPrefix)
	if !gjson.ValidBytes(body) {
		return gjson.Result{}, ErrInstagramWebCheckpointUnsupported
	}
	root := gjson.ParseBytes(body)
	data := root.Get("data." + op.field)
	if !data.IsObject() || len(root.Get("errors").Array()) != 0 || root.Get("error").Exists() {
		return gjson.Result{}, ErrInstagramWebCheckpointUnsupported
	} else if data.Get("error_style").String() == "RATE_LIMIT_BANNER" {
		return gjson.Result{}, httpclient.ErrRateLimited
	}
	return data, nil
}

// Read inert configuration only. The general module loader can log raw scripts containing APC.
func (c *Client) refreshInstagramAuthPlatformConfig(body []byte) error {
	if len(body) > 4<<20 {
		return ErrInstagramWebCheckpointUnsupported
	}
	config := *c.configs.BrowserConfigTable
	config.LSD, config.DTSGInitData, config.DTSGInitialData = types.LSD{}, types.DTSGInitData{}, types.DTSGInitialData{}
	config.SiteData, config.SprinkleConfig = types.SiteData{}, types.SprinkleConfig{}
	config.InstagramSecurityConfig = types.InstagramSecurityConfig{}
	fields := map[string]any{"LSD": &config.LSD, "DTSGInitData": &config.DTSGInitData, "DTSGInitialData": &config.DTSGInitialData,
		"SiteData": &config.SiteData, "SprinkleConfig": &config.SprinkleConfig, "CurrentUserInitialData": &config.CurrentUserInitialData,
		"InstagramWebPushInfo": &config.InstagramWebPushInfo, "PolarisSiteData": &config.PolarisSiteData,
		"InstagramSecurityConfig": &config.InstagramSecurityConfig}
	found, invalid := map[string]bool{}, false
	var visit func(any, int)
	visit = func(value any, depth int) {
		if depth > 64 {
			invalid = true
			return
		}
		switch value := value.(type) {
		case []any:
			if len(value) >= 3 {
				name, _ := value[0].(string)
				if field := fields[name]; field != nil {
					if _, ok := value[2].(map[string]any); !ok {
						invalid = true
						return
					}
					data, _ := json.Marshal(value[2])
					if json.Unmarshal(data, field) != nil {
						invalid = true
					}
					found[name] = true
					return
				}
			}
			for _, child := range value {
				visit(child, depth+1)
			}
		case map[string]any:
			for _, child := range value {
				visit(child, depth+1)
			}
		}
	}
	tokens := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			if tokens.Err() != io.EOF || invalid || config.LSD.Token == "" || !found["SiteData"] || config.SiteData.ClientRevision <= 0 ||
				!found["SprinkleConfig"] || config.SprinkleConfig.ParamName != "jazoest" || (!config.SprinkleConfig.ShouldRandomize && config.SprinkleConfig.Version <= 0) {
				return ErrInstagramWebCheckpointUnsupported
			}
			*c.configs.BrowserConfigTable = config
			c.configs.LSDToken, c.configs.CometReq = config.LSD.Token, strconv.FormatInt(config.SiteData.CometEnv, 10)
			return nil
		case html.StartTagToken:
			tag := tokens.Token()
			if tag.Data != "script" {
				continue
			}
			for _, attr := range tag.Attr {
				if attr.Key == "type" && attr.Val == "application/json" && tokens.Next() == html.TextToken {
					var value any
					if json.Unmarshal(tokens.Text(), &value) == nil {
						visit(value, 0)
					}
					break
				}
			}
		}
	}
}
