package instameow

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"go.mau.fi/util/redact"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
)

var loginRedactPolicy = makeLoginRedactPolicy()

func makeLoginRedactPolicy() redact.Policy {
	p := redact.MakeDefaultPolicy()
	// Error messages, status strings and field names are what make a login
	// response worth logging; keep them. HashPatterns still scrubs any PII
	// embedded in the values that are kept.
	p.KeepProse = true
	return p
}

func redactLoginResponse(body []byte) []byte {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(body), httpclient.AntiJSPrefix))
	if len(trimmed) == 0 {
		return nil
	}
	if json.Valid(trimmed) {
		if redacted, err := loginRedactPolicy.JSON(trimmed); err == nil {
			return redacted
		}
	}
	if trimmed[0] == '<' {
		if redacted, err := loginRedactPolicy.HTML(trimmed); err == nil {
			return redacted
		}
	}
	return []byte(loginRedactPolicy.String(string(trimmed)))
}

func gzipBase64(data []byte) []byte {
	var compressed bytes.Buffer
	compressor := gzip.NewWriter(&compressed)
	_, _ = compressor.Write(data)
	_ = compressor.Close()
	return base64.StdEncoding.AppendEncode(nil, compressed.Bytes())
}

func (c *Client) logRedactedLoginResponse(request string, response *http.Response, body []byte) {
	if !c.logRedactedLoginResponses {
		return
	}
	evt := c.log.Debug().Str("login_request", request)
	if response != nil {
		evt = evt.Int("status_code", response.StatusCode).Str("content_type", response.Header.Get("content-type"))
	}
	redacted := redactLoginResponse(body)
	if json.Valid(redacted) {
		evt = evt.RawJSON("response_redacted", redacted)
	} else if len(redacted) > 0 {
		evt = evt.Bytes("response_redacted_gz", gzipBase64(redacted))
	}
	evt.Msg("Instagram web login response")
}
