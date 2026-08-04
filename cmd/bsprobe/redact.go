package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Everything written to disk by bsprobe passes through this file. The harness
// records real customer conversations, so redaction is applied at capture time
// rather than as a cleanup pass — a fixture that was never written unredacted
// cannot leak.

var (
	emailRe = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]{2,}`)
	phoneRe = regexp.MustCompile(`\+?\d[\d\s().-]{8,}\d`)
	urlRe   = regexp.MustCompile(`https?://[^\s"']+`)
)

// safeKeys survive redaction untouched. These are the protocol facts the whole
// exercise exists to capture, and several of them (friendly_name) would
// otherwise be caught by the contentKeys "name" rule.
var safeKeys = map[string]bool{
	"fb_api_req_friendly_name": true,
	"friendly_name":            true,
	"operation_name":           true,
	"doc_id":                   true,
	"query_id":                 true,
	"__typename":               true,
	"type":                     true,
	"id":                       true,
	"cursor":                   true,
	"count":                    true,
	"has_next_page":            true,
	"end_cursor":               true,
	"request_id":               true,
	"target":                   true,
	"sp":                       true,
	"sync_group":               true,
	"database_id":              true,
	"mailbox_id":               true,
	"asset_id":                 true,
	"business_id":              true,
	"page_id":                  true,
	"thread_id":                true,
	"folder":                   true,
	"__rev":                    true,
	"__spin_r":                 true,
	"av":                       true,
	"server_timestamp":         true,
	"timestamp":                true,
}

// secretKeys are auth material. Redacted wholesale, no length hint — length is
// itself a weak signal for a token.
var secretKeys = map[string]bool{
	"fb_dtsg":       true,
	"lsd":           true,
	"jazoest":       true,
	"access_token":  true,
	"accesstoken":   true,
	"cookie":        true,
	"set-cookie":    true,
	"authorization": true,
	"xs":            true,
	"c_user":        true,
	"datr":          true,
	"sb":            true,
	"fr":            true,
	"password":      true,
	"secret":        true,
	"csrf":          true,
	"csrf_token":    true,
	"session_key":   true,
}

// contentKeys carry customer identity or message text. Redacted, but the key
// and a length hint stay so the response shape remains readable.
var contentKeys = map[string]bool{
	"text":            true,
	"body":            true,
	"message":         true,
	"snippet":         true,
	"preview":         true,
	"caption":         true,
	"subtitle":        true,
	"name":            true,
	"full_name":       true,
	"display_name":    true,
	"first_name":      true,
	"last_name":       true,
	"short_name":      true,
	"username":        true,
	"email":           true,
	"phone":           true,
	"phone_number":    true,
	"profile_picture": true,
	"profile_pic_url": true,
	"note":            true,
	"notes":           true,
}

func classifyKey(k string) string {
	lk := strings.ToLower(k)
	if safeKeys[lk] {
		return "safe"
	}
	if secretKeys[lk] {
		return "secret"
	}
	if contentKeys[lk] {
		return "content"
	}
	// Catch-all suffix rules for keys we did not enumerate. Checked after the
	// allowlist so friendly_name and doc_id are already out of the way.
	switch {
	case strings.HasSuffix(lk, "_token"), strings.HasSuffix(lk, "_secret"), strings.Contains(lk, "password"):
		return "secret"
	case strings.HasSuffix(lk, "_name"), strings.HasSuffix(lk, "_text"), strings.HasSuffix(lk, "_email"):
		return "content"
	}
	return "plain"
}

// Redact walks a decoded JSON value and returns a copy that is safe to commit.
func Redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			switch classifyKey(k) {
			case "safe":
				out[k] = val
			case "secret":
				out[k] = "<redacted:secret>"
			case "content":
				out[k] = redactContent(val)
			default:
				out[k] = Redact(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = Redact(val)
		}
		return out
	case string:
		return RedactString(t)
	default:
		return v
	}
}

func redactContent(v any) any {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("<redacted:content len=%d>", len(s))
	}
	return Redact(v)
}

// RedactString scrubs free text: emails, phone numbers, and URL query strings
// (which carry CDN and session tokens). The host and path of a URL survive
// because endpoint discovery is the point of the exercise.
func RedactString(s string) string {
	s = urlRe.ReplaceAllStringFunc(s, redactURL)
	s = emailRe.ReplaceAllString(s, "<redacted:email>")
	s = phoneRe.ReplaceAllString(s, "<redacted:phone>")
	return s
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<redacted:url>"
	}
	if u.RawQuery == "" {
		return u.Scheme + "://" + u.Host + u.Path
	}
	return u.Scheme + "://" + u.Host + u.Path + "?<redacted:query>"
}

// RedactCookieHeader keeps cookie names and drops every value, so a capture
// still shows which cookies the surface requires.
func RedactCookieHeader(v string) string {
	parts := strings.Split(v, ";")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if i := strings.Index(p, "="); i > 0 {
			names = append(names, p[:i]+"=<redacted>")
		}
	}
	return strings.Join(names, "; ")
}

// Shape reduces a decoded JSON value to structure only: keys and type names,
// every value dropped. This is the safest fixture form and the one worth
// committing — it records the schema a connector must parse while carrying no
// customer data at all.
func Shape(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = Shape(val)
		}
		return out
	case []any:
		if len(t) == 0 {
			return []any{}
		}
		return map[string]any{
			"__array_len": len(t),
			"__element":   Shape(t[0]),
		}
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}
