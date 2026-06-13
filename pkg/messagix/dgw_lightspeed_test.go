package messagix

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
)

func TestDGWLightSpeedRequestFrameShape(t *testing.T) {
	lsPayload := &SocketLSRequestPayload{
		AppId:     "2220391788200892",
		Payload:   `{"epoch_id":1}`,
		RequestId: 4,
		Type:      3,
	}
	jsonPayload, err := json.Marshal(lsPayload)
	if err != nil {
		t.Fatal(err)
	}
	data, err := marshalDGWFrames(
		&dgw.OpenFrame{StreamID: 0},
		&dgw.DataFrame{
			StreamID:    0,
			Payload:     append([]byte{0x80}, jsonPayload...),
			RequiresAck: true,
			AckID:       0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	expectedPrefix := []byte{0x0f, 0x00, 0x00, 0x02, 0x00, 0x00, '{', '}', 0x0d, 0x00, 0x00}
	if !bytes.HasPrefix(data, expectedPrefix) {
		t.Fatalf("unexpected DGW request frame prefix: % x", data[:len(expectedPrefix)])
	}

	frame := dgw.CheckFrameType(data)
	rest, err := frame.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	openFrame, ok := frame.(*dgw.OpenFrame)
	if !ok {
		t.Fatalf("first frame has type %T, expected *dgw.OpenFrame", frame)
	}
	if openFrame.StreamID != 0 {
		t.Fatalf("unexpected open stream ID: %d", openFrame.StreamID)
	}

	frame = dgw.CheckFrameType(rest)
	_, err = frame.Unmarshal(rest)
	if err != nil {
		t.Fatal(err)
	}
	dataFrame, ok := frame.(*dgw.DataFrame)
	if !ok {
		t.Fatalf("second frame has type %T, expected *dgw.DataFrame", frame)
	}
	if !dataFrame.RequiresAck {
		t.Fatal("DGW request data frame should request an ack")
	}
	if dataFrame.Payload[0] != 0x80 {
		t.Fatalf("unexpected DGW data payload prefix: %x", dataFrame.Payload[0])
	}
	var decoded SocketLSRequestPayload
	err = json.Unmarshal(dataFrame.Payload[1:], &decoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != *lsPayload {
		t.Fatalf("decoded payload mismatch: got %+v, want %+v", decoded, *lsPayload)
	}
}

func TestExtractDGWJSON(t *testing.T) {
	want := []byte(`{"request_id":4,"payload":"{\"step\":true}","sp":[],"target":3}`)
	got, err := extractDGWJSON(append([]byte{0x80}, want...))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected JSON: got %s, want %s", got, want)
	}
}
