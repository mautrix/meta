package messagix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

var ErrSendTypingSubscription = errors.New("failed to send typing indicator subscription packet")

type RealtimeSocket struct {
	client *Client
	conn   *websocket.Conn
	mu     *sync.Mutex

	cleanClose atomic.Pointer[func()]
}

func (c *Client) newRealtimeSocketClient() *RealtimeSocket {
	return &RealtimeSocket{
		client: c,
		mu:     &sync.Mutex{},
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

func (s *RealtimeSocket) Connect(ctx context.Context) (err error) {
	dialer := s.client.getDialer()
	headers := s.getConnHeaders()
	socketURL := s.BuildSocketURL()

	conn, resp, err := dialer.DialContext(ctx, socketURL, headers)
	if err != nil {
		return fmt.Errorf("%w: %w (status code %d)", ErrDial, err, resp.StatusCode)
	}
	s.conn = conn

	err = s.readLoop(ctx, conn)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInReadLoop, err)
	}

	return nil
}

func (s *RealtimeSocket) handleMessage(p []byte) error {
	enc := base64.StdEncoding.EncodeToString(p)
	s.client.Logger.Error().Msgf("rrosborough: Received realtime message: %+v", enc)
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

	err := s.sendConnectPacket()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSendConnect, err)
	}

	err = s.waitForAck()
	if err != nil {
		return fmt.Errorf("waiting for connect packet ack: %w", err)
	}

	err = s.sendTypingIndicatorSubscriptionPacket()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSendTypingSubscription, err)
	}

	err = s.waitForAck()
	if err != nil {
		return fmt.Errorf("waiting for typing indicator subscription packet ack: %w", err)
	}

	zerolog.Ctx(ctx).Debug().Msg("Realtime connection established, starting read loop")
	for {
		p, err, failedRead := s.recvData()
		if failedRead {
			closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("failed to read message: %w", err)))
			if !closedCleanly.Load() {
				s.client.Logger.Err(err).Msg("Error reading message from socket")
				closeDueToError("reading message failed")
			}
			// Hacky sleep to give the ready handler time to run and set the best available error
			time.Sleep(100 * time.Millisecond)
			return *closeErr.Load()
		}
		if err != nil {
			s.client.Logger.Warn().Bytes("bytes", p).Msg("Failed to parse realtime websocket message")
		}

		err = s.handleMessage(p)
		if err != nil {
			s.client.Logger.Warn().Bytes("bytes", p).Msg("Failed to handle realtime websocket message")
		}
	}
}

func (s *RealtimeSocket) getConnHeaders() http.Header {
	// reuse headers from other socket for now
	return s.client.socket.getConnHeaders()
}

func (s *RealtimeSocket) BuildSocketURL() string {
	// The web client sends a new UUID every time I reload the
	// page, and that UUID does not appear anywhere else in the
	// HAR that I checked, so guessing it may be okay to generate
	// randomly here?
	deviceID := uuid.Must(uuid.NewRandom())

	query := &url.Values{}
	query.Add("x-dgw-appid", "936619743392459") // static-ish?
	query.Add("x-dgw-appversion", "0")
	query.Add("x-dgw-authtype", "6:0")
	query.Add("x-dgw-version", "5")
	query.Add("x-dgw-uuid", "0")
	query.Add("x-dgw-tier", "prod")
	query.Add("x-dgw-deviceid", deviceID.String())
	query.Add("x-dgw-app-stream-group", "group1")

	encodedQuery := query.Encode()
	return "wss://gateway.instagram.com/ws/realtime?" + encodedQuery
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

type RealtimeRequest struct {
	XRSSMethod      string `json:"x-dgw-app-XRSS-method"`
	XRSSDocID       string `json:"x-dgw-XRSS-doc_id"`
	XRSSRoutingHint string `json:"x-dgw-XRSS-routing_hint"`
	XRSBody         string `json:"x-dgw-app-xrs-body"`
	XRSAcceptAck    string `json:"x-dgw-app-XRS-Accept-Ack"`
	XRSSReferer     string `json:"x-dgw-app-XRSS-http_referer"`
}

type RealtimeResponse struct {
	Code int `json:"code"`
}

func (s *RealtimeSocket) sendConnectPacket() error {
	payload := RealtimeRequest{
		XRSSMethod:   "Falco",
		XRSBody:      "true",
		XRSAcceptAck: "RSAck",
		XRSSReferer:  "https://www.instagram.com/direct/",
	}
	jsonData, err := json.Marshal(&payload)
	if err != nil {
		return err
	}
	// The binary header is generated by magic but is always the
	// same for each given message
	return s.sendData(append([]byte{0x0f, 0x00, 0x00, 0xa2, 0x00, 0x00}, jsonData...))
}

func (s *RealtimeSocket) sendTypingIndicatorSubscriptionPacket() error {
	payload := RealtimeRequest{
		XRSSMethod:      "FBGQLS:XDT_DIRECT_REALTIME_EVENT",
		XRSSDocID:       "9712315318850438", // ???
		XRSSRoutingHint: "useIGDTypingIndicatorSubscription",
		XRSBody:         "true",
		XRSAcceptAck:    "RSAck",
		XRSSReferer:     "https://www.instagram.com/direct/",
	}
	jsonData, err := json.Marshal(&payload)
	if err != nil {
		return err
	}
	return s.sendData(append([]byte{0x0f, 0x01, 0x00, 0x3e, 0x01, 0x00}, jsonData...))
}

func (s *RealtimeSocket) waitForAck() error {
	for {
		jsonData, err, _ := s.recvData()
		if err != nil {
			return err
		}
		response := RealtimeResponse{}
		err = json.Unmarshal(jsonData, &response)
		if err != nil {
			return err
		}
		switch response.Code {
		case 0:
			s.client.Logger.Warn().Bytes("bytes", jsonData).Msg("Unexpected JSON while waiting for ack")
		case 200:
			return nil
		default:
			return fmt.Errorf("got code %d from realtime socket request", response.Code)
		}
	}
}

func (s *RealtimeSocket) recvData() ([]byte, error, bool) {
	conn := s.conn
	if conn == nil {
		return nil, fmt.Errorf("not connected"), false
	}

	for {
		msgtype, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read from websocket: %w", err), true
		}
		switch msgtype {
		case websocket.TextMessage:
			s.client.Logger.Warn().Bytes("bytes", data).Msg("Unexpected text message in websocket")
		case websocket.BinaryMessage:
			if len(data) == 1 && data[0] == '\n' {
				continue
			}
			msgStart := bytes.Index(data, []byte("{"))
			msgEnd := bytes.LastIndex(data, []byte("}"))
			if msgStart < 0 || msgEnd < 0 {
				s.client.Logger.Warn().Bytes("bytes", data).Msg("Failed to parse JSON from realtime websocket message")
				continue
			}
			return data[msgStart : msgEnd+1], nil, false
		}
	}
}

func (s *RealtimeSocket) sendData(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn := s.conn
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	err := conn.WriteMessage(websocket.BinaryMessage, append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to websocket: %w", err)
	}
	return nil
}
