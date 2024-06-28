package connector

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix/bridgev2"
)

func (m *MetaConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	if flowID != "cookies" {
		return nil, fmt.Errorf("unknown flow ID %s", flowID)
	}

	return &CookieLogin{}, nil
}

func (m *MetaConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{{
		Name:        "Cookies",
		Description: "Login using cookies from a web browser",
		ID:          "cookies",
	}}
}

type CookieLogin struct{}

var _ bridgev2.LoginProcessCookies = (*CookieLogin)(nil)

func (p *CookieLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return &bridgev2.LoginStep{
		Type:          bridgev2.LoginStepTypeCookies,
		StepID:        "fi.mau.meta.cookies",
		Instructions:  "Please enter cookies from your browser",
		CookiesParams: &bridgev2.LoginCookiesParams{},
	}, nil
}

func (c *CookieLogin) Cancel() {}

func (p *CookieLogin) SubmitCookies(ctx context.Context, cookies map[string]string) (*bridgev2.LoginStep, error) {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       "fi.mau.meta.complete",
		Instructions: "Login successful",
	}, nil
}
