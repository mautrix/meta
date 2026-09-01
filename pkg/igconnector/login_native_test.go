package igconnector

import (
	"context"
	"net/http"
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
)

type nativeLoginRoundTripper struct{}

func (*nativeLoginRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

func TestInstagramLoginFlowsExposeCookiesFirstAndKeepNative(t *testing.T) {
	connector := &IGConnector{}
	flows := connector.GetLoginFlows()
	if len(flows) != 2 {
		t.Fatalf("expected two login flows, got %d", len(flows))
	}
	if flows[0].ID != FlowIDInstagramCookies {
		t.Fatalf("expected cookie flow first, got %q", flows[0].ID)
	}
	if flows[1].ID != FlowIDInstagramPassword {
		t.Fatalf("expected native flow second, got %q", flows[1].ID)
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

func TestInstagramCookieLoginIncludesOptionalRoutingCookies(t *testing.T) {
	step, err := (&MetaCookieLogin{}).Start(context.Background())
	if err != nil {
		t.Fatalf("failed to start Instagram cookie login: %v", err)
	}
	if step.CookiesParams == nil {
		t.Fatal("cookie login did not provide cookie parameters")
	}
	fields := make(map[string]bool, len(step.CookiesParams.Fields))
	for _, field := range step.CookiesParams.Fields {
		fields[field.ID] = field.Required
	}
	for _, cookie := range cookies.IGRequiredCookies {
		if required, ok := fields[string(cookie)]; !ok || !required {
			t.Fatalf("required cookie %q is missing or optional", cookie)
		}
	}
	for _, cookie := range cookies.IGOptionalCookies {
		if required, ok := fields[string(cookie)]; !ok || required {
			t.Fatalf("routing cookie %q is missing or required", cookie)
		}
	}
}

func TestInstagramNativeCredentialsStep(t *testing.T) {
	assertInstagramCredentialsStep(t, instagramCredentialsStep("Enter your credentials"))
}

func TestInstagramNativeLoginUsesClientHTTPTransport(t *testing.T) {
	transport := &nativeLoginRoundTripper{}
	login := &MetaNativeLogin{
		User: &bridgev2.User{Log: zerolog.Nop()},
		Main: &IGConnector{Bridge: &bridgev2.Bridge{}},
	}
	step, err := login.StartWithParams(context.Background(), bridgev2.LoginStartParams{
		HTTP: transport,
	})
	if err != nil {
		t.Fatalf("failed to start native login: %v", err)
	}
	assertInstagramCredentialsStep(t, step)
	if login.client.GetHTTP().HTTP.Transport != transport {
		t.Fatal("native login did not install the client HTTP transport")
	}
	login.Cancel()
	if login.transport != nil {
		t.Fatal("cancel did not clear the client HTTP transport")
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
