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

func TestInstagramCAAValueSkipsEmptyDuplicates(t *testing.T) {
	newAACVariable := func(value string) *bloks.BloksVariable {
		script := &bloks.BloksTreeScript{}
		if err := script.Parse(`(bk.action.ref.Make "` + value + `")`); err != nil {
			t.Fatalf("failed to parse AAC fixture: %v", err)
		}
		return &bloks.BloksVariable{
			Info: bloks.BloksDatumInfo{
				Name:          "CAA_ACCOUNT_ACCESS_CONTEXT:aac",
				InitialScript: script,
			},
		}
	}
	bundle := &bloks.BloksBundle{}
	bundle.Layout.Payload.Variables = []*bloks.BloksVariable{
		newAACVariable(""),
		newAACVariable("current-aac"),
	}

	if got := instagramCAAValue(bundle, ""); got != "current-aac" {
		t.Fatalf("expected populated AAC, got %q", got)
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
		MachineID:       "machine-id",
		USDIDHeader:     "test-usdid-header",
	}
	client.caaLogin = &instagramCAALoginState{
		AAC:              "server-aac",
		WaterfallID:      "server-waterfall",
		AttestationNonce: "server-attestation-nonce",
		Mobile:           client.mobileLogin,
	}

	requestSeen := false
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestSeen = true
		if request.Method != http.MethodPost {
			t.Fatalf("expected Bloks POST, got %s", request.Method)
		}
		expectedPath := "/api/v1/bloks/async_action/" + instagramCAASendEntrypoint + "/"
		if request.URL.Host != "b.i.instagram.com" || request.URL.Path != expectedPath {
			t.Fatalf("unexpected CAA endpoint %q", request.URL.String())
		}
		if request.Header.Get("X-Bloks-Version-Id") != bloks.BloksVersionInstagramAndroid ||
			request.Header.Get("X-Ig-App-Id") != useragent.IGAndroidAppID ||
			!strings.HasPrefix(request.Header.Get("User-Agent"), "Instagram 440.0.0.19.86 Android") ||
			request.Header.Get("X-Meta-Usdid") != "test-usdid-header" ||
			request.Header.Get("X-Fb-Friendly-Name") != "IgApi: bloks/async_action/"+instagramCAASendEntrypoint+"/" {
			t.Fatal("Instagram CAA headers do not match the current signed APK profile")
		}
		var attestParams struct {
			Attestation []struct {
				Version        int    `json:"version"`
				Type           string `json:"type"`
				Errors         []int  `json:"errors"`
				ChallengeNonce string `json:"challenge_nonce"`
			} `json:"attestation"`
		}
		if err := json.Unmarshal([]byte(request.Header.Get("X-Ig-Attest-Params")), &attestParams); err != nil ||
			len(attestParams.Attestation) != 1 ||
			attestParams.Attestation[0].Version != 2 ||
			attestParams.Attestation[0].Type != "keystore" ||
			!reflect.DeepEqual(attestParams.Attestation[0].Errors, []int{-1013}) ||
			attestParams.Attestation[0].ChallengeNonce != "server-attestation-nonce" {
			t.Fatalf("unexpected Instagram attestation parameters: %+v (%v)", attestParams, err)
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
			form.Get("bloks_versioning_id") != bloks.BloksVersionInstagramAndroid {
			t.Fatalf("CAA request did not retain the login-scoped device/version: %v", form)
		}
		var clientContext map[string]string
		if err = json.Unmarshal([]byte(form.Get("bk_client_context")), &clientContext); err != nil {
			t.Fatalf("invalid CAA client context: %v", err)
		}
		if clientContext["bloks_version"] != bloks.BloksVersionInstagramAndroid ||
			clientContext["styles_id"] != "instagram" {
			t.Fatalf("unexpected CAA client context: %+v", clientContext)
		}
		var params map[string]any
		if err = json.Unmarshal([]byte(form.Get("params")), &params); err != nil {
			t.Fatalf("invalid CAA params: %v", err)
		}
		clientParams, clientOK := params["client_input_params"].(map[string]any)
		serverParams, serverOK := params["server_params"].(map[string]any)
		if !clientOK || !serverOK || clientParams["aac"] != "server-aac" ||
			clientParams["password"] != "#PWD_INSTAGRAM:4:test-envelope" ||
			clientParams["contact_point"] != "test-user" ||
			clientParams["password_contains_non_ascii"] != "true" ||
			clientParams["login_attempt_count"] != float64(3) || clientParams["try_num"] != float64(2) ||
			clientParams["device_id"] != "android-0123456789abcdef" ||
			clientParams["family_device_id"] != "family-device-id" || clientParams["machine_id"] != "machine-id" ||
			serverParams["waterfall_id"] != "server-waterfall" || serverParams["qe_device_id"] != "qe-device-id" ||
			serverParams["credential_type"] != "password" || serverParams["caller"] != "gslr" {
			t.Fatalf("unexpected CAA params: %+v", params)
		}
		passwordInputID, passwordInputOK := serverParams["password_text_input_id"].(string)
		usernameInputID, usernameInputOK := serverParams["username_text_input_id"].(string)
		if !passwordInputOK || !usernameInputOK || !strings.HasSuffix(passwordInputID, "ig:82") ||
			!strings.HasSuffix(usernameInputID, "ig:81") ||
			strings.TrimSuffix(passwordInputID, ":82") != strings.TrimSuffix(usernameInputID, ":81") {
			t.Fatalf("unexpected CAA input IDs: password=%q username=%q", passwordInputID, usernameInputID)
		}
		for _, key := range []string{
			"blocked_uids", "sim_phones", "aymh_accounts", "si_device_param_network_info", "sso_accounts_auth_data",
			"flash_call_permission_status", "accounts_list", "gms_incoming_call_retriever_eligibility", "lois_settings", "openid_tokens",
		} {
			if _, present := clientParams[key]; !present {
				t.Fatalf("current CAA client params are missing %q", key)
			}
		}
		for _, key := range []string{
			"two_step_login_type", "login_entry_point", "offline_experiment_group", "ar_event_source",
			"login_surface", "reg_flow_source", "access_flow_version",
		} {
			if _, present := serverParams[key]; !present {
				t.Fatalf("current CAA server params are missing %q", key)
			}
		}
		return mobileLoginTestResponse(request, http.StatusOK, nil, `{}`), nil
	})

	_, _ = client.makeInstagramBloksRequest(
		context.Background(),
		&bloks.BloksActionDocInstagram,
		instagramCAASendEntrypoint,
		bloks.BloksParamsInner{
			"client_input_params": map[string]any{
				"password":                    "#PWD_INSTAGRAM:4:test-envelope",
				"contact_point":               "test-user",
				"password_contains_non_ascii": true,
				"login_attempt_count":         3,
				"try_num":                     2,
			},
			"server_params": map[string]any{},
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
		[]byte(`{"sessionid":"456%3Acaa-session","ds_user_id":"456","shbid":"caa-shbid"}`),
	)
	headers, err := json.Marshal(map[string]any{
		"IG-Set-Authorization": "Bearer IGT:2:" + authorization,
		"IG-Set-Ig-U-Rur":      `"NAO\054456\054caa-rur"`,
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
	if got := loginCookies.Get(cookies.IGCookieRUR); got != `"NAO\054456\054caa-rur"` {
		t.Fatalf("unexpected CAA routing cookie %q", got)
	}
	if got := loginCookies.Get(cookies.IGCookieSHBID); got != "caa-shbid" {
		t.Fatalf("unexpected CAA shbid cookie %q", got)
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
	session := &instagramMobileSession{
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
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	client.mobileSession = session
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
			return mobileLoginTestResponse(request, http.StatusOK, http.Header{
				"Set-Cookie": {"mid=rotated-mid; Path=/; Secure"},
			}, `{
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
	if got := loginCookies.Get(cookies.IGCookieMachineID); got != "rotated-mid" {
		t.Fatalf("Account Manager discovery did not retain rotated machine ID: %q", got)
	}
	step := instagramAccountManagerSelectionStep(accounts)
	if step.StepID != "fi.mau.meta.instagram.account_manager" ||
		step.UserInputParams == nil ||
		len(step.UserInputParams.Fields) != 1 ||
		!reflect.DeepEqual(step.UserInputParams.Fields[0].Options, []string{"current_user", "linked_user"}) {
		t.Fatalf("unexpected Account Manager selection step: %#v", step)
	}
	if err = client.switchInstagramAccountManagerMobileAccount(
		context.Background(),
		mobile,
		accounts[1],
	); err != nil {
		t.Fatalf("failed to switch Account Manager profile: %v", err)
	}
	switchedSession := client.mobileSession
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

func TestInstagramAccountManagerWebSwitchUsesCurrentContract(t *testing.T) {
	loginCookies := &cookies.Cookies{Platform: types.Instagram}
	loginCookies.UpdateValues(map[cookies.MetaCookieName]string{
		cookies.IGCookieCSRFToken: "test-csrf",
		cookies.IGCookieMachineID: "test-mid",
		cookies.IGCookieDeviceID:  "test-device-id",
		cookies.IGCookieSessionID: "111%3Aprimary-session",
		cookies.IGCookieDSUserID:  "111",
	})
	client := NewClient(ClientParams{
		Cookies: loginCookies,
		Log:     zerolog.Nop(),
	})
	client.configs.BrowserConfigTable.DTSGInitData.Token = "test-dtsg"
	client.configs.Jazoest = "test-jazoest"

	requestSeen := false
	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestSeen = true
		if request.Method != http.MethodPost ||
			request.URL.Path != "/api/v1/web/fxcal/ig_sso_login/" {
			t.Fatalf("unexpected Account Manager web request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Requested-With") != "XMLHttpRequest" ||
			request.Header.Get("Origin") != client.GetEndpoint("base_url") ||
			request.Header.Get("Referer") != client.GetEndpoint("messages") {
			t.Fatalf("unexpected Account Manager web headers: %v", request.Header)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("failed to read Account Manager web request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse Account Manager web request: %v", err)
		}
		if form.Get("igUsername") != "linked_user" ||
			form.Get("fb_dtsg") != "test-dtsg" ||
			form.Get("jazoest") != "test-jazoest" {
			t.Fatalf("unexpected Account Manager web form: %v", form)
		}
		return mobileLoginTestResponse(request, http.StatusOK, http.Header{
			"Set-Cookie": {
				"sessionid=222%3Atarget-session; Path=/; Secure; HttpOnly",
				"ds_user_id=222; Path=/; Secure",
			},
		}, `{"status":"ok","authenticated":true,"failure_message":null}`), nil
	})

	if err := client.switchInstagramAccountManagerWebAccount(
		context.Background(),
		"linked_user",
	); err != nil {
		t.Fatalf("failed to switch Account Manager web profile: %v", err)
	}
	if !requestSeen ||
		loginCookies.Get(cookies.IGCookieSessionID) != "222%3Atarget-session" ||
		loginCookies.Get(cookies.IGCookieDSUserID) != "222" {
		t.Fatal("Account Manager web switch did not retain the selected web session")
	}

	client.http.HTTP.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return mobileLoginTestResponse(
			request,
			http.StatusOK,
			nil,
			`{"status":"ok","authenticated":false,"failure_message":null}`,
		), nil
	})
	if err := client.switchInstagramAccountManagerWebAccount(
		context.Background(),
		"linked_user",
	); err == nil {
		t.Fatal("expected an unauthenticated Account Manager web response to be rejected")
	}
}
