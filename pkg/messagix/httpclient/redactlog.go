package httpclient

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"

	"github.com/rs/zerolog"
	"go.mau.fi/util/redact"
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
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(body), AntiJSPrefix))
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

func AddRedactedResponse(evt *zerolog.Event, body []byte) *zerolog.Event {
	redacted := redactLoginResponse(body)
	if json.Valid(redacted) {
		return evt.RawJSON("response_redacted", redacted)
	} else if len(redacted) > 0 {
		return evt.Bytes("response_redacted_gz", gzipBase64(redacted))
	}
	return evt
}
