package instameow

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

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
	var formKeys []string
	for key, values := range form {
		if len(values) > 0 && values[0] != "" {
			formKeys = append(formKeys, key)
		}
	}
	var variables any
	_ = json.Unmarshal([]byte(form.Get("variables")), &variables)
	headerNames := make([]string, 0, len(headers))
	for name := range headers {
		headerNames = append(headerNames, strings.ToLower(name))
	}
	var cookieNames []string
	for _, cookie := range strings.Split(headers.Get("cookie"), ";") {
		if name, _, ok := strings.Cut(strings.TrimSpace(cookie), "="); ok {
			cookieNames = append(cookieNames, name)
		}
	}
	slices.Sort(formKeys)
	slices.Sort(headerNames)
	slices.Sort(cookieNames)
	c.log.Debug().
		Str("login_request", request).
		Strs("form_keys", formKeys).
		Strs("variable_keys", jsonKeyPaths("", variables, nil)).
		Strs("header_names", headerNames).
		Strs("cookie_names", cookieNames).
		Int("dyn_length", len(form.Get("__dyn"))).
		Int("csr_length", len(form.Get("__csr"))).
		Msg("Instagram web login request")
}

func jsonKeyPaths(prefix string, value any, paths []string) []string {
	obj, ok := value.(map[string]any)
	if !ok {
		return paths
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		path := prefix + key
		paths = append(paths, path)
		paths = jsonKeyPaths(path+".", obj[key], paths)
	}
	return paths
}
