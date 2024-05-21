package messagix

import (
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

	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"

	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/packets"
	"go.mau.fi/mautrix-meta/messagix/types"
)

var (
	protocolName     = "MQIsdp"
	protocolClientId = "mqttwsclient"
	protocolLevel    = 3
	keepAliveTimeout = 15
	connectionTypes  = map[types.Platform]string{
		types.Instagram:   "cookie_auth",
		types.Facebook:    "websocket",
		types.Messenger:   "websocket",
		types.FacebookTor: "websocket",
	}
	ErrSocketClosed      = errors.New("messagix-socket: socket is closed")
	ErrSocketAlreadyOpen = errors.New("messagix-socket: socket is already open")
	ErrNotAuthenticated  = errors.New("messagix-socket: client has not been authenticated successfully yet")

	igReconnectSync = []int64{1, 2, 16}
	fbReconnectSync = []int64{1, 2, 5, 16, 95, 104}
	igInitialSync   = []int64{1, 2, 6, 7, 16, 28, 198}
	fbInitialSync   = []int64{1, 2, 5, 16, 26, 28, 95, 104, 120, 140, 141, 142, 143, 196, 198}

	minimalReconnectSync = []int64{1, 2}
	minimalInitialSync   = []int64{1}

	initialSync = map[types.Platform][]int64{
		types.Instagram:   minimalInitialSync, // igInitialSync,
		types.Facebook:    minimalInitialSync, // fbInitialSync,
		types.Messenger:   minimalInitialSync, // fbInitialSync,
		types.FacebookTor: minimalInitialSync, // fbInitialSync,
	}
	reconnectSync = map[types.Platform][]int64{
		types.Instagram:   minimalReconnectSync, // igReconnectSync,
		types.Facebook:    minimalReconnectSync, // fbReconnectSync,
		types.Messenger:   minimalReconnectSync, // fbReconnectSync,
		types.FacebookTor: minimalReconnectSync, // fbReconnectSync,
	}

	shouldRecurseDatabase = map[int64]bool{
		1:  true,
		2:  true,
		95: true,
	}
)

type Socket struct {
	client          *Client
	conn            *websocket.Conn
	responseHandler *ResponseHandler
	mu              *sync.Mutex
	packetsSent     uint16
	sessionId       int64
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
		sessionId:   methods.GenerateSessionId(),
	}
}

var ErrDial = errors.New("failed to dial socket")
var ErrSendConnect = errors.New("failed to send connect packet")
var ErrInReadLoop = errors.New("error in read loop")

func (s *Socket) CanConnect() error {
	if s.conn != nil {
		return ErrSocketAlreadyOpen
	} else if !s.client.IsAuthenticated() {
		return ErrNotAuthenticated
	} else if s.broker == "" {
		return fmt.Errorf("broker has not been set in socket struct (broker=%s)", s.broker)
	}
	return nil
}

func (s *Socket) Connect() error {
	err := s.CanConnect()
	if err != nil {
		return err
	}

	headers := s.getConnHeaders()
	brokerUrl := s.BuildBrokerUrl()

	dialer := websocket.Dialer{HandshakeTimeout: 20 * time.Second}
	if s.client.httpProxy != nil {
		dialer.Proxy = s.client.httpProxy
	} else if s.client.socksProxy != nil {
		dialer.NetDial = s.client.socksProxy.Dial

		contextDialer, ok := s.client.socksProxy.(proxy.ContextDialer)
		if ok {
			dialer.NetDialContext = contextDialer.DialContext
		}
	}

	s.client.Logger.Debug().Str("broker", brokerUrl).Msg("Dialing socket")
	conn, _, err := dialer.Dial(brokerUrl, headers)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDial, err)
	}

	s.conn = conn
	err = s.sendConnectPacket()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSendConnect, err)
	}

	err = s.readLoop(conn)
	s.responseHandler.CancelAllRequests()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInReadLoop, err)
	}

	return nil
}

func (s *Socket) BuildBrokerUrl() string {
	query := &url.Values{}
	query.Add("cid", s.client.configs.browserConfigTable.MqttWebDeviceID.ClientID)
	query.Add("sid", strconv.FormatInt(s.sessionId, 10))

	encodedQuery := query.Encode()
	if !strings.HasSuffix(s.broker, "=") {
		return s.broker + "&" + encodedQuery
	} else {
		return s.broker + "&" + encodedQuery
	}
}

const pongTimeout = 30 * time.Second
const packetTimeout = 30 * time.Second
const pingInterval = 10 * time.Second

func ptr[T any](val T) *T {
	return &val
}

func (s *Socket) Disconnect() {
	if fn := s.cleanClose.Load(); fn != nil {
		(*fn)()
	}
	if s.conn != nil {
		_ = s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(3*time.Second))
		_ = s.conn.Close()
	}
}

func (s *Socket) readLoop(conn *websocket.Conn) error {
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
	wsMessages := make(chan []byte, 32)
	defer close(wsMessages)
	closeDueToError := func(reason string) {
		err := conn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			s.client.Logger.Debug().Err(err).Msg("Error closing connection after " + reason)
		}
	}
	handleBinaryMessage := func(data []byte) {
		resp := &Response{}
		err := resp.Read(data)
		if err != nil {
			closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("failed to parse websocket data: %w", err)))
			s.client.Logger.Err(err).Uint8("packet_type", resp.PacketType()).Msg("Failed to parse websocket data")
			closeDueToError("failed to parse websocket data")
			return
		}
		switch evt := resp.ResponseData.(type) {
		case *Event_PingResp:
			s.client.Logger.Trace().Msg("Got ping response")
			pongTimeoutTicker.Reset(pongTimeout)
		case *Event_PublishResponse:
			s.handlePublishResponseEvent(evt, resp.QOS())
		case *Event_PublishACK, *Event_SubscribeACK:
			s.handleACKEvent(evt.(AckEvent))
		case *Event_Ready:
			if evt.ConnectionCode != CONNECTION_ACCEPTED {
				closeErr.Store(ptr(fmt.Errorf("connection refused: %w", evt.ConnectionCode)))
				s.client.Logger.Err(evt.ConnectionCode).Msg("Connection refused")
				closeDueToError("connection refused")
				return
			}
			go func() {
				err := s.handleReadyEvent(evt)
				if err != nil {
					closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("failed to handle connect ack: %w", err)))
					s.client.Logger.Err(err).Msg("Failed to handle connect ack")
					closeDueToError("connect ack failed")
				}
			}()
		default:
			s.client.Logger.Warn().Any("data", data).Msg("Unexpected data in websocket")
		}
	}
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case p := <-wsMessages:
				if p == nil {
					return
				}
				handleBinaryMessage(p)
			case <-ticker.C:
				err := s.sendData([]byte{packets.PINGREQ << 4, 0})
				if err != nil {
					closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("failed to send ping: %w", err)))
					s.client.Logger.Err(err).Msg("Error sending ping")
					closeDueToError("ping failed")
					return
				}
			case <-pongTimeoutTicker.C:
				closeErr.CompareAndSwap(nil, ptr(fmt.Errorf("pong timeout")))
				s.client.Logger.Error().Msg("Pong timeout")
				closeDueToError("pong timeout")
				return
			}
		}
	}()
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
			select {
			case wsMessages <- p:
			default:
				s.client.Logger.Warn().Msg("Websocket message channel is full")
				wsMessages <- p
			}
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
	err := conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		return fmt.Errorf("failed to write to websocket: %w", err)
	}
	return nil
}

func (s *Socket) SafePacketId() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.packetsSent++
	if s.packetsSent == 0 || s.packetsSent > 65535 {
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

func (s *Socket) sendSubscribePacket(topic Topic, qos packets.QoS, wait bool) (*Event_SubscribeACK, error) {
	subscribeRequestPayload, packetId, err := s.client.NewSubscribeRequest(topic, qos)
	if err != nil {
		return nil, err
	}

	err = s.sendData(subscribeRequestPayload)
	if err != nil {
		return nil, err
	}

	var resp *Event_SubscribeACK
	if wait {
		resp, err = s.responseHandler.waitForSubACKDetails(packetId)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("did not receive SubACK packet for packetid: %d", packetId)
		}
	}

	return resp, nil
}

func (s *Socket) sendPublishPacket(topic Topic, jsonData string, packet *packets.PublishPacket, packetId uint16) (uint16, error) {
	publishRequestPayload, packetId, err := s.client.NewPublishRequest(topic, jsonData, packet.Compress(), packetId)
	if err != nil {
		return packetId, err
	}

	err = s.sendData(publishRequestPayload)
	if err != nil {
		s.responseHandler.deleteDetails(packetId, PacketChannel)
		s.responseHandler.deleteDetails(packetId, RequestChannel)
		return packetId, err
	}
	_, err = s.responseHandler.waitForPubACKDetails(packetId)
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

func (s *Socket) makeLSRequest(payload []byte, t int) (*Event_PublishResponse, error) {
	packetId := s.SafePacketId()
	lsPayload := &SocketLSRequestPayload{
		AppId:     s.client.configs.browserConfigTable.CurrentUserInitialData.AppID,
		Payload:   string(payload),
		RequestId: int(packetId),
		Type:      t,
	}

	jsonPayload, err := json.Marshal(lsPayload)
	if err != nil {
		return nil, err
	}

	_, err = s.sendPublishPacket(LS_REQ, string(jsonPayload), &packets.PublishPacket{QOSLevel: packets.QOS_LEVEL_1}, packetId)
	if err != nil {
		return nil, err
	}

	return s.responseHandler.waitForPubResponseDetails(packetId)
}

func (s *Socket) getConnHeaders() http.Header {
	h := http.Header{}

	h.Set("cookie", s.client.cookies.String())
	h.Set("user-agent", UserAgent)
	h.Set("origin", s.client.getEndpoint("base_url"))
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "websocket")
	h.Set("Sec-Fetch-Site", "same-site")

	return h
}

func (s *Socket) getConnectionType() string {
	return connectionTypes[s.client.platform]
}
