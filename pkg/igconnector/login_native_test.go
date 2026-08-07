package igconnector

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func newNativeLoginTestProcess(t *testing.T) *MetaNativeLogin {
	t.Helper()
	return &MetaNativeLogin{
		User: &bridgev2.User{Log: zerolog.New(io.Discard)},
		Main: &IGConnector{},
		newClient: func(
			context.Context,
			zerolog.Logger,
			*IGConnector,
			*cookies.Cookies,
			bool,
		) (*instameow.Client, error) {
			loginCookies := &cookies.Cookies{Platform: types.Instagram}
			loginCookies.UpdateValues(map[cookies.MetaCookieName]string{
				cookies.IGCookieCSRFToken: "test-csrf",
				cookies.IGCookieMachineID: "test-mid",
				cookies.IGCookieDeviceID:  "test-ig-did",
			})
			return instameow.NewClient(instameow.ClientParams{
				Cookies: loginCookies,
				Log:     zerolog.New(io.Discard),
			}), nil
		},
	}
}

func TestInstagramLoginFlowsExposeNativeFirstAndKeepCookies(t *testing.T) {
	connector := &IGConnector{}
	flows := connector.GetLoginFlows()
	if len(flows) != 2 {
		t.Fatalf("expected two login flows, got %d", len(flows))
	}
	if flows[0].ID != FlowIDInstagramPassword {
		t.Fatalf("expected native flow first, got %q", flows[0].ID)
	}
	if flows[1].ID != FlowIDInstagramCookies {
		t.Fatalf("expected cookie fallback second, got %q", flows[1].ID)
	}
	process, err := connector.CreateLogin(
		context.Background(),
		&bridgev2.User{},
		FlowIDInstagramPassword,
	)
	if err != nil {
		t.Fatalf("failed to create native login: %v", err)
	}
	if _, ok := process.(*MetaNativeLogin); !ok {
		t.Fatalf("expected MetaNativeLogin, got %T", process)
	}
}

func TestInstagramNativeLoginStartReturnsCredentialFields(t *testing.T) {
	process := newNativeLoginTestProcess(t)
	step, err := process.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	assertInstagramCredentialsStep(t, step)
	if process.client == nil {
		t.Fatal("Start did not create a login-scoped client")
	}
}

func TestInstagramNativeLoginRequiresBothCredentialFields(t *testing.T) {
	process := newNativeLoginTestProcess(t)
	if _, err := process.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	called := false
	process.submitCAA = func(
		context.Context,
		*instameow.Client,
		map[string]string,
	) (*bridgev2.LoginStep, error) {
		called = true
		return nil, nil
	}
	step, err := process.SubmitUserInput(context.Background(), map[string]string{
		loginFieldIdentifier: "user@example.com",
	})
	if err != nil {
		t.Fatalf("SubmitUserInput returned error: %v", err)
	}
	assertInstagramCredentialsStep(t, step)
	if called || process.caaStarted {
		t.Fatal("incomplete credentials started the CAA flow")
	}
}

func TestInstagramNativeLoginReturnsCAAStepsUnchanged(t *testing.T) {
	process := newNativeLoginTestProcess(t)
	if _, err := process.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	expectedClient := process.client
	process.submitCAA = func(
		_ context.Context,
		client *instameow.Client,
		input map[string]string,
	) (*bridgev2.LoginStep, error) {
		if client != expectedClient {
			t.Fatal("CAA login received a different login-scoped client")
		}
		if input[loginFieldIdentifier] != "user@example.com" ||
			input[loginFieldPassword] != "secret" {
			t.Fatal("CAA login did not receive the native credential fields")
		}
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeUserInput,
			StepID:       "fi.mau.meta.instagram.caa.totp",
			Instructions: "Instagram needs another code.",
		}, nil
	}
	step, err := process.SubmitUserInput(context.Background(), map[string]string{
		loginFieldIdentifier: "user@example.com",
		loginFieldPassword:   "secret",
	})
	if err != nil {
		t.Fatalf("SubmitUserInput returned error: %v", err)
	}
	if !process.caaStarted {
		t.Fatal("native login did not start the Instagram CAA path")
	}
	if step.StepID != "fi.mau.meta.instagram.caa.totp" {
		t.Fatalf("CAA step ID changed: %q", step.StepID)
	}
	if step.Instructions != "Instagram needs another code." {
		t.Fatalf("CAA instructions changed: %q", step.Instructions)
	}
}

func TestInstagramNativeLoginKeepsCAAStateAcrossInputs(t *testing.T) {
	process := newNativeLoginTestProcess(t)
	if _, err := process.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	expectedClient := process.client
	calls := 0
	process.submitCAA = func(
		_ context.Context,
		client *instameow.Client,
		input map[string]string,
	) (*bridgev2.LoginStep, error) {
		calls++
		if client != expectedClient {
			t.Fatal("CAA follow-up received a different login-scoped client")
		}
		switch calls {
		case 1:
			return &bridgev2.LoginStep{
				Type:   bridgev2.LoginStepTypeUserInput,
				StepID: "fi.mau.meta.instagram.caa.account_selection",
			}, nil
		case 2:
			if input["account"] != "selected-profile" {
				t.Fatalf("CAA follow-up did not receive the selected profile: %+v", input)
			}
			return nil, nil
		default:
			t.Fatalf("unexpected CAA submission %d", calls)
			return nil, nil
		}
	}
	process.complete = func(
		_ context.Context,
		_ zerolog.Logger,
		client *instameow.Client,
		_ *bridgev2.User,
		_ *IGConnector,
		loginCookies *cookies.Cookies,
	) (*bridgev2.LoginStep, error) {
		if client != expectedClient || loginCookies != expectedClient.GetCookies() {
			t.Fatal("CAA completion did not retain the login-scoped client")
		}
		return &bridgev2.LoginStep{
			Type:   bridgev2.LoginStepTypeComplete,
			StepID: LoginStepIDComplete,
		}, nil
	}

	firstStep, err := process.SubmitUserInput(context.Background(), map[string]string{
		loginFieldIdentifier: "user@example.com",
		loginFieldPassword:   "secret",
	})
	if err != nil {
		t.Fatalf("initial CAA submission returned error: %v", err)
	}
	if firstStep == nil ||
		firstStep.StepID != "fi.mau.meta.instagram.caa.account_selection" {
		t.Fatalf("unexpected initial CAA step: %+v", firstStep)
	}
	finalStep, err := process.SubmitUserInput(context.Background(), map[string]string{
		"account": "selected-profile",
	})
	if err != nil {
		t.Fatalf("CAA follow-up returned error: %v", err)
	}
	if finalStep == nil ||
		finalStep.Type != bridgev2.LoginStepTypeComplete ||
		calls != 2 {
		t.Fatalf("unexpected CAA completion: step=%+v calls=%d", finalStep, calls)
	}
}

func TestInstagramNativeLoginWrapsCAAErrors(t *testing.T) {
	process := newNativeLoginTestProcess(t)
	if _, err := process.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	expectedErr := errors.New("synthetic CAA failure")
	process.submitCAA = func(
		context.Context,
		*instameow.Client,
		map[string]string,
	) (*bridgev2.LoginStep, error) {
		return nil, expectedErr
	}

	_, err := process.SubmitUserInput(context.Background(), map[string]string{
		loginFieldIdentifier: "user@example.com",
		loginFieldPassword:   "secret",
	})
	if !errors.Is(err, expectedErr) ||
		!strings.Contains(err.Error(), "through CAA") {
		t.Fatalf("expected wrapped CAA failure, got %v", err)
	}
}

func TestInstagramNativeLoginDoesNotFallBackToCookieCompletion(t *testing.T) {
	process := newNativeLoginTestProcess(t)
	if _, err := process.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	process.submitCAA = func(
		context.Context,
		*instameow.Client,
		map[string]string,
	) (*bridgev2.LoginStep, error) {
		return nil, nil
	}

	_, err := process.SubmitUserInput(context.Background(), map[string]string{
		loginFieldIdentifier: "user@example.com",
		loginFieldPassword:   "secret",
	})
	if err == nil ||
		!strings.Contains(err.Error(), "did not return a mobile session") {
		t.Fatalf("expected native mobile-session completion error, got %v", err)
	}
}

func assertInstagramCredentialsStep(t *testing.T, step *bridgev2.LoginStep) {
	t.Helper()
	if step == nil ||
		step.Type != bridgev2.LoginStepTypeUserInput ||
		step.StepID != LoginStepIDCredentials ||
		step.UserInputParams == nil ||
		len(step.UserInputParams.Fields) != 2 {
		t.Fatalf("unexpected Instagram credential step: %+v", step)
	}
	if step.UserInputParams.Fields[0].ID != loginFieldIdentifier ||
		step.UserInputParams.Fields[1].ID != loginFieldPassword {
		t.Fatalf("unexpected Instagram credential fields: %+v", step.UserInputParams.Fields)
	}
}
