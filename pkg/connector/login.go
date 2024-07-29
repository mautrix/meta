package connector

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	//"go.mau.fi/util/exslices"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"

	//"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
	//"go.mau.fi/mautrix-meta/pkg/store"
)

const FlowIDFacebookCookies = "cookies-facebook"
const FlowIDInstagramCookies = "cookies-instagram"

func (m *MetaConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	if flowID != FlowIDFacebookCookies && flowID != FlowIDInstagramCookies {
		return nil, fmt.Errorf("unknown flow ID %s", flowID)
	}

	return &MetaCookieLogin{
		Flow: flowID,
		User: user,
		Main: m,
	}, nil
}

func (m *MetaConnector) GetLoginFlows() []bridgev2.LoginFlow {
	facebook := bridgev2.LoginFlow{
		Name:        "Facebook Cookies",
		Description: "Login using cookies from Facebook Messenger",
		ID:          FlowIDFacebookCookies,
	}
	instagram := bridgev2.LoginFlow{
		Name:        "Instagram Cookies",
		Description: "Login using cookies from Instagram",
		ID:          FlowIDInstagramCookies,
	}
	if m.Config.Mode == "" {
		return []bridgev2.LoginFlow{facebook, instagram}
	} else if m.Config.Mode == "facebook" {
		return []bridgev2.LoginFlow{facebook}
	} else if m.Config.Mode == "instagram" {
		return []bridgev2.LoginFlow{instagram}
	} else {
		panic("unknown mode in config") // This should never happen if ValidateConfig is implemented correctly
	}
}

type MetaCookieLogin struct {
	Flow string
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
	if m.Flow == FlowIDFacebookCookies {
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeCookies,
			StepID:       "fi.mau.meta.cookies",
			Instructions: "Please enter cookies from your browser",
			CookiesParams: &bridgev2.LoginCookiesParams{
				URL:       "https://www.facebook.com/",
				UserAgent: messagix.UserAgent,
				Fields:    cookieListToFields(cookies.FBRequiredCookies, "www.facebook.com"),
			},
		}, nil
	} else if m.Flow == FlowIDInstagramCookies {
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeCookies,
			StepID:       "fi.mau.meta.cookies",
			Instructions: "Please enter cookies from your browser",
			CookiesParams: &bridgev2.LoginCookiesParams{
				URL:       "https://www.instagram.com/",
				UserAgent: messagix.UserAgent,
				Fields:    cookieListToFields(cookies.IGRequiredCookies, "www.instagram.com"),
			},
		}, nil
	} else {
		return nil, fmt.Errorf("unknown flow ID %s", m.Flow)
	}
}

func (m *MetaCookieLogin) Cancel() {}

func (m *MetaCookieLogin) SubmitCookies(ctx context.Context, strCookies map[string]string) (*bridgev2.LoginStep, error) {
	c := &cookies.Cookies{
		Platform: types.Instagram,
	}
	if m.Flow == FlowIDFacebookCookies {
		c.Platform = types.Facebook
	}
	c.UpdateValues(strCookies)

	if !c.IsLoggedIn() {
		return nil, fmt.Errorf("invalid cookies")
	}

	log := m.User.Log.With().Str("component", "messagix").Logger()
	client := messagix.NewClient(c, log)

	user, tbl, err := client.LoadMessagesPage()
	if err != nil {
		return nil, fmt.Errorf("failed to load messages page: %w", err)
	}

	id := user.GetFBID()
	if client.Instagram != nil {
		id, err = client.Instagram.ExtractFBID(user, tbl)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch FBID: %w", err)
		}
	}

	login_id := networkid.UserLoginID(fmt.Sprint(id))

	ul, err := m.Main.Bridge.GetExistingUserLoginByID(ctx, login_id)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing login: %w", err)
	}
	if ul != nil && ul.UserMXID != m.User.MXID {
		// TODO: Do we actually want to do this?
		ul.Delete(ctx, status.BridgeState{StateEvent: status.StateLoggedOut, Error: "overridden-by-another-user"}, false)
		ul = nil
	}
	if ul == nil {
		ul, err = m.User.NewLogin(ctx, &database.UserLogin{
			ID: login_id,
			Metadata: &MetaLoginMetadata{
				Platform: c.Platform,
				Cookies:  c,
			},
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to save new login: %w", err)
		}
	} else {
		ul.RemoteName = user.GetName()
		err := ul.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to update existing login: %w", err)
		}
	}

	backgroundCtx := ul.Log.WithContext(context.Background())
	err = m.Main.LoadUserLogin(backgroundCtx, ul)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare connection after login: %w", err)
	}

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       "fi.mau.meta.complete",
		Instructions: fmt.Sprintf("Logged in as %s (%d)", user.GetName(), id),
	}, nil
}
