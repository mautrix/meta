package instameow

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func newInstagramWebTwoFactorTestClient() (*Client, *cookies.Cookies) {
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(map[cookies.MetaCookieName]string{
		cookies.IGCookieCSRFToken: "test-csrf",
		cookies.IGCookieMachineID: "test-mid",
		cookies.IGCookieDeviceID:  "test-device-id",
	})
	client := NewClient(ClientParams{
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	config := client.configs.BrowserConfigTable
	config.InstagramWebPushInfo.RolloutHash = "test-rollout"
	config.CurrentUserInitialData.AppID = "936619743392459"
	config.SprinkleConfig = types.SprinkleConfig{ParamName: "jazoest", Version: 2}
	client.configs.WebSessionID = "test-web-session"
	return client, loginCookies
}

func legacyInstagramWebTwoFactorResponse(sms, totp bool) instagramWebLoginResponse {
	var result instagramWebLoginResponse
	result.TwoFactorRequired = true
	result.TwoFactorInfo.Identifier = "legacy-identifier"
	result.TwoFactorInfo.Username = "legacy_user"
	result.TwoFactorInfo.SMS = sms
	result.TwoFactorInfo.TOTP = totp
	return result
}

func TestInstagramWebLegacySMSTwoFactorRequestsCode(t *testing.T) {
	client, _ := newInstagramWebTwoFactorTestClient()
	requests := 0
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/web/accounts/send_two_factor_login_sms/" {
			t.Fatalf("unexpected two-factor request: %s %s", request.Method, request.URL.Path)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("failed to read two-factor SMS request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse two-factor SMS request: %v", err)
		}
		if form.Get("identifier") != "legacy-identifier" || form.Get("username") != "legacy_user" || form.Get("jazoest") == "" {
			t.Fatalf("unexpected two-factor SMS form: %v", form)
		}
		if request.Header.Get("x-csrftoken") != "test-csrf" {
			t.Fatalf("two-factor SMS request is missing the CSRF header: %v", request.Header)
		}
		return mobileLoginTestResponse(request, http.StatusOK, nil,
			`{"status":"ok","two_factor_info":{"two_factor_identifier":"sent-identifier"}}`), nil
	})

	challenge, err := client.captureInstagramWebTwoFactor(
		context.Background(),
		legacyInstagramWebTwoFactorResponse(true, false),
		"legacy_user",
		"test-csrf",
		http.StatusOK,
	)
	if err != nil {
		t.Fatalf("captureInstagramWebTwoFactor returned error: %v", err)
	}
	if challenge == nil || !challenge.SMS {
		t.Fatalf("expected an SMS challenge, got %#v", challenge)
	}
	if requests != 1 {
		t.Fatalf("expected one SMS request, got %d", requests)
	}
	state := client.webTwoFactor
	if state.identifier != "sent-identifier" {
		t.Fatalf("identifier from the SMS response was not kept: %q", state.identifier)
	}
	if state.smsReplacementSent {
		t.Fatal("the initial SMS must not use up the replacement SMS after a rejected code")
	}
}

func TestInstagramWebLegacySMSTwoFactorRequestFailureIsNotFatal(t *testing.T) {
	client, _ := newInstagramWebTwoFactorTestClient()
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return mobileLoginTestResponse(request, http.StatusBadRequest, nil,
			`{"status":"fail","error_type":"rate_limit_error","message":"Please wait a few minutes before you try again."}`), nil
	})

	challenge, err := client.captureInstagramWebTwoFactor(
		context.Background(),
		legacyInstagramWebTwoFactorResponse(true, false),
		"legacy_user",
		"test-csrf",
		http.StatusOK,
	)
	if err != nil {
		t.Fatalf("a failed SMS request should still show the code step, got %v", err)
	}
	if challenge == nil || !challenge.SMS || client.webTwoFactor.identifier != "legacy-identifier" {
		t.Fatalf("unexpected challenge state: %#v %#v", challenge, client.webTwoFactor)
	}
}

func TestInstagramWebLegacyTOTPTwoFactorDoesNotRequestSMS(t *testing.T) {
	client, _ := newInstagramWebTwoFactorTestClient()
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected request for an authenticator-app challenge: %s %s", request.Method, request.URL.Path)
		return nil, nil
	})

	challenge, err := client.captureInstagramWebTwoFactor(
		context.Background(),
		legacyInstagramWebTwoFactorResponse(true, true),
		"legacy_user",
		"test-csrf",
		http.StatusOK,
	)
	if err != nil {
		t.Fatalf("captureInstagramWebTwoFactor returned error: %v", err)
	}
	if challenge == nil || !challenge.TOTP {
		t.Fatalf("expected a TOTP challenge, got %#v", challenge)
	}
}
