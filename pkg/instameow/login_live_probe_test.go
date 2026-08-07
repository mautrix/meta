//go:build liveprobe

package instameow

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestLivePrepareInstagramCAALogin(t *testing.T) {
	if os.Getenv("MAUTRIX_META_LIVE_CAA_LOGIN_BOOTSTRAP") != "1" {
		t.Skip(
			"set MAUTRIX_META_LIVE_CAA_LOGIN_BOOTSTRAP=1 " +
				"to verify Instagram's anonymous CAA login contract",
		)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	testLog := zerolog.New(zerolog.NewTestWriter(t)).Level(zerolog.DebugLevel)
	ctx = testLog.WithContext(ctx)
	client := NewClient(ClientParams{
		Cookies:                    loginCookies,
		Log:                        testLog,
		EnableMobileTLSFingerprint: true,
	})
	state, err := client.prepareInstagramCAALogin(ctx)
	if err != nil {
		t.Fatalf("failed to prepare anonymous Instagram CAA browser: %v", err)
	}
	if state.Mobile.MachineID == "" {
		t.Fatal("anonymous Instagram CAA preflight did not receive a server-issued machine ID")
	}
	if !reflect.DeepEqual(
		state.Browser.Bridge.DeviceNetworkInfo,
		instagramDeviceNetworkInfo(),
	) {
		t.Fatal("anonymous Instagram CAA browser did not use the coherent network profile")
	}

	originalRequest := state.Browser.Config.MakeBloksRequest
	var initialAppID string
	state.Browser.Config.MakeBloksRequest = func(
		requestContext context.Context,
		doc *bloks.BloksDoc,
		appID string,
		params bloks.BloksParamsInner,
		deviceID string,
		familyDeviceID string,
	) (*bloks.BloksBundle, error) {
		initialAppID = appID
		return originalRequest(
			requestContext,
			doc,
			appID,
			params,
			deviceID,
			familyDeviceID,
		)
	}

	step, err := state.Browser.DoLoginStep(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("anonymous Instagram CAA initial action returned error: %v", err)
	}
	if step != nil || state.Browser.State != bloks.StateEmailPasswordPage {
		t.Fatalf(
			"anonymous Instagram CAA initial action did not reach credential page: "+
				"state=%q step=%+v",
			state.Browser.State,
			step,
		)
	}
	if initialAppID != "com.bloks.www.caa.login.login_homepage" {
		t.Fatalf("anonymous Instagram CAA bootstrap used unexpected RPC %q", initialAppID)
	}

	step, err = state.Browser.DoLoginStep(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("anonymous Instagram CAA credential step returned error: %v", err)
	}
	if step == nil ||
		step.StepID != "fi.mau.meta.instagram.caa.email_password" ||
		step.UserInputParams == nil ||
		len(step.UserInputParams.Fields) != 2 ||
		step.UserInputParams.Fields[0].ID != "username" ||
		step.UserInputParams.Fields[1].ID != "password" {
		t.Fatalf("anonymous Instagram CAA login fields are incomplete: %+v", step)
	}

	syntheticNetworkStop := errors.New("synthetic stop before login request")
	var syntheticAppID string
	var syntheticParams bloks.BloksParamsInner
	state.Browser.Config.MakeBloksRequest = func(
		_ context.Context,
		_ *bloks.BloksDoc,
		appID string,
		params bloks.BloksParamsInner,
		_ string,
		_ string,
	) (*bloks.BloksBundle, error) {
		syntheticAppID = appID
		syntheticParams = params
		return nil, syntheticNetworkStop
	}
	_, err = state.Browser.DoLoginStep(ctx, map[string]string{
		"username": "synthetic-caa@example.invalid",
		"password": "synthetic-test-password",
	})
	if !errors.Is(err, syntheticNetworkStop) &&
		(err == nil ||
			!strings.Contains(err.Error(), "tapping instagram login button failed (other)")) {
		t.Fatalf("synthetic CAA submit did not reach the network boundary cleanly: %v", err)
	}
	if syntheticAppID != "com.bloks.www.bloks.caa.login.async.send_login_request" {
		t.Fatalf("synthetic CAA submit reached unexpected action %q", syntheticAppID)
	}
	clientInput, ok := syntheticParams["client_input_params"].(map[string]any)
	if !ok {
		t.Fatalf(
			"synthetic CAA submit did not contain client input parameters: %T",
			syntheticParams["client_input_params"],
		)
	}
	if secureNonces, exists := clientInput["ig_vetted_device_nonces"]; !exists ||
		secureNonces != nil {
		t.Fatalf(
			"synthetic CAA submit did not match clean-installation secure nonces: "+
				"value=%v type=%T",
			secureNonces,
			secureNonces,
		)
	}
	serverParams, ok := syntheticParams["server_params"].(map[string]any)
	if !ok {
		t.Fatalf(
			"synthetic CAA submit did not contain server parameters: %T",
			syntheticParams["server_params"],
		)
	}
	fromLoggedOut := false
	switch value := serverParams["is_from_logged_out"].(type) {
	case int64:
		fromLoggedOut = value == 1
	case float64:
		fromLoggedOut = value == 1
	}
	if !fromLoggedOut {
		t.Fatalf(
			"synthetic CAA submit did not retain logged-out routing: value=%v type=%T",
			serverParams["is_from_logged_out"],
			serverParams["is_from_logged_out"],
		)
	}
}
