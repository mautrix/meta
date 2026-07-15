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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"
)

type OneOffStream struct {
	baseStream
	acked    *exsync.Event
	received *exsync.Event
	closed   atomic.Bool
	data     atomic.Pointer[[]byte]
}

func newOneOffStream(conn *websocket.Conn, id StreamID, log *zerolog.Logger) *OneOffStream {
	return &OneOffStream{
		baseStream: newBaseStream(conn, id, log),
		acked:      exsync.NewEvent(),
		received:   exsync.NewEvent(),
	}
}

const oneOffAckID = 0

var ErrReceiveTimeout = errors.New("dgw: oneoffstream: data receive timed out")

func (s *OneOffStream) Do(ctx context.Context, parameters json.RawMessage, initPayload []byte) ([]byte, error) {
	if parameters == nil {
		parameters = json.RawMessage("{}")
	}
	err := writeFrames(ctx, s.conn, &EstablishStreamFrame{
		StreamID:      s.id,
		RawParameters: parameters,
	}, &DataFrame{
		StreamID:    s.id,
		Payload:     initPayload,
		RequiresAck: true,
		AckID:       oneOffAckID,
	})
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(AckTimeout):
		return nil, fmt.Errorf("%w for establish frame", ErrAckTimeout)
	case <-s.established.GetChan():
	}
	if errPtr := s.establishErr.Load(); errPtr != nil {
		return nil, fmt.Errorf("dgw: establish error: %w", *errPtr)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(AckTimeout):
		return nil, fmt.Errorf("%w for init payload", ErrAckTimeout)
	case <-s.acked.GetChan():
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(AckTimeout):
		return nil, ErrReceiveTimeout
	case <-s.received.GetChan():
	}
	data := s.data.Load()
	if data == nil {
		if s.closed.Load() {
			return nil, ErrClosedBeforeTimeout
		}
		return nil, fmt.Errorf("dgw: data is unexpectedly nil")
	}
	return *data, nil
}

func (s *OneOffStream) receiveFrame(ctx context.Context, f *DataFrame) error {
	if s.data.Swap(&f.Payload) == nil {
		s.log.Trace().
			Uint16("stream_id", uint16(f.StreamID)).
			Uint16("frame_ack_id", f.AckID).
			Msg("Received data frame for one-off stream")
		s.received.Set()
	} else {
		s.log.Warn().
			Uint16("stream_id", uint16(f.StreamID)).
			Uint16("frame_ack_id", f.AckID).
			Msg("Received multiple frames in one-off stream")
	}
	go func() {
		if f.RequiresAck {
			_ = writeFrames(ctx, s.conn, &AckFrame{
				StreamID: s.id,
				AckID:    f.AckID,
			})
		}
		_ = writeFrames(ctx, s.conn, &EndOfDataFrame{StreamID: s.id})
	}()
	return nil
}

func (s *OneOffStream) receiveAck(id uint16) bool {
	return id == oneOffAckID && s.acked.Set()
}

func (s *OneOffStream) close() {
	s.log.Trace().Uint16("stream_id", uint16(s.id)).Msg("Close called for one-off stream")
	s.closed.Store(true)
	s.received.Set()
	s.acked.Set()
	s.established.Set()
}
