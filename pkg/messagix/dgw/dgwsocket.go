package dgw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"

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
	HandleEvent(context.Context, any)
}

type Socket struct {
	client MessagixClient
	conn   *websocket.Conn
	err    atomic.Pointer[error]
}

func NewSocketClient(c MessagixClient) *Socket {
	return &Socket{
		client: c,
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
		statusCode := 999
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return fmt.Errorf("DGW: %w: %w (status code %d)", socket.ErrDial, err, statusCode)
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

type XDTData struct {
	Data struct {
		Event struct {
			EventType string `json:"event"`
			Data      []struct {
				Operation string `json:"op"`
				Path      string `json:"path"`
				Value     string `json:"value"`
			} `json:"data"`
		} `json:"xdt_direct_realtime_event"`
	} `json:"data"`
}

type ActivityIndicator struct {
	TimestampNano  int64 `json:"timestamp"`
	SenderID       int64 `json:"sender_id"`
	TTL            int   `json:"ttl"`
	ActivityStatus int   `json:"activity_status"`
	Attribution    any   `json:"attribution"` // always null?
}

type DGWEvent struct {
	Event any
}

type DGWTypingActivityIndicator struct {
	InstagramThreadID string
	InstagramUserID   int64
	IsTyping          bool
	Timestamp         time.Time
}

var activityIndicatorPathRegexp = regexp.MustCompile(`^/direct_v2/threads/([0-9]+)/activity_indicator_id/([0-9a-f-]+)$`)

func (s *Socket) readLoop(ctx context.Context, conn *websocket.Conn) error {
	done := make(chan struct{})
	var errorOnce sync.Once
	var wg sync.WaitGroup

	// Setup a function to call if we get a fatal error, that will
	// close the connection and store the error so that it can be
	// returned from the main loop. It's wrapped in Once so only
	// the first error is stored.

	fatalError := func(err error) {
		errorOnce.Do(func() {
			s.err.Store(&err)
			close(done)
			closeErr := conn.Close()
			if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				s.client.GetLogger().Debug().Err(closeErr).Msg("Error closing DGW connection after " + err.Error())
			}
		})
	}

	// Setup channels for inbound and outbound frames, so that
	// they can be processed in-order and from a single callsite.

	incoming := make(chan Frame)
	outgoing := make(chan Frame)

	// Copy inbound frames from the websocket to the channel.

	wg.Add(1)
	go func() {
		defer wg.Done()
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-pongTimeoutTimer.C:
				fatalError(fmt.Errorf("pong timeout"))
				return
			case <-done:
				return
			}
		}
	}()

	// Process incoming frames from the channel and take any
	// needed action based on their contents.

	ackSubscription := exsync.NewEvent()
	ackParameters := exsync.NewEvent()

	wg.Add(1)
	go func() {
		defer wg.Done()
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
					continue
				}
				ackSubscription.Set()
			case *DataFrame:
				if f.RequiresAck {
					outgoing <- &AckFrame{
						StreamID: f.StreamID,
						AckID:    f.AckID,
					}
				}
				if bytes.Contains(f.Payload, []byte("activity_indicator_id")) {
					msgStart := bytes.IndexByte(f.Payload, '{')
					msgEnd := bytes.LastIndexByte(f.Payload, '}') + 1
					var data XDTData
					err := json.Unmarshal(f.Payload[msgStart:msgEnd], &data)
					if err != nil {
						s.client.GetLogger().Warn().Err(err).Bytes("json", f.Payload[msgStart:msgEnd]).Msg("Failed to parse XDT event JSON, dropping")
						continue
					}
					for _, event := range data.Data.Event.Data {
						var ind ActivityIndicator
						err = json.Unmarshal([]byte(event.Value), &ind)
						if err != nil {
							s.client.GetLogger().Warn().Err(err).Str("json", event.Value).Msg("Failed to parse XDT event op JSON, dropping")
							continue
						}
						m := activityIndicatorPathRegexp.FindStringSubmatch(event.Path)
						if len(m) == 0 {
							s.client.GetLogger().Warn().Err(err).Str("json", event.Value).Msg("Failed to parse XDT event op path, dropping")
							continue
						}
						threadID := m[1]
						s.client.HandleEvent(ctx, &DGWEvent{
							Event: DGWTypingActivityIndicator{
								InstagramThreadID: threadID,
								InstagramUserID:   ind.SenderID,
								IsTyping:          ind.ActivityStatus > 0,
								Timestamp:         time.UnixMicro(ind.TimestampNano / 1000),
							},
						})
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
				ackParameters.Set()
			}
		}
	}()

	// Set up a ping timer.

	wg.Add(1)
	go func() {
		pingTicker := time.NewTicker(PingInterval)
		defer wg.Done()
		defer pingTicker.Stop()
		for {
			select {
			case <-pingTicker.C:
				outgoing <- &PingFrame{}
			case <-done:
				return
			}
		}
	}()

	s.client.GetLogger().Debug().Msg("Initiating DGW socket handshake")

	// Send a frame to subscribe to typing indicators (1 of 2)

	outgoing <- s.getTypingIndicatorSubscriptionFrame()
	err := ackSubscription.WaitTimeoutCtx(ctx, AckTimeout)
	if err != nil {
		err = fmt.Errorf("didn't get ack for first subscription frame: %w", err)
		fatalError(err)
		wg.Wait()
		return err
	}

	// Send a frame to subscribe to typing indicators (2 of 2)

	frame, err := s.getTypingIndicatorParametersFrame()
	if err != nil {
		return err
	}
	outgoing <- frame
	err = ackParameters.WaitTimeoutCtx(ctx, AckTimeout)
	if err != nil {
		err = fmt.Errorf("didn't get ack for second subscription frame: %w", err)
		fatalError(err)
		wg.Wait()
		return err
	}

	s.client.GetLogger().Debug().Msg("DGW socket handshake is completed")

	select {
	case <-done:
	case <-ctx.Done():
		fatalError(ctx.Err())
	}

	wg.Wait()

	s.client.GetLogger().Debug().Msg("DGW socket closed")

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
	if s != nil && s.conn != nil {
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
