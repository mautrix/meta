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

func (s *RealtimeSocket) handleMessage(data []byte) error {
	msgStart := bytes.Index(data, []byte("{"))
	msgEnd := bytes.LastIndex(data, []byte("}"))
	if msgStart < 0 || msgEnd < 0 {
		return nil
	}
	data = data[msgStart : msgEnd+1]
	s.client.Logger.Error().Msgf("rrosborough: Received realtime message: %s", data)
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

	err := s.sendTypingIndicatorSubscriptionPacket()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSendTypingSubscription, err)
	}

	err = s.waitForEstabStreamAck()
	if err != nil {
		return fmt.Errorf("waiting for typing indicator subscription packet ack: %w", err)
	}

	err = s.sendTypingIndicatorParamsPacket()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSendTypingSubscription, err)
	}

	err = s.waitForDataAck()
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

type RealtimeEstabStreamFrame struct {
	XRSSMethod      string `json:"x-dgw-app-XRSS-method,omitempty"`
	XRSSDocID       string `json:"x-dgw-app-XRSS-doc_id,omitempty"`
	XRSSRoutingHint string `json:"x-dgw-app-XRSS-routing_hint,omitempty"`
	XRSBody         string `json:"x-dgw-app-xrs-body,omitempty"`
	XRSAcceptAck    string `json:"x-dgw-app-XRS-Accept-Ack,omitempty"`
	XRSSReferer     string `json:"x-dgw-app-XRSS-http_referer,omitempty"`
}

type RealtimeDataFrame struct {
	InputData RealtimeDataFrameInputData `json:"input_data"`
	Options   RealtimeDataFrameOptions   `json:"%options"`
}

type RealtimeDataFrameInputData struct {
	UserID string `json:"user_id"`
}

type RealtimeDataFrameOptions struct {
	UseOSSResponseFormat        bool `json:"useOSSResponseFormat"`
	ClientHasODSUsecaseCounters bool `json:"client_has_ods_usecase_counters"`
}

type RealtimeEstabStreamResponse struct {
	Code int `json:"code"`
}

func (s *RealtimeSocket) sendTypingIndicatorSubscriptionPacket() error {
	payload := RealtimeEstabStreamFrame{
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
	return s.sendData(append([]byte{
		// Byte 1 is message type (0x0f = ESTAB_STREAM)
		0x0f,
		// Bytes 2-3 are stream index, little endian
		0x01, 0x00,
		// Bytes 4-5 are encoded params length, little endian
		0x2a, 0x01,
		// Byte 6 has unknown function
		0x00,
	}, jsonData...))
}

func (s *RealtimeSocket) sendTypingIndicatorParamsPacket() error {
	userID := s.client.configs.BrowserConfigTable.CurrentUserInitialData.UserID
	payload := RealtimeDataFrame{
		InputData: RealtimeDataFrameInputData{
			UserID: userID,
		},
		Options: RealtimeDataFrameOptions{
			UseOSSResponseFormat:        true,
			ClientHasODSUsecaseCounters: true,
		},
	}
	jsonData, err := json.Marshal(&payload)
	if err != nil {
		return err
	}
	encodedParamsLength := 0x74 + len(userID)
	return s.sendData(append(append([]byte{
		// Byte 1 is message type (0x0d = DATA)
		0x0d,
		// Bytes 2-3 are stream index, little endian
		0x01, 0x00,
		// Bytes 4-5 are encoded params length, little endian
		byte(encodedParamsLength), 0x00,
		// Bytes 6-7 have unknown function
		0x00, 0x00,
		// Bytes 8-11 have unknown function
		0x80, 0x2c, 0x18, 0x78,
	}, jsonData...), 0x00, 0x00))
}

func (s *RealtimeSocket) waitForEstabStreamAck() error {
	for {
		data, err, _ := s.recvData()
		if err != nil {
			return err
		}
		msgStart := bytes.Index(data, []byte("{"))
		msgEnd := bytes.LastIndex(data, []byte("}"))
		if msgStart < 0 || msgEnd < 0 {
			continue
		}
		data = data[msgStart : msgEnd+1]
		response := RealtimeEstabStreamResponse{}
		err = json.Unmarshal(data, &response)
		if err != nil {
			return err
		}
		switch response.Code {
		case 0:
			s.client.Logger.Warn().Bytes("bytes", data).Msg("Unexpected message while waiting for estabstream frame ack")
		case 200:
			return nil
		default:
			return fmt.Errorf("got code %d from estabstream request", response.Code)
		}
	}
}

func (s *RealtimeSocket) waitForDataAck() error {
	for {
		ackData, err, _ := s.recvData()
		if err != nil {
			return err
		}
		if ackData[0] == 0x0c {
			return nil
		}
		s.client.Logger.Warn().Bytes("bytes", ackData).Msg("Unexpected message while waiting for data frame ack")
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
			return data, nil, false
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
	err := conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		return fmt.Errorf("failed to write to websocket: %w", err)
	}
	return nil
}
