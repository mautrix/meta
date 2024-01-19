package messagix

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	badGlobalLog "github.com/rs/zerolog/log"

	"go.mau.fi/mautrix-meta/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/packets"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

func (s *Socket) handleReadyEvent(data *Event_Ready) error {
	if s.previouslyConnected {
		err := s.client.SyncManager.EnsureSyncedSocket([]int64{
			1,
			2,
			//16,
		})
		if err != nil {
			return fmt.Errorf("failed to sync after reconnect: %w", err)
		}
		s.client.eventHandler(&Event_Reconnected{})
		return nil
	}
	appSettingPublishJSON, err := s.newAppSettingsPublishJSON(s.client.configs.VersionId)
	if err != nil {
		return fmt.Errorf("failed to get app settings JSON: %w", err)
	}

	packetId, err := s.sendPublishPacket(LS_APP_SETTINGS, appSettingPublishJSON, &packets.PublishPacket{QOSLevel: packets.QOS_LEVEL_1}, s.SafePacketId())
	if err != nil {
		return fmt.Errorf("failed to send app settings packet: %w", err)
	}

	appSettingAck := s.responseHandler.waitForPubACKDetails(packetId)
	if appSettingAck == nil {
		return fmt.Errorf("didn't get ack for app settings packet %d", packetId)
	}

	_, err = s.sendSubscribePacket(LS_FOREGROUND_STATE, packets.QOS_LEVEL_0, true)
	if err != nil {
		return fmt.Errorf("failed to subscribe to /ls_foreground_state: %w", err)
	}

	_, err = s.sendSubscribePacket(LS_RESP, packets.QOS_LEVEL_0, true)
	if err != nil {
		return fmt.Errorf("failed to subscribe to /ls_resp: %w", err)
	}

	tskm := s.client.NewTaskManager()
	tskm.AddNewTask(&socket.FetchThreadsTask{
		IsAfter:                    0,
		ParentThreadKey:            -1,
		ReferenceThreadKey:         0,
		ReferenceActivityTimestamp: 9999999999999,
		AdditionalPagesToFetch:     0,
		Cursor:                     s.client.SyncManager.GetCursor(1),
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

	syncGroupKeyStore1 := s.client.SyncManager.getSyncGroupKeyStore(1)
	if syncGroupKeyStore1 != nil {
		//  syncGroupKeyStore95 := s.client.SyncManager.getSyncGroupKeyStore(95)
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey:         syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch:     0,
			Cursor:                     s.client.SyncManager.GetCursor(1),
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
	packetId, err = s.makeLSRequest(payload, 3)
	if err != nil {
		return fmt.Errorf("failed to send sync tasks: %w", err)
	}

	resp := s.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return fmt.Errorf("didn't receive response to sync task %d", packetId)
	}

	err = s.client.Account.ReportAppState(table.FOREGROUND)
	if err != nil {
		return fmt.Errorf("failed to report app state: %w", err)
	}

	err = s.client.SyncManager.EnsureSyncedSocket([]int64{
		1,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure db 1 is synced: %w", err)
	}

	data.client = s.client
	s.client.eventHandler(data.Finish())
	s.previouslyConnected = true
	return nil
}

func (s *Socket) handleACKEvent(ackData AckEvent) {
	ok := s.responseHandler.updatePacketChannel(ackData.GetPacketId(), ackData)
	if !ok {
		s.client.Logger.Debug().Uint16("packet_id", ackData.GetPacketId()).Msg("Got unexpected publish ack")
	}
}

func (s *Socket) handlePublishResponseEvent(resp *Event_PublishResponse, qos packets.QoS) {
	packetId := resp.Data.RequestID
	hasPacket := s.responseHandler.hasPacket(uint16(packetId))
	// s.client.Logger.Debug().Any("packetId", packetId).Any("resp", resp).Msg("got response!")
	switch resp.Topic {
	case string(LS_RESP):
		resp.Finish()
		if hasPacket {
			ok := s.responseHandler.updateRequestChannel(uint16(packetId), resp)
			if !ok {
				s.client.Logger.Warn().Int64("packet_id", packetId).Msg("Dropped response to packet")
			}
		} else if packetId == 0 {
			syncGroupsNeedUpdate := methods.NeedUpdateSyncGroups(resp.Table)
			if syncGroupsNeedUpdate {
				s.client.Logger.Debug().
					Any("LSExecuteFirstBlockForSyncTransaction", resp.Table.LSExecuteFirstBlockForSyncTransaction).
					Any("LSUpsertSyncGroupThreadsRange", resp.Table.LSUpsertSyncGroupThreadsRange).
					Msg("Updating sync groups")
				//err := s.client.SyncManager.SyncTransactions(transactions)
				err := s.client.SyncManager.updateSyncGroupCursors(resp.Table)
				if err != nil {
					s.client.Logger.Err(err).Msg("Failed to sync transactions from publish response event")
				}
			}
			s.client.eventHandler(resp)
		} else {
			s.client.Logger.Debug().Int64("packet_id", packetId).Msg("Got unexpected lightspeed publish response")
		}
	default:
		s.client.Logger.Info().Any("packetId", packetId).Any("topic", resp.Topic).Any("data", resp.Data).
			Msg("Got unknown publish response topic")
	}
	go func() {
		if qos == packets.QOS_LEVEL_1 {
			err := s.sendData(binary.BigEndian.AppendUint16([]byte{packets.PUBACK << 4, 2}, resp.MessageIdentifier))
			if err != nil {
				s.client.Logger.Err(err).Uint16("message_id", resp.MessageIdentifier).Msg("Failed to send puback")
			}
		}
	}()
}

type Event_PingResp struct{}

func (pr *Event_PingResp) SetIdentifier(identifier uint16) {}
func (e *Event_PingResp) Finish() ResponseData             { return e }

type Event_SocketError struct{ Err error }

type Event_PermanentError struct{ Err error }

type Event_Reconnected struct{}

// Event_Ready represents the CONNACK packet's response.
//
// The library provides the raw parsed data, so handle connection codes as needed for your application.
type Event_Ready struct {
	client         *Client
	IsNewSession   bool
	ConnectionCode ConnectionCode
	CurrentUser    types.UserInfo `skip:"1"`
	Table          *table.LSTable
	//Threads []table.LSDeleteThenInsertThread `skip:"1"`
	//Messages []table.LSUpsertMessage `skip:"1"`
	//Contacts []table.LSVerifyContactRowExists `skip:"1"`
}

func (pb *Event_Ready) SetIdentifier(identifier uint16) {}

func (e *Event_Ready) Finish() ResponseData {
	if e.client.platform == types.Facebook {
		e.CurrentUser = &e.client.configs.browserConfigTable.CurrentUserInitialData
	} else {
		e.CurrentUser = &e.client.configs.browserConfigTable.PolarisViewer
	}
	e.Table = e.client.configs.accountConfigTable
	//e.Threads = e.client.configs.accountConfigTable.LSDeleteThenInsertThread
	//e.Messages = e.client.configs.accountConfigTable.LSUpsertMessage
	//e.Contacts = e.client.configs.accountConfigTable.LSVerifyContactRowExists
	return e
}

type AckEvent interface {
	GetPacketId() uint16
}

// Event_PublishACK is never emitted, it only handles the acknowledgement after a PUBLISH packet has been sent.
type Event_PublishACK struct {
	PacketId uint16
}

func (pb *Event_PublishACK) GetPacketId() uint16 {
	return pb.PacketId
}

func (pb *Event_PublishACK) SetIdentifier(identifier uint16) {}

func (pb *Event_PublishACK) Finish() ResponseData {
	return pb
}

// Event_SubscribeACK is never emitted, it only handles the acknowledgement after a SUBSCRIBE packet has been sent.
type Event_SubscribeACK struct {
	PacketId uint16
	QoSLevel uint8 // 0, 1, 2, 128
}

func (pb *Event_SubscribeACK) GetPacketId() uint16 {
	return pb.PacketId
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
		badGlobalLog.Err(err).Msg("failed to unmarshal PublishResponseData JSON payload into lightspeed.LightSpeedData struct")
		return pb
	}

	dependencies := table.SPToDepMap(pb.Data.Sp)
	decoder := lightspeed.NewLightSpeedDecoder(dependencies, pb.Table)
	decoder.Decode(lsData.Steps)
	return pb
}
