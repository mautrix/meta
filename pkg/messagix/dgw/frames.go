// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package dgw

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/coder/websocket"
)

type DrainReason uint8

const (
	DrainReasonELB                DrainReason = 0
	DrainReasonSLB                DrainReason = 1
	DrainReasonAppServerPush      DrainReason = 2
	DrainReasonGracePeriodExpired DrainReason = 3
	DrainReasonUnknown            DrainReason = 4
)

func (dr DrainReason) String() string {
	switch dr {
	case DrainReasonELB:
		return "ELB"
	case DrainReasonSLB:
		return "SLB"
	case DrainReasonAppServerPush:
		return "AppServerPush"
	case DrainReasonGracePeriodExpired:
		return "GracePeriodExpired"
	case DrainReasonUnknown:
		return "Unknown"
	default:
		return fmt.Sprintf("DrainReason(%d)", dr)
	}
}

type FrameType uint8

const (
	// There are some additional frame types in the web client
	// enum, but they are not used in the code.

	FrameTypeDrain       FrameType = 0x03 // Drain
	FrameTypePing        FrameType = 0x09 // Ping
	FrameTypePong        FrameType = 0x0a // Pong
	FrameTypeAck         FrameType = 0x0c // StreamGroup_Ack
	FrameTypeData        FrameType = 0x0d // StreamGroup_Data
	FrameTypeEndOfData   FrameType = 0x0e // StreamGroup_EndOfData
	FrameTypeEstabStream FrameType = 0x0f // StreamGroup_EstabStream
)

func (ft FrameType) String() string {
	switch ft {
	case FrameTypeDrain:
		return "FrameTypeDrain"
	case FrameTypePing:
		return "FrameTypePing"
	case FrameTypePong:
		return "FrameTypePong"
	case FrameTypeAck:
		return "FrameTypeAck"
	case FrameTypeData:
		return "FrameTypeData"
	case FrameTypeEndOfData:
		return "FrameTypeEndOfData"
	case FrameTypeEstabStream:
		return "FrameTypeEstabStream"
	default:
		return fmt.Sprintf("FrameType(%d)", ft)
	}
}

func CheckFrameType(b []byte) Frame {
	switch FrameType(b[0]) {
	case FrameTypeDrain:
		return &DrainFrame{}
	case FrameTypePing:
		return &PingFrame{}
	case FrameTypePong:
		return &PongFrame{}
	case FrameTypeAck:
		return &AckFrame{}
	case FrameTypeData:
		return &DataFrame{}
	case FrameTypeEstabStream:
		return &EstablishStreamFrame{}
	case FrameTypeEndOfData:
		return &EndOfDataFrame{}
	default:
		return &UnsupportedFrame{}
	}
}

const MaxStreamID = 1<<16 - 1

type StreamID uint16

type Frame interface {
	Length() int
	MarshalAppend([]byte) []byte
	Unmarshal([]byte) ([]byte, error)
}

type UnsupportedFrame struct {
	Raw []byte
}

func (f *UnsupportedFrame) Length() int {
	return len(f.Raw)
}

func (f *UnsupportedFrame) MarshalAppend(b []byte) []byte {
	return append(b, f.Raw...)
}

func (f *UnsupportedFrame) Unmarshal(b []byte) ([]byte, error) {
	f.Raw = b
	return nil, nil
}

func (f *UnsupportedFrame) String() string {
	return fmt.Sprintf("UnsupportedFrame{Raw: %s}", base64.RawStdEncoding.EncodeToString(f.Raw))
}

type DrainFrame struct {
	DrainReason DrainReason
}

func (f *DrainFrame) Length() int {
	return 5
}

func (f *DrainFrame) MarshalAppend(b []byte) []byte {
	b = append(b, byte(FrameTypeDrain))
	b = appendUint24LE(b, 1)
	b = append(b, byte(f.DrainReason))
	return b
}

func (f *DrainFrame) Unmarshal(bytes []byte) ([]byte, error) {
	if len(bytes) < 5 {
		return nil, fmt.Errorf("input too short for DrainFrame")
	}
	length := uint24LE(bytes[1:4])
	if length != 1 {
		return nil, fmt.Errorf("unnatural DrainFrame")
	}
	f.DrainReason = DrainReason(bytes[4])
	return bytes[5:], nil
}

func (f *DrainFrame) String() string {
	return fmt.Sprintf("DrainFrame{Reason: %s}", f.DrainReason)
}

type PingFrame struct {
	//
}

func (f *PingFrame) Length() int {
	return 1
}

func (f *PingFrame) MarshalAppend(b []byte) []byte {
	return append(b, byte(FrameTypePing))
}

func (f *PingFrame) Unmarshal(b []byte) ([]byte, error) {
	return b[1:], nil
}

func (f *PingFrame) String() string {
	return "PingFrame{}"
}

type PongFrame struct {
	//
}

func (f *PongFrame) Length() int {
	return 1
}

func (f *PongFrame) MarshalAppend(b []byte) []byte {
	return append(b, byte(FrameTypePong))
}

func (f *PongFrame) Unmarshal(b []byte) ([]byte, error) {
	return b[1:], nil
}

func (f *PongFrame) String() string {
	return "PongFrame{}"
}

type DataFrame struct {
	StreamID    StreamID
	Payload     []byte
	RequiresAck bool
	AckID       uint16
}

func appendUint24LE(b []byte, v uint32) []byte {
	return append(b,
		byte(v),
		byte(v>>8),
		byte(v>>16),
	)
}

func uint24LE(b []byte) uint32 {
	_ = b[2]
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
}

func (f *DataFrame) Length() int {
	return 8 + len(f.Payload)
}

func (f *DataFrame) MarshalAppend(b []byte) []byte {
	b = append(b, byte(FrameTypeData))
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	b = appendUint24LE(b, uint32(len(f.Payload)+2))
	b = binary.LittleEndian.AppendUint16(b, f.AckID)
	b[len(b)-1] &= 0b0111_1111
	if f.RequiresAck {
		b[len(b)-1] |= 0b1000_0000
	}
	b = append(b, f.Payload...)
	return b
}

func (f *DataFrame) Unmarshal(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("input too short for DataFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(b[1:3]))
	payloadLength := uint24LE(b[3:6]) - 2
	f.AckID = binary.LittleEndian.Uint16(b[6:8]) & 0b0111_1111_1111_1111
	f.RequiresAck = b[7]&0b1000_0000 > 0
	if len(b) < int(8+payloadLength) {
		return nil, fmt.Errorf("input too short for DataFrame payload")
	}
	f.Payload = b[8 : 8+payloadLength]
	return b[8+payloadLength:], nil
}

func (f *DataFrame) String() string {
	if utf8.Valid(f.Payload) && bytes.HasPrefix(f.Payload, []byte{'{'}) {
		// Looks like JSON, print as string for convenience
		return fmt.Sprintf("DataFrame{StreamID: %d, AckID: %d, RequiresAck: %t, Payload: %s}", f.StreamID, f.AckID, f.RequiresAck, f.Payload)
	}
	return fmt.Sprintf("DataFrame{StreamID: %d, AckID: %d, RequiresAck: %t, Payload: %s}", f.StreamID, f.AckID, f.RequiresAck, base64.RawStdEncoding.EncodeToString(f.Payload))
}

type AckFrame struct {
	StreamID StreamID
	AckID    uint16
}

func (f *AckFrame) Length() int {
	return 8
}

func (f *AckFrame) MarshalAppend(b []byte) []byte {
	b = append(b, byte(FrameTypeAck))
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	b = appendUint24LE(b, 2)
	b = binary.LittleEndian.AppendUint16(b, f.AckID)
	return b
}

func (f *AckFrame) Unmarshal(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("input too short for AckFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(b[1:3]))
	payloadLength := uint24LE(b[3:6])
	if payloadLength != 2 {
		return nil, fmt.Errorf("unnatural AckFrame")
	}
	f.AckID = binary.LittleEndian.Uint16(b[6:8])
	return b[6+payloadLength:], nil
}

func (f *AckFrame) String() string {
	return fmt.Sprintf("AckFrame{StreamID: %d, AckID: %d}", f.StreamID, f.AckID)
}

type EndOfDataFrame struct {
	StreamID StreamID
}

func (f *EndOfDataFrame) Length() int {
	return 3
}

func (f *EndOfDataFrame) MarshalAppend(b []byte) []byte {
	b = append(b, byte(FrameTypeEndOfData))
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	return b
}

func (f *EndOfDataFrame) Unmarshal(bytes []byte) ([]byte, error) {
	if len(bytes) < 3 {
		return nil, fmt.Errorf("input too short for EndOfDataFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(bytes[1:3]))
	return bytes[3:], nil
}

func (f *EndOfDataFrame) String() string {
	return fmt.Sprintf("EndOfDataFrame{StreamID: %d}", f.StreamID)
}

type EstablishStreamFrame struct {
	StreamID      StreamID
	RawParameters json.RawMessage
}

func (f *EstablishStreamFrame) Length() int {
	return 5 + len(f.RawParameters)
}

func (f *EstablishStreamFrame) MarshalAppend(b []byte) []byte {
	b = append(b, byte(FrameTypeEstabStream))
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	b = appendUint24LE(b, uint32(len(f.RawParameters)))
	b = append(b, f.RawParameters...)
	return b
}

func (f *EstablishStreamFrame) Unmarshal(b []byte) ([]byte, error) {
	if len(b) < 6 {
		return nil, fmt.Errorf("input too short for OpenFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(b[1:3]))
	payloadLength := uint24LE(b[3:6])
	if len(b) < int(6+payloadLength) {
		return nil, fmt.Errorf("input too short for OpenFrame payload")
	}
	f.RawParameters = b[6 : 6+payloadLength]
	if !json.Valid(f.RawParameters) {
		return nil, fmt.Errorf("invalid JSON in OpenFrame parameters")
	}
	return b[6+payloadLength:], nil
}

func (f *EstablishStreamFrame) String() string {
	return fmt.Sprintf("OpenFrame{StreamID: %d, Parameters: %s}", f.StreamID, f.RawParameters)
}

func writeFrames(ctx context.Context, conn *websocket.Conn, frames ...Frame) error {
	var totalLength int
	for _, frame := range frames {
		totalLength += frame.Length()
	}
	b := make([]byte, 0, totalLength)
	for _, frame := range frames {
		b = frame.MarshalAppend(b)
	}
	writeCtx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageBinary, b)
}
