package igconnector

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/util/exslices"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	FlowIDInstagramCookies = "instagram"

	LoginStepIDCookies  = "fi.mau.meta.cookies"
	LoginStepIDComplete = "fi.mau.meta.complete"
)

func (ic *IGConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	switch flowID {
	case FlowIDInstagramCookies:
	default:
		return nil, bridgev2.ErrInvalidLoginFlowID
	}

	return &MetaCookieLogin{
		User: user,
		Main: ic,
	}, nil
}

var (
	loginFlowInstagram = bridgev2.LoginFlow{
		Name:        "instagram.com",
		Description: "Login using cookies from instagram.com",
		ID:          FlowIDInstagramCookies,
	}
)

func (ic *IGConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{loginFlowInstagram}
}

type MetaCookieLogin struct {
	User *bridgev2.User
	Main *IGConnector
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
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeCookies,
		StepID:       LoginStepIDCookies,
		Instructions: "Enter a JSON object with your cookies, or a cURL command copied from browser devtools.",
		CookiesParams: &bridgev2.LoginCookiesParams{
			//UserAgent:         useragent.UserAgent,
			URL:               "https://www.instagram.com/accounts/login/",
			Fields:            cookieListToFields(cookies.IGRequiredCookies, "instagram.com"),
			WaitForURLPattern: "^https://www\\.instagram\\.com/(?:direct/(?:inbox/|t/[0-9]+/)?)?(?:\\?.*)?$",
		},
	}, nil
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

func getInstaClient(log zerolog.Logger, conn *IGConnector, c *cookies.Cookies, useProxy bool) (*instameow.Client, error) {
	client := instameow.NewClient(instameow.ClientParams{
		Cookies:  c,
		Log:      log,
		Settings: conn.Bridge.GetHTTPClientSettings(),
	})
	if useProxy && (conn.Config.GetProxyFrom != "" || conn.Config.Proxy != "") {
		client.GetHTTP().GetNewProxy = conn.getProxy
		if !client.GetHTTP().UpdateProxy("login") {
			return nil, fmt.Errorf("failed to update proxy")
		}
	}
	return client, nil
}

func loginWithCookies(
	ctx context.Context,
	log zerolog.Logger,
	client *instameow.Client,
	bridgeUser *bridgev2.User,
	conn *IGConnector,
	c *cookies.Cookies,
) (*bridgev2.LoginStep, error) {
	log.Debug().
		Strs("cookie_names", exslices.CastToString[string](slices.Collect(maps.Keys(c.GetAll())))).
		Msg("Logging in with cookies")
	user, mailbox, err := client.LoadIndex(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to load messages page for login")
		if errors.Is(err, httpclient.ErrChallengeRequired) {
			return nil, ErrLoginChallenge
		} else if errors.Is(err, httpclient.ErrCheckpointRequired) {
			return nil, ErrLoginCheckpoint
		} else if errors.Is(err, httpclient.ErrConsentRequired) {
			return nil, ErrLoginConsent
		} else if errors.Is(err, httpclient.ErrTokenInvalidated) {
			return nil, ErrLoginTokenInvalidated
		} else {
			return nil, fmt.Errorf("%w: %w", ErrLoginUnknown, err)
		}
	}

	ownFBID := client.GetOwnFBID()
	if ownFBID == 0 {
		return nil, fmt.Errorf("own fbid not found")
	}
	loginID := metaid.MakeUserLoginID(ownFBID)
	var loginUA string
	if req, ok := ctx.Value("fi.mau.provision.request").(*http.Request); ok {
		loginUA = req.Header.Get("User-Agent")
	}

	ul, err := bridgeUser.NewLogin(ctx, &database.UserLogin{
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

	igClient := ul.Client.(*IGClient)
	// Override the client because LoadMessagesPage saves some state and we don't want to call it again
	client.SetLogger(ul.Log.With().Str("component", "instameow").Logger())
	client.SetEventHandler(igClient.handleIGEvent)
	igClient.Client = client

	backgroundCtx := ul.Log.WithContext(conn.Bridge.BackgroundCtx)
	ul.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	go igClient.connectWithMailbox(backgroundCtx, backgroundCtx, user, mailbox)
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepIDComplete,
		Instructions: fmt.Sprintf("Logged in as %s (%s)", user.GetName(), ul.ID),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
}

func (m *MetaCookieLogin) SubmitCookies(ctx context.Context, strCookies map[string]string) (*bridgev2.LoginStep, error) {
	c := &cookies.Cookies{Platform: types.Instagram}
	strCookiesCopy := map[cookies.MetaCookieName]string{}
	for key, val := range strCookies {
		strCookiesCopy[cookies.MetaCookieName(key)] = val
	}
	c.UpdateValues(strCookiesCopy)

	missingCookies := c.GetMissingCookieNames()
	if len(missingCookies) > 0 {
		return nil, ErrLoginMissingCookies.AppendMessage(": %v", missingCookies)
	}

	log := m.User.Log.With().Str("component", "instameow").Logger()
	client, err := getInstaClient(log, m.Main, c, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	return loginWithCookies(ctx, log, client, m.User, m.Main, c)
}
