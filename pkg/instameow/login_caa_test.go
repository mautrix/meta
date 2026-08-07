package instameow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

func TestInstagramDeviceNetworkInfoMatchesWiFiDeviceContract(t *testing.T) {
	expected := map[string]any{
		"active_subscriptions_info":  []any{},
		"default_subscription_info":  nil,
		"is_airplane_mode":           false,
		"is_active_network_cellular": false,
		"is_device_sms_capable":      true,
		"sim_count":                  2,
		"is_wifi":                    true,
	}
	if actual := instagramDeviceNetworkInfo(); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("unexpected Instagram device-network info:\nactual: %#v\nexpected: %#v", actual, expected)
	}
}

func TestInstagramCAALoginUsesClientLoggerWithoutContextLogger(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.DebugLevel)
	browser, err := bloks.NewBrowser(&bloks.BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	browser.State = bloks.StateEmailPasswordPage
	client := &Client{
		log: &logger,
		caaLogin: &instagramCAALoginState{
			Browser: browser,
		},
	}

	step, err := client.DoInstagramCAALoginSteps(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("DoInstagramCAALoginSteps returned error: %v", err)
	}
	if step == nil || step.Type != bridgev2.LoginStepTypeUserInput {
		t.Fatalf("expected credential input step, got %#v", step)
	}
	logOutput := output.String()
	if !strings.Contains(logOutput, `"message":"Executing login step"`) ||
		!strings.Contains(logOutput, `"cur_state":"enter-email-and-password-page"`) {
		t.Fatalf("CAA state was not written through the client logger: %s", logOutput)
	}
}

func TestInstagramCAABloksRequestUsesCurrentNativeContract(t *testing.T) {
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	client := NewClient(ClientParams{
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	client.mobileLogin = &mobileLoginState{
		PhoneID:         "family-device-id",
		DeviceID:        "qe-device-id",
		AndroidDeviceID: "android-0123456789abcdef",
	}

	requestSeen := false
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestSeen = true
		if request.Method != http.MethodPost {
			t.Fatalf("expected Bloks POST, got %s", request.Method)
		}
		expectedPath := "/api/v1/bloks/async_action/" + instagramCAALoginEntrypoint + "/"
		if request.URL.Path != expectedPath {
			t.Fatalf("unexpected CAA path %q", request.URL.Path)
		}
		if request.Header.Get("X-Bloks-Version-Id") != bloks.BloksVersionInstagram ||
			request.Header.Get("X-Ig-App-Id") != useragent.IGAndroidAppID ||
			!strings.HasPrefix(request.Header.Get("User-Agent"), "Instagram 440.0.0.19.86 Android") {
			t.Fatal("Instagram CAA headers do not match the current signed APK profile")
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("failed to read CAA request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse CAA request: %v", err)
		}
		if form.Get("_uuid") != "qe-device-id" ||
			form.Get("bloks_versioning_id") != bloks.BloksVersionInstagram {
			t.Fatalf("CAA request did not retain the login-scoped device/version: %v", form)
		}
		var clientContext map[string]string
		if err = json.Unmarshal([]byte(form.Get("bk_client_context")), &clientContext); err != nil {
			t.Fatalf("invalid CAA client context: %v", err)
		}
		if clientContext["bloks_version"] != bloks.BloksVersionInstagram ||
			clientContext["styles_id"] != "instagram" {
			t.Fatalf("unexpected CAA client context: %+v", clientContext)
		}
		var params map[string]any
		if err = json.Unmarshal([]byte(form.Get("params")), &params); err != nil {
			t.Fatalf("invalid CAA params: %v", err)
		}
		if params["offline_experiment_group"] != "caa_iteration_v3_perf_ig_4" ||
			params["device_id"] != "android-0123456789abcdef" ||
			params["qe_device_id"] != "qe-device-id" {
			t.Fatalf("unexpected CAA params: %+v", params)
		}
		return mobileLoginTestResponse(request, http.StatusOK, nil, `{}`), nil
	})

	_, _ = client.makeInstagramBloksRequest(
		context.Background(),
		&bloks.BloksActionDocInstagram,
		instagramCAALoginEntrypoint,
		bloks.BloksParamsInner{
			"device_id":                "android-0123456789abcdef",
			"qe_device_id":             "qe-device-id",
			"offline_experiment_group": "caa_iteration_v3_perf_ig_4",
		},
		"",
		"",
	)
	if !requestSeen {
		t.Fatal("Instagram CAA request was not sent")
	}
}

func TestApplyInstagramCAALoginConvertsAuthorizationToCookies(t *testing.T) {
	authorization := base64.StdEncoding.EncodeToString(
		[]byte(`{"sessionid":"456%3Acaa-session","ds_user_id":"456"}`),
	)
	headers, err := json.Marshal(map[string]any{
		"IG-Set-Authorization": "Bearer IGT:2:" + authorization,
		"IG-Set-X-Mid":         "caa-mid",
		"X-Response-Code":      200,
		"X-Response-Flags":     []any{"native", true},
	})
	if err != nil {
		t.Fatalf("failed to marshal test headers: %v", err)
	}
	loginResponse := `{"status":"ok","logged_in_user":{"pk":"456","username":"caa-user"}}`
	loginData, err := json.Marshal(instagramCAALoginResponse{
		Headers:       string(headers),
		LoginResponse: loginResponse,
	})
	if err != nil {
		t.Fatalf("failed to marshal test login response: %v", err)
	}

	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(nil)
	client := NewClient(ClientParams{
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	mobile := &mobileLoginState{
		PhoneID:         "family-device-id",
		DeviceID:        "qe-device-id",
		AndroidDeviceID: "android-0123456789abcdef",
		CSRFToken:       "caa-csrf",
	}
	state := &instagramCAALoginState{
		Browser: &bloks.Browser{LoginData: string(loginData)},
		Mobile:  mobile,
	}
	if err = client.applyInstagramCAALogin(context.Background(), state); err != nil {
		t.Fatalf("applyInstagramCAALogin returned error: %v", err)
	}
	if got := loginCookies.Get(cookies.IGCookieSessionID); got != "456%3Acaa-session" {
		t.Fatalf("unexpected CAA session cookie %q", got)
	}
	if got := loginCookies.Get(cookies.IGCookieDSUserID); got != "456" {
		t.Fatalf("unexpected CAA user ID cookie %q", got)
	}
	if got := loginCookies.Get(cookies.IGCookieMachineID); got != "caa-mid" {
		t.Fatalf("unexpected CAA machine ID cookie %q", got)
	}
	if loginCookies.Get(cookies.IGCookieCSRFToken) == "" ||
		loginCookies.Get(cookies.IGCookieDeviceID) == "" {
		t.Fatal("CAA login did not create the required web-session cookies")
	}
}

func TestParseInstagramCAAResponseHeadersRejectsStructuredValues(t *testing.T) {
	_, err := parseInstagramCAAResponseHeaders(`{"X-Invalid":{"nested":"value"}}`)
	if err == nil {
		t.Fatal("expected structured header value to be rejected")
	}
}
