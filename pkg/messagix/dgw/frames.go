package dgw

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

type FrameType uint8
type DrainReason uint8
type StreamID uint16

const (
	// There are some additional frame types in the web client
	// enum but they are not used in the code.
	FrameType_DRAIN FrameType = 0x03 // Drain
	FrameType_PING  FrameType = 0x09 // Ping
	FrameType_PONG  FrameType = 0x0a // Pong
	FrameType_ACK   FrameType = 0x0c // StreamGroup_Ack
	FrameType_DATA  FrameType = 0x0d // StreamGroup_Data
	FrameType_CLOSE FrameType = 0x0e // StreamGroup_EndOfData
	FrameType_OPEN  FrameType = 0x0f // StreamGroup_EstabStream
)

func (ft FrameType) Marshal() []byte {
	return []byte{byte(ft)}
}

func CheckFrameType(b []byte) Frame {
	switch FrameType(b[0]) {
	case FrameType_PING:
		return &PingFrame{}
	case FrameType_PONG:
		return &PongFrame{}
	case FrameType_ACK:
		return &AckFrame{}
	case FrameType_DATA:
		return &DataFrame{}
	case FrameType_OPEN:
		return &OpenFrame{}
	default:
		return &UnsupportedFrame{}
	}
}

const (
	Stream_FALCO    StreamID = 0x00
	Stream_TYPING   StreamID = 0x01
	Stream_PRESENCE StreamID = 0x02
)

type Frame interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) ([]byte, error)
}

type UnsupportedFrame struct {
	Raw []byte
}

func (f *UnsupportedFrame) Marshal() ([]byte, error) {
	return f.Raw, nil
}

func (f *UnsupportedFrame) Unmarshal(b []byte) ([]byte, error) {
	f.Raw = b
	return nil, nil
}

func (f *UnsupportedFrame) MayHaveLostData() bool {
	return true
}

type DrainFrame struct {
	DrainReason DrainReason
}

type PingFrame struct {
	//
}

func (f *PingFrame) Marshal() ([]byte, error) {
	return FrameType_PING.Marshal(), nil
}

func (f *PingFrame) Unmarshal(b []byte) ([]byte, error) {
	return b[1:], nil
}

type PongFrame struct {
	//
}

func (f *PongFrame) Marshal() ([]byte, error) {
	return FrameType_PONG.Marshal(), nil
}

func (f *PongFrame) Unmarshal(b []byte) ([]byte, error) {
	return b[1:], nil
}

type DataFrame struct {
	StreamID    StreamID
	Payload     []byte
	RequiresAck bool
	AckID       uint16
}

func (f *DataFrame) Marshal() ([]byte, error) {
	b := FrameType_DATA.Marshal()
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	b = binary.LittleEndian.AppendUint16(b, uint16(len(f.Payload)+2))
	b = append(b, 0x00) // unknown function
	b = binary.LittleEndian.AppendUint16(b, f.AckID)
	b[len(b)-1] &= 0b0111_1111
	if f.RequiresAck {
		b[len(b)-1] |= 0b1000_0000
	}
	b = append(b, f.Payload...)
	return b, nil
}

func (f *DataFrame) Unmarshal(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("too short for DataFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(b[1:3]))
	payloadLength := binary.LittleEndian.Uint16(b[3:5]) - 2
	f.AckID = binary.LittleEndian.Uint16(b[6:8]) & 0b0111_1111_1111_1111
	f.RequiresAck = b[7]&0b1000_0000 > 0
	if len(b) < int(8+payloadLength) {
		return nil, fmt.Errorf("too short for DataFrame payload")
	}
	f.Payload = b[8 : 8+payloadLength]
	return b[8+payloadLength:], nil
}

type AckFrame struct {
	StreamID StreamID
	AckID    uint16
}

func (f *AckFrame) Marshal() ([]byte, error) {
	b := FrameType_ACK.Marshal()
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	b = binary.LittleEndian.AppendUint16(b, 2)
	b = append(b, 0x00) // unknown function
	b = binary.LittleEndian.AppendUint16(b, f.AckID)
	return b, nil
}

func (f *AckFrame) Unmarshal(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("too short for AckFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(b[1:3]))
	payloadLength := binary.LittleEndian.Uint16(b[3:5])
	if payloadLength != 2 {
		return nil, fmt.Errorf("unnatural ack frame")
	}
	f.AckID = binary.LittleEndian.Uint16(b[6:8])
	return b[6+payloadLength:], nil
}

type CloseFrame struct {
	StreamID StreamID
}

type OpenFrameParams struct {
	XRSSMethod      string `json:"x-dgw-app-XRSS-method,omitempty"`
	XRSSDocID       string `json:"x-dgw-app-XRSS-doc_id,omitempty"`
	XRSSRoutingHint string `json:"x-dgw-app-XRSS-routing_hint,omitempty"`
	XRSBody         string `json:"x-dgw-app-xrs-body,omitempty"`
	XRSAcceptAck    string `json:"x-dgw-app-XRS-Accept-Ack,omitempty"`
	XRSSReferer     string `json:"x-dgw-app-XRSS-http_referer,omitempty"`

	StatusCode int `json:"code,omitempty"`
}

type OpenFrame struct {
	StreamID   StreamID
	Parameters OpenFrameParams

	// read only
	Raw []byte
}

func (f *OpenFrame) Marshal() ([]byte, error) {
	var err error
	b := FrameType_OPEN.Marshal()
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	payload, err := json.Marshal(&f.Parameters)
	if err != nil {
		return nil, err
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(payload)))
	b = append(b, 0x00)
	b = append(b, payload...)
	return b, nil
}

func (f *OpenFrame) Unmarshal(b []byte) ([]byte, error) {
	if len(b) < 6 {
		return nil, fmt.Errorf("too short for OpenFrame")
	}
	f.Raw = b
	f.StreamID = StreamID(binary.LittleEndian.Uint16(b[1:3]))
	payloadLength := binary.LittleEndian.Uint16(b[3:5])
	if len(b) < int(6+payloadLength) {
		return nil, fmt.Errorf("too short for OpenFrame payload")
	}
	err := json.Unmarshal(b[6:6+payloadLength], &f.Parameters)
	if err != nil {
		return b, err
	}
	return b[6+payloadLength:], nil
}
