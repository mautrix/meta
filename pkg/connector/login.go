package connector

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/util/exslices"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
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
	LoginStepIDProfile  = "fi.mau.meta.facebook_profile"
	LoginStepIDComplete = "fi.mau.meta.complete"

	LoginStepIDCredentials = "fi.mau.meta.credentials"
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

	cookieLogin := login.(*MetaCookieLogin)
	cookieLogin.SkipProfileSelection = true
	step, err := cookieLogin.SubmitCookies(ctx, cleanCreds)
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
		Description: "Login in using Messenger iOS API",
		ID:          FlowIDMessengerLiteIOS,
	}
	loginFlowMessengerLiteAndroid = bridgev2.LoginFlow{
		Name:        "Messenger Android",
		Description: "Login in using Messenger Android API",
		ID:          FlowIDMessengerLiteAndroid,
	}
)

func (m *MetaConnector) GetLoginFlows() []bridgev2.LoginFlow {
	if len(m.Config.AllowedModes) > 0 {
		flows := []bridgev2.LoginFlow{}
		// Note that we return login flows in whatever order
		// the user specified them in the config file.
		for _, mode := range m.Config.AllowedModes {
			switch mode {
			case types.Facebook:
				flows = append(flows, loginFlowFacebook)
			case types.Messenger:
				flows = append(flows, loginFlowMessenger)
			case types.MessengerLiteIOS:
				flows = append(flows, loginFlowMessengerLiteIOS)
			case types.MessengerLiteAndroid:
				flows = append(flows, loginFlowMessengerLiteAndroid)
			default:
				panic("unknown mode in config")
			}
		}
		return flows
	}
	switch m.Config.Mode {
	case types.Unset:
		return []bridgev2.LoginFlow{loginFlowFacebook, loginFlowMessenger, loginFlowMessengerLiteIOS, loginFlowMessengerLiteAndroid}
	case types.Facebook:
		if m.Config.AllowMessengerComOnFB {
			return []bridgev2.LoginFlow{loginFlowMessenger, loginFlowFacebook}
		}
		fallthrough
	case types.FacebookTor:
		return []bridgev2.LoginFlow{loginFlowFacebook}
	case types.Messenger:
		return []bridgev2.LoginFlow{loginFlowMessenger}
	case types.MessengerLiteIOS:
		return []bridgev2.LoginFlow{loginFlowMessengerLiteIOS}
	case types.MessengerLiteAndroid:
		return []bridgev2.LoginFlow{loginFlowMessengerLiteAndroid}
	default:
		panic("unknown mode in config")
	}
}

type MetaCookieLogin struct {
	Mode types.Platform
	User *bridgev2.User
	Main *MetaConnector

	SavedClient          *messagix.Client
	SavedCookies         *cookies.Cookies
	SavedUser            types.UserInfo
	SavedTable           *table.LSTable
	ProfileByOption      map[string]*types.SwitchableProfile
	SkipProfileSelection bool
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

func (m *MetaCookieLogin) Cancel() {
	m.SavedClient = nil
	m.SavedCookies = nil
	m.SavedUser = nil
	m.SavedTable = nil
	m.ProfileByOption = nil
}

var (
	ErrLoginMissingCookies   = bridgev2.RespError{ErrCode: "FI.MAU.META_MISSING_COOKIES", Err: "Missing cookies", StatusCode: http.StatusBadRequest}
	ErrLoginChallenge        = bridgev2.RespError{ErrCode: "FI.MAU.META_CHALLENGE_ERROR", Err: "Challenge required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	ErrLoginConsent          = bridgev2.RespError{ErrCode: "FI.MAU.META_CONSENT_ERROR", Err: "Consent required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	ErrLoginCheckpoint       = bridgev2.RespError{ErrCode: "FI.MAU.META_CHECKPOINT_ERROR", Err: "Checkpoint required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	ErrLoginTokenInvalidated = bridgev2.RespError{ErrCode: "FI.MAU.META_TOKEN_ERROR", Err: "Got logged out immediately", StatusCode: http.StatusBadRequest}
	ErrLoginProfileInvalid   = bridgev2.RespError{ErrCode: "FI.MAU.META_PROFILE_INVALID", Err: "That Facebook Page is not available to this account", StatusCode: http.StatusBadRequest}
	ErrLoginProfileSwitch    = bridgev2.RespError{ErrCode: "FI.MAU.META_PROFILE_SWITCH_ERROR", Err: "Facebook did not activate the selected Page", StatusCode: http.StatusBadRequest}
	ErrLoginNoPages          = bridgev2.RespError{ErrCode: "FI.MAU.META_NO_PAGES", Err: "No managed Facebook Pages were found for this account", StatusCode: http.StatusBadRequest}
	ErrLoginUnknown          = bridgev2.RespError{ErrCode: "M_UNKNOWN", Err: "Internal error logging in", StatusCode: http.StatusInternalServerError}
)

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

func loadLoginWithCookies(
	ctx context.Context,
	log zerolog.Logger,
	client *messagix.Client,
) (types.UserInfo, *table.LSTable, error) {

	log.Debug().
		Strs("cookie_names", exslices.CastToString[string](slices.Collect(maps.Keys(client.GetCookies().GetAll())))).
		Msg("Logging in with cookies")
	user, tbl, err := client.LoadMessagesPage(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to load messages page for login")
		if errors.Is(err, httpclient.ErrChallengeRequired) {
			return nil, nil, ErrLoginChallenge
		} else if errors.Is(err, httpclient.ErrCheckpointRequired) {
			return nil, nil, ErrLoginCheckpoint
		} else if errors.Is(err, httpclient.ErrConsentRequired) {
			return nil, nil, ErrLoginConsent
		} else if errors.Is(err, httpclient.ErrTokenInvalidated) {
			return nil, nil, ErrLoginTokenInvalidated
		} else {
			return nil, nil, fmt.Errorf("%w: %w", ErrLoginUnknown, err)
		}
	}
	return user, tbl, nil
}

func completeLoginWithCookies(
	ctx context.Context,
	client *messagix.Client,
	user types.UserInfo,
	tbl *table.LSTable,
	bridgeUser *bridgev2.User,
	conn *MetaConnector,
	c *cookies.Cookies,
	selectedProfile *types.SwitchableProfile,
) (*bridgev2.LoginStep, error) {

	id := user.GetFBID()
	remoteName := user.GetName()
	var businessSuite *messagix.BusinessSuiteContext
	if client.Platform == types.BusinessSuite {
		businessSuite = client.GetBusinessSuiteContext()
		if businessSuite == nil {
			return nil, fmt.Errorf("Business Suite Page context is missing")
		}
		pageID, parseErr := strconv.ParseInt(businessSuite.PageID, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid Business Suite Page ID: %w", parseErr)
		}
		id = pageID
		remoteName = businessSuite.PageName
	}

	loginID := metaid.MakeUserLoginID(id)
	var loginUA string
	if req, ok := ctx.Value("fi.mau.provision.request").(*http.Request); ok {
		loginUA = req.Header.Get("User-Agent")
	}

	ul, err := bridgeUser.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: remoteName,
		RemoteProfile: status.RemoteProfile{
			Name: remoteName,
		},
		Metadata: &metaid.UserLoginMetadata{
			Platform:  c.Platform,
			Cookies:   c,
			LoginUA:   loginUA,
			ActorID:   id,
			ActorName: remoteName,
			ActorType: func() string {
				if selectedProfile != nil {
					return "facebook_page"
				}
				return "personal"
			}(),
			BusinessID: func() string {
				if businessSuite != nil {
					return businessSuite.BusinessID
				}
				return ""
			}(),
			AssetID: func() string {
				if businessSuite != nil {
					return businessSuite.AssetID
				}
				return ""
			}(),
			PageID: func() string {
				if businessSuite != nil {
					return businessSuite.PageID
				}
				return ""
			}(),
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
		Instructions: fmt.Sprintf("Logged in as %s (%d) · %s", remoteName, id, map[bool]string{true: "Facebook Page", false: "Personal profile"}[selectedProfile != nil]),
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
		return nil, ErrLoginMissingCookies.AppendMessage(": %v", missingCookies)
	}

	log := m.User.Log.With().Str("component", "messagix").Logger()
	client, err := getMessagixClient(log, m.Main, c, m.Main.Config.ProxyOther)
	if err != nil {
		return nil, err
	}
	user, tbl, err := loadLoginWithCookies(ctx, log, client)
	if err != nil {
		return nil, err
	}

	profiles := client.GetSwitchableProfiles()
	requirePage := m.Main.Config.RequirePageSelection && !m.SkipProfileSelection && m.Mode.IsMessenger() && c.Get(cookies.FBCookieIUser) == ""
	if requirePage && len(profiles) == 0 {
		if err = client.DiscoverSwitchableProfiles(ctx); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoginNoPages, err)
		}
		profiles = client.GetSwitchableProfiles()
	}
	if requirePage && len(profiles) == 0 {
		return nil, ErrLoginNoPages
	}
	if !m.SkipProfileSelection && m.Mode.IsMessenger() && c.Get(cookies.FBCookieIUser) == "" && len(profiles) > 0 {
		m.SavedClient = client
		m.SavedCookies = c
		m.SavedUser = user
		m.SavedTable = tbl
		m.ProfileByOption = make(map[string]*types.SwitchableProfile, len(profiles)+1)

		options := make([]string, 0, len(profiles)+1)
		if !m.Main.Config.RequirePageSelection {
			personalOption := fmt.Sprintf("Personal inbox - %s [%d]", user.GetName(), user.GetFBID())
			m.ProfileByOption[personalOption] = nil
			options = append(options, personalOption)
		}
		for i := range profiles {
			profile := &profiles[i]
			if profile.ID == fmt.Sprint(user.GetFBID()) {
				continue
			}
			option := fmt.Sprintf("%s [%s]", profile.Name, profile.ID)
			m.ProfileByOption[option] = profile
			options = append(options, option)
		}
		if len(options) == 0 {
			return nil, ErrLoginNoPages
		}
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeUserInput,
			StepID:       LoginStepIDProfile,
			Instructions: "Choose the Facebook Page whose messages you want to bridge.",
			UserInputParams: &bridgev2.LoginUserInputParams{Fields: []bridgev2.LoginInputDataField{{
				Type:        bridgev2.LoginInputFieldTypeSelect,
				ID:          "facebook_profile",
				Name:        "Facebook Page",
				Description: "Pages are read from Facebook's signed-in profile switcher.",
				Options:     options,
				Validate: func(value string) (string, error) {
					if _, ok := m.ProfileByOption[value]; !ok {
						return "", ErrLoginProfileInvalid
					}
					return value, nil
				},
			}}},
		}, nil
	}

	var selectedProfile *types.SwitchableProfile
	if actorID := c.Get(cookies.FBCookieIUser); actorID != "" {
		for i := range profiles {
			if profiles[i].ID == actorID {
				selectedProfile = &profiles[i]
				break
			}
		}
	}
	return completeLoginWithCookies(ctx, client, user, tbl, m.User, m.Main, c, selectedProfile)
}

func (m *MetaCookieLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if m.SavedCookies == nil || m.SavedClient == nil || m.SavedUser == nil || m.SavedTable == nil {
		return nil, ErrLoginProfileInvalid
	}
	option := input["facebook_profile"]
	profile, ok := m.ProfileByOption[option]
	if !ok {
		return nil, ErrLoginProfileInvalid
	}
	if profile == nil {
		m.SavedCookies.Delete(cookies.FBCookieIUser)
		return completeLoginWithCookies(ctx, m.SavedClient, m.SavedUser, m.SavedTable, m.User, m.Main, m.SavedCookies, nil)
	}

	log := m.User.Log.With().Str("component", "messagix").Str("facebook_actor_id", profile.ID).Logger()
	pageContext, err := m.SavedClient.ResolveBusinessSuitePage(ctx, *profile)
	if err != nil {
		return nil, err
	}
	pageCookies := &cookies.Cookies{Platform: types.BusinessSuite}
	pageCookies.UpdateValues(m.SavedCookies.GetAll())
	pageCookies.Delete(cookies.FBCookieIUser)
	config := m.Main.getMessagixConfig()
	config.BusinessSuite = pageContext
	client := messagix.NewClient(pageCookies, log, config)
	if m.Main.Config.ProxyOther && (m.Main.Config.GetProxyFrom != "" || m.Main.Config.Proxy != "") {
		client.GetHTTP().GetNewProxy = m.Main.getProxy
		if !client.GetHTTP().UpdateProxy("login") {
			return nil, fmt.Errorf("failed to update proxy")
		}
	}
	user, tbl, err := loadLoginWithCookies(ctx, log, client)
	if err != nil {
		return nil, err
	}
	return completeLoginWithCookies(ctx, client, user, tbl, m.User, m.Main, pageCookies, profile)
}

type MetaNativeLogin struct {
	Mode types.Platform
	User *bridgev2.User
	Main *MetaConnector

	SavedClient *messagix.Client
}

func (m *MetaNativeLogin) Cancel() {}

func (m *MetaNativeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	log := m.User.Log.With().Str("component", "messagix").Logger()
	log.Debug().Msg("Starting Messenger Lite login flow")

	fakeCookies := &cookies.Cookies{
		Platform: m.Mode,
	}
	client, err := getMessagixClient(log, m.Main, fakeCookies, m.Main.Config.ProxyMessengerLite)
	if err != nil {
		return nil, err
	}
	m.SavedClient = client

	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	return m.proceed(ctx, input)
}

func (m *MetaNativeLogin) Wait(ctx context.Context) (*bridgev2.LoginStep, error) {
	return m.proceed(ctx, nil)
}

func (m *MetaNativeLogin) proceed(ctx context.Context, userInput map[string]string) (*bridgev2.LoginStep, error) {
	log := m.User.Log.With().Str("component", "messagix").Logger()

	step, newCookies, err := m.SavedClient.MessengerLite.DoLoginSteps(ctx, userInput)
	if err != nil {
		log.Error().Err(err).Msg("Login steps returned error")
		if errors.As(err, &bloks.CheckpointError{}) {
			err = ErrLoginCheckpoint
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

	user, tbl, err := loadLoginWithCookies(ctx, log, newClient)
	if err != nil {
		return nil, err
	}
	step, err = completeLoginWithCookies(ctx, newClient, user, tbl, m.User, m.Main, newCookies, nil)
	if err != nil {
		return nil, err
	}

	return step, nil
}

var _ bridgev2.LoginProcessUserInput = (*MetaNativeLogin)(nil)
var _ bridgev2.LoginProcessDisplayAndWait = (*MetaNativeLogin)(nil)
