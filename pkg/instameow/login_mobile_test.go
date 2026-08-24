package instameow

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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
