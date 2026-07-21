package lightspeed_test

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func TestDecodeDeletePartialThread(t *testing.T) {
	var output table.LSTable
	decoder := lightspeed.NewLightSpeedDecoder(
		table.SPToDepMap([]string{"deletePartialThread"}),
		&output,
	)
	decoder.Decode([]any{
		float64(lightspeed.CALL_STORED_PROCEDURE),
		"deletePartialThread",
		[]any{float64(lightspeed.I64_FROM_STRING), "1355757559300725"},
	})

	if len(output.LSDeletePartialThread) != 1 {
		t.Fatalf("expected one deleted partial thread, got %d", len(output.LSDeletePartialThread))
	}
	if threadKey := output.LSDeletePartialThread[0].ThreadKey; threadKey != 1355757559300725 {
		t.Fatalf("unexpected thread key: %d", threadKey)
	}
}
