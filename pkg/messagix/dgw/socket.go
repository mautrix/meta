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
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"

	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type SocketOptions struct {
	GetCookies     func() string
	OnConnect      func(context.Context) error
	Origin         string
	WSURL          string
	DialOpts       websocket.DialOptions
	Log            zerolog.Logger
	Facebook       bool
	LoggingID      bool
	AppStreamGroup string
	AppID          string
	UserID         string
	DeviceID       string
}

type Socket struct {
	SocketOptions
	conn         atomic.Pointer[websocket.Conn]
	err          atomic.Pointer[error]
	nextStreamID atomic.Uint64
	streams      *exsync.Map[StreamID, Stream]
	stopped      *exsync.Event
	stopping     atomic.Bool
	fatalError   atomic.Pointer[func(error)]
}

func NewSocket(opts SocketOptions) *Socket {
	if opts.DeviceID == "" {
		opts.DeviceID = uuid.NewString()
	}
	s := &Socket{
		SocketOptions: opts,
		stopped:       exsync.NewEvent(),
		streams:       exsync.NewMap[StreamID, Stream](),
	}
	s.stopped.Set()
	return s
}

func (s *Socket) rawNextStreamID() (streamID StreamID, overflow bool) {
	placeholderID := s.nextStreamID.Add(1) - 1
	if placeholderID > MaxStreamID {
		overflow = true
		streamID = StreamID(placeholderID % MaxStreamID)
	} else {
		streamID = StreamID(placeholderID)
	}
	return
}

func (s *Socket) claimNextStreamID() (StreamID, error) {
	for i := 0; i <= MaxStreamID; i++ {
		streamID, overflow := s.rawNextStreamID()
		if !overflow || !s.streams.Has(streamID) {
			return streamID, nil
		}
	}
	// This should generally never happen, there shouldn't be more than a few persistent streams
	return 0, errExhaustedStreamIDs
}

func (s *Socket) sendFatalError(err error) {
	if fatalErrorPtr := s.fatalError.Load(); fatalErrorPtr != nil {
		(*fatalErrorPtr)(err)
	}
}

type StreamInit struct {
	Parameters   json.RawMessage
	InitPayload  []byte
	LogName      string
	FrameHandler FrameHandler
}

func (s *Socket) DoOneOffStream(ctx context.Context, payload []byte) ([]byte, error) {
	conn := s.conn.Load()
	if conn == nil {
		return nil, ErrSocketNotOpen
	}
	streamID, err := s.claimNextStreamID()
	if err != nil {
		s.sendFatalError(err)
		return nil, err
	}
	oos := newOneOffStream(conn, streamID, &s.Log)
	_, replaced := s.streams.Swap(streamID, oos)
	if replaced {
		err = fmt.Errorf("dgw: stream ID collision on %d", streamID)
		s.sendFatalError(err)
		return nil, err
	}
	resp, err := oos.Do(ctx, nil, payload)
	if err != nil {
		if !s.stopping.Load() {
			go func() {
				_ = writeFrames(ctx, conn, &EndOfDataFrame{StreamID: streamID})
				s.streams.Delete(streamID)
			}()
		}
		return nil, err
	}
	return resp, nil
}

func (s *Socket) EstablishStream(ctx context.Context, init StreamInit) (stream *PersistentStream, err error) {
	conn := s.conn.Load()
	if conn == nil {
		return nil, ErrSocketNotOpen
	}
	defer func() {
		if err != nil {
			s.sendFatalError(err)
		}
	}()
	var streamID StreamID
	streamID, err = s.claimNextStreamID()
	if err != nil {
		return
	}
	logWith := s.Log.With().Uint16("stream_id", uint16(streamID))
	if init.LogName != "" {
		logWith = logWith.Str("stream_name", init.LogName)
	}
	stream = newStream(conn, streamID, init.FrameHandler, logWith.Logger())
	_, replaced := s.streams.Swap(streamID, stream)
	if replaced {
		// This should never happen in practice, since it'd require 65535 simultaneous stream creations
		err = fmt.Errorf("dgw: stream ID collision on %d", streamID)
	} else if err = stream.establish(ctx, init.Parameters, init.InitPayload); err != nil {
		err = fmt.Errorf("dgw: failed to establish stream %d: %w", streamID, err)
	}
	return
}

var errExhaustedStreamIDs = errors.New("dgw: exhausted stream IDs")
var ErrSocketNotOpen = errors.New("dgw: socket is not open")
var ErrSocketAlreadyOpen = errors.New("dgw: socket is already open")
var ErrDial = errors.New("dgw: failed to dial socket")
var ErrPongTimeout = errors.New("dgw: pong timeout")

type wrappedDataFrame struct {
	s Stream
	f *DataFrame
}

func (s *Socket) Connect(ctx context.Context) (err error) {
	if s.conn.Load() != nil {
		return ErrSocketAlreadyOpen
	} else if s.stopping.Load() {
		return nil
	}
	s.DialOpts.HTTPHeader = s.getConnHeaders()

	conn, resp, err := websocket.Dial(ctx, s.getConnURL(), &s.DialOpts)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("%w: %w (status code %d)", ErrDial, err, resp.StatusCode)
		}
		return fmt.Errorf("%w: %w", ErrDial, err)
	}
	conn.SetReadLimit(-1)
	s.conn.Store(conn)
	if s.stopping.Load() {
		s.conn.Store(nil)
		_ = conn.CloseNow()
		return nil
	}
	return s.readLoop(ctx, conn)
}

func (s *Socket) ForceReconnect() {
	if s == nil {
		return
	}
	if conn := s.conn.Load(); conn != nil {
		_ = conn.CloseNow()
	}
}

func (s *Socket) Disconnect() {
	if s == nil {
		return
	}
	s.stopping.Store(true)
	if conn := s.conn.Load(); conn != nil {
		// TODO do a graceful dgw drain instead of just closing the websocket?
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	if !s.stopped.WaitTimeout(5 * time.Second) {
		s.Log.Warn().Msg("Timed out waiting for DGW socket to close")
	}
}

const AckTimeout = 5 * time.Second
const PingInterval = 10 * time.Second
const PongTimeout = 30 * time.Second
const WriteTimeout = 20 * time.Second

func (s *Socket) readLoop(ctx context.Context, conn *websocket.Conn) error {
	done := make(chan struct{})
	var errorOnce sync.Once
	var wg sync.WaitGroup

	fatalError := func(err error) {
		errorOnce.Do(func() {
			s.err.Store(&err)
			close(done)
			closeErr := conn.CloseNow()
			if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				s.Log.Debug().
					AnErr("close_error", closeErr).
					AnErr("original_error", err).
					Msg("Error closing DGW connection after error")
			}
		})
	}
	fatalErrorPtr := &fatalError
	s.fatalError.Store(fatalErrorPtr)
	defer s.fatalError.CompareAndSwap(fatalErrorPtr, nil)

	tryWriteFrame := func(frame Frame, task string) {
		err := writeFrames(ctx, conn, frame)
		if err != nil {
			fatalError(fmt.Errorf("dgw: failed to write %s: %w", task, err))
		}
	}

	catchPanics := func(thing string) {
		v := recover()
		if v != nil {
			err, ok := v.(error)
			if !ok {
				err = fmt.Errorf("%v", v)
			}
			zerolog.Ctx(ctx).Err(err).
				Str("loop_name", thing).
				Bytes(zerolog.ErrorStackFieldName, debug.Stack()).
				Msg("Panic in DGW socket loop")
			fatalError(fmt.Errorf("dgw: panic in %s: %w", thing, err))
		}
	}

	pongTimeoutTimer := time.NewTimer(PongTimeout)
	defer pongTimeoutTimer.Stop()

	incoming := make(chan wrappedDataFrame, 64)
	wg.Add(1)
	go func() {
		defer catchPanics("frame handler")
		defer wg.Done()
		for {
			select {
			case frame := <-incoming:
				if frame.s == nil {
					return
				}
				if frame.f != nil {
					err := frame.s.receiveFrame(ctx, frame.f)
					if err != nil {
						fatalError(fmt.Errorf("dgw: frame handler error: %w", err))
						return
					}
				} else {
					frame.s.close()
				}
			case <-done:
				return
			}
		}
	}()

	handleFrame := func(frame Frame) {
		switch f := frame.(type) {
		case *PongFrame:
			pongTimeoutTimer.Reset(PongTimeout)
		case *PingFrame:
			pongTimeoutTimer.Reset(PongTimeout)
			go tryWriteFrame(&PongFrame{}, "pong")
		case *EstablishStreamFrame:
			if stream, ok := s.streams.Get(f.StreamID); !ok {
				s.Log.Warn().Any("frame", f).Msg("Received establish stream response for unknown stream, dropping")
			} else {
				stream.receiveEstablish(f)
			}
		case *DataFrame:
			stream, ok := s.streams.Get(f.StreamID)
			if !ok {
				s.Log.Warn().Any("frame", f).Msg("Received data frame for unknown stream")
				if f.RequiresAck {
					go tryWriteFrame(&AckFrame{
						StreamID: f.StreamID,
						AckID:    f.AckID,
					}, "ack for unknown stream")
				}
			} else {
				incoming <- wrappedDataFrame{
					s: stream,
					f: f,
				}
			}
		case *AckFrame:
			if stream, ok := s.streams.Get(f.StreamID); !ok {
				s.Log.Warn().Any("frame", f).Msg("Encountered ack for unknown stream")
			} else if !stream.receiveAck(f.AckID) {
				s.Log.Warn().Any("frame", f).Msg("Encountered ack for unknown request")
			} else {
				s.Log.Trace().Any("frame", f).Msg("Received ack frame")
			}
		case *EndOfDataFrame:
			if stream, ok := s.streams.Pop(f.StreamID); !ok {
				s.Log.Debug().Uint16("stream_id", uint16(f.StreamID)).Msg("Received end of data frame for unknown stream")
			} else {
				s.Log.Debug().Uint16("stream_id", uint16(f.StreamID)).Msg("Received end of data frame")
				incoming <- wrappedDataFrame{
					s: stream,
				}
				s.streams.Delete(f.StreamID)
			}
		case *DrainFrame:
			s.Log.Debug().Stringer("reason", f.DrainReason).Msg("Received drain frame")
		case *UnsupportedFrame:
			s.Log.Warn().
				Stringer("frame_type", FrameType(f.Raw[0])).
				Hex("bytes", f.Raw).
				Msg("Encountered unknown unsupported frame, dropping with potential data loss")
		default:
			panic(fmt.Errorf("unhandled frame type %T", frame))
		}
	}

	wg.Add(1)
	go func() {
		defer catchPanics("read loop")
		defer wg.Done()
		defer close(incoming)
		for {
			msgtype, data, err := conn.Read(ctx)
			if err != nil {
				fatalError(fmt.Errorf("dgw: reading message: %w", err))
				return
			} else if msgtype != websocket.MessageBinary {
				s.Log.Warn().
					Bytes("bytes", data).
					Int("msg_type", int(msgtype)).
					Msg("Unexpected non-binary DGW message, dropping")
				continue
			}
			for len(data) > 0 {
				frame := CheckFrameType(data)
				data, err = frame.Unmarshal(data)
				if err != nil {
					// TODO allow frames to cross websocket message boundaries?
					s.Log.Warn().Hex("bytes", data).Err(err).Msg("Failed to unmarshal DGW frame, dropping")
					continue
				}
				handleFrame(frame)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer catchPanics("pong timeout loop")
		defer wg.Done()
		for {
			select {
			case <-pongTimeoutTimer.C:
				fatalError(ErrPongTimeout)
				return
			case <-done:
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer catchPanics("ping loop")
		pingTicker := time.NewTicker(PingInterval)
		defer wg.Done()
		defer pingTicker.Stop()
		for {
			select {
			case <-pingTicker.C:
				tryWriteFrame(&PingFrame{}, "ping")
			case <-done:
				return
			}
		}
	}()

	err := s.OnConnect(ctx)
	if err != nil {
		fatalError(fmt.Errorf("dgw: OnConnect error: %w", err))
	} else {
		s.Log.Debug().Msg("DGW socket initialized")
	}

	select {
	case <-done:
	case <-ctx.Done():
		fatalError(ctx.Err())
	}

	wg.Wait()

	s.stopped.Set()
	s.conn.Store(nil)
	s.Log.Debug().Msg("DGW socket closed")

	for _, stream := range s.streams.SwapData(nil) {
		stream.close()
	}
	s.nextStreamID.Store(0)

	if s.stopping.Load() {
		return nil
	}
	return *s.err.Load()
}

func (s *Socket) getConnHeaders() http.Header {
	h := http.Header{}

	h.Set("cookie", s.GetCookies())
	h.Set("user-agent", useragent.UserAgent)
	h.Set("origin", s.Origin)
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "websocket")
	h.Set("sec-fetch-site", "same-site")

	return h
}

func (s *Socket) getConnURL() string {
	query := &url.Values{}
	query.Add("x-dgw-appid", s.AppID)
	query.Add("x-dgw-appversion", "0")
	if s.Facebook {
		query.Add("x-dgw-authtype", "1:0")
		//query.Add("x-dgw-regionhint", "TODO")
	} else {
		query.Add("x-dgw-authtype", "6:0")
	}
	query.Add("x-dgw-version", "5")
	query.Add("x-dgw-uuid", s.UserID)
	query.Add("x-dgw-tier", "prod")
	if s.LoggingID {
		query.Add("x-dgw-loggingid", uuid.NewString())
	}
	query.Add("x-dgw-deviceid", s.DeviceID)
	if s.AppStreamGroup != "" {
		query.Add("x-dgw-app-stream-group", s.AppStreamGroup)
	}

	encodedQuery := query.Encode()
	return s.WSURL + "?" + encodedQuery
}
