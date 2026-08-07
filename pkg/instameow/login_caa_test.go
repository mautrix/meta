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

func TestInstagramAccountManagerDiscoveryAndSwitchUseCurrentNativeContract(t *testing.T) {
	currentAuthorization := "Bearer IGT:2:current-authorization"
	targetAuthorization := "Bearer IGT:2:" + base64.StdEncoding.EncodeToString(
		[]byte(`{"sessionid":"222%3Atarget-session","ds_user_id":"222"}`),
	)
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(map[cookies.MetaCookieName]string{
		cookies.IGCookieCSRFToken: "test-csrf",
		cookies.IGCookieMachineID: "test-mid",
		cookies.IGCookieDeviceID:  "test-device-id",
		cookies.IGCookieSessionID: "111%3Acurrent-session",
		cookies.IGCookieDSUserID:  "111",
	})
	session := &types.InstagramMobileSession{
		Authorization: currentAuthorization,
		UserID:        "111",
		Username:      "current_user",
		Device: types.InstagramLoginDevice{
			PhoneID:         "phone-id",
			DeviceID:        "device-id",
			AdvertisingID:   "advertising-id",
			AndroidDeviceID: "android-0123456789abcdef",
		},
	}
	client := NewClient(ClientParams{
		Cookies:       loginCookies,
		Log:           zerolog.Nop(),
		MobileSession: session,
	})
	mobile := &mobileLoginState{
		PhoneID:         session.Device.PhoneID,
		DeviceID:        session.Device.DeviceID,
		AdvertisingID:   session.Device.AdvertisingID,
		AndroidDeviceID: session.Device.AndroidDeviceID,
	}
	requestCount := 0
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodPost ||
			request.Header.Get("Authorization") != currentAuthorization {
			t.Fatalf("unexpected Account Manager request: %s %v", request.Method, request.Header)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("failed to read Account Manager request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse Account Manager request: %v", err)
		}
		switch request.URL.Path {
		case "/api/v1/fxcal/get_sso_accounts/":
			var tokens []instagramAccountManagerToken
			if err = json.Unmarshal([]byte(form.Get("tokens")), &tokens); err != nil {
				t.Fatalf("invalid Account Manager discovery token: %v", err)
			}
			expectedToken := newInstagramAccountManagerToken(session)
			if len(tokens) != 1 || tokens[0] != expectedToken ||
				form.Get("surface") != "account_switcher" ||
				form.Get("include_social_context") != "false" {
				t.Fatalf("unexpected Account Manager discovery form: %v", form)
			}
			return mobileLoginTestResponse(request, http.StatusOK, nil, `{
				"result": [
					{
						"token": {"account_type": "Instagram"},
						"connected_accounts": [
							{
								"is_sso_enabled": true,
								"user_fbid": "222",
								"user": {"pk": 222, "pk_id": "222", "id": "222", "username": "linked_user"}
							},
							{
								"is_sso_enabled": true,
								"user_fbid": "111",
								"user": {"pk": 111, "pk_id": "111", "id": "111", "username": "current_user"}
							}
						]
					},
					{
						"token": {"account_type": "Threads"},
						"connected_accounts": [
							{
								"is_sso_enabled": true,
								"user_fbid": "333",
								"user": {"pk": 333, "pk_id": "333", "id": "333", "username": "current_user"}
							}
						]
					}
				]
			}`), nil
		case "/api/v1/fxcal/sso_login/":
			var token instagramAccountManagerToken
			if err = json.Unmarshal([]byte(form.Get("token")), &token); err != nil {
				t.Fatalf("invalid Account Manager login token: %v", err)
			}
			if token != newInstagramAccountManagerToken(session) ||
				form.Get("pk") != "222" ||
				form.Get("adid") != mobile.AdvertisingID ||
				form.Get("device_id") != mobile.AndroidDeviceID ||
				form.Get("guid") != mobile.DeviceID ||
				form.Get("phone_id") != mobile.PhoneID ||
				form.Get("surface") != "account_switcher" {
				t.Fatalf("unexpected Account Manager login form: %v", form)
			}
			return mobileLoginTestResponse(request, http.StatusOK, http.Header{
				"Ig-Set-Authorization": {targetAuthorization},
			}, `{"status":"ok","logged_in_user":{"pk":"222","username":"linked_user"}}`), nil
		default:
			t.Fatalf("unexpected Account Manager path %q", request.URL.Path)
			return nil, nil
		}
	})

	accounts, err := client.getInstagramAccountManagerAccounts(context.Background())
	if err != nil {
		t.Fatalf("failed to discover Account Manager profiles: %v", err)
	}
	expectedAccounts := []instagramAccountManagerAccount{
		{UserID: "111", Username: "current_user"},
		{UserID: "222", Username: "linked_user"},
	}
	if !reflect.DeepEqual(accounts, expectedAccounts) {
		t.Fatalf("unexpected Account Manager profiles: %#v", accounts)
	}
	step := instagramAccountManagerSelectionStep(accounts)
	if step.StepID != "fi.mau.meta.instagram.account_manager" ||
		step.UserInputParams == nil ||
		len(step.UserInputParams.Fields) != 1 ||
		!reflect.DeepEqual(step.UserInputParams.Fields[0].Options, []string{"current_user", "linked_user"}) {
		t.Fatalf("unexpected Account Manager selection step: %#v", step)
	}
	if err = client.switchInstagramAccountManagerAccount(
		context.Background(),
		mobile,
		accounts[1],
	); err != nil {
		t.Fatalf("failed to switch Account Manager profile: %v", err)
	}
	switchedSession := client.GetMobileSession()
	if switchedSession == nil ||
		switchedSession.UserID != "222" ||
		switchedSession.Username != "linked_user" ||
		switchedSession.Authorization != targetAuthorization ||
		loginCookies.Get(cookies.IGCookieSessionID) != "222%3Atarget-session" ||
		loginCookies.Get(cookies.IGCookieDSUserID) != "222" ||
		requestCount != 2 {
		t.Fatalf(
			"Account Manager switch did not persist the target session: session=%#v requests=%d",
			switchedSession,
			requestCount,
		)
	}
}
