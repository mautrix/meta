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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
)

const (
	instagramMQTTIrisSubscriptionTopic = "134"
	instagramMQTTIrisSubscriptionID    = 1
	instagramMQTTMessageSyncTopic      = "146"
	instagramMQTTRealtimeSubTopic      = "149"
	maxInstagramMQTTPayloadSize        = 32 << 20
)

var mobileThreadPathPattern = regexp.MustCompile(`/direct_v2/(?:inbox/)?threads/([0-9]+)`)

type mobileMQTTPublish struct {
	Topic    string
	PacketID uint16
	QoS      byte
	Payload  []byte
}

func parseMobileMQTTPublish(header byte, body []byte) (*mobileMQTTPublish, error) {
	if len(body) < 2 {
		return nil, errors.New("instagram MQTToT PUBLISH had no topic")
	}
	topicLength := int(binary.BigEndian.Uint16(body[:2]))
	offset := 2 + topicLength
	if len(body) < offset {
		return nil, errors.New("instagram MQTToT PUBLISH had a truncated topic")
	}
	publish := &mobileMQTTPublish{
		Topic: string(body[2:offset]),
		QoS:   (header >> 1) & 0x03,
	}
	if publish.QoS == 3 {
		return nil, errors.New("instagram MQTToT PUBLISH had invalid QoS")
	} else if publish.QoS > 0 {
		if len(body) < offset+2 {
			return nil, errors.New("instagram MQTToT PUBLISH had no packet ID")
		}
		publish.PacketID = binary.BigEndian.Uint16(body[offset : offset+2])
		offset += 2
	}
	publish.Payload = bytes.Clone(body[offset:])
	return publish, nil
}

func acknowledgeMobilePublish(writer io.Writer, publish *mobileMQTTPublish) error {
	if publish.QoS == 0 {
		return nil
	} else if publish.QoS != 1 {
		return fmt.Errorf("unsupported Instagram MQTToT PUBLISH QoS %d", publish.QoS)
	}
	_, err := writer.Write([]byte{
		0x40,
		0x02,
		byte(publish.PacketID >> 8),
		byte(publish.PacketID),
	})
	return err
}

func buildMobileMQTTPublishPacket(topic string, payload []byte, packetID uint16) ([]byte, error) {
	if topic == "" || len(topic) > 65535 {
		return nil, errors.New("invalid Instagram MQTToT publish topic")
	} else if packetID == 0 {
		return nil, errors.New("invalid Instagram MQTToT publish packet ID")
	}
	var body bytes.Buffer
	_ = binary.Write(&body, binary.BigEndian, uint16(len(topic)))
	body.WriteString(topic)
	_ = binary.Write(&body, binary.BigEndian, packetID)
	body.Write(payload)

	var packet bytes.Buffer
	packet.WriteByte(0x32)
	if err := writeMQTTRemainingLength(&packet, body.Len()); err != nil {
		return nil, err
	}
	packet.Write(body.Bytes())
	return packet.Bytes(), nil
}

func compressMobileMQTTPayload(payload []byte) ([]byte, error) {
	var compressed bytes.Buffer
	writer, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err = writer.Write(payload); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func sendMobileIrisSubscription(writer io.Writer, seqID int64, snapshotAt time.Time) error {
	if seqID <= 0 {
		return errors.New("instagram mobile message sync has no sequence cursor")
	}
	if snapshotAt.IsZero() {
		snapshotAt = time.Now()
	}
	payload, err := json.Marshal(map[string]any{
		"seq_id":               seqID,
		"snapshot_at_ms":       snapshotAt.UnixMilli(),
		"snapshot_app_version": instagramMobileAppVersion,
	})
	if err != nil {
		return err
	}
	payload, err = compressMobileMQTTPayload(payload)
	if err != nil {
		return fmt.Errorf("failed to compress Instagram mobile subscription: %w", err)
	}
	packet, err := buildMobileMQTTPublishPacket(
		instagramMQTTIrisSubscriptionTopic,
		payload,
		instagramMQTTIrisSubscriptionID,
	)
	if err != nil {
		return err
	}
	_, err = writer.Write(packet)
	return err
}

func (c *Client) waitForMobileIrisSubscription(ctx context.Context, connection net.Conn) error {
	if err := connection.SetReadDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return err
	}
	for {
		header, body, err := readMQTTPacket(connection)
		if err != nil {
			return err
		}
		switch header >> 4 {
		case 3:
			if err = connection.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return err
			}
			if err = c.handleMobilePublish(ctx, connection, header, body); err != nil {
				return err
			}
		case 4:
			if len(body) < 2 {
				return errors.New("instagram MQTToT returned a truncated PUBACK")
			}
			if binary.BigEndian.Uint16(body[:2]) == instagramMQTTIrisSubscriptionID {
				return nil
			}
		case 13:
			// PINGRESP
		}
	}
}

func decompressMobileMQTTPayload(payload []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		if len(payload) > maxInstagramMQTTPayloadSize {
			return nil, errors.New("instagram mobile realtime payload is too large")
		}
		return bytes.Clone(payload), nil
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, maxInstagramMQTTPayloadSize+1))
	if err != nil {
		return nil, err
	} else if len(decoded) > maxInstagramMQTTPayloadSize {
		return nil, errors.New("decompressed Instagram mobile realtime payload is too large")
	}
	return decoded, nil
}

func parseMobileRealtimePayload(payload []byte) ([]string, int64, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var parsed any
	if err := decoder.Decode(&parsed); err != nil {
		return nil, 0, err
	}
	threadIDs := make(map[string]struct{})
	var maxSeqID int64
	collectMobileRealtimeFields(parsed, 0, threadIDs, &maxSeqID)
	threads := make([]string, 0, len(threadIDs))
	for threadID := range threadIDs {
		threads = append(threads, threadID)
	}
	slices.Sort(threads)
	return threads, maxSeqID, nil
}

func collectMobileRealtimeFields(value any, depth int, threadIDs map[string]struct{}, maxSeqID *int64) {
	if depth > 12 {
		return
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectMobileRealtimeFields(item, depth+1, threadIDs, maxSeqID)
		}
	case map[string]any:
		for key, item := range typed {
			switch key {
			case "path":
				if path, ok := item.(string); ok {
					collectMobileThreadPaths(path, threadIDs)
				}
			case "thread_id", "thread_v2_id":
				if threadID := mobileJSONScalarString(item); isDecimalString(threadID) {
					threadIDs[threadID] = struct{}{}
				}
			case "seq_id":
				if seqID, ok := mobileJSONInt64(item); ok && seqID > *maxSeqID {
					*maxSeqID = seqID
				}
			}
			collectMobileRealtimeFields(item, depth+1, threadIDs, maxSeqID)
		}
	case string:
		collectMobileThreadPaths(typed, threadIDs)
		trimmed := strings.TrimSpace(typed)
		if len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			var nested any
			decoder := json.NewDecoder(strings.NewReader(trimmed))
			decoder.UseNumber()
			if decoder.Decode(&nested) == nil {
				collectMobileRealtimeFields(nested, depth+1, threadIDs, maxSeqID)
			}
		}
	}
}

func collectMobileThreadPaths(value string, threadIDs map[string]struct{}) {
	for _, match := range mobileThreadPathPattern.FindAllStringSubmatch(value, -1) {
		threadIDs[match[1]] = struct{}{}
	}
}

func mobileJSONScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func mobileJSONInt64(value any) (int64, bool) {
	raw := mobileJSONScalarString(value)
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	return parsed, err == nil
}

func isDecimalString(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (c *Client) handleMobilePublish(
	ctx context.Context,
	writer io.Writer,
	header byte,
	body []byte,
) error {
	publish, err := parseMobileMQTTPublish(header, body)
	if err != nil {
		return err
	}
	if err = acknowledgeMobilePublish(writer, publish); err != nil {
		return err
	}
	if publish.Topic != instagramMQTTMessageSyncTopic &&
		publish.Topic != instagramMQTTRealtimeSubTopic {
		return nil
	}
	payload, err := decompressMobileMQTTPayload(publish.Payload)
	if err != nil {
		return fmt.Errorf("failed to decompress Instagram mobile topic %s: %w", publish.Topic, err)
	}
	threadIDs, seqID, err := parseMobileRealtimePayload(payload)
	if err != nil {
		c.log.Warn().
			Str("topic", publish.Topic).
			Int("payload_length", len(payload)).
			Msg("Failed to parse Instagram mobile realtime payload")
		return nil
	}
	if c.log != nil {
		c.log.Debug().
			Str("topic", publish.Topic).
			Int("thread_count", len(threadIDs)).
			Bool("has_sequence_update", seqID > c.seqID).
			Msg("Processed Instagram mobile realtime update")
	}
	for _, threadID := range threadIDs {
		if err = c.eventHandler(ctx, &slidetypes.MobileThreadSync{ThreadIGID: threadID}); err != nil {
			return fmt.Errorf("failed to dispatch Instagram mobile thread sync: %w", err)
		}
	}
	if seqID > c.seqID {
		c.seqID = seqID
		c.seqIDTS = time.Now()
		if err = c.eventHandler(ctx, &slidetypes.SeqIDUpdate{
			SeqID:     c.seqID,
			Timestamp: c.seqIDTS,
		}); err != nil {
			return fmt.Errorf("failed to save Instagram mobile sequence ID: %w", err)
		}
	}
	return nil
}
