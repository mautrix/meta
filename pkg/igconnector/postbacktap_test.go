package igconnector

import (
	"errors"
	"testing"
	"time"

	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func cardMsg(_ string, ts time.Time, buttons ...*metaid.PostbackButton) *database.Message {
	return &database.Message{ID: "mid:" + "x", Timestamp: ts, Metadata: &metaid.MessageMetadata{PostbackButtons: buttons}}
}

func TestSelectPostbackForText(t *testing.T) {
	now := time.Now()
	guide := &metaid.PostbackButton{Title: "Guide", Payload: "payload-a"}
	follow := &metaid.PostbackButton{Title: "Following", Payload: "payload-b"}
	older := cardMsg("1", now.Add(-time.Hour), &metaid.PostbackButton{Title: "Guide", Payload: "payload-old"})
	newer := cardMsg("2", now, guide, follow)
	plain := &database.Message{Metadata: &metaid.MessageMetadata{}}
	recent := func() ([]*database.Message, error) { return []*database.Message{older, plain, newer}, nil }
	failing := func() ([]*database.Message, error) { return nil, errors.New("db") }

	if got := selectPostbackForText(newer, failing, "2"); got != follow {
		t.Errorf("reply+number: %v", got)
	}
	if got := selectPostbackForText(plain, recent, "following"); got != follow {
		t.Errorf("reply to plain, falls back to newest card title: %v", got)
	}
	if got := selectPostbackForText(nil, recent, "guide"); got != guide {
		t.Errorf("newest card wins over older: %v", got)
	}
	if got := selectPostbackForText(nil, recent, "1"); got != nil {
		t.Errorf("bare number outside reply must be text: %v", got)
	}
	if got := selectPostbackForText(newer, recent, "hello"); got != nil {
		t.Errorf("unknown text: %v", got)
	}
	if got := selectPostbackForText(nil, failing, "guide"); got != nil {
		t.Errorf("db error must fall through to text: %v", got)
	}
	if got := selectPostbackForText(nil, recent, "  "); got != nil {
		t.Errorf("empty text: %v", got)
	}
}
