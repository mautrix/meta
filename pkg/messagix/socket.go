package messagix

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"
	"go.mau.fi/util/ptr"
	"golang.org/x/net/proxy"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/packets"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

var (
	protocolName     = "MQIsdp"
	protocolClientId = "mqttwsclient"
	protocolLevel    = 3
	keepAliveTimeout = 15

	//lint:ignore U1000 - alternatives for minimal*Sync
	igReconnectSync = []int64{1, 2, 16}
	//lint:ignore U1000 - alternatives for minimal*Sync
	fbReconnectSync = []int64{1, 2, 5, 16, 95, 104}
	//lint:ignore U1000 - alternatives for minimal*Sync
	igInitialSync = []int64{1, 2, 6, 7, 16 /*28,*/, 89, 197, 198}
	//lint:ignore U1000 - alternatives for minimal*Sync
	fbInitialSync = []int64{1, 2 /*5,*/, 16, 26, 28, 89, 95, 104, 120, 140, 141, 142, 143, 145, 196, 197, 198, 202}

	minimalReconnectSync = []int64{1, 2}
	minimalInitialSync   = []int64{1}
	// DB 95 carries message-request threads.
	minimalFBInitialSync   = []int64{1, 95, 104}
	minimalFBReconnectSync = []int64{1, 2, 95, 104}

	shouldRecurseDatabase = map[int64]bool{
		1:   true,
		2:   true,
		95:  true,
		104: true,
	}
)

type Socket struct {
	client          *Client
	conn            *websocket.Conn
	responseHandler *ResponseHandler
	mu              *sync.Mutex
	packetsSent     uint16
	sessionID       int64
	broker          string

	previouslyConnected bool
	cleanClose          atomic.Pointer[func()]
}

func (c *Client) newSocketClient() *Socket {
	return &Socket{
		client: c,
		responseHandler: &ResponseHandler{
			client:          c,
			requestChannels: make(map[uint16]chan any),
			packetChannels:  make(map[uint16]chan any),
		},
		mu:          &sync.Mutex{},
		packetsSent: 0,
		sessionID:   methods.GenerateSessionID(),
	}
}

func (s *Socket) CanConnect() error {
	if s.conn != nil {
		return socket.ErrSocketAlreadyOpen
	} else if !s.client.IsAuthenticated() {
		return socket.ErrNotAuthenticated
	} else if s.broker == "" {
		return fmt.Errorf("broker has not been set in socket struct (broker=%s)", s.broker)
	}
	return nil
}

func (c *Client) GetDialer() *websocket.DialOptions {
	transport := &http.Transport{}
	if c.httpProxy != nil {
		transport.Proxy = c.httpProxy
	} else if c.socksProxy != nil {
		if contextDialer, ok := c.socksProxy.(proxy.ContextDialer); ok {
			transport.DialContext = contextDialer.DialContext
		} else {
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return c.socksProxy.Dial(network, addr)
			}
		}
	}
	if DisableTLSVerification {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}
	return &websocket.DialOptions{
		HTTPClient: &http.Client{
			Timeout:   HandshakeTimeout,
			Transport: transport,
		},
	}
}

func (s *Socket) Connect(ctx context.Context) error {
	err := s.CanConnect()
	if err != nil {
		return err
	}

	brokerUrl := s.BuildBrokerURL()

	opts := s.client.GetDialer()
	opts.HTTPHeader = s.getConnHeaders()
	s.client.Logger.Debug().Str("broker", brokerUrl).Msg("Dialing socket")
	conn, _, err := websocket.Dial(ctx, brokerUrl, opts)
	if err != nil {
		return fmt.Errorf("%w: %w", socket.ErrDial, err)
	}
	// Disable read size limit; MQTT messages can exceed the default 32KiB.
	conn.SetReadLimit(-1)

	s.conn = conn
	err = s.sendConnectPacket()
	if err != nil {
		return fmt.Errorf("%w: %w", socket.ErrSendConnect, err)
	}

	err = s.readLoop(ctx, conn)
	s.responseHandler.CancelAllRequests()
	if err != nil {
		return fmt.Errorf("%w: %w", socket.ErrInReadLoop, err)
	}

	return nil
}

func (s *Socket) BuildBrokerURL() string {
	query := &url.Values{}
	query.Add("sid", strconv.FormatInt(s.sessionID, 10))
	query.Add("cid", s.client.configs.BrowserConfigTable.MqttWebDeviceID.ClientID)

	encodedQuery := query.Encode()
	if strings.HasSuffix(s.broker, "?") {
		return s.broker + encodedQuery
	} else {
		return s.broker + "&" + encodedQuery
	}
}

var (
	PongTimeout      = 30 * time.Second
	PacketTimeout    = 30 * time.Second
	PingInterval     = 10 * time.Second
	HandshakeTimeout = 20 * time.Second
	WriteTimeout     = 20 * time.Second
)

func (s *Socket) Disconnect() {
	if s == nil {
		return
	}
	if fn := s.cleanClose.Load(); fn != nil {
		(*fn)()
	}
	if s.conn != nil {
		_ = s.conn.Close(websocket.StatusNormalClosure, "")
	}
}

func (s *Socket) readLoop(ctx context.Context, conn *websocket.Conn) error {
	defer func() {
		s.conn = nil
	}()
	var closedCleanly atomic.Bool
	var closeErr atomic.Pointer[error]
	s.cleanClose.Store(ptr.Ptr(func() {
		closeErr.CompareAndSwap(nil, ptr.Ptr(fmt.Errorf("closed cleanly")))
		closedCleanly.Store(true)
	}))
	pongTimeoutTimer := time.NewTimer(PongTimeout)
	defer pongTimeoutTimer.Stop()
	wsQueue := make(chan any, 32)
	closeDueToError := func(reason string) {
		err := conn.CloseNow()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			s.client.Logger.Debug().Err(err).Msg("Error closing connection after " + reason)
		}
	}
	var loopWg sync.WaitGroup
	loopWg.Add(1)
	defer func() {
		close(wsQueue)
		loopWg.Wait()
	}()
	go func() {
		defer loopWg.Done()
		for item := range wsQueue {
			if ctx.Err() != nil {
				return
			}
			switch evt := item.(type) {
			case *Event_PublishResponse:
				s.handlePublishResponseEvent(ctx, evt, true)
			case *Event_Ready:
				err := s.handleReadyEvent(ctx, evt)
				if err != nil {
					closeErr.CompareAndSwap(nil, ptr.Ptr(fmt.Errorf("failed to handle connect ack: %w", err)))
					s.client.Logger.Err(err).Msg("Failed to handle connect ack")
					closeDueToError("connect ack failed")
				}
			default:
				panic(fmt.Errorf("invalid type %T in websocket item queue", item))
			}
		}
	}()
	handleBinaryMessage := func(data []byte) {
		resp := &Response{}
		err := resp.Read(data)
		if err != nil {
			closeErr.CompareAndSwap(nil, ptr.Ptr(fmt.Errorf("failed to parse websocket data: %w", err)))
			s.client.Logger.Err(err).Uint8("packet_type", resp.PacketType()).Msg("Failed to parse websocket data")
			closeDueToError("failed to parse websocket data")
			return
		}
		switch evt := resp.ResponseData.(type) {
		case *Event_PingResp:
			s.client.Logger.Trace().Msg("Got ping response")
		case *Event_PublishResponse:
			evt.QoS = resp.QOS()
			if s.handlePublishResponseEvent(ctx, evt, false) {
				select {
				case wsQueue <- evt:
				default:
					s.client.Logger.Warn().Msg("Websocket queue is full")
					wsQueue <- evt
				}
			}
		case *Event_PublishACK, *Event_SubscribeACK:
			s.handleACKEvent(evt.(AckEvent))
		case *Event_Ready:
			if evt.ConnectionCode != CONNECTION_ACCEPTED {
				closeErr.Store(ptr.Ptr(fmt.Errorf("connection refused: %w", evt.ConnectionCode)))
				s.client.Logger.Err(evt.ConnectionCode).Msg("Connection refused")
				closeDueToError("connection refused")
				return
			}
			select {
			case wsQueue <- evt:
			default:
				s.client.Logger.Warn().Msg("Websocket queue is full")
				wsQueue <- evt
			}
		default:
			s.client.Logger.Warn().Any("data", data).Msg("Unexpected data in websocket")
		}
	}
	go func() {
		ticker := time.NewTicker(PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if closeErr.Load() != nil {
					return
				}
				err := s.sendData([]byte{packets.PINGREQ << 4, 0})
				if err != nil {
					closeErr.CompareAndSwap(nil, ptr.Ptr(fmt.Errorf("failed to send ping: %w", err)))
					s.client.Logger.Err(err).Msg("Error sending ping")
					closeDueToError("ping failed")
					return
				}
			case <-pongTimeoutTimer.C:
				closeErr.CompareAndSwap(nil, ptr.Ptr(fmt.Errorf("pong timeout")))
				s.client.Logger.Error().Msg("Pong timeout")
				closeDueToError("pong timeout")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	zerolog.Ctx(ctx).Debug().Msg("Connection established, starting read loop")
	for {
		messageType, p, err := conn.Read(ctx)
		if err != nil {
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				closeErr.CompareAndSwap(nil, ptr.Ptr(fmt.Errorf("closed by server: %d %s", ce.Code, ce.Reason)))
				closedCleanly.Store(true)
				s.client.Logger.Info().Int("code", int(ce.Code)).Str("text", ce.Reason).Msg("Websocket closed by server")
			} else {
				closeErr.CompareAndSwap(nil, ptr.Ptr(fmt.Errorf("failed to read message: %w", err)))
				if !closedCleanly.Load() {
					s.client.Logger.Err(err).Msg("Error reading message from socket")
					closeDueToError("reading message failed")
				}
			}
			// Hacky sleep to give the ready handler time to run and set the best available error
			time.Sleep(100 * time.Millisecond)
			return *closeErr.Load()
		}
		// Any inbound traffic counts as proof-of-life; reset the pong timeout.
		pongTimeoutTimer.Reset(PongTimeout)

		switch messageType {
		case websocket.MessageText:
			s.client.Logger.Warn().Bytes("bytes", p).Msg("Unexpected text message in websocket")
		case websocket.MessageBinary:
			handleBinaryMessage(p)
		}
	}
}

func (s *Socket) sendData(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn := s.conn
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), WriteTimeout)
	defer cancel()
	err := conn.Write(ctx, websocket.MessageBinary, data)
	if exhttp.IsNetworkError(err) {
		closeErr := conn.CloseNow()
		if closeErr != nil && !errors.Is(err, net.ErrClosed) {
			s.client.Logger.Debug().Err(closeErr).Msg("Error closing connection after network error")
			return errors.Join(err, closeErr)
		}
		return err
	} else if err != nil {
		return fmt.Errorf("failed to write to websocket: %w", err)
	}
	return nil
}

func (s *Socket) SafePacketID() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.packetsSent++
	if s.packetsSent == 0 {
		s.packetsSent = 1
	}
	return s.packetsSent
}

func (s *Socket) sendConnectPacket() error {
	connectAdditionalData, err := s.newConnectJSON()
	if err != nil {
		return err
	}

	connectFlags := packets.CreateConnectFlagByte(packets.ConnectFlags{CleanSession: true, Username: true})
	connectPayload, err := newConnectRequest(connectAdditionalData, connectFlags)
	if err != nil {
		return err
	}
	return s.sendData(connectPayload)
}

func (s *Socket) sendSubscribePacket(ctx context.Context, topic Topic, qos packets.QoS, wait bool) (*Event_SubscribeACK, error) {
	subscribeRequestPayload, packetId, err := s.client.newSubscribeRequest(topic, qos)
	if err != nil {
		return nil, err
	}

	err = s.sendData(subscribeRequestPayload)
	if err != nil {
		return nil, err
	}

	var resp *Event_SubscribeACK
	if wait {
		resp, err = s.responseHandler.waitForSubACKDetails(ctx, packetId)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("did not receive SubACK packet for packetid: %d", packetId)
		}
	}

	return resp, nil
}

func (s *Socket) sendPublishPacket(ctx context.Context, topic Topic, jsonData string, packet *packets.PublishPacket, packetId uint16) (uint16, error) {
	publishRequestPayload, packetId, err := s.client.newPublishRequest(topic, jsonData, packet.Compress(), packetId)
	if err != nil {
		return packetId, err
	}

	err = s.sendData(publishRequestPayload)
	if err != nil {
		s.responseHandler.deleteDetails(packetId, PacketChannel)
		s.responseHandler.deleteDetails(packetId, RequestChannel)
		return packetId, err
	}
	_, err = s.responseHandler.waitForPubACKDetails(ctx, packetId)
	if err != nil {
		s.responseHandler.deleteDetails(packetId, RequestChannel)
		return packetId, err
	}
	return packetId, nil
}

type SocketLSRequestPayload struct {
	AppId     string `json:"app_id"`
	Payload   string `json:"payload"`
	RequestId int    `json:"request_id"`
	Type      int    `json:"type"`
}

func (s *Socket) makeLSRequest(ctx context.Context, payload []byte, t int) (*Event_PublishResponse, error) {
	packetId := s.SafePacketID()
	lsPayload := &SocketLSRequestPayload{
		AppId:     s.client.configs.BrowserConfigTable.CurrentUserInitialData.AppID,
		Payload:   string(payload),
		RequestId: int(packetId),
		Type:      t,
	}

	jsonPayload, err := json.Marshal(lsPayload)
	if err != nil {
		return nil, err
	}

	_, err = s.sendPublishPacket(ctx, LS_REQ, string(jsonPayload), &packets.PublishPacket{QOSLevel: packets.QOS_LEVEL_1}, packetId)
	if err != nil {
		return nil, err
	}

	// Request type 4 is for requests that aren't expected to
	// receive a response.
	if t == 4 {
		return nil, nil
	}
	return s.responseHandler.waitForPubResponseDetails(ctx, packetId)
}

func (s *Socket) getConnHeaders() http.Header {
	h := http.Header{}

	h.Set("cookie", s.client.cookies.String())
	h.Set("user-agent", useragent.UserAgent)
	h.Set("origin", s.client.GetEndpoint("base_url"))
	//h.Set("Sec-Fetch-Dest", "empty")
	//h.Set("Sec-Fetch-Mode", "websocket")
	//h.Set("Sec-Fetch-Site", "same-site")

	return h
}

func (s *Socket) getConnectionType() string {
	if s.client.Platform.IsInstagram() {
		return "cookie_auth"
	}
	return "websocket"
}
