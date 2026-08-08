package bloks

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestInstagramLoginSubmissionErrorDiagnosticIsPrivacySafe(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		expectedKind   string
		expectedDetail string
	}{
		{
			name:           "server rejection",
			err:            CheckpointError{errors.New("private account-specific rejection text")},
			expectedKind:   "server_rejection",
			expectedDetail: "",
		},
		{
			name:           "unimplemented function",
			err:            errors.New("callback: unimplemented function jmu (2 args)"),
			expectedKind:   "unimplemented_function",
			expectedDetail: "jmu",
		},
		{
			name:           "unexpected screen",
			err:            errors.New("callback: unexpected new screen com.bloks.www.test.screen"),
			expectedKind:   "unexpected_screen",
			expectedDetail: "com.bloks.www.test.screen",
		},
		{
			name:           "unknown error",
			err:            errors.New("private opaque provider text"),
			expectedKind:   "other",
			expectedDetail: "",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			kind, detail := instagramLoginSubmissionErrorDiagnostic(testCase.err)
			if kind != testCase.expectedKind || detail != testCase.expectedDetail {
				t.Fatalf(
					"unexpected diagnostic kind=%q detail=%q, want kind=%q detail=%q",
					kind,
					detail,
					testCase.expectedKind,
					testCase.expectedDetail,
				)
			}
		})
	}
}

func TestInstagramInitialLoginParamsMatchAndroidContract(t *testing.T) {
	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	browser.Bridge.DeviceID = "qe-device-id"
	browser.Bridge.AndroidDeviceID = "android-device-id"
	browser.Bridge.FamilyDeviceID = "family-device-id"

	params, err := browser.initialLoginParams()
	if err != nil {
		t.Fatalf("failed to build initial Instagram login parameters: %v", err)
	}
	waterfallID, ok := params["waterfall_id"].(string)
	if !ok {
		t.Fatalf("Instagram waterfall ID has unexpected type %T", params["waterfall_id"])
	}
	if _, err = uuid.Parse(waterfallID); err != nil {
		t.Fatalf("Instagram waterfall ID is not a UUID: %v", err)
	}
	delete(params, "waterfall_id")

	expected := BloksParamsInner{
		"INTERNAL_INFRA_THEME": "THREE_NEUTRAL_GRAY",
		"account_list":         []any{},
		"auto_login_interstitial_experiment_group_name": "",
		"blocked_uid":        []any{},
		"device_id":          "android-device-id",
		"disable_auto_login": false,
		"disable_recursive_auto_login_interstitial": true,
		"family_device_id":                          "family-device-id",
		"is_from_logged_in_switcher":                false,
		"is_from_logged_out":                        true,
		"is_from_registration_reminder":             false,
		"last_auto_login_time":                      int64(0),
		"launched_url":                              "",
		"layered_homepage_experiment_group":         "Deploy: Not in Experiment",
		"logged_out_user":                           "",
		"logout_source":                             "",
		"offline_experiment_group":                  "caa_iteration_v3_perf_ig_4",
		"qe_device_id":                              "qe-device-id",
		"qpl_join_id":                               nil,
		"show_internal_settings":                    false,
		"sim_phone_numbers":                         []any{},
		"switcher_logged_in_uid":                    "",
		"use_auto_login_interstitial":               true,
	}
	if !reflect.DeepEqual(params, expected) {
		t.Fatalf("unexpected initial Instagram login parameters:\nactual: %#v\nexpected: %#v", params, expected)
	}
}

func TestInstagramDirectLoginParamsMatchAndroidContract(t *testing.T) {
	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	browser.Bridge.DeviceID = "qe-device-id"
	browser.Bridge.AndroidDeviceID = "android-device-id"
	browser.Bridge.FamilyDeviceID = "family-device-id"

	params := browser.instagramDirectLoginParams()
	waterfallID, ok := params["waterfall_id"].(string)
	if !ok {
		t.Fatalf("Instagram direct-login waterfall ID has unexpected type %T", params["waterfall_id"])
	}
	if _, err = uuid.Parse(waterfallID); err != nil {
		t.Fatalf("Instagram direct-login waterfall ID is not a UUID: %v", err)
	}
	delete(params, "waterfall_id")

	expected := BloksParamsInner{
		"device_id":                  "android-device-id",
		"disable_auto_login":         false,
		"family_device_id":           "family-device-id",
		"is_caa_perf_enabled":        true,
		"is_from_logged_in_switcher": false,
		"is_from_logged_out":         true,
		"last_auto_login_time":       int64(0),
		"logged_out_user":            "",
		"logout_source":              "",
		"offline_experiment_group":   "caa_iteration_v3_perf_ig_4",
		"qe_device_id":               "qe-device-id",
		"qpl_join_id":                nil,
		"show_internal_settings":     false,
	}
	if !reflect.DeepEqual(params, expected) {
		t.Fatalf("unexpected direct Instagram login parameters:\nactual: %#v\nexpected: %#v", params, expected)
	}
}

func TestMessengerInitialLoginParamsRemainUnchanged(t *testing.T) {
	testCases := []struct {
		name     string
		platform types.Platform
		expected BloksParamsInner
	}{
		{
			name:     "iOS",
			platform: types.MessengerLiteIOS,
			expected: BloksParamsInner{
				"account_list": []any{},
				"auto_login_interstitial_experiment_group": "",
				"blocked_uid":        []any{},
				"device_id":          "device-id",
				"disable_auto_login": false,
				"disable_recursive_auto_login_interstitial": true,
				"family_device_id":                          "family-device-id",
				"is_from_logged_in_switcher":                false,
				"layered_homepage_experiment_group":         "not_in_experiment",
				"machine_id":                                "machine-id",
				"offline_experiment_group":                  "caa_iteration_v2_perf_ls_ios_test_1",
				"show_internal_settings":                    false,
				"use_auto_login_interstitial":               true,
			},
		},
		{
			name:     "Android",
			platform: types.MessengerLiteAndroid,
			expected: BloksParamsInner{
				"INTERNAL_INFRA_THEME":     "THREE_NEUTRAL_GRAY",
				"account_list":             []any{},
				"blocked_uid":              []any{},
				"device_emails":            []any{},
				"device_id":                "device-id",
				"disable_auto_login":       false,
				"family_device_id":         "family-device-id",
				"offline_experiment_group": "caa_iteration_v3_perf_msg_6",
				"openid_tokens":            map[string]any{},
				"show_internal_settings":   false,
				"spectra_guardian_token":   "",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			browser, err := NewBrowser(&BrowserConfig{Platform: testCase.platform})
			if err != nil {
				t.Fatalf("failed to create Messenger Bloks browser: %v", err)
			}
			browser.Bridge.DeviceID = "device-id"
			browser.Bridge.FamilyDeviceID = "family-device-id"
			browser.Bridge.MachineID = "machine-id"

			params, err := browser.initialLoginParams()
			if err != nil {
				t.Fatalf("failed to build Messenger initial login parameters: %v", err)
			}
			waterfallID, ok := params["waterfall_id"].(string)
			if !ok || len(waterfallID) != 32 {
				t.Fatalf("unexpected Messenger waterfall ID %#v", params["waterfall_id"])
			}
			delete(params, "waterfall_id")
			if !reflect.DeepEqual(params, testCase.expected) {
				t.Fatalf("Messenger initial parameters changed:\nactual: %#v\nexpected: %#v", params, testCase.expected)
			}
		})
	}
}

func TestInstagramAuthenticationConfirmationScreen(t *testing.T) {
	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	page := &BloksBundle{}
	err = browser.Bridge.DisplayNewScreen(
		context.Background(),
		"com.bloks.www.caa.ar.authentication_confirmation",
		page,
	)
	if err != nil {
		t.Fatalf("authentication confirmation screen was rejected: %v", err)
	}
	if browser.State != StateAuthenticationConfirm {
		t.Fatalf("unexpected authentication confirmation state %q", browser.State)
	}
	if browser.CurrentPage != page {
		t.Fatal("authentication confirmation page was not retained")
	}
}

func TestInstagramAuthenticationConfirmationIsAccountRecovery(t *testing.T) {
	title := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("Enter a code"),
	})
	detail := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("We sent a code for this login."),
	})
	input := testTreeComponent("bk.components.TextInput", nil)
	continueButton := testTreeButton("Continue")
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(title, detail, input, continueButton),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
	}

	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	err = browser.Bridge.DisplayNewScreen(
		context.Background(),
		"com.bloks.www.caa.ar.authentication_confirmation",
		page,
	)
	if err != nil {
		t.Fatalf("authentication confirmation code screen was rejected: %v", err)
	}
	if browser.State != StateAccountRecoveryPage {
		t.Fatalf("authentication confirmation input mapped to %q", browser.State)
	}

	step, err := browser.DoLoginStep(context.Background(), map[string]string{})
	if step != nil {
		t.Fatalf("account recovery unexpectedly returned a login step: %#v", step)
	}
	var responseError bridgev2.RespError
	if !errors.As(err, &responseError) {
		t.Fatalf("account recovery returned unexpected error: %v", err)
	}
	if responseError.ErrCode != "FI.MAU.META_ACCOUNT_RECOVERY_REQUIRED" ||
		responseError.StatusCode != http.StatusBadRequest ||
		!strings.Contains(responseError.Err, "This is not a two-factor code") {
		t.Fatalf("unexpected account recovery response: %#v", responseError)
	}

	browser.State = StateAuthenticationConfirm
	step, err = browser.DoLoginStep(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("failed to recover code input from confirmation state: %v", err)
	}
	if step != nil || browser.State != StateAccountRecoveryPage {
		t.Fatalf("confirmation state did not recover code input: state=%q step=%#v", browser.State, step)
	}
}

func TestInstagramAccountSelectionScreens(t *testing.T) {
	for _, screen := range []string{
		"com.bloks.www.caa.ar.select_account",
		"com.bloks.www.caa.login.aymh_multiple_profiles_screen_entry",
	} {
		t.Run(screen, func(t *testing.T) {
			browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
			if err != nil {
				t.Fatalf("failed to create Bloks browser: %v", err)
			}
			page := &BloksBundle{}
			err = browser.Bridge.DisplayNewScreen(context.Background(), screen, page)
			if err != nil {
				t.Fatalf("account selection screen was rejected: %v", err)
			}
			if browser.State != StateAccountSelectionPage {
				t.Fatalf("unexpected account selection state %q", browser.State)
			}
		})
	}
}

func TestInstagramAccountSelectionStep(t *testing.T) {
	firstButton := testTreeButton("First profile", "@first")
	secondButton := testTreeButton("Second profile", "@second")
	otherButton := testTreeButton("Log in to another account")
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(firstButton, secondButton, otherButton),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
	}
	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	browser.State = StateAccountSelectionPage
	browser.CurrentPage = page

	step, err := browser.DoLoginStep(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("failed to build account selection step: %v", err)
	}
	if step == nil || step.StepID != "fi.mau.meta.instagram.caa.account_selection" {
		t.Fatalf("unexpected account selection step: %#v", step)
	}
	if step.UserInputParams == nil || len(step.UserInputParams.Fields) != 1 {
		t.Fatalf("unexpected account selection fields: %#v", step.UserInputParams)
	}
	field := step.UserInputParams.Fields[0]
	if field.ID != "account" {
		t.Fatalf("unexpected account selection field ID %q", field.ID)
	}
	expected := []string{"First profile · @first", "Second profile · @second"}
	if !reflect.DeepEqual(field.Options, expected) {
		t.Fatalf("unexpected account options: %#v", field.Options)
	}
}

func TestPlatformLoginErrors(t *testing.T) {
	facebookBrowser, err := NewBrowser(&BrowserConfig{Platform: types.MessengerLiteIOS})
	if err != nil {
		t.Fatalf("failed to create Facebook Bloks browser: %v", err)
	}
	instagramBrowser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Instagram Bloks browser: %v", err)
	}

	testCases := []struct {
		name             string
		getError         func(*Browser) bridgev2.RespError
		facebookError    bridgev2.RespError
		instagramMessage string
	}{
		{
			name:             "invalid username",
			getError:         (*Browser).invalidUsernameLoginError,
			facebookError:    ErrLoginInvalidUsername,
			instagramMessage: "That doesn't look like a valid Instagram username or email address",
		},
		{
			name:             "mandatory OAuth",
			getError:         (*Browser).mandatoryOAuthLoginError,
			facebookError:    ErrLoginMandatoryOAuth,
			instagramMessage: "Meta is requiring Google sign-in, which is not supported. Please add a different two-factor method in the official app or website",
		},
		{
			name:             "no supported MFA",
			getError:         (*Browser).noSupportedMFALoginError,
			facebookError:    ErrLoginNoSupportedMFA,
			instagramMessage: "None of the available two-factor methods are supported. Please add a different method in the official app or website",
		},
		{
			name:             "reCAPTCHA",
			getError:         (*Browser).reCaptchaLoginError,
			facebookError:    ErrLoginReCaptcha,
			instagramMessage: "Meta is requiring Google reCAPTCHA, which is not supported in this native flow. Try again later or complete the security check in the official app or website",
		},
		{
			name:             "no SMS available",
			getError:         (*Browser).noSMSAvailableLoginError,
			facebookError:    ErrLoginNoSMSAvailable,
			instagramMessage: "Meta can't send an SMS code right now. Try again later or use a different two-factor method",
		},
		{
			name:             "mandatory passkey",
			getError:         (*Browser).mandatoryPasskeyLoginError,
			facebookError:    ErrLoginMandatoryPasskey,
			instagramMessage: "Meta is requiring a passkey, which is not supported. Please add a different two-factor method in the official app or website",
		},
		{
			name: "uninformative rejection",
			getError: func(browser *Browser) bridgev2.RespError {
				return browser.uninformativeLoginError("test callsite")
			},
			facebookError:    ErrLoginUninformative("test callsite"),
			instagramMessage: "Instagram rejected the login without providing a reason. Try again later or complete the security check in the official app or website",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual := testCase.getError(facebookBrowser); !reflect.DeepEqual(actual, testCase.facebookError) {
				t.Errorf("Facebook error changed:\nactual: %#v\nexpected: %#v", actual, testCase.facebookError)
			}

			expectedInstagramError := testCase.facebookError
			expectedInstagramError.Err = testCase.instagramMessage
			if actual := testCase.getError(instagramBrowser); !reflect.DeepEqual(actual, expectedInstagramError) {
				t.Errorf("unexpected Instagram error:\nactual: %#v\nexpected: %#v", actual, expectedInstagramError)
			}
		})
	}
}

func TestMessengerLoginScreenBehaviorRemainsUnchanged(t *testing.T) {
	for screen, expectedState := range map[string]BrowserState{
		"com.bloks.www.caa.ar.code_entry":                   StateCodeEntryPage,
		"com.bloks.www.caa.ar.auth_method":                  StateMFALandingPage,
		"com.bloks.www.two_step_verification.entrypoint":    StateMFALandingPage,
		"com.bloks.www.ap.two_step_verification.code_entry": StateCodeEntryPage,
	} {
		t.Run(screen, func(t *testing.T) {
			browser, err := NewBrowser(&BrowserConfig{Platform: types.MessengerLiteIOS})
			if err != nil {
				t.Fatalf("failed to create Messenger Bloks browser: %v", err)
			}
			if err = browser.Bridge.DisplayNewScreen(
				context.Background(),
				screen,
				&BloksBundle{},
			); err != nil {
				t.Fatalf("existing Messenger screen was rejected: %v", err)
			}
			if browser.State != expectedState {
				t.Fatalf("Messenger screen mapped to %q instead of %q", browser.State, expectedState)
			}
		})
	}
}

func TestMessengerCodeEntryCopyRemainsUnchanged(t *testing.T) {
	instructions := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("Enter the code we sent you"),
	})
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(instructions),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
	}

	browser, err := NewBrowser(&BrowserConfig{Platform: types.MessengerLiteIOS})
	if err != nil {
		t.Fatalf("failed to create Messenger Bloks browser: %v", err)
	}
	browser.State = StateCodeEntryPage
	browser.CurrentPage = page
	step, err := browser.DoLoginStep(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("Messenger code-entry step failed: %v", err)
	}
	if step == nil || step.Instructions != "Enter the code we sent you" {
		t.Fatalf("Messenger code instructions changed: %#v", step)
	}
	if step.UserInputParams == nil ||
		len(step.UserInputParams.Fields) != 1 ||
		step.UserInputParams.Fields[0].Name != "One-time code sent to you" {
		t.Fatalf("Messenger code field changed: %#v", step.UserInputParams)
	}
}

func TestResetPendingCodeSubmissionFlags(t *testing.T) {
	var onClick BloksTreeScript
	err := onClick.Parse(
		`(bk.action.core.If (bk.action.bloks.GetVariable2 "CAA_AR_AUTHENTICATION_CONFIRMATION:is_verification_pending") true false)`,
	)
	if err != nil {
		t.Fatalf("failed to parse Continue callback: %v", err)
	}
	button := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"on_click": {BloksTreeNodeContent: &onClick},
	})
	pendingID := BloksVariableID("CAA_AR_AUTHENTICATION_CONFIRMATION:is_verification_pending")
	unrelatedID := BloksVariableID("CAA_AR_AUTHENTICATION_CONFIRMATION:is_other_state")
	interp := &Interpreter{
		LocalVars: map[BloksVariableID]*BloksScriptLiteral{
			pendingID:   BloksLiteralOf(true),
			unrelatedID: BloksLiteralOf(true),
		},
		GlobalVars: map[BloksVariableID]*BloksScriptLiteral{},
	}

	if reset := resetPendingCodeSubmissionFlags(button, interp); reset != 1 {
		t.Fatalf("reset %d pending flags instead of 1", reset)
	}
	if interp.LocalVars[pendingID].IsTruthy() {
		t.Fatal("verification pending flag remained true")
	}
	if !interp.LocalVars[unrelatedID].IsTruthy() {
		t.Fatal("unrelated local state was changed")
	}
}

func TestCodeSubmissionWithoutRPCUsesLocalRetryError(t *testing.T) {
	var noOp BloksTreeScript
	if err := noOp.Parse("true "); err != nil {
		t.Fatalf("failed to parse no-op callback: %v", err)
	}
	input := testTreeComponent("bk.components.TextInput", map[BloksAttributeID]*BloksTreeNode{
		"on_text_change": {BloksTreeNodeContent: &noOp},
	})
	continueText := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("Continue"),
	})
	continueButton := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(continueText),
		"on_click": {BloksTreeNodeContent: &noOp},
	})
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(input, continueButton),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
		Interpreter: &Interpreter{
			LocalVars:  map[BloksVariableID]*BloksScriptLiteral{},
			GlobalVars: map[BloksVariableID]*BloksScriptLiteral{},
		},
	}
	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	browser.State = StateCodeEntryPage
	browser.CurrentPage = page

	step, err := browser.DoLoginStep(context.Background(), map[string]string{"otp_code": "000000"})
	if err != nil {
		t.Fatalf("local code submission returned error: %v", err)
	}
	if step != nil {
		t.Fatalf("local code submission unexpectedly returned step %#v", step)
	}
	if browser.LastError != browser.codeNotSentMessage() {
		t.Fatalf("unexpected local retry error %q", browser.LastError)
	}
	if browser.ActionRPCCount != 0 {
		t.Fatalf("local code submission invoked %d action RPCs", browser.ActionRPCCount)
	}
}

func TestTOTPSubmissionResetsPendingAndUsesLocalRetryError(t *testing.T) {
	var noOp BloksTreeScript
	if err := noOp.Parse("true "); err != nil {
		t.Fatalf("failed to parse no-op callback: %v", err)
	}
	var pendingGuard BloksTreeScript
	const pendingVariable = "TWO_STEP_VERIFICATION:is_verification_pending"
	if err := pendingGuard.Parse(
		`(bk.action.core.If (bk.action.bloks.GetVariable2 "` + pendingVariable + `") true false)`,
	); err != nil {
		t.Fatalf("failed to parse pending callback: %v", err)
	}
	input := testTreeComponent("bk.components.TextInput", map[BloksAttributeID]*BloksTreeNode{
		"type":           testTreeLiteral("number"),
		"on_text_change": {BloksTreeNodeContent: &noOp},
	})
	continueText := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("Continue"),
	})
	continueButton := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(continueText),
		"on_click": {BloksTreeNodeContent: &pendingGuard},
	})
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(input, continueButton),
	})
	root.Unminify(&Unminifier{}, nil)
	pendingID := BloksVariableID(pendingVariable)
	interp := &Interpreter{
		LocalVars: map[BloksVariableID]*BloksScriptLiteral{
			pendingID: BloksLiteralOf(true),
		},
		GlobalVars: map[BloksVariableID]*BloksScriptLiteral{},
	}
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
		Interpreter: interp,
	}
	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	browser.State = StateTOTPPage
	browser.CurrentPage = page

	step, err := browser.DoLoginStep(context.Background(), map[string]string{"totp_code": "000000"})
	if err != nil {
		t.Fatalf("local TOTP submission returned error: %v", err)
	}
	if step != nil {
		t.Fatalf("local TOTP submission unexpectedly returned step %#v", step)
	}
	if interp.LocalVars[pendingID].IsTruthy() {
		t.Fatal("TOTP verification pending flag remained true")
	}
	if browser.LastError != browser.codeNotSentMessage() {
		t.Fatalf("unexpected local retry error %q", browser.LastError)
	}
	if browser.ActionRPCCount != 0 {
		t.Fatalf("local TOTP submission invoked %d action RPCs", browser.ActionRPCCount)
	}
}

func TestInstagramNativeDialogStep(t *testing.T) {
	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	callbackCalled := false
	browser.State = StateDialog
	browser.DialogPreviousState = StateAuthenticationConfirm
	browser.PendingDialog = &BloksDialog{
		Title:   "Confirm login",
		Message: "Continue this Instagram login?",
		Buttons: []BloksDialogButton{{
			Label: "Continue",
			Role:  "positive",
			Callback: func(context.Context) error {
				callbackCalled = true
				browser.State = StateSuccess
				return nil
			},
		}, {
			Label: "Cancel",
			Role:  "negative",
		}},
	}

	step, err := browser.DoLoginStep(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("failed to build dialog step: %v", err)
	}
	if step == nil || step.StepID != "fi.mau.meta.instagram.caa.dialog" {
		t.Fatalf("unexpected dialog step: %#v", step)
	}
	if step.Instructions != "Confirm login\n\nContinue this Instagram login?" {
		t.Fatalf("unexpected dialog instructions %q", step.Instructions)
	}
	if step.UserInputParams == nil || len(step.UserInputParams.Fields) != 1 {
		t.Fatalf("unexpected dialog fields: %#v", step.UserInputParams)
	}
	field := step.UserInputParams.Fields[0]
	if field.ID != dialogActionFieldID || field.Type != bridgev2.LoginInputFieldTypeSelect {
		t.Fatalf("unexpected dialog field: %#v", field)
	}
	if !reflect.DeepEqual(field.Options, []string{"Continue", "Cancel"}) {
		t.Fatalf("unexpected dialog options: %#v", field.Options)
	}

	step, err = browser.DoLoginStep(context.Background(), map[string]string{
		dialogActionFieldID: "Continue",
	})
	if err != nil {
		t.Fatalf("failed to execute dialog step: %v", err)
	}
	if step != nil {
		t.Fatalf("dialog selection unexpectedly returned another step: %#v", step)
	}
	if !callbackCalled {
		t.Fatal("dialog callback was not called")
	}
	if browser.State != StateSuccess || browser.PendingDialog != nil {
		t.Fatalf("dialog did not advance cleanly: state=%q pending=%#v", browser.State, browser.PendingDialog)
	}
}

func TestInstagramTwoFactorScreens(t *testing.T) {
	screens := map[string]BrowserState{
		"com.bloks.www.caa.ar.code_entry":                                    StateAccountRecoveryPage,
		"com.bloks.www.caa.ar.auth_method":                                   StateChooseMFAPage,
		"com.bloks.www.ap.two_step_verification.code_entry":                  StateCodeEntryPage,
		"com.bloks.www.ap.two_step_verification.challenge_picker":            StateChooseMFAPage,
		"com.bloks.www.two_step_verification.method_picker":                  StateChooseMFAPage,
		"com.bloks.www.two_factor_login.enter_totp_code":                     StateTOTPPage,
		"com.bloks.www.two_step_verification.enter_totp_code":                StateTOTPPage,
		"com.bloks.www.ap.two_step_verification.enter_totp_code":             StateTOTPPage,
		"com.bloks.www.two_step_verification.enter_sms_code":                 StateSMSPage,
		"com.bloks.www.two_factor_login.enter_backup_code":                   StateBackupCodePage,
		"com.bloks.www.two_step_verification.enter_backup_code":              StateBackupCodePage,
		"com.bloks.www.ap.two_step_verification.enter_backup_code":           StateBackupCodePage,
		"com.bloks.www.ap.two_step_verification.contactpoint_chooser":        StateChooseContactPointPage,
		"com.bloks.www.two_step_verification.contactpoint_chooser":           StateChooseContactPointPage,
		"com.bloks.www.two_step_verification.enter_whatsapp_code":            StateWhatsAppPage,
		"com.bloks.www.ap.two_step_verification.approve_from_another_device": StateMFALandingPage,
	}
	for screen, expectedState := range screens {
		t.Run(screen, func(t *testing.T) {
			browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
			if err != nil {
				t.Fatalf("failed to create Bloks browser: %v", err)
			}
			err = browser.Bridge.DisplayNewScreen(context.Background(), screen, &BloksBundle{})
			if err != nil {
				t.Fatalf("two-factor screen was rejected: %v", err)
			}
			if browser.State != expectedState {
				t.Fatalf("unexpected state %q, expected %q", browser.State, expectedState)
			}
		})
	}
}

func TestInstagramMFAMethodDiscovery(t *testing.T) {
	totpButton := testTreeButton("Authentication app")
	smsButton := testTreeButton("Send code via SMS")
	passkeyButton := testTreeButton("Use a passkey")
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(totpButton, smsButton, passkeyButton),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
	}

	found, methodNames, unsupported := findMFAMethodOptions(page)
	if len(found) != 2 {
		t.Fatalf("unexpected supported MFA method count %d", len(found))
	}
	if !reflect.DeepEqual(methodNames, []string{"Authentication app", "Send code via SMS"}) {
		t.Fatalf("unexpected MFA methods: %#v", methodNames)
	}
	if unsupported != 1 {
		t.Fatalf("unexpected unsupported MFA method count %d", unsupported)
	}
}

func TestInstagramTwoStepEntrypointAuthenticatorForm(t *testing.T) {
	methodDescription := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("Authentication app"),
	})
	input := testTreeComponent("bk.components.TextInput", map[BloksAttributeID]*BloksTreeNode{
		"type": testTreeLiteral("number"),
	})
	continueButton := testTreeButton("Continue")
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(methodDescription, input, continueButton),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
	}

	if found, _, _ := findMFAMethodOptions(page); len(found) != 0 {
		t.Fatalf("descriptive code-form text was misclassified as an MFA method: %#v", found)
	}
	if state := twoStepVerificationEntrypointState(page); state != StateTOTPPage {
		t.Fatalf("authenticator entrypoint mapped to %q", state)
	}

	browser, err := NewBrowser(&BrowserConfig{Platform: types.Instagram})
	if err != nil {
		t.Fatalf("failed to create Bloks browser: %v", err)
	}
	err = browser.Bridge.DisplayNewScreen(
		context.Background(),
		"com.bloks.www.two_step_verification.entrypoint",
		page,
	)
	if err != nil {
		t.Fatalf("authenticator entrypoint was rejected: %v", err)
	}
	if browser.State != StateTOTPPage {
		t.Fatalf("unexpected authenticator entrypoint state %q", browser.State)
	}

	step, err := browser.DoLoginStep(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("failed to create authenticator-code step: %v", err)
	}
	if step == nil || step.StepID != "fi.mau.meta.instagram.caa.totp" {
		t.Fatalf("unexpected authenticator-code step: %#v", step)
	}
	if step.UserInputParams == nil || len(step.UserInputParams.Fields) != 1 {
		t.Fatalf("unexpected authenticator-code fields: %#v", step.UserInputParams)
	}
	field := step.UserInputParams.Fields[0]
	if field.ID != "totp_code" || field.Type != bridgev2.LoginInputFieldType2FACode {
		t.Fatalf("unexpected authenticator-code field: %#v", field)
	}
}

func TestInstagramTwoStepEntrypointMethodPicker(t *testing.T) {
	methodButton := testTreeButton("Authentication app")
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(methodButton),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
	}

	if state := twoStepVerificationEntrypointState(page); state != StateChooseMFAPage {
		t.Fatalf("method-picker entrypoint mapped to %q", state)
	}
}

func TestFindAuthenticationConfirmationButton(t *testing.T) {
	negativeText := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("Try another way"),
	})
	positiveText := testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
		"text": testTreeLiteral("Continue"),
	})
	negativeButton := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(negativeText),
		"on_click": testTreeScript(),
	})
	positiveButton := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(positiveText),
		"on_click": testTreeScript(),
	})
	root := testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(negativeButton, positiveButton),
	})
	root.Unminify(&Unminifier{}, nil)
	page := &BloksBundle{
		Layout: BloksLayout{Payload: BloksPayload{
			Tree: &BloksTreeNode{BloksTreeNodeContent: root},
		}},
	}

	found, clickableTextCount := findAuthenticationConfirmationButton(page)
	if found != positiveButton {
		t.Fatal("did not select the positive authentication confirmation button")
	}
	if clickableTextCount != 2 {
		t.Fatalf("unexpected clickable text count %d", clickableTextCount)
	}
}

func testTreeComponent(
	id BloksComponentID,
	attributes map[BloksAttributeID]*BloksTreeNode,
) *BloksTreeComponent {
	return &BloksTreeComponent{ComponentID: id, Attributes: attributes}
}

func testTreeLiteral(value string) *BloksTreeNode {
	return &BloksTreeNode{BloksTreeNodeContent: &BloksTreeLiteral{BloksJavaScriptValue: value}}
}

func testTreeChildren(children ...*BloksTreeComponent) *BloksTreeNode {
	list := BloksTreeComponentList(children)
	return &BloksTreeNode{BloksTreeNodeContent: &list}
}

func testTreeScript() *BloksTreeNode {
	return &BloksTreeNode{BloksTreeNodeContent: &BloksTreeLiteral{BloksJavaScriptValue: true}}
}

func testTreeButton(labels ...string) *BloksTreeComponent {
	children := make([]*BloksTreeComponent, 0, len(labels))
	for _, label := range labels {
		children = append(children, testTreeComponent("bk.data.TextSpan", map[BloksAttributeID]*BloksTreeNode{
			"text": testTreeLiteral(label),
		}))
	}
	return testTreeComponent("bk.components.Flexbox", map[BloksAttributeID]*BloksTreeNode{
		"children": testTreeChildren(children...),
		"on_click": testTreeScript(),
	})
}
