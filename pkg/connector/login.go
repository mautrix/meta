package connector

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	FlowIDFacebookCookies  = "facebook"
	FlowIDMessengerCookies = "messenger"
	FlowIDInstagramCookies = "instagram"

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
		return nil, bridgev2.ErrInvalidLoginFlowID
	}

	return &MetaCookieLogin{
		Mode: plat,
		User: user,
		Main: m,
	}, nil
}

// This creates a user login using credentials transferred from another instance of the meta bridge,
// via the `ExportCredentials` API.
func (m *MetaConnector) CreateUserLoginFromCredentials(ctx context.Context, user *bridgev2.User, credentials any) error {
	creds, ok := credentials.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid credentials type: %T", credentials)
	}
	cleanCreds := make(map[string]string, len(creds))
	for k, v := range creds {
		cleanCreds[k] = v.(string)
	}

	login, err := m.CreateLogin(ctx, user, FlowIDFacebookCookies)
	if err != nil {
		return err
	}

	step, err := login.(bridgev2.LoginProcessCookies).SubmitCookies(ctx, cleanCreds)
	if err != nil {
		return err
	} else if step.Type != bridgev2.LoginStepTypeComplete {
		return fmt.Errorf("expected complete step from credential login, got: %s", step.Type)
	}

	return nil
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
	case types.Facebook:
		if m.Config.AllowMessengerComOnFB {
			return []bridgev2.LoginFlow{loginFlowMessenger, loginFlowFacebook}
		}
		fallthrough
	case types.FacebookTor:
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
			ID:       string(cookie),
			Required: true,
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
		Type:         bridgev2.LoginStepTypeCookies,
		StepID:       LoginStepIDCookies,
		Instructions: "Enter a JSON object with your cookies, or a cURL command copied from browser devtools.",
		CookiesParams: &bridgev2.LoginCookiesParams{
			UserAgent: messagix.UserAgent,
		},
	}
	switch m.Mode {
	case types.Facebook, types.FacebookTor:
		step.CookiesParams.URL = "https://www.facebook.com/"
		step.CookiesParams.Fields = cookieListToFields(cookies.FBRequiredCookies, "facebook.com")
		step.CookiesParams.WaitForURLPattern = "^https://www\\.facebook\\.com/(?:messages/(?:e2ee/)?(?:t/[0-9]+/?)?)?(?:\\?.*)?$"
	case types.Messenger:
		step.CookiesParams.URL = "https://www.messenger.com/?no_redirect=true"
		step.CookiesParams.Fields = cookieListToFields(cookies.FBRequiredCookies, "messenger.com")
		step.CookiesParams.WaitForURLPattern = "^https://www\\.messenger\\.com/(?:e2ee/)?(?:t/[0-9]+/?)?(?:\\?.*)?$"
	case types.Instagram:
		step.CookiesParams.URL = "https://www.instagram.com/"
		step.CookiesParams.Fields = cookieListToFields(cookies.IGRequiredCookies, "instagram.com")
		step.CookiesParams.WaitForURLPattern = "^https://www\\.instagram\\.com/(?:direct/(?:inbox/|t/[0-9]+/)?)?(?:\\?.*)?$"
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
	ErrLoginCheckpoint       = bridgev2.RespError{ErrCode: "FI.MAU.META_CHECKPOINT_ERROR", Err: "Checkpoint required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
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

	log := m.User.Log.With().Str("component", "messagix").Logger()
	client := messagix.NewClient(c, log)
	if m.Main.Config.GetProxyFrom != "" || m.Main.Config.Proxy != "" {
		client.GetNewProxy = m.Main.getProxy
		if !client.UpdateProxy("login") {
			return nil, fmt.Errorf("failed to update proxy")
		}
	}

	log.Debug().
		Strs("cookie_names", slices.Collect(maps.Keys(strCookies))).
		Msg("Logging in with cookies")
	user, tbl, err := client.LoadMessagesPage(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to load messages page for login")
		if errors.Is(err, messagix.ErrChallengeRequired) {
			return nil, ErrLoginChallenge
		} else if errors.Is(err, messagix.ErrCheckpointRequired) {
			return nil, ErrLoginCheckpoint
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

	loginID := metaid.MakeUserLoginID(id)
	var loginUA string
	if req, ok := ctx.Value("fi.mau.provision.request").(*http.Request); ok {
		loginUA = req.Header.Get("User-Agent")
	}

	ul, err := m.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: user.GetName(),
		RemoteProfile: status.RemoteProfile{
			Name: user.GetName(),
		},
		Metadata: &metaid.UserLoginMetadata{
			Platform: c.Platform,
			Cookies:  c,
			LoginUA:  loginUA,
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to save new login: %w", err)
	}

	metaClient := ul.Client.(*MetaClient)
	// Override the client because LoadMessagesPage saves some state and we don't want to call it again
	client.Logger = metaClient.Client.Logger
	client.SetEventHandler(metaClient.handleMetaEvent)
	metaClient.lastFullReconnect = time.Time{}
	metaClient.Client = client

	backgroundCtx := ul.Log.WithContext(m.Main.Bridge.BackgroundCtx)
	go metaClient.connectWithTable(backgroundCtx, tbl, user)
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepIDComplete,
		Instructions: fmt.Sprintf("Logged in as %s (%d)", user.GetName(), id),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
}
