package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/util/l10n"
	"go.mau.fi/util/shape"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	FlowIDFacebookCookies      = "facebook"
	FlowIDMessengerCookies     = "messenger"
	FlowIDMessengerLiteIOS     = "messenger-lite"
	FlowIDMessengerLiteAndroid = "messenger-lite-android"

	LoginStepIDCookies      = "fi.mau.meta.cookies"
	LoginStepIDPhoneCookies = "fi.mau.meta.phone_cookies"
	LoginStepIDComplete     = "fi.mau.meta.complete"

	LoginStepIDCredentials = "fi.mau.meta.credentials"
)

func (m *MetaConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	var plat types.Platform
	switch flowID {
	case FlowIDFacebookCookies:
		plat = types.Facebook
		if m.Config.Tor {
			plat = types.FacebookTor
		}
	case FlowIDMessengerCookies:
		plat = types.Messenger
	case FlowIDMessengerLiteIOS:
		plat = types.MessengerLiteIOS
		return &MetaNativeLogin{
			Mode: plat,
			User: user,
			Main: m,
		}, nil
	case FlowIDMessengerLiteAndroid:
		plat = types.MessengerLiteAndroid
		return &MetaNativeLogin{
			Mode: plat,
			User: user,
			Main: m,
		}, nil
	default:
		return nil, bridgev2.ErrInvalidLoginFlowID
	}

	return &MetaCookieLogin{
		Mode: plat,
		User: user,
		Main: m,
	}, nil
}

func validateNativeSession(session *types.NativeSession) error {
	if session != nil && (session.AccessToken == "" || (session.AppID != useragent.MessengerLiteAndroidAppID && session.AppID != useragent.MessengerLiteIOSAppID) || session.DeviceID == uuid.Nil || session.FamilyDeviceID == uuid.Nil) {
		return fmt.Errorf("invalid native Messenger session")
	}
	return nil
}

var (
	loginFlowFacebook = bridgev2.LoginFlow{
		Name:        msgFlowFacebookName.Message(),
		Description: msgFlowFacebookDescription.Message(),
		ID:          FlowIDFacebookCookies,
	}
	loginFlowMessenger = bridgev2.LoginFlow{
		Name:        msgFlowMessengerName.Message(),
		Description: msgFlowMessengerDescription.Message(),
		ID:          FlowIDMessengerCookies,
	}
	loginFlowMessengerLiteIOS = bridgev2.LoginFlow{
		Name:        msgFlowMessengerLiteIOSName.Message(),
		Description: msgFlowMessengerLiteIOSDescription.Message(),
		ID:          FlowIDMessengerLiteIOS,
	}
	loginFlowMessengerLiteAndroid = bridgev2.LoginFlow{
		Name:        msgFlowMessengerLiteAndroidName.Message(),
		Description: msgFlowMessengerLiteAndroidDescription.Message(),
		ID:          FlowIDMessengerLiteAndroid,
	}
)

func (m *MetaConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{loginFlowFacebook, loginFlowMessenger, loginFlowMessengerLiteIOS, loginFlowMessengerLiteAndroid}
}

type MetaCookieLogin struct {
	MessengerHistoryLogin

	Mode types.Platform
	User *bridgev2.User
	Main *MetaConnector
	HTTP http.RoundTripper
}

var _ bridgev2.LoginProcessCookies = (*MetaCookieLogin)(nil)
var _ bridgev2.LoginProcessUserInput = (*MetaCookieLogin)(nil)

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

const facebookCookiesWaitForURLPattern = `^https://www\.facebook\.com/(?:messages/(?:e2ee/)?(?:t/[0-9]+/?)?)?(?:\?[^#]*)?(?:#.*)?$`

func facebookCookieStep(stepID string, instructions l10n.Message) *bridgev2.LoginStep {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeCookies,
		StepID:       stepID,
		Instructions: instructions,
		CookiesParams: &bridgev2.LoginCookiesParams{
			URL:               "https://www.facebook.com/",
			UserAgent:         useragent.UserAgent,
			Fields:            cookieListToFields(cookies.FBRequiredCookies, "facebook.com"),
			WaitForURLPattern: facebookCookiesWaitForURLPattern,
		},
	}
}

func (m *MetaCookieLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	instructions := msgStepCookiesInstructions.Message()
	switch m.Mode {
	case types.Facebook, types.FacebookTor:
		return facebookCookieStep(LoginStepIDCookies, instructions), nil
	case types.Messenger:
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeCookies,
			StepID:       LoginStepIDCookies,
			Instructions: instructions,
			CookiesParams: &bridgev2.LoginCookiesParams{
				URL:               "https://www.messenger.com/?no_redirect=true",
				UserAgent:         useragent.UserAgent,
				Fields:            cookieListToFields(cookies.FBRequiredCookies, "messenger.com"),
				WaitForURLPattern: "^https://www\\.messenger\\.com/(?:e2ee/)?(?:t/[0-9]+/?)?(?:\\?.*)?$",
			},
		}, nil
	default:
		return nil, fmt.Errorf("unknown mode %s", m.Mode)
	}
}

func (m *MetaCookieLogin) Cancel() {}

func getMessagixClient(ctx context.Context, log zerolog.Logger, conn *MetaConnector, c *cookies.Cookies, useProxy bool) (*messagix.Client, error) {
	client := messagix.NewClient(c, log, conn.getMessagixConfig())
	if useProxy && conn.Bridge.Config.Proxy.IsConfigured() {
		client.GetHTTP().GetNewProxy = conn.Bridge.GetProxy
		if !client.GetHTTP().UpdateProxy(ctx, "login") {
			return nil, fmt.Errorf("failed to update proxy")
		}
	}
	return client, nil
}

func loginWithCookies(
	ctx context.Context,
	log zerolog.Logger,
	client *messagix.Client,
	bridgeUser *bridgev2.User,
	conn *MetaConnector,
	c *cookies.Cookies,
	nativeSession *types.NativeSession,
) (*bridgev2.LoginStep, error) {
	if missing := c.GetMissingCookieNames(); len(missing) > 0 {
		return nil, loginerrors.MissingCookies.AppendMessage(": %v", missing)
	} else if c.Platform.IsMessengerLite() && nativeSession == nil {
		return nil, loginerrors.TokenExchange
	} else if err := validateNativeSession(nativeSession); err != nil {
		return nil, err
	}
	client.MessengerLite.SetNativeSession(nativeSession)

	log.Debug().
		Strs("cookie_names", shape.Keys(c.GetAll())).
		Msg("Logging in with cookies")
	user, tbl, err := client.LoadMessagesPage(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to load messages page for login")
		if errors.Is(err, httpclient.ErrChallengeRequired) {
			return nil, loginerrors.Challenge
		} else if errors.Is(err, httpclient.ErrCheckpointRequired) {
			return nil, loginerrors.Checkpoint
		} else if errors.Is(err, httpclient.ErrConsentRequired) {
			return nil, loginerrors.Consent
		} else if errors.Is(err, httpclient.ErrTokenInvalidated) {
			return nil, loginerrors.TokenInvalidated
		} else if errors.Is(err, httpclient.ErrRateLimited) {
			return nil, loginerrors.RateLimited
		} else if errors.Is(err, httpclient.ErrUnexpectedError) {
			return nil, fmt.Errorf("%w: %w", loginerrors.InboxLoadFailed.WithInternalError(err), err)
		} else {
			return nil, fmt.Errorf("%w: %w", loginerrors.Unknown.WithInternalError(err), err)
		}
	}

	id := user.GetFBID()
	loginID := metaid.MakeUserLoginID(id)
	if id != c.GetUserID() {
		return nil, fmt.Errorf("logged-in account does not match cookies")
	}
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
			Platform:      c.Platform,
			Cookies:       c,
			LoginUA:       loginUA,
			NativeSession: nativeSession,
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to save new login: %w", err)
	}

	metaClient := ul.Client.(*MetaClient)
	// Override the client because LoadMessagesPage saves some state and we don't want to call it again
	client.Logger = ul.Log.With().Str("component", "messagix").Logger()
	client.SetEventHandler(metaClient.handleMetaEvent)
	metaClient.lastFullReconnect = time.Time{}
	metaClient.Client = client

	backgroundCtx := ul.Log.WithContext(conn.Bridge.BackgroundCtx)
	ul.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	go metaClient.connectWithTable(backgroundCtx, tbl, user)
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepIDComplete,
		Instructions: msgStepCompleteInstructions.Render(l10n.Params{"name": user.GetName(), "id": id}),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
}

func (m *MetaCookieLogin) SubmitCookies(ctx context.Context, strCookies map[string]string) (*bridgev2.LoginStep, error) {
	c := &cookies.Cookies{Platform: m.Mode}
	strCookiesCopy := map[cookies.MetaCookieName]string{}
	for key, val := range strCookies {
		strCookiesCopy[cookies.MetaCookieName(key)] = val
	}
	c.UpdateValues(strCookiesCopy)

	missingCookies := c.GetMissingCookieNames()
	if len(missingCookies) > 0 {
		return nil, loginerrors.MissingCookies.AppendMessage(": %v", missingCookies)
	}

	log := zerolog.Ctx(ctx).With().Str("component", "messagix").Logger()
	client, err := getMessagixClient(ctx, log, m.Main, c, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	m.historyRelogin = slices.Contains(m.User.GetUserLoginIDs(), metaid.MakeUserLoginID(c.GetUserID()))
	step, err := loginWithCookies(ctx, log, client, m.User, m.Main, c, nil)
	if err != nil {
		return nil, err
	}
	return m.startHistory(ctx, step)
}

type MetaNativeLogin struct {
	MessengerHistoryLogin

	Mode types.Platform
	User *bridgev2.User
	Main *MetaConnector

	SavedClient         *messagix.Client
	loginDevice         types.MessengerLoginDevice
	phoneCookieFallback bool
}

func (m *MetaNativeLogin) Cancel() {}

func (m *MetaNativeLogin) loginDeviceAppID() string {
	if m.Mode == types.MessengerLiteAndroid {
		return useragent.MessengerLiteAndroidAppID
	}
	return useragent.MessengerLiteIOSAppID
}

func (m *MetaNativeLogin) loadLoginDevice(ctx context.Context, override *bridgev2.UserLogin) (*types.MessengerLoginDevice, error) {
	if m.Main.DB == nil {
		return nil, nil
	}
	appID := m.loginDeviceAppID()
	device, err := m.Main.DB.GetMessengerLoginDevice(ctx, m.User.MXID, appID)
	if err != nil || device != nil {
		return device, err
	}
	logins := m.User.GetUserLogins()
	slices.SortFunc(logins, func(a, b *bridgev2.UserLogin) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	if override != nil {
		logins = append([]*bridgev2.UserLogin{override}, logins...)
	}
	for _, login := range logins {
		session := login.Metadata.(*metaid.UserLoginMetadata).NativeSession
		if session == nil || session.AppID != appID || session.DeviceID == uuid.Nil || session.FamilyDeviceID == uuid.Nil {
			continue
		}
		device = &types.MessengerLoginDevice{
			DeviceID:       session.DeviceID,
			FamilyDeviceID: session.FamilyDeviceID,
			MachineID:      session.MachineID,
		}
		if err = m.Main.DB.PutMessengerLoginDevice(ctx, m.User.MXID, appID, *device); err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save Messenger login device")
		}
		return device, nil
	}
	return nil, nil
}

func (m *MetaNativeLogin) saveLoginDevice(ctx context.Context) {
	device := m.SavedClient.MessengerLite.GetLoginDevice()
	if m.Main.DB == nil || device.DeviceID == uuid.Nil || device == m.loginDevice {
		return
	}
	err := m.Main.DB.PutMessengerLoginDevice(ctx, m.User.MXID, m.loginDeviceAppID(), device)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to save Messenger login device")
		return
	}
	m.loginDevice = device
}

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.StartWithParams(ctx, bridgev2.LoginStartParams{})
}

func (m *MetaNativeLogin) StartWithParams(ctx context.Context, params bridgev2.LoginStartParams) (*bridgev2.LoginStep, error) {
	log := zerolog.Ctx(ctx).With().Str("component", "messagix").Logger()
	device, err := m.loadLoginDevice(ctx, params.Override)
	if err != nil {
		return nil, fmt.Errorf("failed to load Messenger login device: %w", err)
	}
	log.Debug().
		Bool("client_http", params.HTTP != nil).
		Bool("known_device", device != nil).
		Msg("Starting Messenger Lite login flow")

	fakeCookies := &cookies.Cookies{
		Platform: m.Mode,
	}
	client, err := getMessagixClient(ctx, log, m.Main, fakeCookies, m.Main.Config.ProxyMessengerLite)
	if err != nil {
		return nil, err
	}
	if params.HTTP != nil {
		client.GetHTTP().GetNewProxy = nil
		client.GetHTTP().SetTransportOverride(params.HTTP)
	}
	if device != nil {
		client.MessengerLite.SetLoginDevice(device)
		m.loginDevice = *device
	}
	m.SavedClient = client

	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if m.historyComplete != nil {
		return m.MessengerHistoryLogin.SubmitUserInput(ctx, input)
	}
	if m.phoneCookieFallback {
		return nil, fmt.Errorf("waiting for browser login cookies")
	}
	return m.proceed(ctx, input)
}

func (m *MetaNativeLogin) SubmitCookies(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if m.phoneCookieFallback {
		return m.completePhoneCookieLogin(ctx, input)
	}
	return m.proceed(ctx, input)
}

func (m *MetaNativeLogin) Wait(ctx context.Context) (*bridgev2.LoginStep, error) {
	if m.phoneCookieFallback {
		return nil, fmt.Errorf("waiting for browser login cookies")
	}
	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) CancelStep(ctx context.Context) (*bridgev2.LoginStep, error) {
	if m.historyComplete != nil {
		return m.finishHistory(ctx)
	}
	if m.phoneCookieFallback {
		return nil, fmt.Errorf("current login step cannot be cancelled")
	}
	if err := m.SavedClient.MessengerLite.CancelLoginStep(ctx); err != nil {
		return nil, wrapMetaRespError(err)
	}
	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) completePhoneCookieLogin(ctx context.Context, strCookies map[string]string) (*bridgev2.LoginStep, error) {
	webCookies := &cookies.Cookies{Platform: types.Facebook}
	cookieMap := make(map[cookies.MetaCookieName]string, len(strCookies))
	for key, val := range strCookies {
		cookieMap[cookies.MetaCookieName(key)] = val
	}
	webCookies.UpdateValues(cookieMap)

	if missing := webCookies.GetMissingCookieNames(); len(missing) > 0 {
		return nil, loginerrors.MissingCookies.AppendMessage(": %v", missing)
	}

	log := zerolog.Ctx(ctx).With().Str("component", "messagix").Logger()
	nativeCookies, nativeSession, err := m.SavedClient.MessengerLite.ExchangeCookiesForNativeSession(ctx, webCookies)
	if err != nil {
		log.Error().Err(err).Msg("Failed to exchange phone-login cookies for native session")
		var respErr bridgev2.RespError
		switch {
		case errors.As(err, &respErr):
			return nil, respErr
		case errors.Is(err, httpclient.ErrChallengeRequired):
			return nil, loginerrors.Challenge
		case errors.Is(err, httpclient.ErrCheckpointRequired):
			return nil, loginerrors.Checkpoint
		case errors.Is(err, httpclient.ErrTokenInvalidated):
			return nil, loginerrors.TokenInvalidated
		case errors.Is(err, httpclient.ErrRateLimited):
			return nil, loginerrors.RateLimited
		case httpclient.IsClientHTTPError(err):
			return nil, loginerrors.ClientHTTPFailed.WithInternalError(err)
		default:
			return nil, loginerrors.TokenExchange.WithInternalError(err)
		}
	}

	newClient, err := getMessagixClient(ctx, log, m.Main, &cookies.Cookies{Platform: types.MessengerLiteAndroid}, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	newClient.GetCookies().UpdateValues(nativeCookies.GetAll())

	m.historyRelogin = slices.Contains(m.User.GetUserLoginIDs(), metaid.MakeUserLoginID(nativeCookies.GetUserID()))
	step, err := loginWithCookies(ctx, log, newClient, m.User, m.Main, nativeCookies, nativeSession)
	if err != nil {
		return nil, err
	}
	m.phoneCookieFallback = false
	return m.startHistory(ctx, step)
}

func (m *MetaNativeLogin) proceed(ctx context.Context, userInput map[string]string) (*bridgev2.LoginStep, error) {
	log := zerolog.Ctx(ctx).With().Str("component", "messagix").Logger()

	step, newCookies, err := m.SavedClient.MessengerLite.DoLoginSteps(ctx, userInput)
	m.saveLoginDevice(ctx)
	if err != nil {
		if errors.Is(err, loginerrors.PhoneNumber) {
			m.phoneCookieFallback = true
			return facebookCookieStep(
				LoginStepIDPhoneCookies,
				msgStepPhoneCookiesInstructions.Message(),
			), nil
		}
		log.Error().Err(err).Msg("Login steps returned error")
		if errors.As(err, &bloks.CheckpointError{}) {
			err = loginerrors.Checkpoint
		} else if httpclient.IsClientHTTPError(err) {
			err = loginerrors.ClientHTTPFailed.WithInternalError(err)
		}
		return nil, err
	}
	if step != nil {
		return step, nil
	}

	// Create a new messagix.Client here so that we can change
	// proxy settings between the login and post-login.
	fakeCookies := &cookies.Cookies{
		Platform: m.Mode,
	}
	newClient, err := getMessagixClient(ctx, log, m.Main, fakeCookies, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}

	newClient.GetCookies().UpdateValues(newCookies.GetAll())

	m.historyRelogin = slices.Contains(m.User.GetUserLoginIDs(), metaid.MakeUserLoginID(newCookies.GetUserID()))
	step, err = loginWithCookies(ctx, log, newClient, m.User, m.Main, newCookies, m.SavedClient.MessengerLite.GetNativeSession())
	if err != nil {
		return nil, err
	}

	return m.startHistory(ctx, step)
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessCookies = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessDisplayAndWait = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessStepCancel = (*MetaNativeLogin)(nil)
