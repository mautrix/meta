package messagix

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	badGlobalLog "github.com/rs/zerolog/log"

	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/packets"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func (s *Socket) handleReadyEvent(ctx context.Context, data *Event_Ready) error {
	if s.previouslyConnected {
		s.client.canSendMessages.Set()
		reconnectSync := minimalFBReconnectSync
		if s.client.Platform.IsInstagram() {
			reconnectSync = minimalReconnectSync
		}
		err := s.client.syncManager.EnsureSyncedSocket(ctx, reconnectSync)
		if err != nil {
			return fmt.Errorf("failed to sync after reconnect: %w", err)
		}
		s.client.HandleEvent(ctx, &Event_Reconnected{})
		return nil
	}
	appSettingPublishJSON, err := s.newAppSettingsPublishJSON(s.client.configs.VersionID)
	if err != nil {
		return fmt.Errorf("failed to get app settings JSON: %w", err)
	}

	_, err = s.sendPublishPacket(ctx, LS_APP_SETTINGS, appSettingPublishJSON, &packets.PublishPacket{QOSLevel: packets.QOS_LEVEL_1}, s.SafePacketID())
	if err != nil {
		return fmt.Errorf("failed to send app settings packet: %w", err)
	}

	_, err = s.sendSubscribePacket(ctx, LS_FOREGROUND_STATE, packets.QOS_LEVEL_0, true)
	if err != nil {
		return fmt.Errorf("failed to subscribe to /ls_foreground_state: %w", err)
	}

	_, err = s.sendSubscribePacket(ctx, LS_RESP, packets.QOS_LEVEL_0, true)
	if err != nil {
		return fmt.Errorf("failed to subscribe to /ls_resp: %w", err)
	}

	s.client.canSendMessages.Set()

	tskm := s.client.newTaskManager()
	tskm.AddNewTask(&socket.FetchThreadsTask{
		IsAfter:                    0,
		ParentThreadKey:            -1,
		ReferenceThreadKey:         0,
		ReferenceActivityTimestamp: 9999999999999,
		AdditionalPagesToFetch:     0,
		Cursor:                     s.client.syncManager.GetCursor(1),
		SyncGroup:                  1,
	})
	tskm.AddNewTask(&socket.FetchThreadsTask{
		IsAfter:                    0,
		ParentThreadKey:            -1,
		ReferenceThreadKey:         0,
		ReferenceActivityTimestamp: 9999999999999,
		AdditionalPagesToFetch:     0,
		SyncGroup:                  95,
	})

	syncGroupKeyStore1 := s.client.syncManager.getSyncGroupKeyStore(1)
	if syncGroupKeyStore1 != nil {
		//  syncGroupKeyStore95 := s.client.syncManager.getSyncGroupKeyStore(95)
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey:         syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch:     0,
			Cursor:                     s.client.syncManager.GetCursor(1),
			SyncGroup:                  1,
		})
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey:         syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch:     0,
			SyncGroup:                  95,
		})
	}

	payload, err := tskm.FinalizePayload()
	if err != nil {
		return fmt.Errorf("failed to finalize sync tasks: %w", err)
	}

	s.client.Logger.Trace().Any("data", string(payload)).Msg("Sync groups tasks")
	_, err = s.makeLSRequest(ctx, payload, 3)
	if err != nil {
		return fmt.Errorf("failed to send sync tasks: %w", err)
	}

	_, err = s.client.ExecuteTasks(ctx, &socket.ReportAppStateTask{AppState: table.FOREGROUND, RequestID: uuid.NewString()})
	if err != nil {
		return fmt.Errorf("failed to report app state: %w", err)
	}

	initialSync := minimalFBInitialSync
	if s.client.Platform.IsInstagram() {
		initialSync = minimalInitialSync
	}
	err = s.client.syncManager.EnsureSyncedSocket(ctx, initialSync)
	if err != nil {
		return fmt.Errorf("failed to ensure db 1 is synced: %w", err)
	}

	data.client = s.client
	s.client.HandleEvent(ctx, data.Finish())
	s.previouslyConnected = true

	return nil
}

func (s *Socket) handleACKEvent(ackData AckEvent) {
	ok := s.responseHandler.updatePacketChannel(ackData.GetPacketID(), ackData)
	if !ok {
		s.client.Logger.Debug().Uint16("packet_id", ackData.GetPacketID()).Msg("Got unexpected publish ack")
	}
}

func (s *Socket) postHandlePublishResponse(tbl *table.LSTable) {
	syncGroupsNeedUpdate := methods.NeedUpdateSyncGroups(tbl)
	if syncGroupsNeedUpdate {
		s.client.Logger.Debug().
			Any("LSExecuteFirstBlockForSyncTransaction", tbl.LSExecuteFirstBlockForSyncTransaction).
			Any("LSUpsertSyncGroupThreadsRange", tbl.LSUpsertSyncGroupThreadsRange).
			Msg("Updating sync groups")
		err := s.client.syncManager.updateSyncGroupCursors(tbl)
		if err != nil {
			s.client.Logger.Err(err).Msg("Failed to sync transactions from publish response event")
		}
	}
}

func (c *Client) PostHandlePublishResponse(tbl *table.LSTable) {
	if c == nil {
		return
	}
	if s := c.socket; s != nil {
		s.postHandlePublishResponse(tbl)
	}
}

func (s *Socket) handlePublishResponseEvent(ctx context.Context, resp *Event_PublishResponse, isQueue bool) (addToQueue bool) {
	packetID := resp.Data.RequestID
	hasPacket := s.responseHandler.hasPacket(uint16(packetID))
	switch resp.Topic {
	case string(LS_RESP):
		resp.Finish()
		if !isQueue && hasPacket {
			ok := s.responseHandler.updateRequestChannel(uint16(packetID), resp)
			if !ok {
				s.client.Logger.Warn().Int64("packet_id", packetID).Msg("Dropped response to packet")
			}
		} else if packetID == 0 {
			if !isQueue {
				return true
			}
			s.client.HandleEvent(ctx, resp)
		} else {
			s.client.Logger.Debug().Int64("packet_id", packetID).Msg("Got unexpected lightspeed publish response")
		}
	default:
		s.client.Logger.Info().Any("packet_id", packetID).Any("topic", resp.Topic).Any("data", resp.Data).
			Msg("Got unknown publish response topic")
	}
	if ctx.Err() != nil {
		return false
	}
	go func() {
		if resp.QoS == packets.QOS_LEVEL_1 {
			err := s.sendData(binary.BigEndian.AppendUint16([]byte{packets.PUBACK << 4, 2}, resp.MessageIdentifier))
			if err != nil {
				s.client.Logger.Err(err).Uint16("message_id", resp.MessageIdentifier).Msg("Failed to send puback")
			}
		} else if resp.QoS == packets.QOS_LEVEL_2 {
			s.client.Logger.Error().Msg("Got packet with QoS level 2")
		}
	}()
	return false
}

type Event_PingResp struct{}

func (pr *Event_PingResp) SetIdentifier(identifier uint16) {}
func (e *Event_PingResp) Finish() ResponseData             { return e }

type Event_SocketError struct {
	Err                error
	ConnectionAttempts int
}

type Event_PermanentError struct{ Err error }

type Event_Reconnected struct{}

// Event_Ready represents the CONNACK packet's response.
//
// The library provides the raw parsed data, so handle connection codes as needed for your application.
type Event_Ready struct {
	client         *Client
	IsNewSession   bool
	ConnectionCode ConnectionCode
}

func (pb *Event_Ready) SetIdentifier(identifier uint16) {}

func (e *Event_Ready) Finish() ResponseData {
	return e
}

type AckEvent interface {
	GetPacketID() uint16
}

// Event_PublishACK is never emitted, it only handles the acknowledgement after a PUBLISH packet has been sent.
type Event_PublishACK struct {
	PacketID uint16
}

func (pb *Event_PublishACK) GetPacketID() uint16 {
	return pb.PacketID
}

func (pb *Event_PublishACK) SetIdentifier(identifier uint16) {}

func (pb *Event_PublishACK) Finish() ResponseData {
	return pb
}

// Event_SubscribeACK is never emitted, it only handles the acknowledgement after a SUBSCRIBE packet has been sent.
type Event_SubscribeACK struct {
	PacketID uint16
	QoSLevel uint8 // 0, 1, 2, 128
}

func (pb *Event_SubscribeACK) GetPacketID() uint16 {
	return pb.PacketID
}

func (pb *Event_SubscribeACK) SetIdentifier(identifier uint16) {}

func (pb *Event_SubscribeACK) Finish() ResponseData {
	return pb
}

// Event_PublishResponse is emitted if the packetId/requestId from the websocket is 0 or nil
//
// It will also be used for handling the responses after calling a function like GetContacts through the requestId
type Event_PublishResponse struct {
	Topic             string              `lengthType:"uint16" endian:"big"`
	Data              PublishResponseData `jsonString:"1"`
	Table             *table.LSTable
	MessageIdentifier uint16
	QoS               packets.QoS
}

type PublishResponseData struct {
	RequestID int64    `json:"request_id,omitempty"`
	Payload   string   `json:"payload,omitempty"`
	Sp        []string `json:"sp,omitempty"` // dependencies
	Target    int      `json:"target,omitempty"`
}

func (pb *Event_PublishResponse) SetIdentifier(identifier uint16) {
	pb.MessageIdentifier = identifier
}

func (pb *Event_PublishResponse) Finish() ResponseData {
	pb.Table = &table.LSTable{}
	var lsData *lightspeed.LightSpeedData
	err := json.Unmarshal([]byte(pb.Data.Payload), &lsData)
	if err != nil {
		logEvt := badGlobalLog.Err(err).
			Int("payload_length", len(pb.Data.Payload)).
			Str("topic", pb.Topic).
			Int64("request_id", pb.Data.RequestID).
			Uint16("mqtt_message_id", pb.MessageIdentifier).
			Strs("sp", pb.Data.Sp).
			Int("target", pb.Data.Target)
		if len(pb.Data.Payload) < 8192 {
			logEvt.Str("payload", pb.Data.Payload)
		}
		logEvt.Msg("failed to unmarshal PublishResponseData JSON payload into lightspeed.LightSpeedData struct")
		return pb
	}

	dependencies := table.SPToDepMap(pb.Data.Sp)
	decoder := lightspeed.NewLightSpeedDecoder(dependencies, pb.Table)
	decoder.Decode(lsData.Steps)
	return pb
}
