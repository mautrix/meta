package igconnector

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
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

func TestPlanRefreshEditCTAOnlyCard(t *testing.T) {
	var item slidetypes.ThreadItem
	raw := `{"item_id":"1","generic_xma":{"cta_buttons":[{"cta_type":"postback","title":"Go","postback_payload":"pb-1"}]}}`
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatal(err)
	}
	card := igconv.CardHTMLFromThreadItem(&item, igconv.CTAOptions{})
	cta := igconv.BuildCTAButtonsHTML(item.AllCTAButtons(), igconv.ButtonsFromItem(&item).Postbacks, nil, igconv.CTAOptions{})
	if card == "" || cta == "" {
		t.Fatalf("card=%q cta=%q", card, cta)
	}
	got, send := planRefreshEdit(card, cta, nil, errors.New("can't decrypt the event"))
	if !send || got == nil || got.FormattedBody != card {
		t.Fatalf("send=%v got=%+v", send, got)
	}
}

func TestPlanRefreshEdit(t *testing.T) {
	const card = `<strong>Card</strong><div data-ig-cta=""><ul></ul></div>`
	const cta = `<div data-ig-cta=""><ol></ol></div>`
	evtWith := func(content *event.MessageEventContent) *event.Event {
		return &event.Event{Content: event.Content{Parsed: content}}
	}
	t.Run("get error still sends", func(t *testing.T) {
		got, send := planRefreshEdit(card, cta, nil, errors.New("can't decrypt the event"))
		if !send || got.FormattedBody != card || got.MsgType != event.MsgText {
			t.Fatalf("send=%v got=%+v", send, got)
		}
	})
	t.Run("identical html is a no-op", func(t *testing.T) {
		evt := evtWith(&event.MessageEventContent{MsgType: event.MsgText, Body: "Card", Format: event.FormatHTML, FormattedBody: card})
		if _, send := planRefreshEdit(card, cta, evt, nil); send {
			t.Fatal("expected no edit")
		}
	})
	t.Run("image keeps msgtype", func(t *testing.T) {
		evt := evtWith(&event.MessageEventContent{MsgType: event.MsgImage, Body: "pic.png", URL: "mxc://example.org/abc"})
		got, send := planRefreshEdit(card, cta, evt, nil)
		if !send || got.MsgType != event.MsgImage || got.URL != "mxc://example.org/abc" || got.FormattedBody != card || got.Body != "**Card**" {
			t.Fatalf("send=%v got=%+v", send, got)
		}
	})
	t.Run("no card html and unknown event is skipped", func(t *testing.T) {
		if _, send := planRefreshEdit("", cta, nil, errors.New("boom")); send {
			t.Fatal("expected no edit")
		}
	})
	t.Run("no card html patches footer", func(t *testing.T) {
		evt := evtWith(&event.MessageEventContent{MsgType: event.MsgText, Body: "x", Format: event.FormatHTML, FormattedBody: `<p>Cap</p><div data-ig-cta=""><ul></ul></div>`})
		got, send := planRefreshEdit("", cta, evt, nil)
		if !send || got.FormattedBody != `<p>Cap</p>`+cta {
			t.Fatalf("send=%v got=%+v", send, got)
		}
	})
	t.Run("no card html and caption-only original is skipped", func(t *testing.T) {
		evt := evtWith(&event.MessageEventContent{MsgType: event.MsgText, Body: "x", Format: event.FormatHTML, FormattedBody: `<p>Cap</p>`})
		if _, send := planRefreshEdit("", cta, evt, nil); send {
			t.Fatal("expected no edit")
		}
	})
}

func TestRefreshCardSkipPaths(t *testing.T) {
	logger := zerolog.Nop()
	ce := &commands.Event{Log: &logger, Bridge: &bridgev2.Bridge{Network: &IGConnector{MsgConv: &igconv.MessageConverter{}}}}
	meta := &metaid.MessageMetadata{}
	var withCTA slidetypes.ThreadItem
	if err := json.Unmarshal([]byte(`{"item_id":"42","cta_buttons":[{"cta_type":"postback","title":"Go","postback_payload":"pb"}]}`), &withCTA); err != nil {
		t.Fatal(err)
	}
	var noCTA slidetypes.ThreadItem
	if err := json.Unmarshal([]byte(`{"item_id":"42"}`), &noCTA); err != nil {
		t.Fatal(err)
	}
	id := metaid.MakeFBMessageID("42")
	for name, tc := range map[string]struct {
		msg   *database.Message
		items []*slidetypes.ThreadItem
	}{
		"no mxid":     {&database.Message{ID: id, MXID: "", Metadata: meta}, []*slidetypes.ThreadItem{&withCTA}},
		"no metadata": {&database.Message{ID: id, MXID: "$a"}, []*slidetypes.ThreadItem{&withCTA}},
		"no match":    {&database.Message{ID: id, MXID: "$a", Metadata: meta}, nil},
		"no cta html": {&database.Message{ID: id, MXID: "$a", Metadata: meta}, []*slidetypes.ThreadItem{&noCTA}},
		"no item id":  {&database.Message{ID: "", MXID: "$a", Metadata: meta}, []*slidetypes.ThreadItem{&withCTA}},
	} {
		if got := refreshCard(ce, nil, tc.items, tc.msg); got != refreshSkipped {
			t.Errorf("%s: got %v, want skipped", name, got)
		}
	}
}

func TestNewestPostbackMessageSkipsButtonPoll(t *testing.T) {
	now := time.Now()
	buttons := []*metaid.PostbackButton{{Title: "Yes", Payload: "p", CTAType: metaid.CTATypeQuickReply}}
	card := &database.Message{Timestamp: now.Add(-time.Minute), Metadata: &metaid.MessageMetadata{QuickReplies: buttons}}
	poll := &database.Message{PartID: metaid.ButtonPollPartID, Timestamp: now, Metadata: &metaid.MessageMetadata{QuickReplies: buttons, ButtonPoll: true}}
	if got := newestPostbackMessage([]*database.Message{poll, card}); got != card {
		t.Errorf("the card must be tapped, got %+v", got)
	}
	if got := newestPostbackMessage([]*database.Message{poll}); got != nil {
		t.Errorf("a lone poll row is not a card: %+v", got)
	}
}
