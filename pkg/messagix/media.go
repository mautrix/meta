package messagix

import (
	"encoding/base64"
	"encoding/json"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
)

type RUploadToken struct {
	DSUserID  string `json:"ds_user_id"`
	SessionID string `json:"sessionid"`
}

type RUploadResponse struct {
	ID      int   `json:"id"`
	MediaID int64 `json:"media_id"`
}

func (c *Client) GetRUploadToken() string {
	token, _ := json.Marshal(RUploadToken{
		DSUserID:  c.cookies.Get(cookies.IGCookieDSUserID),
		SessionID: c.cookies.Get(cookies.IGCookieSessionID),
	})
	return "Bearer IGT:2:" + base64.StdEncoding.EncodeToString(token)
}
