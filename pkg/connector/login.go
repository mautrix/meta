package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	waTypes "go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/util/exslices"

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

	LoginStepIDCookies  = "fi.mau.meta.cookies"
	LoginStepIDComplete = "fi.mau.meta.complete"

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

type metaCredentials struct {
	Platform      types.Platform       `json:"platform"`
	Cookies       *cookies.Cookies     `json:"cookies"`
	NativeSession *types.NativeSession `json:"native_session,omitempty"`
}

func validateNativeSession(session *types.NativeSession) error {
	if session != nil && (session.AccessToken == "" || session.AppID != useragent.MessengerLiteAndroidAppID || session.DeviceID == uuid.Nil || session.FamilyDeviceID == uuid.Nil) {
		return fmt.Errorf("invalid native Messenger session")
	}
	return nil
}

func (m *MetaConnector) CreateUserLoginFromCredentials(ctx context.Context, user *bridgev2.User, credentials any) error {
	creds, ok := credentials.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid credentials type: %T", credentials)
	}
	var transferred metaCredentials
	if _, structured := creds["cookies"]; structured {
		data, err := json.Marshal(creds)
		if err != nil {
			return fmt.Errorf("failed to encode transferred credentials: %w", err)
		} else if err = json.Unmarshal(data, &transferred); err != nil {
			return fmt.Errorf("failed to decode transferred credentials: %w", err)
		} else if transferred.Cookies == nil || !transferred.Platform.IsMessenger() {
			return fmt.Errorf("invalid transferred Messenger credentials")
		} else if err = validateNativeSession(transferred.NativeSession); err != nil {
			return err
		}
		transferred.Cookies.Platform = transferred.Platform
	} else {
		transferred.Platform = types.Facebook
		if m.Config.Tor {
			transferred.Platform = types.FacebookTor
		}
		transferred.Cookies = &cookies.Cookies{Platform: transferred.Platform}
		values := make(map[cookies.MetaCookieName]string, len(creds))
		for key, value := range creds {
			str, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid cookie value type for %s", key)
			}
			values[cookies.MetaCookieName(key)] = str
		}
		transferred.Cookies.UpdateValues(values)
	}
	log := zerolog.Ctx(ctx).With().Str("component", "messagix").Logger()
	client, err := getMessagixClient(log, m, transferred.Cookies, m.Config.ProxyOther)
	if err != nil {
		return err
	}
	step, err := loginWithCookies(ctx, log, client, user, m, transferred.Cookies, transferred.NativeSession, true)
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
	loginFlowMessengerLiteIOS = bridgev2.LoginFlow{
		Name:        "Messenger iOS",
		Description: "Login with username/password using Messenger iOS API",
		ID:          FlowIDMessengerLiteIOS,
	}
	loginFlowMessengerLiteAndroid = bridgev2.LoginFlow{
		Name:        "Messenger Android",
		Description: "Login with username/password using Messenger Android API",
		ID:          FlowIDMessengerLiteAndroid,
	}
)

func (m *MetaConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{loginFlowMessengerLiteAndroid, loginFlowFacebook, loginFlowMessenger, loginFlowMessengerLiteIOS}
}

type MetaCookieLogin struct {
	Mode types.Platform
	User *bridgev2.User
	Main *MetaConnector
	HTTP http.RoundTripper
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
			UserAgent: useragent.UserAgent,
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
	default:
		return nil, fmt.Errorf("unknown mode %s", m.Mode)
	}
	return step, nil
}

func (m *MetaCookieLogin) Cancel() {}

func getMessagixClient(log zerolog.Logger, conn *MetaConnector, c *cookies.Cookies, useProxy bool) (*messagix.Client, error) {
	client := messagix.NewClient(c, log, conn.getMessagixConfig())
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
	client *messagix.Client,
	bridgeUser *bridgev2.User,
	conn *MetaConnector,
	c *cookies.Cookies,
	nativeSession *types.NativeSession,
	transfer bool,
) (*bridgev2.LoginStep, error) {
	if missing := c.GetMissingCookieNames(); len(missing) > 0 {
		return nil, loginerrors.MissingCookies.AppendMessage(": %v", missing)
	} else if err := validateNativeSession(nativeSession); err != nil {
		return nil, err
	}
	client.MessengerLite.SetNativeSession(nativeSession)

	log.Debug().
		Strs("cookie_names", exslices.CastToString[string](slices.Collect(maps.Keys(c.GetAll())))).
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
		} else {
			return nil, fmt.Errorf("%w: %w", loginerrors.Unknown, err)
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
	metadata := &metaid.UserLoginMetadata{}
	existing, err := conn.Bridge.GetExistingUserLoginByID(ctx, loginID)
	if err != nil {
		return nil, fmt.Errorf("failed to load existing login: %w", err)
	} else if existing != nil && existing.UserMXID != bridgeUser.MXID {
		return nil, fmt.Errorf("that account is already logged in by another user")
	} else if existing != nil {
		*metadata = *existing.Metadata.(*metaid.UserLoginMetadata)
	}
	if nativeSession != nil {
		deviceID := nativeSession.DeviceID
		if transfer {
			deviceID = uuid.New()
		}
		if metadata.WADeviceID != 0 {
			device, err := conn.DeviceStore.GetDevice(ctx, waTypes.JID{User: string(loginID), Device: metadata.WADeviceID, Server: waTypes.MessengerServer})
			if err != nil {
				return nil, fmt.Errorf("failed to load existing encrypted device: %w", err)
			} else if device != nil {
				if device.FacebookUUID == uuid.Nil {
					return nil, fmt.Errorf("existing encrypted device has no native identity")
				}
				deviceID = device.FacebookUUID
			} else {
				metadata.WADeviceID = 0
			}
		}
		if deviceID != nativeSession.DeviceID {
			authClient, err := getMessagixClient(log, conn, &cookies.Cookies{Platform: types.MessengerLiteAndroid}, conn.Config.ProxyMessengerLite)
			if err != nil {
				return nil, err
			}
			updated := *nativeSession
			updated.DeviceID = deviceID
			authClient.MessengerLite.SetNativeSession(&updated)
			newCookies, err := authClient.MessengerLite.ExchangeTransientToken(ctx, nativeSession.AccessToken)
			if err != nil {
				return nil, fmt.Errorf("failed to prepare native session for encrypted device: %w", err)
			} else if newCookies.GetUserID() != id {
				return nil, fmt.Errorf("exchanged native session account does not match login")
			}
			nativeSession = authClient.MessengerLite.GetNativeSession()
		}
	}
	metadata.Platform = c.Platform
	metadata.Cookies = c
	metadata.LoginUA = loginUA
	metadata.NativeSession = nativeSession
	client.MessengerLite.SetNativeSession(nativeSession)
	if existing != nil && existing.Client != nil {
		existing.Client.Disconnect()
	}

	ul, err := bridgeUser.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: user.GetName(),
		RemoteProfile: status.RemoteProfile{
			Name: user.GetName(),
		},
		Metadata: metadata,
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
		Instructions: fmt.Sprintf("Logged in as %s (%d)", user.GetName(), id),
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
	client, err := getMessagixClient(log, m.Main, c, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	return loginWithCookies(ctx, log, client, m.User, m.Main, c, nil, false)
}

type MetaNativeLogin struct {
	Mode types.Platform
	User *bridgev2.User
	Main *MetaConnector

	SavedClient *messagix.Client
}

func (m *MetaNativeLogin) Cancel() {}

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.StartWithParams(ctx, bridgev2.LoginStartParams{})
}

func (m *MetaNativeLogin) StartWithParams(ctx context.Context, params bridgev2.LoginStartParams) (*bridgev2.LoginStep, error) {
	log := zerolog.Ctx(ctx).With().Str("component", "messagix").Logger()
	log.Debug().
		Bool("client_http", params.HTTP != nil).
		Msg("Starting Messenger Lite login flow")

	fakeCookies := &cookies.Cookies{
		Platform: m.Mode,
	}
	client, err := getMessagixClient(log, m.Main, fakeCookies, m.Main.Config.ProxyMessengerLite)
	if err != nil {
		return nil, err
	}
	if params.HTTP != nil {
		client.GetHTTP().GetNewProxy = nil
		client.GetHTTP().HTTP.Transport = params.HTTP
	}
	m.SavedClient = client

	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	return m.proceed(ctx, input)
}

func (m *MetaNativeLogin) SubmitCookies(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	return m.proceed(ctx, input)
}

func (m *MetaNativeLogin) Wait(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) CancelStep(ctx context.Context) (*bridgev2.LoginStep, error) {
	if err := m.SavedClient.MessengerLite.CancelLoginStep(ctx); err != nil {
		return nil, err
	}
	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) proceed(ctx context.Context, userInput map[string]string) (*bridgev2.LoginStep, error) {
	log := zerolog.Ctx(ctx).With().Str("component", "messagix").Logger()

	step, newCookies, err := m.SavedClient.MessengerLite.DoLoginSteps(ctx, userInput)
	if err != nil {
		log.Error().Err(err).Msg("Login steps returned error")
		if errors.As(err, &bloks.CheckpointError{}) {
			err = loginerrors.Checkpoint
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
	newClient, err := getMessagixClient(log, m.Main, fakeCookies, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}

	newClient.GetCookies().UpdateValues(newCookies.GetAll())

	step, err = loginWithCookies(ctx, log, newClient, m.User, m.Main, newCookies, m.SavedClient.MessengerLite.GetNativeSession(), false)
	if err != nil {
		return nil, err
	}

	return step, nil
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessCookies = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessDisplayAndWait = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessStepCancel = (*MetaNativeLogin)(nil)
