package messagix

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.mau.fi/util/exsync"

	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

// Business Suite still delivers Page Lightspeed traffic over Meta's MQTT 3.1
// websocket. Personal Messenger moved to DGW; Pages have not. Keeping this
// transport isolated prevents Page semantics from leaking into the DGW client.
type businessSuiteSocket struct {
	client     *Client
	conn       atomic.Pointer[websocket.Conn]
	writeLock  sync.Mutex
	nextPacket atomic.Uint32
	waiters    *exsync.Map[int64, chan *PublishResponseData]
	connAck    *exsync.Event
	packetAcks *exsync.Map[uint16, chan byte]
}

func newBusinessSuiteSocket(client *Client) *businessSuiteSocket {
	return &businessSuiteSocket{
		client:     client,
		waiters:    exsync.NewMap[int64, chan *PublishResponseData](),
		connAck:    exsync.NewEvent(),
		packetAcks: exsync.NewMap[uint16, chan byte](),
	}
}

func (s *businessSuiteSocket) isConnected() bool { return s != nil && s.conn.Load() != nil }

func (s *businessSuiteSocket) packetID() uint16 {
	id := uint16(s.nextPacket.Add(1))
	if id == 0 {
		id = uint16(s.nextPacket.Add(1))
	}
	return id
}

func appendMQTTString(dst []byte, value string) []byte {
	dst = binary.BigEndian.AppendUint16(dst, uint16(len(value)))
	return append(dst, value...)
}

func appendRemainingLength(dst []byte, value int) []byte {
	for {
		encoded := byte(value % 128)
		value /= 128
		if value > 0 {
			encoded |= 128
		}
		dst = append(dst, encoded)
		if value == 0 {
			return dst
		}
	}
}

func mqttPacket(header byte, body []byte) []byte {
	out := appendRemainingLength([]byte{header}, len(body))
	return append(out, body...)
}

func (s *businessSuiteSocket) write(ctx context.Context, packet []byte) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	conn := s.conn.Load()
	if conn == nil {
		return errors.New("Business Suite socket is closed")
	}
	writeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageBinary, packet)
}

func (s *businessSuiteSocket) connectPacket() ([]byte, error) {
	config := s.client.configs.BrowserConfigTable.MqttWebConfig
	deviceID := s.client.configs.BrowserConfigTable.MqttWebDeviceID.ClientID
	username, err := json.Marshal(map[string]any{
		"u": s.client.configs.BrowserConfigTable.CurrentUserInitialData.AccountID,
		"s": time.Now().UnixNano(), "cp": config.ClientCapabilities,
		"ecp": config.Capabilities, "chat_on": config.ChatVisibility,
		"fg": false, "d": deviceID, "ct": "websocket", "mqtt_sid": "",
		"aid": config.AppID, "st": config.SubscribedTopics, "pm": []any{},
		"dc": "", "no_auto_fg": true, "gas": nil, "pack": []any{},
		"php_override": config.HostNameOverride, "p": nil,
		"a": useragent.UserAgent, "aids": nil,
	})
	if err != nil {
		return nil, err
	}
	body := appendMQTTString(nil, "MQIsdp")
	body = append(body, 3, 0x82)
	body = binary.BigEndian.AppendUint16(body, 15)
	body = appendMQTTString(body, "mqttwsclient")
	body = appendMQTTString(body, string(username))
	return mqttPacket(0x10, body), nil
}

// isTrustedMQTTBrokerHost restricts the server-supplied MQTT broker endpoint
// to Meta-owned hosts. The endpoint is read out of live page config, and the
// dial in Connect attaches the full session cookie jar, so an unvalidated
// host here would let a tampered config value exfiltrate the user's session.
func isTrustedMQTTBrokerHost(host string) bool {
	host = strings.ToLower(host)
	return host == "facebook.com" || strings.HasSuffix(host, ".facebook.com")
}

func (s *businessSuiteSocket) brokerURL() (string, error) {
	raw := s.client.configs.BrowserConfigTable.MqttWebConfig.Endpoint
	if raw == "" {
		return "", errors.New("Business Suite MQTT endpoint is missing")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "wss" {
		return "", fmt.Errorf("refusing to dial Business Suite MQTT endpoint with scheme %q", u.Scheme)
	}
	if !isTrustedMQTTBrokerHost(u.Hostname()) {
		return "", fmt.Errorf("refusing to dial untrusted Business Suite MQTT host %q", u.Hostname())
	}
	q := u.Query()
	q.Set("sid", strconv.FormatInt(time.Now().UnixNano(), 10))
	q.Set("cid", s.client.configs.BrowserConfigTable.MqttWebDeviceID.ClientID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *businessSuiteSocket) Connect(ctx context.Context) error {
	broker, err := s.brokerURL()
	if err != nil {
		return err
	}
	dialOpts := *s.client.http.GetWebsocketDialer()
	dialOpts.HTTPHeader = http.Header{
		"cookie": {s.client.cookies.String()}, "user-agent": {useragent.UserAgent},
		"origin":         {s.client.GetEndpoint("base_url")},
		"sec-fetch-dest": {"empty"}, "sec-fetch-mode": {"websocket"},
		"sec-fetch-site": {"same-site"},
	}
	conn, resp, err := websocket.Dial(ctx, broker, &dialOpts)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("Business Suite MQTT dial failed (%d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("Business Suite MQTT dial failed: %w", err)
	}
	conn.SetReadLimit(-1)
	s.conn.Store(conn)
	defer s.conn.Store(nil)
	s.connAck.Clear()
	errCh := make(chan error, 1)
	go func() { errCh <- s.readLoop(ctx, conn) }()
	packet, err := s.connectPacket()
	if err == nil {
		err = s.write(ctx, packet)
	}
	if err != nil {
		_ = conn.CloseNow()
		return err
	}
	select {
	case <-s.connAck.GetChan():
	case err = <-errCh:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("Business Suite MQTT connection acknowledgement timed out")
	case <-ctx.Done():
		return ctx.Err()
	}
	if err = s.subscribe(ctx, "/ls_foreground_state"); err != nil {
		return err
	}
	if err = s.subscribe(ctx, "/ls_resp"); err != nil {
		return err
	}
	settings, _ := json.Marshal(map[string]string{"ls_fdid": "", "ls_sv": strconv.FormatInt(s.client.configs.VersionID, 10)})
	if err = s.publish(ctx, "/ls_app_settings", settings, 0); err != nil {
		return err
	}
	if err = s.client.onSocketConnect(ctx); err != nil {
		return err
	}
	return <-errCh
}

func (s *businessSuiteSocket) Disconnect() {
	if conn := s.conn.Load(); conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

func (s *businessSuiteSocket) ForceReconnect() {
	if conn := s.conn.Load(); conn != nil {
		_ = conn.CloseNow()
	}
}

func (s *businessSuiteSocket) subscribe(ctx context.Context, topic string) error {
	id := s.packetID()
	body := binary.BigEndian.AppendUint16(nil, id)
	body = appendMQTTString(body, topic)
	body = append(body, 0)
	return s.writeAndWaitAck(ctx, mqttPacket(0x82, body), id, 9)
}

func (s *businessSuiteSocket) publish(ctx context.Context, topic string, payload []byte, requestID int64) error {
	id := s.packetID()
	body := appendMQTTString(nil, topic)
	body = binary.BigEndian.AppendUint16(body, id)
	body = append(body, payload...)
	if err := s.writeAndWaitAck(ctx, mqttPacket(0x32, body), id, 4); err != nil {
		return err
	}
	return nil
}

func (s *businessSuiteSocket) writeAndWaitAck(ctx context.Context, packet []byte, id uint16, packetType byte) error {
	ack := make(chan byte, 1)
	s.packetAcks.Set(id, ack)
	defer s.packetAcks.Delete(id)
	if err := s.write(ctx, packet); err != nil {
		return err
	}
	select {
	case got := <-ack:
		if got != packetType {
			return fmt.Errorf("unexpected MQTT acknowledgement %d", got)
		}
		return nil
	case <-time.After(15 * time.Second):
		return errors.New("Business Suite MQTT acknowledgement timed out")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *businessSuiteSocket) request(ctx context.Context, payload []byte, requestID int64) (*PublishResponseData, error) {
	waiter := make(chan *PublishResponseData, 1)
	s.waiters.Set(requestID, waiter)
	defer s.waiters.Delete(requestID)
	if err := s.publish(ctx, "/ls_req", payload, requestID); err != nil {
		return nil, err
	}
	select {
	case response := <-waiter:
		return response, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("Business Suite Lightspeed response timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func decodeRemainingLength(packet []byte) (value, offset int, err error) {
	multiplier := 1
	for i := 1; i < len(packet) && i <= 4; i++ {
		value += int(packet[i]&127) * multiplier
		if packet[i]&128 == 0 {
			return value, i + 1, nil
		}
		multiplier *= 128
	}
	return 0, 0, errors.New("invalid MQTT remaining length")
}

func (s *businessSuiteSocket) readLoop(ctx context.Context, conn *websocket.Conn) error {
	ping := time.NewTicker(10 * time.Second)
	defer ping.Stop()
	go func() {
		for {
			select {
			case <-ping.C:
				_ = s.write(ctx, []byte{0xC0, 0})
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		messageType, packet, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if messageType != websocket.MessageBinary || len(packet) < 2 {
			continue
		}
		packetType := packet[0] >> 4
		_, offset, err := decodeRemainingLength(packet)
		if err != nil || offset > len(packet) {
			return err
		}
		switch packetType {
		case 2:
			if len(packet) < offset+2 || packet[offset+1] != 0 {
				return errors.New("Business Suite MQTT connection rejected")
			}
			s.connAck.Set()
		case 4, 9:
			if len(packet) >= offset+2 {
				id := binary.BigEndian.Uint16(packet[offset : offset+2])
				if ch, ok := s.packetAcks.Pop(id); ok {
					ch <- packetType
				}
			}
		case 3:
			if err := s.handlePublish(ctx, packet, offset); err != nil {
				return err
			}
		case 13:
			// PINGRESP
		default:
			s.client.Logger.Debug().Int("packet_type", int(packetType)).Msg("Ignoring unsupported Business Suite MQTT packet")
		}
	}
}

func (s *businessSuiteSocket) handlePublish(ctx context.Context, packet []byte, offset int) error {
	if len(packet) < offset+2 {
		return errors.New("short MQTT publish")
	}
	topicLength := int(binary.BigEndian.Uint16(packet[offset : offset+2]))
	start := offset + 2
	if len(packet) < start+topicLength {
		return errors.New("short MQTT topic")
	}
	topic := string(packet[start : start+topicLength])
	start += topicLength
	qos := (packet[0] >> 1) & 3
	var messageID uint16
	if qos > 0 {
		if len(packet) < start+2 {
			return errors.New("short MQTT message ID")
		}
		messageID = binary.BigEndian.Uint16(packet[start : start+2])
		start += 2
	}
	if qos == 1 {
		_ = s.write(ctx, mqttPacket(0x40, binary.BigEndian.AppendUint16(nil, messageID)))
	}
	if topic != "/ls_resp" {
		return nil
	}
	var response PublishResponseData
	if err := json.Unmarshal(packet[start:], &response); err != nil {
		return fmt.Errorf("invalid Business Suite Lightspeed response: %w", err)
	}
	if waiter, ok := s.waiters.Pop(response.RequestID); ok {
		waiter <- &response
		return nil
	}
	table, err := response.Parse(ctx)
	if err != nil {
		return err
	}
	s.client.HandleEvent(ctx, table)
	return nil
}
