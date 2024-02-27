package messagix

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.mau.fi/mautrix-meta/messagix/types"
)

func (c *Client) processLogin(resp *http.Response, respBody []byte) error {
	statusCode := resp.StatusCode
	var err error
	switch c.platform {
	case types.Facebook, types.Messenger, types.FacebookTor:
		if hasUserCookie := c.findCookie(resp.Cookies(), "c_user"); hasUserCookie == nil {
			err = fmt.Errorf("failed to login to facebook")
		}
	case types.Instagram:
		var loginResp *types.InstagramLoginResponse
		err = json.Unmarshal(respBody, &loginResp)
		if err != nil {
			return fmt.Errorf("failed to unmarshal instagram login response to *types.InstagramLoginResponse (statusCode=%d): %v", statusCode, err)
		}
		if loginResp.Status == "fail" {
			err = fmt.Errorf("failed to process login request (message=%s, statusText=%s, statusCode=%d)", loginResp.Message, loginResp.Status, statusCode)
		} else if !loginResp.Authenticated {
			err = fmt.Errorf("failed to login, invalid password (userExists=%t, statusText=%s, statusCode=%d)", loginResp.User, loginResp.Status, statusCode)
		}
	}

	if err == nil {
		c.cookies.UpdateFromResponse(resp)
	}

	return err
}
