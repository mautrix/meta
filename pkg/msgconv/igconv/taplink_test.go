package igconv

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

func testCTAOptions() CTAOptions {
	return CTAOptions{Signer: NewTapSigner([]byte("test-only-signing-key")), Portal: networkid.PortalKey{ID: "thread", Receiver: "login"}, MessageID: "message"}
}

func TestTapSigner(t *testing.T) {
	signer := testCTAOptions().Signer
	target := TapTarget{Portal: testCTAOptions().Portal, MessageID: "message", Index: 1, Kind: TapPostback}
	token := signer.Sign(target)
	if token == "" || token != signer.Sign(target) {
		t.Fatal("signing must be deterministic")
	}
	got, err := signer.Verify(token)
	if err != nil || got != target {
		t.Fatal("round trip failed")
	}
	parts := strings.Split(token, ".")
	for _, invalid := range []string{"", "v1", token[:len(token)-5], "v2." + parts[1] + "." + parts[2], "v1.A" + parts[1][1:] + "." + parts[2], token + ".extra"} {
		if _, err := signer.Verify(invalid); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	if _, err := NewTapSigner([]byte("other-test-key")).Verify(token); err == nil {
		t.Fatal("rotated key accepted old token")
	}
}

func TestTapLinksOnCardsAndArrival(t *testing.T) {
	opts := testCTAOptions()
	xma := &slidetypes.XMAContent{TitleText: "Card", CTAButtons: []*slidetypes.CTAButton{
		{Title: "Open site", CTAType: "web_url", ActionURL: "https://example.com/"},
		{Title: "Choose", CTAType: "postback", PostbackPayload: "fixture-only"},
	}}
	raw, err := json.Marshal(xma)
	if err != nil {
		t.Fatal(err)
	}
	item := &slidetypes.ThreadItem{GenericXMA: raw, QuickReplies: []*slidetypes.QuickReply{{Title: "Reply"}}}
	buttons := ButtonsFromItem(item)
	assertLinks := func(t *testing.T, body string) {
		t.Helper()
		if strings.Count(body, `<a href="http://127.0.0.1:29330/_ig/tap/`) != 2 {
			t.Fatal("expected two tap hyperlinks")
		}
		if !strings.Contains(body, `[`+" "+`Choose`+" "+`]</a>`) || !strings.Contains(body, `[`+" "+`Reply`+" "+`]</a>`) {
			t.Fatal("missing linked pseudo-button titles")
		}
		if !strings.Contains(body, `<a href="https://example.com/">[`+" "+`Open site`+" "+`]</a>`) || strings.Contains(body, " - ") || strings.Contains(body, "<ul>") {
			t.Fatal("URL CTA layout changed")
		}
		if strings.Contains(body, " — postback") || strings.Contains(body, " — quick reply") {
			t.Fatal("per-button instructions must not appear")
		}
		if strings.Contains(body, "fixture-only") {
			t.Fatal("button payload leaked")
		}
	}
	t.Run("thread rebuild", func(t *testing.T) { assertLinks(t, CardHTMLFromThreadItem(item, opts)) })
	t.Run("CTA helper", func(t *testing.T) {
		assertLinks(t, xmaCTAButtonsHTML(xma.CTAButtons, buttons.Postbacks, buttons.QuickReplies, opts))
	})
	t.Run("incoming XMA", func(t *testing.T) {
		mc := &MessageConverter{TapSigner: opts.Signer}
		ctx := context.WithValue(context.Background(), mediadl.ContextKeyPortal, &bridgev2.Portal{Portal: &database.Portal{PortalKey: opts.Portal}})
		ctx = context.WithValue(ctx, mediadl.ContextKeyMsgID, opts.MessageID)
		part := mc.wrapXMAWithButtons(ctx, xma, buttons)
		if part == nil {
			t.Fatal("no incoming card")
		}
		assertLinks(t, part.Content.FormattedBody)
		if part.Content.FormattedBody != CardHTMLFromThreadItem(item, opts) {
			t.Fatal("refresh rotated incoming links")
		}
	})
	t.Run("incoming text quick reply", func(t *testing.T) {
		part := &bridgev2.ConvertedMessagePart{Content: &event.MessageEventContent{MsgType: event.MsgText, Body: "Hello"}}
		appendButtonsToText(part, Buttons{QuickReplies: []*metaid.PostbackButton{{Title: "Reply", CTAType: "quick_reply"}}}, opts)
		if !strings.Contains(part.Content.FormattedBody, `<a href="http://127.0.0.1:29330/_ig/tap/`) {
			t.Fatal("text quick reply has no link")
		}
	})
}
