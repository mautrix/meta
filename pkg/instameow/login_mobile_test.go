package instameow

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestMobileLoginDevicePersistsAcrossClients(t *testing.T) {
	var persisted *types.InstagramLoginDevice
	saveCalls := 0
	newClientWithDevice := func(device *types.InstagramLoginDevice) (*Client, *bool) {
		loginCookies := &cookies.Cookies{Platform: types.Instagram}
		loginCookies.UpdateValues(nil)
		client := NewClient(ClientParams{
			Cookies:           loginCookies,
			Log:               zerolog.Nop(),
			MobileLoginDevice: device,
			SaveMobileLoginDevice: func(
				_ context.Context,
				saved types.InstagramLoginDevice,
			) error {
				saveCalls++
				savedCopy := saved
				persisted = &savedCopy
				return nil
			},
		})
		machineIDHeaderPresent := false
		client.http.HTTP.Transport = roundTripFunc(
			func(request *http.Request) (*http.Response, error) {
				machineIDHeaderPresent = request.Header.Get("X-Mid") != ""
				return mobileLoginTestResponse(request, http.StatusOK, http.Header{
					"Ig-Set-Password-Encryption-Key-Id":  {"145"},
					"Ig-Set-Password-Encryption-Pub-Key": {"test-public-key"},
					"Ig-Set-X-Mid":                       {"stable-machine-id"},
				}, `{}`), nil
			},
		)
		return client, &machineIDHeaderPresent
	}

	firstClient, firstHadMachineID := newClientWithDevice(nil)
	firstState, err := firstClient.prepareMobilePasswordLogin(context.Background())
	if err != nil {
		t.Fatalf("failed to prepare first mobile login: %v", err)
	}
	if *firstHadMachineID {
		t.Fatal("new app installation unexpectedly sent a machine ID before the server issued one")
	}
	if persisted == nil || persisted.MachineID != "stable-machine-id" {
		t.Fatalf(
			"first mobile login did not persist the complete installation identity: %#v",
			persisted,
		)
	}
	firstDevice := firstState.device()
	if firstDevice != *persisted {
		t.Fatalf(
			"persisted installation identity does not match the active client: %#v != %#v",
			*persisted,
			firstDevice,
		)
	}
	firstSaveCalls := saveCalls

	persistedCopy := *persisted
	secondClient, secondHadMachineID := newClientWithDevice(&persistedCopy)
	secondState, err := secondClient.prepareMobilePasswordLogin(context.Background())
	if err != nil {
		t.Fatalf("failed to prepare restored mobile login: %v", err)
	}
	if !*secondHadMachineID {
		t.Fatal("restored app installation did not send its server-issued machine ID")
	}
	if secondState.device() != firstDevice {
		t.Fatalf(
			"restored app installation changed identity: %#v != %#v",
			secondState.device(),
			firstDevice,
		)
	}
	if saveCalls != firstSaveCalls {
		t.Fatalf(
			"unchanged restored identity was persisted again: before=%d after=%d",
			firstSaveCalls,
			saveCalls,
		)
	}
}

func TestMobileResponseCookiesDoNotChangeSharedCookieSemantics(t *testing.T) {
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(map[cookies.MetaCookieName]string{
		cookies.IGCookieSessionID: "old-session",
		cookies.IGCookieCSRFToken: "old-csrf",
	})
	client := NewClient(ClientParams{
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	response := &http.Response{Header: make(http.Header)}
	response.Header.Add("Set-Cookie", "sessionid=new-session; Path=/; Secure")
	response.Header.Add("Set-Cookie", "csrftoken=deleted; Path=/; Max-Age=0")
	response.Header.Add(
		"Set-Cookie",
		"ds_user_id=123; Path=/; Expires="+
			time.Now().Add(time.Hour).UTC().Format(http.TimeFormat),
	)
	response.Header.Set("x-ig-set-www-claim", "claim")

	client.updateMobileResponseCookies(response)

	if got := loginCookies.Get(cookies.IGCookieSessionID); got != "new-session" {
		t.Fatalf("expected mobile session cookie to be retained, got %q", got)
	}
	if got := loginCookies.Get(cookies.IGCookieCSRFToken); got != "" {
		t.Fatalf("expected deleted mobile CSRF cookie to be absent, got %q", got)
	}
	if got := loginCookies.Get(cookies.IGCookieDSUserID); got != "123" {
		t.Fatalf("expected mobile user ID cookie, got %q", got)
	}
	if loginCookies.IGWWWClaim != "claim" {
		t.Fatalf("expected mobile www claim to be updated, got %q", loginCookies.IGWWWClaim)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func mobileLoginTestResponse(
	request *http.Request,
	statusCode int,
	headers http.Header,
	body string,
) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
