package connector

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

var FACEBOOK_COOKIES_FLOW = "cookies-facebook"
var INSTAGRAM_COOKIES_FLOW = "cookies-instagram"

func (m *MetaConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	if flowID != FACEBOOK_COOKIES_FLOW && flowID != INSTAGRAM_COOKIES_FLOW {
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
		ID:          FACEBOOK_COOKIES_FLOW,
	}
	instagram := bridgev2.LoginFlow{
		Name:        "Instagram Cookies",
		Description: "Login using cookies from Instagram",
		ID:          INSTAGRAM_COOKIES_FLOW,
	}
	if m.Config == nil {
		return []bridgev2.LoginFlow{facebook, instagram}
	} else if m.Config.Mode == "facebook" {
		return []bridgev2.LoginFlow{facebook}
	} else if m.Config.Mode == "instagram" {
		return []bridgev2.LoginFlow{instagram}
	} else {
		panic("unknown mode in config")
	}
}

type MetaCookieLogin struct {
	Flow string
	User *bridgev2.User
	Main *MetaConnector
}

var _ bridgev2.LoginProcessCookies = (*MetaCookieLogin)(nil)

func requiredCookiesToKeys(requiredCookies []cookies.MetaCookieName) []string {
	var keys []string
	// Turn the array of MetaCookieName into an array of strings
	for _, key := range requiredCookies {
		keys = append(keys, string(key))
	}
	return keys
}

func (m *MetaCookieLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	if m.Flow == FACEBOOK_COOKIES_FLOW {
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeCookies,
			StepID:       "fi.mau.meta.cookies",
			Instructions: "Please enter cookies from your browser",
			CookiesParams: &bridgev2.LoginCookiesParams{
				URL:          "https://www.facebook.com/",
				UserAgent:    messagix.UserAgent,
				CookieDomain: "www.facebook.com",
				CookieKeys:   requiredCookiesToKeys(cookies.FBRequiredCookies),
			},
		}, nil
	} else if m.Flow == INSTAGRAM_COOKIES_FLOW {
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeCookies,
			StepID:       "fi.mau.meta.cookies",
			Instructions: "Please enter cookies from your browser",
			CookiesParams: &bridgev2.LoginCookiesParams{
				URL:          "https://www.instagram.com/",
				UserAgent:    messagix.UserAgent,
				CookieDomain: "www.instagram.com",
				CookieKeys:   requiredCookiesToKeys(cookies.IGRequiredCookies),
			},
		}, nil
	} else {
		return nil, fmt.Errorf("unknown flow ID %s", m.Flow)
	}
}

func (m *MetaCookieLogin) Cancel() {}

func (m *MetaCookieLogin) getFBID(currentUser types.UserInfo, tbl *table.LSTable) int64 {
	var newFBID int64
	// TODO figure out why the contact IDs for self is different than the fbid in the ready event
	for _, row := range tbl.LSVerifyContactRowExists {
		if row.IsSelf && row.ContactId != newFBID {
			if newFBID != 0 {
				// Hopefully this won't happen
				m.User.Log.Warn().Int64("prev_fbid", newFBID).Int64("new_fbid", row.ContactId).Msg("Got multiple fbids for self")
			} else {
				m.User.Log.Debug().Int64("fbid", row.ContactId).Msg("Found own fbid")
			}
			newFBID = row.ContactId
		}
	}
	if newFBID == 0 {
		newFBID = currentUser.GetFBID()
		m.User.Log.Warn().Int64("fbid", newFBID).Msg("Own contact entry not found, falling back to fbid in current user object")
	}
	return newFBID
}

func (m *MetaCookieLogin) SubmitCookies(ctx context.Context, strCookies map[string]string) (*bridgev2.LoginStep, error) {
	c := &cookies.Cookies{
		Platform: types.Instagram,
	}
	if m.Flow == FACEBOOK_COOKIES_FLOW {
		c.Platform = types.Facebook
	}

	c.UnmarshalJSON([]byte(`{}`))
	for key, value := range strCookies {
		c.Set(cookies.MetaCookieName(key), value)
	}

	// Check if the cookies are valid
	if !c.IsLoggedIn() {
		return nil, fmt.Errorf("invalid cookies")
	}

	log := m.User.Log.With().Str("component", "messagix").Logger()
	client := messagix.NewClient(c, log)

	currentUser, initialTable, err := client.LoadMessagesPage()
	if err != nil {
		return nil, err
	}

	FBID := m.getFBID(currentUser, initialTable)

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       "fi.mau.meta.complete",
		Instructions: fmt.Sprintf("Logged in as %s (%d)", currentUser.GetName(), FBID),
	}, nil
}
