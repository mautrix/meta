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
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"
	"go.mau.fi/util/ptr"
)

type PersistentStream struct {
	baseStream
	lock         sync.Mutex
	closed       *exsync.Event
	nextAckID    uint16
	pendingAcks  map[uint16]chan bool
	frameHandler func([]byte) error
}

func newStream(conn *websocket.Conn, id StreamID, frameHandler func([]byte) error, log zerolog.Logger) *PersistentStream {
	return &PersistentStream{
		baseStream:   newBaseStream(conn, id, &log),
		pendingAcks:  make(map[uint16]chan bool),
		closed:       exsync.NewEvent(),
		frameHandler: frameHandler,
	}
}

var (
	ErrAckTimeout          = errors.New("dgw: ack timed out")
	ErrClosedBeforeTimeout = errors.New("dgw: stream closed before ack")
	ErrAlreadyClosed       = errors.New("dgw: stream already closed")
)

func (s *PersistentStream) establish(ctx context.Context, parameters json.RawMessage, initPayload []byte) error {
	frames := []Frame{&EstablishStreamFrame{
		StreamID:      s.id,
		RawParameters: parameters,
	}}
	var ch <-chan bool
	if initPayload != nil {
		var frame *DataFrame
		frame, ch = s.formDataFrame(initPayload)
		if ch == nil {
			return fmt.Errorf("%w before establish", ErrAlreadyClosed)
		}
		defer s.cancelAck(frame.AckID, ch)
		frames = append(frames, frame)
	}
	err := writeFrames(ctx, s.conn, frames...)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(AckTimeout):
		return fmt.Errorf("%w for establish frame", ErrAckTimeout)
	case <-s.established.GetChan():
	}
	if errPtr := s.establishErr.Load(); errPtr != nil {
		return fmt.Errorf("dgw: establish error: %w", *errPtr)
	}
	if ch != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case acked := <-ch:
			if !acked {
				return fmt.Errorf("%w for init payload", ErrClosedBeforeTimeout)
			}
			return nil
		case <-time.After(AckTimeout):
			return fmt.Errorf("%w for init payload", ErrAckTimeout)
		}
	}
	return nil
}

func (s *PersistentStream) receiveFrame(ctx context.Context, f *DataFrame) error {
	err := s.frameHandler(f.Payload)
	if err != nil {
		s.log.Err(err).Msg("Failed to handle frame")
		return err
	}
	if f.RequiresAck {
		go func() {
			err := writeFrames(ctx, s.conn, &AckFrame{
				StreamID: s.id,
				AckID:    f.AckID,
			})
			if err != nil {
				s.log.Err(err).Uint16("frame_ack_id", f.AckID).Msg("Failed to send ack")
			}
		}()
	}
	return nil
}

func (s *PersistentStream) SendData(ctx context.Context, payload []byte) error {
	frame, ch := s.formDataFrame(payload)
	if ch == nil {
		return ErrAlreadyClosed
	}
	defer s.cancelAck(frame.AckID, ch)
	err := writeFrames(ctx, s.conn, frame)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case acked := <-ch:
		if !acked {
			return ErrClosedBeforeTimeout
		}
		return nil
	case <-time.After(AckTimeout):
		return ErrAckTimeout
	}
}

func (s *PersistentStream) SendDataNoAck(ctx context.Context, payload []byte) error {
	frame := s.formNoAckDataFrame(payload)
	if s.closed.IsSet() {
		return ErrAlreadyClosed
	}
	return writeFrames(ctx, s.conn, frame)
}

func (s *PersistentStream) formNoAckDataFrame(payload []byte) *DataFrame {
	return &DataFrame{
		StreamID: s.id,
		Payload:  payload,
	}
}

func (s *PersistentStream) formDataFrame(payload []byte) (frame *DataFrame, ch <-chan bool) {
	frame = s.formNoAckDataFrame(payload)
	frame.RequiresAck = true
	frame.AckID, ch = s.addAck()
	return frame, ch
}

func (s *PersistentStream) receiveAck(id uint16) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	ch, isPending := s.pendingAcks[id]
	if !isPending {
		return false
	}
	delete(s.pendingAcks, id)
	ch <- true
	close(ch)
	return true
}

const MaxAckID = 0x7fff

func (s *PersistentStream) addAck() (uint16, <-chan bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.closed.IsSet() {
		return 0, nil
	}
	nextID := s.nextAckID
	s.nextAckID++
	if s.nextAckID > MaxAckID {
		s.nextAckID = 0
	}
	ch := make(chan bool, 1)
	s.pendingAcks[nextID] = ch
	return nextID, ch
}

func (s *PersistentStream) cancelAck(id uint16, expectedCh <-chan bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	ch, isPending := s.pendingAcks[id]
	if !isPending || ch != expectedCh {
		return
	}
	delete(s.pendingAcks, id)
	close(ch)
}

func (s *PersistentStream) close() {
	if !s.closed.Set() {
		return
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, ch := range s.pendingAcks {
		ch <- false
		close(ch)
	}
	clear(s.pendingAcks)
	if !s.established.IsSet() {
		s.establishErr.Store(ptr.Ptr(ErrClosedBeforeTimeout))
		s.established.Set()
	}
}
