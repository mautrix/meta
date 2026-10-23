package instameow

import (
	"net/http"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
)

func (c *Client) logRedactedLoginResponse(request string, response *http.Response, body []byte) {
	if !c.logRedactedLoginResponses {
		return
	}
	evt := c.log.Debug().Str("login_request", request)
	if response != nil {
		evt = evt.Int("status_code", response.StatusCode).Str("content_type", response.Header.Get("content-type"))
	}
	httpclient.AddRedactedResponse(evt, body).Msg("Instagram web login response")
}
