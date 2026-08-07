// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
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

package instameow

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"time"

	apachethrift "github.com/apache/thrift/lib/go/thrift"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	instathrift "go.mau.fi/mautrix-meta/pkg/instameow/thrift"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

const (
	instagramMQTTAppID              int64 = 567067343352427
	instagramMQTTClientCapabilities int64 = 183
	instagramMQTTKeepAlive                = 60 * time.Second
)

var instagramMQTTBrokers = []string{
	"edge-mqtt.facebook.com:443",
	"mqtt-ig-p4.facebook.com:443",
}

var instagramMQTTTopics = []int32{
	88,  // /pubsub
	133, // /ig_send_message_response
	146, // /ig_message_sync
	149, // /ig_realtime_sub
	279, // /ig_large_scale_fire_and_forget_sync
	283, // /disable_presence_reporting
}

type mqttotClientInfo struct {
	UserID                int64
	UserAgent             []byte
	ClientCapabilities    int64
	EndpointCapabilities  int64
	PublishFormat         int32
	NoAutomaticForeground bool
	MakeUserAvailable     bool
	DeviceID              []byte
	IsInitiallyForeground bool
	NetworkType           int32
	NetworkSubtype        int32
	ClientMQTTSessionID   int64
	SubscribeTopics       []int32
	ClientType            []byte
	AppID                 int64
	OverrideNectarLogging bool
	ConnectTokenHash      []byte
	ClientStack           int8
}

func (*mqttotClientInfo) Read(context.Context, apachethrift.TProtocol) error {
	return errors.New("reading MQTToT client info is not implemented")
}

func (info *mqttotClientInfo) Write(ctx context.Context, protocol apachethrift.TProtocol) error {
	if err := protocol.WriteStructBegin(ctx, "ClientInfo"); err != nil {
		return err
	}
	writeI64 := func(name string, id int16, value int64) error {
		if err := protocol.WriteFieldBegin(ctx, name, apachethrift.I64, id); err != nil {
			return err
		}
		if err := protocol.WriteI64(ctx, value); err != nil {
			return err
		}
		return protocol.WriteFieldEnd(ctx)
	}
	writeI32 := func(name string, id int16, value int32) error {
		if err := protocol.WriteFieldBegin(ctx, name, apachethrift.I32, id); err != nil {
			return err
		}
		if err := protocol.WriteI32(ctx, value); err != nil {
			return err
		}
		return protocol.WriteFieldEnd(ctx)
	}
	writeBool := func(name string, id int16, value bool) error {
		if err := protocol.WriteFieldBegin(ctx, name, apachethrift.BOOL, id); err != nil {
			return err
		}
		if err := protocol.WriteBool(ctx, value); err != nil {
			return err
		}
		return protocol.WriteFieldEnd(ctx)
	}
	writeBinary := func(name string, id int16, value []byte) error {
		if err := protocol.WriteFieldBegin(ctx, name, apachethrift.STRING, id); err != nil {
			return err
		}
		if err := protocol.WriteBinary(ctx, value); err != nil {
			return err
		}
		return protocol.WriteFieldEnd(ctx)
	}

	for _, field := range []struct {
		name  string
		id    int16
		value int64
	}{
		{"userId", 1, info.UserID},
	} {
		if err := writeI64(field.name, field.id, field.value); err != nil {
			return err
		}
	}
	if err := writeBinary("userAgent", 2, info.UserAgent); err != nil {
		return err
	}
	if err := writeI64("clientCapabilities", 3, info.ClientCapabilities); err != nil {
		return err
	}
	if err := writeI64("endpointCapabilities", 4, info.EndpointCapabilities); err != nil {
		return err
	}
	if err := writeI32("publishFormat", 5, info.PublishFormat); err != nil {
		return err
	}
	if err := writeBool("noAutomaticForeground", 6, info.NoAutomaticForeground); err != nil {
		return err
	}
	if err := writeBool("makeUserAvailableInForeground", 7, info.MakeUserAvailable); err != nil {
		return err
	}
	if err := writeBinary("deviceId", 8, info.DeviceID); err != nil {
		return err
	}
	if err := writeBool("isInitiallyForeground", 9, info.IsInitiallyForeground); err != nil {
		return err
	}
	if err := writeI32("networkType", 10, info.NetworkType); err != nil {
		return err
	}
	if err := writeI32("networkSubtype", 11, info.NetworkSubtype); err != nil {
		return err
	}
	if err := writeI64("clientMqttSessionId", 12, info.ClientMQTTSessionID); err != nil {
		return err
	}
	if err := protocol.WriteFieldBegin(ctx, "subscribeTopics", apachethrift.LIST, 14); err != nil {
		return err
	}
	if err := protocol.WriteListBegin(ctx, apachethrift.I32, len(info.SubscribeTopics)); err != nil {
		return err
	}
	for _, topic := range info.SubscribeTopics {
		if err := protocol.WriteI32(ctx, topic); err != nil {
			return err
		}
	}
	if err := protocol.WriteListEnd(ctx); err != nil {
		return err
	}
	if err := protocol.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := writeBinary("clientType", 15, info.ClientType); err != nil {
		return err
	}
	if err := writeI64("appId", 16, info.AppID); err != nil {
		return err
	}
	if err := writeBool("overrideNectarLogging", 17, info.OverrideNectarLogging); err != nil {
		return err
	}
	if len(info.ConnectTokenHash) > 0 {
		if err := writeBinary("connectTokenHash", 18, info.ConnectTokenHash); err != nil {
			return err
		}
	}
	if err := protocol.WriteFieldBegin(ctx, "clientStack", apachethrift.BYTE, 21); err != nil {
		return err
	}
	if err := protocol.WriteByte(ctx, info.ClientStack); err != nil {
		return err
	}
	if err := protocol.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := protocol.WriteFieldStop(ctx); err != nil {
		return err
	}
	return protocol.WriteStructEnd(ctx)
}

type mqttotConnectPayload struct {
	ClientIdentifier []byte
	ClientInfo       *mqttotClientInfo
	Password         []byte
	AppSpecificInfo  map[string]string
}

func (*mqttotConnectPayload) Read(context.Context, apachethrift.TProtocol) error {
	return errors.New("reading MQTToT connect payload is not implemented")
}

func (payload *mqttotConnectPayload) Write(ctx context.Context, protocol apachethrift.TProtocol) error {
	if err := protocol.WriteStructBegin(ctx, "ConnectPayload"); err != nil {
		return err
	}
	if err := protocol.WriteFieldBegin(ctx, "clientIdentifier", apachethrift.STRING, 1); err != nil {
		return err
	}
	if err := protocol.WriteBinary(ctx, payload.ClientIdentifier); err != nil {
		return err
	}
	if err := protocol.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := protocol.WriteFieldBegin(ctx, "clientInfo", apachethrift.STRUCT, 4); err != nil {
		return err
	}
	if err := payload.ClientInfo.Write(ctx, protocol); err != nil {
		return err
	}
	if err := protocol.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := protocol.WriteFieldBegin(ctx, "password", apachethrift.STRING, 5); err != nil {
		return err
	}
	if err := protocol.WriteBinary(ctx, payload.Password); err != nil {
		return err
	}
	if err := protocol.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := protocol.WriteFieldBegin(ctx, "appSpecificInfo", apachethrift.MAP, 10); err != nil {
		return err
	}
	if err := protocol.WriteMapBegin(
		ctx,
		apachethrift.STRING,
		apachethrift.STRING,
		len(payload.AppSpecificInfo),
	); err != nil {
		return err
	}
	keys := make([]string, 0, len(payload.AppSpecificInfo))
	for key := range payload.AppSpecificInfo {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if err := protocol.WriteBinary(ctx, []byte(key)); err != nil {
			return err
		}
		if err := protocol.WriteBinary(ctx, []byte(payload.AppSpecificInfo[key])); err != nil {
			return err
		}
	}
	if err := protocol.WriteMapEnd(ctx); err != nil {
		return err
	}
	if err := protocol.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := protocol.WriteFieldStop(ctx); err != nil {
		return err
	}
	return protocol.WriteStructEnd(ctx)
}

func buildMobileMQTToTConnectPacket(
	session *types.InstagramMobileSession,
	now time.Time,
) ([]byte, error) {
	if session == nil || session.Authorization == "" || session.UserID == "" {
		return nil, errors.New("instagram mobile session is incomplete")
	}
	userID, err := strconv.ParseInt(session.UserID, 10, 64)
	if err != nil || userID <= 0 {
		return nil, errors.New("instagram mobile session has an invalid user ID")
	}
	deviceID := session.Device.DeviceID
	if deviceID == "" {
		return nil, errors.New("instagram mobile session has no installation device ID")
	}
	clientIdentifier := deviceID
	if len(clientIdentifier) > 20 {
		clientIdentifier = clientIdentifier[:20]
	}
	password := "authorization=" + session.Authorization
	connectHash := md5.Sum([]byte(instagramMobileUserAgent + " " + password + " " + deviceID + " "))
	payload := &mqttotConnectPayload{
		ClientIdentifier: []byte(clientIdentifier),
		ClientInfo: &mqttotClientInfo{
			UserID:                userID,
			UserAgent:             []byte(instagramMobileUserAgent),
			ClientCapabilities:    instagramMQTTClientCapabilities,
			EndpointCapabilities:  0,
			PublishFormat:         1,
			NoAutomaticForeground: true,
			MakeUserAvailable:     false,
			DeviceID:              []byte(deviceID),
			IsInitiallyForeground: true,
			NetworkType:           1,
			NetworkSubtype:        0,
			ClientMQTTSessionID:   now.UnixMilli(),
			SubscribeTopics:       instagramMQTTTopics,
			ClientType:            []byte("cookie_auth"),
			AppID:                 instagramMQTTAppID,
			ConnectTokenHash:      connectHash[:],
			ClientStack:           3,
		},
		Password: []byte(password),
		AppSpecificInfo: map[string]string{
			"Accept-Language":           "en-US",
			"User-Agent":                instagramMobileUserAgent,
			"app_version":               instagramMobileAppVersion,
			"capabilities":              "3brTv10=",
			"ig_mqtt_route":             "django",
			"platform":                  "android",
			"pubsub_msg_type_blacklist": "direct, typing_type",
		},
	}
	thriftPayload, err := instathrift.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode Instagram MQTToT connect payload: %w", err)
	}
	var compressed bytes.Buffer
	writer, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Instagram MQTToT compression: %w", err)
	}
	if _, err = writer.Write(thriftPayload); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("failed to compress Instagram MQTToT payload: %w", err)
	}
	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish Instagram MQTToT compression: %w", err)
	}

	var body bytes.Buffer
	_ = binary.Write(&body, binary.BigEndian, uint16(len("MQTToT")))
	body.WriteString("MQTToT")
	body.WriteByte(3)
	body.WriteByte(0xC2)
	_ = binary.Write(&body, binary.BigEndian, uint16(instagramMQTTKeepAlive/time.Second))
	body.Write(compressed.Bytes())

	var packet bytes.Buffer
	packet.WriteByte(0x10)
	if err = writeMQTTRemainingLength(&packet, body.Len()); err != nil {
		return nil, err
	}
	packet.Write(body.Bytes())
	return packet.Bytes(), nil
}

func writeMQTTRemainingLength(writer io.ByteWriter, length int) error {
	if length < 0 || length > 268435455 {
		return errors.New("invalid MQTT remaining length")
	}
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		if err := writer.WriteByte(digit); err != nil {
			return err
		}
		if length == 0 {
			return nil
		}
	}
}

func readMQTTPacket(reader io.Reader) (byte, []byte, error) {
	var header [1]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, nil, err
	}
	multiplier := 1
	remaining := 0
	for count := 0; count < 4; count++ {
		var digit [1]byte
		if _, err := io.ReadFull(reader, digit[:]); err != nil {
			return 0, nil, err
		}
		remaining += int(digit[0]&0x7f) * multiplier
		if digit[0]&0x80 == 0 {
			body := make([]byte, remaining)
			if _, err := io.ReadFull(reader, body); err != nil {
				return 0, nil, err
			}
			return header[0], body, nil
		}
		multiplier *= 128
	}
	return 0, nil, errors.New("invalid MQTT remaining length")
}

func (c *Client) connectMobileRealtime(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.connectionCtx.Store(&ctx)
	if oldCancel := c.cancelSocket.Swap(&cancel); oldCancel != nil {
		(*oldCancel)()
		if !c.socketStopped.IsSet() {
			if err := c.socketStopped.Wait(ctx); err != nil {
				return
			}
		}
	}
	c.connected.Clear()
	c.socketStopped.Clear()
	defer c.socketStopped.Set()

	failures := 0
	for ctx.Err() == nil {
		err := c.connectMobileRealtimeOnce(ctx)
		wasConnected := c.connected.IsSet()
		c.connected.Clear()
		if ctx.Err() != nil {
			return
		}
		if dispatchErr := c.eventHandler(ctx, &slidetypes.Disconnected{Error: err}); dispatchErr != nil {
			c.log.Err(dispatchErr).Msg("Failed to dispatch mobile realtime disconnect")
			return
		}
		if wasConnected {
			failures = 0
		} else {
			failures++
		}
		retryIn := min(time.Duration(1<<min(failures, 6))*time.Second, MaxConnectionRetryInterval)
		c.log.Warn().Err(err).Dur("retry_in", retryIn).Msg("Instagram mobile realtime connection failed")
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryIn):
		}
	}
}

func (c *Client) connectMobileRealtimeOnce(ctx context.Context) error {
	session := c.GetMobileSession()
	connection, err := dialMobileRealtime(ctx, session)
	if err != nil {
		return err
	}
	defer connection.Close()
	closeOnCancel := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-closeOnCancel:
		}
	}()
	defer close(closeOnCancel)

	c.connected.Set()
	if err = c.eventHandler(ctx, &slidetypes.Connected{}); err != nil {
		return fmt.Errorf("failed to dispatch Instagram mobile connected event: %w", err)
	}
	c.log.Info().Msg("Connected to Instagram mobile realtime")

	for {
		if err = connection.SetReadDeadline(time.Now().Add(instagramMQTTKeepAlive)); err != nil {
			return err
		}
		header, body, readErr := readMQTTPacket(connection)
		if networkErr := (&net.OpError{}); errors.As(readErr, &networkErr) && networkErr.Timeout() {
			if writeErr := connection.SetWriteDeadline(time.Now().Add(10 * time.Second)); writeErr != nil {
				return writeErr
			}
			if _, writeErr := connection.Write([]byte{0xC0, 0x00}); writeErr != nil {
				return fmt.Errorf("failed to send Instagram MQTToT keepalive: %w", writeErr)
			}
			continue
		}
		if readErr != nil {
			return readErr
		}
		switch header >> 4 {
		case 3:
			if err = acknowledgeMobilePublish(connection, header, body); err != nil {
				return err
			}
		case 13:
			// PINGRESP
		}
	}
}

func dialMobileRealtime(ctx context.Context, session *types.InstagramMobileSession) (net.Conn, error) {
	packet, err := buildMobileMQTToTConnectPacket(session, time.Now())
	if err != nil {
		return nil, err
	}
	var connection net.Conn
	var lastErr error
	for _, address := range instagramMQTTBrokers {
		host, _, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			return nil, splitErr
		}
		dialer := &tls.Dialer{Config: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: host,
		}}
		connection, lastErr = dialer.DialContext(ctx, "tcp", address)
		if lastErr == nil {
			break
		}
	}
	if connection == nil {
		return nil, fmt.Errorf("failed to dial Instagram mobile realtime: %w", lastErr)
	}
	if err = connection.SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil {
		_ = connection.Close()
		return nil, err
	}
	if _, err = connection.Write(packet); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("failed to write Instagram MQTToT CONNECT: %w", err)
	}
	if err = connection.SetReadDeadline(time.Now().Add(20 * time.Second)); err != nil {
		_ = connection.Close()
		return nil, err
	}
	header, body, err := readMQTTPacket(connection)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("failed to read instagram MQTToT CONNACK: %w", err)
	}
	if header>>4 != 2 || len(body) < 2 {
		_ = connection.Close()
		return nil, fmt.Errorf("instagram MQTToT returned packet type %d instead of CONNACK", header>>4)
	}
	if body[1] != 0 {
		_ = connection.Close()
		return nil, fmt.Errorf("instagram MQTToT rejected the connection with code %d", body[1])
	}
	_ = connection.SetDeadline(time.Time{})
	return connection, nil
}

func acknowledgeMobilePublish(connection net.Conn, header byte, body []byte) error {
	if len(body) < 2 {
		return errors.New("instagram MQTToT PUBLISH had no topic")
	}
	topicLength := int(binary.BigEndian.Uint16(body[:2]))
	if len(body) < 2+topicLength {
		return errors.New("instagram MQTToT PUBLISH had a truncated topic")
	}
	qos := (header >> 1) & 0x03
	if qos == 0 {
		return nil
	}
	packetIDOffset := 2 + topicLength
	if len(body) < packetIDOffset+2 {
		return errors.New("instagram MQTToT PUBLISH had no packet ID")
	}
	if qos == 1 {
		if err := connection.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return err
		}
		_, err := connection.Write([]byte{
			0x40,
			0x02,
			body[packetIDOffset],
			body[packetIDOffset+1],
		})
		return err
	}
	return fmt.Errorf("unsupported Instagram MQTToT PUBLISH QoS %d", qos)
}
