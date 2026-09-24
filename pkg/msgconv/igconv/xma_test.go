package igconv

import (
	"context"
	"strings"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
)

func TestWrapXMACaptionIncludesCTALinks(t *testing.T) {
	mc := &MessageConverter{}
	got := mc.wrapXMACaption(context.Background(), &slidetypes.XMAContent{
		TitleText: "Card",
		CTAButtons: []*slidetypes.CTAButton{
			{Title: "Watch", ActionURL: "https://example.com/watch"},
			{Title: "No URL"},
		},
	})
	if got == nil {
		t.Fatal("expected caption")
	}
	if !strings.Contains(got.FormattedBody, `href="https://example.com/watch"`) {
		t.Fatalf("missing link in %s", got.FormattedBody)
	}
	if !strings.Contains(got.FormattedBody, "Watch") {
		t.Fatalf("missing title in %s", got.FormattedBody)
	}
	if strings.Contains(got.FormattedBody, "No URL") {
		t.Fatalf("button without action_url should be skipped, got %s", got.FormattedBody)
	}
}
