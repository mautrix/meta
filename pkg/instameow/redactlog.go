package instameow

import (
	"net/http"
	"net/url"
	"strings"

	"go.mau.fi/util/shape"

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

func (c *Client) logLoginRequestShape(request string, form url.Values, headers http.Header) {
	if !c.logRedactedLoginResponses {
		return
	}
	cookieNames := make(map[string]struct{})
	for _, cookie := range strings.Split(headers.Get("cookie"), ";") {
		if name, _, ok := strings.Cut(strings.TrimSpace(cookie), "="); ok {
			cookieNames[name] = struct{}{}
		}
	}
	c.log.Debug().
		Str("login_request", request).
		Strs("form_keys", shape.Keys(form)).
		Strs("variable_keys", shape.JSON([]byte(form.Get("variables")), 32)).
		Strs("header_names", shape.Keys(headers)).
		Strs("cookie_names", shape.Keys(cookieNames)).
		Int("dyn_length", len(form.Get("__dyn"))).
		Int("csr_length", len(form.Get("__csr"))).
		Msg("Instagram web login request")
}
