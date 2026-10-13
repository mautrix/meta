package messagix

import "go.mau.fi/mautrix-meta/pkg/messagix/securehistory"

func (c *Client) GetSecureHistoryClient() *securehistory.Client {
	return &securehistory.Client{
		HTTP:           c.http,
		RegistrationID: c.configs.BrowserConfigTable.MessengerWebInitData.SessionID,
		AppID:          c.configs.BrowserConfigTable.CurrentUserInitialData.AppID,
	}
}
