package instameow

import (
	"bytes"
	"compress/zlib"
	"io"
	"testing"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestBuildMobileMQTToTConnectPacket(t *testing.T) {
	packet, err := buildMobileMQTToTConnectPacket(&types.InstagramMobileSession{
		Authorization: "Bearer IGT:2:test-authorization",
		UserID:        "123",
		Device: types.InstagramLoginDevice{
			DeviceID: "11111111-2222-3333-4444-555555555555",
		},
	}, time.UnixMilli(123456789))
	if err != nil {
		t.Fatalf("buildMobileMQTToTConnectPacket returned error: %v", err)
	}
	header, body, err := readMQTTPacket(bytes.NewReader(packet))
	if err != nil {
		t.Fatalf("failed to read generated packet: %v", err)
	}
	if header != 0x10 || len(body) < 12 {
		t.Fatalf(
			"unexpected generated packet header=%#x body_length=%d",
			header,
			len(body),
		)
	}
	if got := string(body[2:8]); got != "MQTToT" {
		t.Fatalf("unexpected protocol name %q", got)
	}
	if body[8] != 3 || body[9] != 0xC2 {
		t.Fatalf(
			"unexpected protocol level or flags: level=%d flags=%#x",
			body[8],
			body[9],
		)
	}
	reader, err := zlib.NewReader(bytes.NewReader(body[12:]))
	if err != nil {
		t.Fatalf("generated compact-Thrift payload is not zlib data: %v", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress generated compact-Thrift payload: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("generated compact-Thrift payload is empty")
	}
}
