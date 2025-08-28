package dgw

import (
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
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

// import cycle
type MessagixClient interface {
	IsAuthenticated() bool
	GetDialer() *websocket.Dialer
	GetLogger() *zerolog.Logger
	GetCookies() *cookies.Cookies
	GetEndpoint(string) string
}

type Socket struct {
	client MessagixClient
	conn   *websocket.Conn
	err    atomic.Pointer[error]

	ackSubscription atomic.Bool
	ackParameters   atomic.Bool

	activity *sync.Cond
}

func NewSocketClient(c MessagixClient) *Socket {
	return &Socket{
		client:   c,
		activity: &sync.Cond{L: &sync.Mutex{}},
	}
}

func (s *Socket) CanConnect() error {
	if s.conn != nil {
		return fmt.Errorf("DGW: %w", socket.ErrSocketAlreadyOpen)
	} else if !s.client.IsAuthenticated() {
		return fmt.Errorf("DGW: %w", socket.ErrNotAuthenticated)
	}
	return nil
}

func (s *Socket) Connect(ctx context.Context) (err error) {
	dialer := s.client.GetDialer()
	headers := s.getConnHeaders()
	socketURL := s.getConnURL()

	conn, resp, err := dialer.DialContext(ctx, socketURL, headers)
	if err != nil {
		return fmt.Errorf("DGW: %w: %w (status code %d)", socket.ErrDial, err, resp.StatusCode)
	}
	s.conn = conn

	err = s.readLoop(ctx, conn)
	if err != nil {
		return fmt.Errorf("DGW: %w: %w", socket.ErrInReadLoop, err)
	}

	return nil
}

const AckTimeout = 5 * time.Second
const PingInterval = 15 * time.Second
const PongTimeout = 30 * time.Second // from web client

func (s *Socket) readLoop(ctx context.Context, conn *websocket.Conn) error {

	done := atomic.Bool{}
	markDone := func() {
		done.Store(true)
		s.activity.Broadcast()
	}
	waitDone := func() <-chan struct{} {
		ch := make(chan struct{})
		go func() {
			s.activity.L.Lock()
			for !done.Load() {
				s.activity.Wait()
			}
			s.activity.L.Unlock()
			ch <- struct{}{}
			close(ch)
		}()
		return ch
	}

	// Setup a function to call if we get a fatal error, that will
	// close the connection and store the error so that it can be
	// returned from the main loop.

	fatalError := func(err error) {
		s.err.Store(&err)
		markDone()
		closeErr := conn.Close()
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			s.client.GetLogger().Debug().Err(closeErr).Msg("Error closing DGW connection after " + err.Error())
		}
	}

	// Setup channels for inbound and outbound frames, so that
	// they can be processed in-order and from a single callsite.

	incoming := make(chan Frame)
	outgoing := make(chan Frame)

	// Copy inbound frames from the websocket to the channel.

	go func() {
		defer close(incoming)
		for {
			msgtype, data, err := conn.ReadMessage()
			if err != nil {
				fatalError(fmt.Errorf("reading message: %w", err))
				return
			}
			for len(data) > 0 {
				switch msgtype {
				case websocket.TextMessage:
					s.client.GetLogger().Warn().Bytes("bytes", data).Msg("Unexpected non-binary DGW message, dropping")
				case websocket.BinaryMessage:
					frame := CheckFrameType(data)
					data, err = frame.Unmarshal(data)
					if err != nil {
						s.client.GetLogger().Warn().Bytes("bytes", data).Err(err).Msg("Failed to unmarshal DGW frame, dropping")
						continue
					}
					incoming <- frame
				}
			}
		}
	}()

	// Copy outbound frames from the channel to the websocket.

	go func() {
		waitDoneCh := waitDone()
		for {
			select {
			case <-waitDoneCh:
				return
			case frame := <-outgoing:
				b, err := frame.Marshal()
				if err != nil {
					s.client.GetLogger().Warn().Any("frame", frame).Msg("Failed to marshal outbound frame, dropping")
					continue
				}
				err = conn.WriteMessage(websocket.BinaryMessage, b)
				if err != nil {
					fatalError(fmt.Errorf("writing message: %w", err))
				}
			}
		}
	}()

	// Setup a timer that will shut us down eventually if we don't
	// receive regular pongs from the server. When receiving pongs
	// we will reset the ticker.

	pongTimeoutTimer := time.NewTimer(PongTimeout)
	defer pongTimeoutTimer.Stop()

	go func() {
		waitDoneCh := waitDone()
		for {
			select {
			case <-pongTimeoutTimer.C:
				fatalError(fmt.Errorf("pong timeout"))
				return
			case <-waitDoneCh:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Process incoming frames from the channel and take any
	// needed action based on their contents.

	go func() {
		for frame := range incoming {
			switch f := frame.(type) {
			case *UnsupportedFrame:
				if f.MayHaveLostData() {
					s.client.GetLogger().Warn().Bytes("bytes", f.Raw).Msg("Encountered unknown unsupported frame, dropping with potential data loss")
				}
			case *PongFrame:
				pongTimeoutTimer.Reset(PongTimeout)
			case *OpenFrame:
				if f.StreamID != Stream_TYPING {
					s.client.GetLogger().Warn().Any("frame", f).Msg("Encountered EstabStream response for wrong stream, dropping")
					continue
				}
				if f.Parameters.StatusCode == 0 {
					s.client.GetLogger().Warn().Bytes("bytes", f.Raw).Msg("Encountered EstabStream response without status code, dropping")
					continue
				}
				if f.Parameters.StatusCode != 200 {
					fatalError(fmt.Errorf("error %d from EstabStream response", f.Parameters.StatusCode))
					return
				}
				s.ackSubscription.Store(true)
				s.activity.Broadcast()
			case *DataFrame:
				if f.RequiresAck {
					outgoing <- &AckFrame{
						StreamID: f.StreamID,
						AckID:    f.AckID,
					}
				}
			case *AckFrame:
				if f.StreamID != Stream_TYPING {
					s.client.GetLogger().Warn().Any("frame", f).Msg("Encountered ack for wrong stream, dropping")
					continue
				}
				if f.AckID != 0 {
					s.client.GetLogger().Warn().Any("frame", f).Msg("Encountered ack with unknown ack id, dropping")
					continue
				}
				s.ackParameters.Store(true)
				s.activity.Broadcast()
			}
		}
	}()

	// Set up a ping timer.

	pingTicker := time.NewTicker(PingInterval)
	defer pingTicker.Stop()
	go func() {
		for range pingTicker.C {
			outgoing <- &PingFrame{}
		}
	}()

	s.client.GetLogger().Debug().Msg("Initiating DGW socket handshake")

	// Create a function to help with timing out waiting on ack
	// frames if no response comes.

	getAckTimeout := func() *atomic.Bool {
		ackTimeout := atomic.Bool{}
		time.AfterFunc(AckTimeout, func() {
			ackTimeout.Store(true)
			s.activity.Broadcast()
		})
		return &ackTimeout
	}

	// Send a frame to subscribe to typing indicators (1 of 2)

	outgoing <- s.getTypingIndicatorSubscriptionFrame()

	ackTimeout := getAckTimeout()
	s.activity.L.Lock()
	for !s.ackSubscription.Load() && !ackTimeout.Load() {
		s.activity.Wait()
	}
	s.activity.L.Unlock()
	if ackTimeout.Load() {
		return fmt.Errorf("didn't get ack for first subscription frame")
	}

	// Send a frame to subscribe to typing indicators (2 of 2)

	frame, err := s.getTypingIndicatorParametersFrame()
	if err != nil {
		return err
	}
	outgoing <- frame

	ackTimeout = getAckTimeout()
	s.activity.L.Lock()
	for !s.ackParameters.Load() && !ackTimeout.Load() {
		s.activity.Wait()
	}
	s.activity.L.Unlock()
	if ackTimeout.Load() {
		return fmt.Errorf("didn't get ack for second subscription frame")
	}

	s.client.GetLogger().Debug().Msg("DGW socket handshake is completed")

	// Wait until done.
	<-waitDone()

	// Return the error, if any, saved from another goroutine.
	return *s.err.Load()
}

func (s *Socket) getConnHeaders() http.Header {
	h := http.Header{}

	h.Set("cookie", s.client.GetCookies().String())
	h.Set("user-agent", useragent.UserAgent)
	h.Set("origin", s.client.GetEndpoint("base_url"))

	return h
}

func (s *Socket) getConnURL() string {
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

func (s *Socket) Disconnect() {
	if s.conn != nil {
		_ = s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(3*time.Second))
		_ = s.conn.Close()
	}
}

type TypingParametersPayload struct {
	InputData TypingParametersInputData `json:"input_data"`
	Options   TypingParametersOptions   `json:"%options"`
}

type TypingParametersInputData struct {
	UserID string `json:"user_id"`
}

type TypingParametersOptions struct {
	UseOSSResponseFormat        bool `json:"useOSSResponseFormat"`
	ClientHasODSUsecaseCounters bool `json:"client_has_ods_usecase_counters"`
}

func (s *Socket) getTypingIndicatorSubscriptionFrame() Frame {
	return &OpenFrame{
		StreamID: Stream_TYPING,
		Parameters: OpenFrameParams{
			XRSSMethod:      "FBGQLS:XDT_DIRECT_REALTIME_EVENT",
			XRSSDocID:       "9712315318850438", // ???
			XRSSRoutingHint: "useIGDTypingIndicatorSubscription",
			XRSBody:         "true",
			XRSAcceptAck:    "RSAck",
			XRSSReferer:     "https://www.instagram.com/direct/",
		},
	}
}

func (s *Socket) getTypingIndicatorParametersFrame() (Frame, error) {
	userID := s.client.GetCookies().Get(cookies.IGCookieDSUserID)
	payload := TypingParametersPayload{
		InputData: TypingParametersInputData{
			UserID: userID,
		},
		Options: TypingParametersOptions{
			UseOSSResponseFormat:        true,
			ClientHasODSUsecaseCounters: true,
		},
	}
	jsonPayload, err := json.Marshal(&payload)
	if err != nil {
		return nil, err
	}
	b := append([]byte{0x2c, 0x18, 0x78}, jsonPayload...)
	b = append(b, 0x00, 0x00)
	return &DataFrame{
		StreamID:    Stream_TYPING,
		Payload:     b,
		RequiresAck: true,
		AckID:       0,
	}, nil
}
