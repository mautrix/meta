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
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestInstagramWebLoginFormUsesCurrentFirstPartyContract(t *testing.T) {
	got, err := newInstagramWebLoginForm(
		"test-identifier",
		"test-encrypted-password",
		"abc",
		types.SprinkleConfig{ParamName: "jazoest", Version: 2},
	)
	if err != nil {
		t.Fatalf("failed to build Instagram web login form: %v", err)
	}
	want := url.Values{
		"caaF2DebugGroup":             {"-1"},
		"enc_password":                {"test-encrypted-password"},
		"isPrivacyPortalReq":          {"false"},
		"jazoest":                     {"2294"},
		"loginAttemptSubmissionCount": {"0"},
		"optIntoOneTap":               {"false"},
		"queryParams":                 {"{}"},
		"stopDeletionNonce":           {""},
		"trustedDeviceRecords":        {"{}"},
		"username":                    {"test-identifier"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected Instagram web login form: got %v, want %v", got, want)
	}
}

func TestInstagramWebTwoFactorFormUsesCurrentFirstPartyContract(t *testing.T) {
	got, err := newInstagramWebTwoFactorForm(
		&instagramWebTwoFactorState{
			identifier: "test-challenge",
			username:   "test-identifier",
		},
		"123456",
		"abc",
		types.SprinkleConfig{ParamName: "jazoest", Version: 2},
	)
	if err != nil {
		t.Fatalf("failed to build Instagram web two-factor form: %v", err)
	}
	want := url.Values{
		"identifier":        {"test-challenge"},
		"jazoest":           {"2294"},
		"queryParams":       {"{}"},
		"trust_this_device": {"1"},
		"username":          {"test-identifier"},
		"verificationCode":  {"123456"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected Instagram web two-factor form: got %v, want %v", got, want)
	}
}

func TestInstagramWebLoginHeadersUseCurrentFirstPartyContract(t *testing.T) {
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(map[cookies.MetaCookieName]string{
		cookies.IGCookieCSRFToken: "test-csrf",
	})
	client := NewClient(ClientParams{
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	moduleLoader := httpclient.NewModuleParser(client, client.http, client.configs)
	configModules := json.RawMessage(`[
		["InstagramWebPushInfo", [], {"rollout_hash":"test-rollout"}, 1],
		["PolarisSiteData", [], {"device_id":"test-web-device","send_device_id_header":true}, 2]
	]`)
	if err := moduleLoader.SSJSHandle(configModules); err != nil {
		t.Fatalf("failed to parse Instagram web request config: %v", err)
	}
	client.configs.WebSessionID = ":test-session"
	headers := http.Header{
		"X-Csrftoken": {"test-csrf"},
		"X-Ig-App-Id": {"test-app"},
	}
	if err := client.addInstagramWebLoginHeaders(headers); err != nil {
		t.Fatalf("failed to add Instagram web login headers: %v", err)
	}
	want := map[string]string{
		"x-instagram-ajax": "test-rollout",
		"x-ig-www-claim":   "0",
		"x-web-device-id":  "test-web-device",
		"x-web-session-id": ":test-session",
	}
	for name, expected := range want {
		if got := headers.Get(name); got != expected {
			t.Fatalf("unexpected %s header: got %q, want %q", name, got, expected)
		}
	}
}

func TestInstagramWebLoginResponseKind(t *testing.T) {
	tests := map[string]struct {
		body []byte
		want string
	}{
		"empty":         {body: nil, want: "empty"},
		"json":          {body: []byte(` {"authenticated":false} `), want: "json"},
		"prefixed json": {body: []byte(`for (;;);{"error":true}`), want: "prefixed_json"},
		"html":          {body: []byte(` <!doctype html><html></html>`), want: "html"},
		"other":         {body: []byte(`not structured`), want: "other"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := instagramWebLoginResponseKind(test.body); got != test.want {
				t.Fatalf("unexpected response kind: got %q, want %q", got, test.want)
			}
		})
	}
}

func TestInstagramWebLoginResponseDiagnostics(t *testing.T) {
	body := []byte(`{"status":"fail","message":"checkpoint_required","checkpoint_url":"/challenge/"}`)
	if got, want := instagramWebLoginResponseKeys(body), []string{"checkpoint_url", "message", "status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected response keys: got %v, want %v", got, want)
	}
	var result instagramWebLoginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse test response: %v", err)
	}
	if got := instagramWebLoginResponseClass(result); got != "challenge_required" {
		t.Fatalf("unexpected response class %q", got)
	}
}

func TestInstagramWebLoginCapturesTwoFactorChallenge(t *testing.T) {
	client := newInstagramWebTwoFactorTestClient(t)
	challenge, err := client.captureInstagramWebTwoFactor(instagramWebLoginResponse{
		TwoFactorRequired: true,
		TwoFactorInfo: instagramWebTwoFactorInfo{
			Identifier:       "test-challenge",
			Username:         "test-identifier",
			TOTP:             true,
			EncryptedContext: "test-encrypted-context",
		},
	}, "fallback-identifier")
	if err != nil {
		t.Fatalf("failed to capture Instagram web two-factor challenge: %v", err)
	}
	if challenge == nil || !challenge.TOTP || challenge.SMS || challenge.WhatsApp {
		t.Fatalf("unexpected public Instagram web two-factor challenge: %+v", challenge)
	}
	if client.webTwoFactor == nil ||
		client.webTwoFactor.encryptedContext != "test-encrypted-context" ||
		client.webTwoFactor.method != "TOTP" {
		t.Fatalf("unexpected private Instagram web two-factor state: %+v", client.webTwoFactor)
	}
}

func TestCompleteInstagramWebSessionTwoFactorLegacy(t *testing.T) {
	client := newInstagramWebTwoFactorTestClient(t)
	client.webTwoFactor = &instagramWebTwoFactorState{
		identifier: "test-challenge",
		username:   "test-identifier",
		method:     "TOTP",
	}
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != client.GetEndpoint("login_two_factor_ajax") {
			t.Fatalf("unexpected Instagram legacy web two-factor request: %s %s", request.Method, request.URL)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("failed to read Instagram legacy web two-factor request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse Instagram legacy web two-factor request: %v", err)
		}
		if form.Get("verificationCode") != "123456" ||
			form.Get("identifier") != "test-challenge" ||
			form.Get("jazoest") != "2294" {
			t.Fatalf("unexpected Instagram legacy web two-factor request fields: %v", form)
		}
		assertInstagramWebTwoFactorHeaders(t, request, client.GetEndpoint("login_two_factor"))
		return instagramWebTwoFactorTestResponse(request, `{"authenticated":true,"status":"ok"}`), nil
	})
	if err := client.CompleteInstagramWebSessionTwoFactor(context.Background(), "123456"); err != nil {
		t.Fatalf("failed to complete Instagram legacy web two-factor login: %v", err)
	}
	if client.webTwoFactor != nil {
		t.Fatal("successful Instagram legacy web two-factor login retained challenge state")
	}
}

func TestCompleteInstagramWebSessionTwoFactorEncrypted(t *testing.T) {
	client := newInstagramWebTwoFactorTestClient(t)
	client.webTwoFactor = &instagramWebTwoFactorState{
		encryptedContext: "test-encrypted-context",
		method:           "TOTP",
	}
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != client.GetEndpoint("graphql") {
			t.Fatalf("unexpected Instagram encrypted web two-factor request: %s %s", request.Method, request.URL)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("failed to read Instagram encrypted web two-factor request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse Instagram encrypted web two-factor request: %v", err)
		}
		if form.Get("doc_id") != instagramWebTwoFactorValidateCodeDocID ||
			form.Get("fb_api_req_friendly_name") != "useTwoFactorLoginValidateCodeMutation" ||
			form.Get("jazoest") != "2294" {
			t.Fatalf("unexpected Instagram encrypted web two-factor wrapper fields: %v", form)
		}
		var variables instagramWebTwoFactorGraphQLVariables
		if err = json.Unmarshal([]byte(form.Get("variables")), &variables); err != nil {
			t.Fatalf("failed to parse Instagram encrypted web two-factor variables: %v", err)
		}
		if variables.Code.Value != "123456" ||
			variables.EncryptedContext != "test-encrypted-context" ||
			variables.Flow != "TWO_FACTOR_LOGIN" ||
			variables.Method != "TOTP" ||
			variables.MaskedContactPoint != nil ||
			!variables.TrustThisDevice {
			t.Fatalf("unexpected Instagram encrypted web two-factor variables: %+v", variables)
		}
		assertInstagramWebTwoFactorHeaders(t, request, client.GetEndpoint("login_two_step_verification"))
		if request.Header.Get("x-fb-friendly-name") != "useTwoFactorLoginValidateCodeMutation" {
			t.Fatalf("unexpected Instagram encrypted web two-factor friendly name: %q", request.Header.Get("x-fb-friendly-name"))
		}
		return instagramWebTwoFactorTestResponse(
			request,
			`{"data":{"xfb_two_factor_login_validate_code":{"is_code_valid":true}}}`,
		), nil
	})
	if err := client.CompleteInstagramWebSessionTwoFactor(context.Background(), "123456"); err != nil {
		t.Fatalf("failed to complete Instagram encrypted web two-factor login: %v", err)
	}
	if client.webTwoFactor != nil {
		t.Fatal("successful Instagram encrypted web two-factor login retained challenge state")
	}
}

func TestCompleteInstagramWebSessionTwoFactorRejectedCodeKeepsChallenge(t *testing.T) {
	client := newInstagramWebTwoFactorTestClient(t)
	client.webTwoFactor = &instagramWebTwoFactorState{
		identifier: "test-challenge",
		username:   "test-identifier",
		method:     "TOTP",
	}
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"status":"fail"}`)),
			Request:    request,
		}, nil
	})
	err := client.CompleteInstagramWebSessionTwoFactor(context.Background(), "000000")
	if !errors.Is(err, ErrInstagramWebTwoFactorCodeRejected) {
		t.Fatalf("expected rejected Instagram web two-factor code error, got %v", err)
	}
	if client.webTwoFactor == nil {
		t.Fatal("rejected Instagram web two-factor code cleared challenge state")
	}
}

func newInstagramWebTwoFactorTestClient(t *testing.T) *Client {
	t.Helper()
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(map[cookies.MetaCookieName]string{
		cookies.IGCookieCSRFToken: "abc",
		cookies.IGCookieMachineID: "test-machine",
		cookies.IGCookieDeviceID:  "test-device",
	})
	client := NewClient(ClientParams{
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	client.configs.BrowserConfigTable.SprinkleConfig = types.SprinkleConfig{
		ParamName: "jazoest",
		Version:   2,
	}
	client.configs.BrowserConfigTable.InstagramWebPushInfo.RolloutHash = "test-rollout"
	client.configs.BrowserConfigTable.PolarisSiteData = types.PolarisSiteData{
		DeviceID:           "test-web-device",
		SendDeviceIDHeader: true,
	}
	client.configs.BrowserConfigTable.CurrentUserInitialData.AppID = "test-app"
	client.configs.BrowserConfigTable.SiteData.Pr = 1
	client.configs.WebSessionID = ":test-session"
	client.configs.LSDToken = "test-lsd"
	return client
}

func assertInstagramWebTwoFactorHeaders(t *testing.T, request *http.Request, referer string) {
	t.Helper()
	want := map[string]string{
		"origin":           "https://www.instagram.com",
		"referer":          referer,
		"x-csrftoken":      "abc",
		"x-ig-app-id":      "test-app",
		"x-ig-www-claim":   "0",
		"x-instagram-ajax": "test-rollout",
		"x-web-device-id":  "test-web-device",
		"x-web-session-id": ":test-session",
	}
	for name, expected := range want {
		if got := request.Header.Get(name); got != expected {
			t.Fatalf("unexpected %s header: got %q, want %q", name, got, expected)
		}
	}
}

func instagramWebTwoFactorTestResponse(request *http.Request, body string) *http.Response {
	headers := http.Header{"Content-Type": {"application/json"}}
	headers.Add("Set-Cookie", "sessionid=test-session-cookie; Path=/")
	headers.Add("Set-Cookie", "ds_user_id=test-user; Path=/")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
