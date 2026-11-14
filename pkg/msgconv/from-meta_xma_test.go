package msgconv

import (
	"context"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func TestXMAWithoutMediaKeepsCTALink(t *testing.T) {
	mc := &MessageConverter{}
	att := &table.WrappedXMA{
		LSInsertXmaAttachment: &table.LSInsertXmaAttachment{TitleText: "Attachment unavailable"},
		CTA: &table.LSInsertAttachmentCta{
			Type_:     "xma_web_url",
			ActionUrl: "https://l.facebook.com/l.php?u=https%3A%2F%2Fwww.facebook.com%2Fexample%2Fposts%2F123&h=x",
		},
	}
	parts := mc.xmaAttachmentToMatrix(context.Background(), att)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	const wantURL = "https://www.facebook.com/example/posts/123"
	if got, want := parts[0].Content.Body, "Attachment unavailable\n\n"+wantURL; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := parts[0].Extra["external_url"]; got != wantURL {
		t.Errorf("external_url = %v, want %q", got, wantURL)
	}
}

func TestXMAWithoutMediaOrCTAKeepsTitle(t *testing.T) {
	mc := &MessageConverter{}
	att := &table.WrappedXMA{
		LSInsertXmaAttachment: &table.LSInsertXmaAttachment{TitleText: "Attachment unavailable"},
	}
	parts := mc.xmaAttachmentToMatrix(context.Background(), att)
	if len(parts) != 1 || parts[0].Content.Body != "Attachment unavailable" {
		t.Fatalf("unexpected parts: %+v", parts)
	}
	if parts[0].Content.FormattedBody != "" {
		t.Errorf("formatted body should be empty without a link, got %q", parts[0].Content.FormattedBody)
	}
}
