package messagix

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type RealtimeSocket struct {
	client *Client
	conn   *websocket.Conn

	cleanClose atomic.Pointer[func()]
}

func (c *Client) newRealtimeSocketClient() *RealtimeSocket {
	return &RealtimeSocket{
		client: c,
	}
}

func (s *RealtimeSocket) CanConnect() error {
	if s.conn != nil {
		return ErrSocketAlreadyOpen
	} else if !s.client.IsAuthenticated() {
		return ErrNotAuthenticated
	}
	return nil
}

func (s *RealtimeSocket) Connect(ctx context.Context) error {
	s.client.Logger.Error().Msg("rrosborough: connecting realtime socket")
	dialer := s.client.getDialer()
	headers := s.getConnHeaders()
	socketURL := s.BuildSocketURL()
	conn, _, err := dialer.DialContext(ctx, socketURL, headers)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDial, err)
	}
	s.conn = conn

	err = s.readLoop(ctx, conn)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInReadLoop, err)
	}

	return nil
}

func (s *RealtimeSocket) readLoop(ctx context.Context, conn *websocket.Conn) error {
	defer func() {
		s.conn = nil
	}()
	var closedCleanly atomic.Bool
	var closeErr atomic.Pointer[error]
	s.cleanClose.Store(ptr(func() {
		closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("closed cleanly")))
		closedCleanly.Store(true)
	}))
	conn.SetCloseHandler(func(code int, text string) error {
		closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("closed by server: %d %s", code, text)))
		closedCleanly.Store(true)
		s.client.Logger.Info().Int("code", code).Str("text", text).Msg("Websocket closed by server")
		return nil
	})
	pongTimeoutTicker := time.NewTicker(pongTimeout)
	defer pongTimeoutTicker.Stop()
	closeDueToError := func(reason string) {
		err := conn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			s.client.Logger.Debug().Err(err).Msg("Error closing connection after " + reason)
		}
	}
	zerolog.Ctx(ctx).Debug().Msg("Realtime connection established, starting read loop")
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("failed to read message: %w", err)))
			if !closedCleanly.Load() {
				s.client.Logger.Err(err).Msg("Error reading message from socket")
				closeDueToError("reading message failed")
			}
			// Hacky sleep to give the ready handler time to run and set the best available error
			time.Sleep(100 * time.Millisecond)
			return *closeErr.Load()
		}

		switch messageType {
		case websocket.TextMessage:
			s.client.Logger.Warn().Bytes("bytes", p).Msg("Unexpected text message in websocket")
		case websocket.BinaryMessage:
			s.client.Logger.Error().Msgf("rrosborough: Received realtime message: %s", string(p))
		}
	}
}

func (s *RealtimeSocket) getConnHeaders() http.Header {
	// reuse headers from other socket for now
	return s.client.socket.getConnHeaders()
}

func (s *RealtimeSocket) BuildSocketURL() string {
	query := &url.Values{}
	query.Add("x-dgw-appid", "936619743392459")
	query.Add("x-dgw-appversion", "0")
	query.Add("x-dgw-authtype", "6:0")
	query.Add("x-dgw-version", "5")
	query.Add("x-dgw-uuid", "0")
	query.Add("x-dgw-tier", "prod")
	query.Add("x-dgw-deviceid", "8364d51e-7d91-4902-84cb-a26d7bf87dfd")
	query.Add("x-dgw-app-stream-group", "group1")

	encodedQuery := query.Encode()
	return "wss://gateway.instagram.com?" + encodedQuery
}

func (s *RealtimeSocket) Disconnect() {
	if fn := s.cleanClose.Load(); fn != nil {
		(*fn)()
	}
	if s.conn != nil {
		_ = s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(3*time.Second))
		_ = s.conn.Close()
	}
}
