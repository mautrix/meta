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

const (
	CloseStatusGracefulClose    websocket.StatusCode = 4000
	CloseStatusKeepaliveTimeout websocket.StatusCode = 4001
	CloseStatusDGWServerError   websocket.StatusCode = 4002
	CloseStatusUnauthorized     websocket.StatusCode = 4003
	CloseStatusRejected         websocket.StatusCode = 4004
	CloseStatusBadRequest       websocket.StatusCode = 4005
)

type DrainReason uint8

const (
	DrainReasonELB                      DrainReason = 0
	DrainReasonSLB                      DrainReason = 1
	DrainReasonAppServerPush            DrainReason = 2
	DrainReasonGracePeriodExpired       DrainReason = 3
	DrainReasonUnknown                  DrainReason = 4
	DrainReasonMaxConnectionAgeExceeded DrainReason = 5
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
	case DrainReasonMaxConnectionAgeExceeded:
		return "MaxConnectionAgeExceeded"
	default:
		return fmt.Sprintf("DrainReason(%d)", dr)
	}
}

type EndOfDataReason byte

const (
	EndOfDataReasonUnknown                    EndOfDataReason = 0
	EndOfDataReasonUpstreamTermination        EndOfDataReason = 1
	EndOfDataReasonClientHasFinishedSending   EndOfDataReason = 2
	EndOfDataReasonStreamError                EndOfDataReason = 3
	EndOfDataReasonAuthError                  EndOfDataReason = 4
	EndOfDataReasonParsingError               EndOfDataReason = 5
	EndOfDataReasonClientPubackError          EndOfDataReason = 6
	EndOfDataReasonUpstreamException          EndOfDataReason = 7
	EndOfDataReasonDraining                   EndOfDataReason = 8
	EndOfDataReasonEndpointRegistrationFailed EndOfDataReason = 9
)

func (eodr EndOfDataReason) String() string {
	switch eodr {
	case EndOfDataReasonUnknown:
		return "Unknown"
	case EndOfDataReasonUpstreamTermination:
		return "UpstreamTermination"
	case EndOfDataReasonClientHasFinishedSending:
		return "ClientHasFinishedSending"
	case EndOfDataReasonStreamError:
		return "StreamError"
	case EndOfDataReasonAuthError:
		return "AuthError"
	case EndOfDataReasonParsingError:
		return "ParsingError"
	case EndOfDataReasonClientPubackError:
		return "ClientPubackError"
	case EndOfDataReasonUpstreamException:
		return "UpstreamException"
	case EndOfDataReasonDraining:
		return "Draining"
	case EndOfDataReasonEndpointRegistrationFailed:
		return "EndpointRegistrationFailed"
	default:
		return fmt.Sprintf("EndOfDataReason(%d)", eodr)
	}
}

type FrameType uint8

const (
	FrameTypeEmpty                 FrameType = 2
	FrameTypeDrain                 FrameType = 3
	FrameTypeDeauth                FrameType = 4
	FrameTypeDeprecatedEstabStream FrameType = 5
	FrameTypeDeprecatedData        FrameType = 6
	FrameTypeSmallAck              FrameType = 7
	FrameTypeDeprecatedEndOfData   FrameType = 8
	FrameTypePing                  FrameType = 9
	FrameTypePong                  FrameType = 10
	FrameTypeAck                   FrameType = 12
	FrameTypeData                  FrameType = 13
	FrameTypeEndOfData             FrameType = 14
	FrameTypeEstabStream           FrameType = 15
	FrameTypeEndOfDataWithReason   FrameType = 16
	FrameTypeExtendedData          FrameType = 17
)

func (ft FrameType) String() string {
	switch ft {
	case FrameTypeEmpty:
		return "FrameTypeEmpty"
	case FrameTypeDrain:
		return "FrameTypeDrain"
	case FrameTypeDeauth:
		return "FrameTypeDeauth"
	case FrameTypeDeprecatedEstabStream:
		return "FrameTypeDeprecatedEstabStream"
	case FrameTypeDeprecatedData:
		return "FrameTypeDeprecatedData"
	case FrameTypeSmallAck:
		return "FrameTypeSmallAck"
	case FrameTypeDeprecatedEndOfData:
		return "FrameTypeDeprecatedEndOfData"
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
	case FrameTypeEndOfDataWithReason:
		return "FrameTypeEndOfDataWithReason"
	case FrameTypeExtendedData:
		return "FrameTypeExtendedData"
	default:
		return fmt.Sprintf("FrameType(%d)", ft)
	}
}

func CheckFrameType(b []byte) Frame {
	switch FrameType(b[0]) {
	case FrameTypeDrain:
		return &DrainFrame{}
	case FrameTypeDeauth:
		return &DeauthFrame{}
	case FrameTypePing:
		return &PingFrame{}
	case FrameTypePong:
		return &PongFrame{}
	case FrameTypeAck:
		return &AckFrame{}
	case FrameTypeData, FrameTypeExtendedData:
		return &DataFrame{}
	case FrameTypeEstabStream:
		return &EstablishStreamFrame{}
	case FrameTypeEndOfData:
		return &EndOfDataFrame{}
	case FrameTypeEndOfDataWithReason:
		return &EndOfDataWithReasonFrame{}
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

type StreamFrame interface {
	Frame
	GetStreamID() StreamID
}

type AnyEndOfDataFrame interface {
	StreamFrame
	GetReason() EndOfDataReason
}

func (f *DataFrame) GetStreamID() StreamID                { return f.StreamID }
func (f *AckFrame) GetStreamID() StreamID                 { return f.StreamID }
func (f *EndOfDataFrame) GetStreamID() StreamID           { return f.StreamID }
func (f *EndOfDataWithReasonFrame) GetStreamID() StreamID { return f.StreamID }
func (f *EstablishStreamFrame) GetStreamID() StreamID     { return f.StreamID }

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

type DeauthFrame struct {
	//
}

func (f *DeauthFrame) Length() int {
	return 1
}

func (f *DeauthFrame) MarshalAppend(b []byte) []byte {
	return append(b, byte(FrameTypeDeauth))
}

func (f *DeauthFrame) Unmarshal(b []byte) ([]byte, error) {
	return b[1:], nil
}

func (f *DeauthFrame) String() string {
	return "DeauthFrame{}"
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
	ContentType *byte
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
	if f.ContentType != nil {
		return 9 + len(f.Payload)
	}
	return 8 + len(f.Payload)
}

func (f *DataFrame) MarshalAppend(b []byte) []byte {
	frameType := FrameTypeData
	if f.ContentType != nil {
		frameType = FrameTypeExtendedData
	}
	b = append(b, byte(frameType))
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	b = appendUint24LE(b, uint32(f.Length()-6))
	b = binary.LittleEndian.AppendUint16(b, f.AckID)
	b[len(b)-1] &= 0b0111_1111
	if f.RequiresAck {
		b[len(b)-1] |= 0b1000_0000
	}
	if f.ContentType != nil {
		b = append(b, *f.ContentType)
	}
	b = append(b, f.Payload...)
	return b
}

func (f *DataFrame) Unmarshal(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, fmt.Errorf("input too short for DataFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(b[1:3]))
	length := int(uint24LE(b[3:6]))
	headerSize := 8
	f.ContentType = nil
	if FrameType(b[0]) == FrameTypeExtendedData {
		headerSize = 9
	}
	if length < headerSize-6 || len(b) < 6+length {
		return nil, fmt.Errorf("input too short for DataFrame payload")
	}
	f.AckID = binary.LittleEndian.Uint16(b[6:8]) & 0b0111_1111_1111_1111
	f.RequiresAck = b[7]&0b1000_0000 > 0
	if headerSize == 9 {
		contentType := b[8]
		f.ContentType = &contentType
	}
	f.Payload = b[headerSize : 6+length]
	return b[6+length:], nil
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

func (f *EndOfDataFrame) GetReason() EndOfDataReason {
	return EndOfDataReasonUnknown
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

type EndOfDataWithReasonFrame struct {
	StreamID StreamID
	Reason   EndOfDataReason
}

func (f *EndOfDataWithReasonFrame) GetReason() EndOfDataReason {
	return f.Reason
}

func (f *EndOfDataWithReasonFrame) Length() int {
	return 7
}

func (f *EndOfDataWithReasonFrame) MarshalAppend(b []byte) []byte {
	b = append(b, byte(FrameTypeEndOfDataWithReason))
	b = binary.LittleEndian.AppendUint16(b, uint16(f.StreamID))
	b = appendUint24LE(b, 1)
	b = append(b, byte(f.Reason))
	return b
}

func (f *EndOfDataWithReasonFrame) Unmarshal(bytes []byte) ([]byte, error) {
	if len(bytes) < 7 {
		return nil, fmt.Errorf("input too short for EndOfDataWithReasonFrame")
	}
	f.StreamID = StreamID(binary.LittleEndian.Uint16(bytes[1:3]))
	remainingLength := uint24LE(bytes[3:6])
	if remainingLength < 1 {
		return nil, fmt.Errorf("unnatural EndOfDataWithReasonFrame")
	}
	f.Reason = EndOfDataReason(bytes[6])
	return bytes[6+remainingLength:], nil
}

func (f *EndOfDataWithReasonFrame) String() string {
	return fmt.Sprintf("EndOfDataWithReasonFrame{StreamID: %d, Reason: %d}", f.StreamID, f.Reason)
}

type EstablishStreamFrame struct {
	StreamID      StreamID
	RawParameters json.RawMessage
}

func (f *EstablishStreamFrame) Length() int {
	return 6 + len(f.RawParameters)
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

func writeFrames(ctx context.Context, conn *connection, frames ...Frame) error {
	var totalLength int
	for _, frame := range frames {
		totalLength += frame.Length()
		if data, ok := frame.(*DataFrame); ok && conn.extendedData && data.ContentType == nil {
			totalLength++
		}
	}
	b := make([]byte, 0, totalLength)
	for _, frame := range frames {
		if data, ok := frame.(*DataFrame); ok && conn.extendedData {
			nativeData := *data
			contentType := byte(0)
			nativeData.ContentType = &contentType
			frame = &nativeData
		}
		b = frame.MarshalAppend(b)
	}
	writeCtx, cancel := context.WithTimeout(ctx, WriteTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageBinary, b)
}
