package instameow

import (
	"bytes"
	"encoding/json"

	"github.com/rs/zerolog"
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

func addRedactedLoginResponse(evt *zerolog.Event, body []byte) *zerolog.Event {
	redacted := redactLoginResponse(body)
	if len(redacted) == 0 {
		return evt
	}
	if json.Valid(redacted) {
		return evt.RawJSON("response_redacted", redacted)
	}
	return evt.Bytes("response_redacted", redacted)
}
