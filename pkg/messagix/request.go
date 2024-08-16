package messagix

import (
	"go.mau.fi/mautrix-meta/pkg/messagix/byter"
)

type Request struct {
	PacketByte      uint8
	RemainingLength uint32 `vlq:"true"`
}

func (r *Request) Write(payload Payload) ([]byte, error) {
	payloadBytes, err := payload.Write()
	if err != nil {
		return nil, err
	}

	r.RemainingLength = uint32(len(payloadBytes))
	header, err := byter.NewWriter().WriteFromStruct(r)
	if err != nil {
		return nil, err
	}

	return append(header, payloadBytes...), nil
}
