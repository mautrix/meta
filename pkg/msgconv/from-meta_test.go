package msgconv

import (
	"context"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func TestXMAPollToMatrix(t *testing.T) {
	tests := []struct {
		name          string
		att           *table.LSInsertXmaAttachment
		wantNil       bool
		wantBody      string
		wantFormatted string
	}{
		{
			name: "question and options with votes",
			att: &table.LSInsertXmaAttachment{
				TitleText:           "Pizza or pasta?",
				ListItemTitleText1:  "Pizza",
				ListItemTotalCount1: 3,
				ListItemTitleText2:  "Pasta",
				ListItemTotalCount2: 1,
				ListItemTitleText3:  "Neither",
				ListItemTotalCount3: 0,
			},
			wantBody:      "**Pizza or pasta?**\n* Pizza (3 votes)\n* Pasta (1 vote)\n* Neither",
			wantFormatted: "<strong>Pizza or pasta?</strong><ul><li>Pizza (3 votes)</li><li>Pasta (1 vote)</li><li>Neither</li></ul>",
		},
		{
			name: "percentage is used when there's no count",
			att: &table.LSInsertXmaAttachment{
				TitleText:                            "Best day?",
				ListItemTitleText1:                   "Friday",
				ListItemProgressBarFilledPercentage1: 75,
			},
			wantBody:      "**Best day?**\n* Friday (75%)",
			wantFormatted: "<strong>Best day?</strong><ul><li>Friday (75%)</li></ul>",
		},
		{
			name: "question falls back to the list description",
			att: &table.LSInsertXmaAttachment{
				ListItemsDescriptionText: "Untitled poll",
				ListItemTitleText1:       "Yes",
			},
			wantBody:      "**Untitled poll**\n* Yes",
			wantFormatted: "<strong>Untitled poll</strong><ul><li>Yes</li></ul>",
		},
		{
			name:          "question with no options",
			att:           &table.LSInsertXmaAttachment{TitleText: "Anyone?"},
			wantBody:      "**Anyone?**",
			wantFormatted: "<strong>Anyone?</strong>",
		},
		{
			name: "html in the question is escaped",
			att: &table.LSInsertXmaAttachment{
				TitleText:          "<script>alert(1)</script>",
				ListItemTitleText1: "a & b",
			},
			wantFormatted: "<strong>&lt;script&gt;alert(1)&lt;/script&gt;</strong><ul><li>a &amp; b</li></ul>",
		},
		{
			name:    "nothing to render",
			att:     &table.LSInsertXmaAttachment{},
			wantNil: true,
		},
	}
	mc := &MessageConverter{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			part := mc.xmaPollToMatrix(context.Background(), &table.WrappedXMA{LSInsertXmaAttachment: test.att})
			if test.wantNil {
				if part != nil {
					t.Fatalf("Expected no part, got %+v", part.Content)
				}
				return
			}
			if part == nil {
				t.Fatal("Expected a part, got nil")
			}
			if test.wantFormatted != "" && part.Content.FormattedBody != test.wantFormatted {
				t.Errorf("FormattedBody =\n  %q\nwant\n  %q", part.Content.FormattedBody, test.wantFormatted)
			}
			if test.wantBody != "" && part.Content.Body != test.wantBody {
				t.Errorf("Body =\n  %q\nwant\n  %q", part.Content.Body, test.wantBody)
			}
		})
	}
}
