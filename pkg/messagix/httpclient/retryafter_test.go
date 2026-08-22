package httpclient

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"garbage", 0},
		{"-5", 0},
		{"0", 0},
		{"30", 30 * time.Second},
		{" 7 ", 7 * time.Second},
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.in); got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	// An HTTP-date in the future parses to roughly that far ahead.
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got < 85*time.Second || got > 91*time.Second {
		t.Errorf("HTTP-date parse gave %v, want ~90s", got)
	}
	// An HTTP-date in the past is treated as absent, never negative.
	past := time.Now().Add(-90 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past); got != 0 {
		t.Errorf("past HTTP-date gave %v, want 0", got)
	}
}
