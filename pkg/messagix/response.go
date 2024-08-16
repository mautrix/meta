package messagix

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/byter"
	"go.mau.fi/mautrix-meta/pkg/messagix/packets"
)

type ResponseData interface {
	Finish() ResponseData
	SetIdentifier(identifier uint16)
}
type responseHandler func() ResponseData

var responseMap = map[uint8]responseHandler{
	packets.CONNACK:  func() ResponseData { return &Event_Ready{} },
	packets.PUBACK:   func() ResponseData { return &Event_PublishACK{} },
	packets.SUBACK:   func() ResponseData { return &Event_SubscribeACK{} },
	packets.PUBLISH:  func() ResponseData { return &Event_PublishResponse{} },
	packets.PINGRESP: func() ResponseData { return &Event_PingResp{} },
}

type Response struct {
	PacketByte      uint8
	RemainingLength uint32 `vlq:"true"`
	ResponseData    ResponseData
}

func (r *Response) QOS() packets.QoS {
	return packets.QoS((r.PacketByte >> 1) & 0x03)
}

func (r *Response) PacketType() uint8 {
	return r.PacketByte >> 4
}

func (r *Response) Read(data []byte) error {
	reader := byter.NewReader(data)
	if err := reader.ReadToStruct(r); err != nil {
		return err
	}

	packetType := r.PacketByte >> 4 // parse the packet type by the leftmost 4 bits
	qosLevel := (r.PacketByte >> 1) & 0x03

	responseHandlerFunc, ok := responseMap[packetType]
	if !ok {
		return fmt.Errorf("could not find response func handler for packet type %d", packetType)
	}
	r.ResponseData = responseHandlerFunc()

	bufferBytes := reader.Buff.Bytes()

	if packetType == packets.PUBLISH && qosLevel == 1 {
		identifierBytes := bufferBytes[10:12]
		identifier := binary.BigEndian.Uint16(identifierBytes)
		r.ResponseData.SetIdentifier(identifier)
		// remove the msg identifier
		bufferBytes = append(bufferBytes[:10], bufferBytes[12:]...)
		reader.Buff = bytes.NewBuffer(bufferBytes)
	}

	return reader.ReadToStruct(r.ResponseData)
}
