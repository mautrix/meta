package igconnector

import (
	"errors"
	"testing"
	"time"

	"go.mau.fi/util/exsync"
	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
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

func TestShouldEditRefreshedCard(t *testing.T) {
	caption := `<p>Resolved caption</p>`
	if shouldEditRefreshedCard(false, caption) {
		t.Error("caption-only original must not be edited")
	}
	if !shouldEditRefreshedCard(true, caption) {
		t.Error("bundled latest edit may be edited")
	}
	for name, html := range map[string]string{
		"app fallback": `<p>Hi<br>Use the Instagram app to click buttons</p>`,
		"link run":     `<p>Cap</p><p><a href="https://a.example">A</a> - <a href="https://b.example">B</a></p>`,
		"marker":       `<p>Cap</p><div data-ig-cta=""><ul></ul></div>`,
	} {
		if !shouldEditRefreshedCard(false, html) {
			t.Errorf("%s: footer in original must allow edit", name)
		}
	}
}

func TestPendingPostbackOTID(t *testing.T) {
	ic := &IGClient{pendingPostbackOTIDs: exsync.NewSet[string]()}
	ic.pendingPostbackOTIDs.Add("123")
	if got := ic.postbackTransactionID(&slidetypes.Message{OfflineThreadingID: "999"}); got != "" {
		t.Errorf("unknown OTID got txn %q", got)
	}
	if got := ic.postbackTransactionID(&slidetypes.Message{}); got != "" {
		t.Errorf("empty OTID got txn %q", got)
	}
	if got := ic.postbackTransactionID(&slidetypes.Message{OfflineThreadingID: "123"}); got != "123" {
		t.Errorf("pending OTID got txn %q", got)
	}
	if got := ic.postbackTransactionID(&slidetypes.Message{OfflineThreadingID: "123"}); got != "" {
		t.Errorf("OTID must be consumed, got %q", got)
	}
}

func TestSelectButtonForTextQuickReply(t *testing.T) {
	now := time.Now()
	follow := &metaid.PostbackButton{Title: "Following", Payload: "payload-b", CTAType: "postback"}
	maybe := &metaid.PostbackButton{Title: "Maybe later", Payload: "payload-qr-1", CTAType: "quick_reply"}
	card := &database.Message{Timestamp: now, Metadata: &metaid.MessageMetadata{
		PostbackButtons: []*metaid.PostbackButton{follow}, QuickReplies: []*metaid.PostbackButton{maybe}}}
	qrOnly := &database.Message{Timestamp: now.Add(-time.Minute), Metadata: &metaid.MessageMetadata{
		QuickReplies: []*metaid.PostbackButton{{Title: "Yes", CTAType: "quick_reply"}}}}
	recent := func() ([]*database.Message, error) { return []*database.Message{qrOnly, card}, nil }

	if got := selectPostbackForText(card, recent, "2"); got != maybe || !got.IsQuickReply() {
		t.Errorf("reply 2 must be the quick reply: %v", got)
	}
	if got := selectPostbackForText(nil, recent, "maybe later"); got != maybe {
		t.Errorf("title: %v", got)
	}
	if got := selectPostbackForText(qrOnly, recent, "1"); got == nil || got.Title != "Yes" {
		t.Errorf("quick reply only card: %v", got)
	}
	if newestPostbackMessage([]*database.Message{qrOnly}) != qrOnly {
		t.Error("quick-reply-only message must count as a button message")
	}
}
