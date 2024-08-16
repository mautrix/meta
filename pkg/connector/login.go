package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	FlowIDFacebookCookies  = "cookies-facebook"
	FlowIDMessengerCookies = "cookies-messenger"
	FlowIDInstagramCookies = "cookies-instagram"

	LoginStepIDCookies  = "fi.mau.meta.cookies"
	LoginStepIDComplete = "fi.mau.meta.complete"
)

func (m *MetaConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	var plat types.Platform
	switch flowID {
	case FlowIDFacebookCookies:
		plat = types.Facebook
		if m.Config.Mode == types.FacebookTor {
			plat = types.FacebookTor
		}
	case FlowIDMessengerCookies:
		plat = types.Messenger
	case FlowIDInstagramCookies:
		plat = types.Instagram
	default:
		return nil, fmt.Errorf("unknown flow ID %s", flowID)
	}

	return &MetaCookieLogin{
		Mode: plat,
		User: user,
		Main: m,
	}, nil
}

var (
	loginFlowFacebook = bridgev2.LoginFlow{
		Name:        "facebook.com",
		Description: "Login using cookies from facebook.com",
		ID:          FlowIDFacebookCookies,
	}
	loginFlowMessenger = bridgev2.LoginFlow{
		Name:        "messenger.com",
		Description: "Login using cookies from messenger.com",
		ID:          FlowIDMessengerCookies,
	}
	loginFlowInstagram = bridgev2.LoginFlow{
		Name:        "instagram.com",
		Description: "Login using cookies from instagram.com",
		ID:          FlowIDInstagramCookies,
	}
)

func (m *MetaConnector) GetLoginFlows() []bridgev2.LoginFlow {
	switch m.Config.Mode {
	case types.Unset:
		return []bridgev2.LoginFlow{loginFlowFacebook, loginFlowMessenger, loginFlowInstagram}
	case types.Facebook, types.FacebookTor:
		return []bridgev2.LoginFlow{loginFlowFacebook}
	case types.Messenger:
		return []bridgev2.LoginFlow{loginFlowMessenger}
	case types.Instagram:
		return []bridgev2.LoginFlow{loginFlowInstagram}
	default:
		panic("unknown mode in config")
	}
}

type MetaCookieLogin struct {
	Mode types.Platform
	User *bridgev2.User
	Main *MetaConnector
}

var _ bridgev2.LoginProcessCookies = (*MetaCookieLogin)(nil)

func cookieListToFields(cookies []cookies.MetaCookieName, domain string) []bridgev2.LoginCookieField {
	fields := make([]bridgev2.LoginCookieField, len(cookies))
	for i, cookie := range cookies {
		fields[i] = bridgev2.LoginCookieField{
			ID: string(cookie),
			Sources: []bridgev2.LoginCookieFieldSource{
				{
					Type:         bridgev2.LoginCookieTypeCookie,
					Name:         string(cookie),
					CookieDomain: domain,
				},
			},
		}
	}
	return fields
}

func (m *MetaCookieLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	step := &bridgev2.LoginStep{
		Type:          bridgev2.LoginStepTypeCookies,
		StepID:        LoginStepIDCookies,
		Instructions:  "Enetr a JSON object with your cookies, or a cURL command copied from browser devtools.",
		CookiesParams: &bridgev2.LoginCookiesParams{},
	}
	switch m.Mode {
	case types.Facebook, types.FacebookTor:
		step.CookiesParams.URL = "https://www.facebook.com/"
		step.CookiesParams.Fields = cookieListToFields(cookies.FBRequiredCookies, "facebook.com")
	case types.Messenger:
		step.CookiesParams.URL = "https://www.messenger.com/"
		step.CookiesParams.Fields = cookieListToFields(cookies.FBRequiredCookies, "messenger.com")
	case types.Instagram:
		step.CookiesParams.URL = "https://www.instagram.com/"
		step.CookiesParams.Fields = cookieListToFields(cookies.FBRequiredCookies, "instagram.com")
	default:
		return nil, fmt.Errorf("unknown mode %s", m.Mode)
	}
	return step, nil
}

func (m *MetaCookieLogin) Cancel() {}

var (
	ErrLoginMissingCookies   = bridgev2.RespError{ErrCode: "FI.MAU.META_MISSING_COOKIES", Err: "Missing cookies", StatusCode: http.StatusBadRequest}
	ErrLoginChallenge        = bridgev2.RespError{ErrCode: "FI.MAU.META_CHALLENGE_ERROR", Err: "Challenge required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	ErrLoginConsent          = bridgev2.RespError{ErrCode: "FI.MAU.META_CONSENT_ERROR", Err: "Consent required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	ErrLoginTokenInvalidated = bridgev2.RespError{ErrCode: "FI.MAU.META_TOKEN_ERROR", Err: "Got logged out immediately", StatusCode: http.StatusBadRequest}
	ErrLoginUnknown          = bridgev2.RespError{ErrCode: "M_UNKNOWN", Err: "Internal error logging in", StatusCode: http.StatusInternalServerError}
)

func (m *MetaCookieLogin) SubmitCookies(ctx context.Context, strCookies map[string]string) (*bridgev2.LoginStep, error) {
	c := &cookies.Cookies{Platform: m.Mode}
	c.UpdateValues(strCookies)

	missingCookies := c.GetMissingCookieNames()
	if len(missingCookies) > 0 {
		return nil, ErrLoginMissingCookies.AppendMessage(": %v", missingCookies)
	}

	err := c.GeneratePushKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to generate push keys: %w", err)
	}

	log := m.User.Log.With().Str("component", "messagix").Logger()
	client := messagix.NewClient(c, log)

	user, tbl, err := client.LoadMessagesPage()
	if err != nil {
		log.Err(err).Msg("Failed to load messages page for login")
		if errors.Is(err, messagix.ErrChallengeRequired) {
			return nil, ErrLoginChallenge
		} else if errors.Is(err, messagix.ErrConsentRequired) {
			return nil, ErrLoginConsent
		} else if errors.Is(err, messagix.ErrTokenInvalidated) {
			return nil, ErrLoginTokenInvalidated
		} else {
			return nil, fmt.Errorf("%w: %w", ErrLoginUnknown, err)
		}
	}

	id := user.GetFBID()
	if client.Instagram != nil {
		id, err = client.Instagram.ExtractFBID(user, tbl)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch FBID: %w", err)
		}
	}

	loginID := networkid.UserLoginID(fmt.Sprint(id))

	ul, err := m.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: user.GetName(),
		RemoteProfile: status.RemoteProfile{
			Name: user.GetName(),
		},
		Metadata: &metaid.UserLoginMetadata{
			Platform: c.Platform,
			Cookies:  c,
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to save new login: %w", err)
	}

	metaClient := ul.Client.(*MetaClient)
	// Override the client because LoadMessagesPage saves some state and we don't want to call it again
	client.Logger = metaClient.Client.Logger
	client.SetEventHandler(metaClient.handleMetaEvent)
	metaClient.Client = client

	backgroundCtx := ul.Log.WithContext(context.Background())
	err = metaClient.connectWithTable(backgroundCtx, tbl, user)
	if err != nil {
		return nil, fmt.Errorf("failed to connect after login: %w", err)
	}

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepIDComplete,
		Instructions: fmt.Sprintf("Logged in as %s (%d)", user.GetName(), id),
	}, nil
}
