package messagix

import (
	"encoding/json"
	"log"
	"github.com/0xzer/messagix/lightspeed"
	"github.com/0xzer/messagix/methods"
	"github.com/0xzer/messagix/packets"
	"github.com/0xzer/messagix/socket"
	"github.com/0xzer/messagix/table"
	"github.com/0xzer/messagix/types"
)

func (s *Socket) handleBinaryMessage(data []byte) {
	//s.client.Logger.Debug().Any("hex-data", debug.BeautifyHex(data)).Bytes("bytes", data).Msg("Received BinaryMessage")
	if s.client.eventHandler == nil {
		return
	}

	resp := &Response{}
	err := resp.Read(data)
	if err != nil {
		s.handleErrorEvent(err)
	} else {
		switch evt := resp.ResponseData.(type) {
		case *Event_PingResp:
			s.client.Logger.Info().Msg("Got PingResp packet")
		case *Event_PublishResponse:
			s.handlePublishResponseEvent(evt)
		case *Event_PublishACK, *Event_SubscribeACK:
			s.handleACKEvent(evt.(AckEvent))
		case *Event_Ready:
			s.handleReadyEvent(evt)
		default:
			s.client.Logger.Info().Any("data", data).Msg("sending default event...")
			s.client.eventHandler(resp.ResponseData.Finish())
		}
	}
}

func (s *Socket) handleReadyEvent(data *Event_Ready) {
	appSettingPublishJSON, err := s.newAppSettingsPublishJSON(s.client.configs.VersionId)
	if err != nil {
		log.Fatal(err)
	}

	packetId, err := s.sendPublishPacket(LS_APP_SETTINGS, appSettingPublishJSON, &packets.PublishPacket{QOSLevel: packets.QOS_LEVEL_1}, s.SafePacketId())
	if err != nil {
		log.Fatalf("failed to send APP_SETTINGS publish packet: %e", err)
	}

	appSettingAck := s.responseHandler.waitForPubACKDetails(packetId)
	if appSettingAck == nil {
		log.Fatalf("failed to get pubAck for packetId: %d", appSettingAck.PacketId)
	}

	_, err = s.sendSubscribePacket(LS_FOREGROUND_STATE, packets.QOS_LEVEL_0, true)
	if err != nil {
		log.Fatalf("failed to subscribe to ls_foreground_state: %e", err)
	}

	_, err = s.sendSubscribePacket(LS_RESP, packets.QOS_LEVEL_0, true)
	if err != nil {
		log.Fatalf("failed to subscribe to ls_resp: %e", err)
	}

	
	tskm := s.client.NewTaskManager()
	tskm.AddNewTask(&socket.FetchThreadsTask{
		IsAfter: 0,
		ParentThreadKey: -1,
		ReferenceThreadKey: 0,
		ReferenceActivityTimestamp: 9999999999999,
		AdditionalPagesToFetch: 0,
		Cursor: s.client.SyncManager.GetCursor(1),
		SyncGroup: 1,
	})
	tskm.AddNewTask(&socket.FetchThreadsTask{
		IsAfter: 0,
		ParentThreadKey: -1,
		ReferenceThreadKey: 0,
		ReferenceActivityTimestamp: 9999999999999,
		AdditionalPagesToFetch: 0,
		SyncGroup: 95,
	})

	syncGroupKeyStore1 := s.client.SyncManager.getSyncGroupKeyStore(1)
	if syncGroupKeyStore1 != nil {
		//  syncGroupKeyStore95 := s.client.SyncManager.getSyncGroupKeyStore(95)
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter: 0,
			ParentThreadKey: syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey: syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch: 0,
			Cursor: s.client.SyncManager.GetCursor(1),
			SyncGroup: 1,
		})
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter: 0,
			ParentThreadKey: syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey: syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch: 0,
			SyncGroup: 95,
		})
	}

	payload, err := tskm.FinalizePayload()
	if err != nil {
		log.Fatal(err)
	}

	s.client.Logger.Debug().Any("data", string(payload)).Msg("Sync groups tasks")
	packetId, err = s.makeLSRequest(payload, 3)
	if err != nil {
		log.Fatal(err)
	}

	resp := s.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		log.Fatalf("failed to receive response from task 145 request")
	}

	s.client.Logger.Info().Any("syncgroup", resp.Table.LSUpsertSyncGroupThreadsRange).Any("threads", resp.Table.LSDeleteThenInsertThread).Msg("145 RESP.")

	err = s.client.Account.ReportAppState(table.FOREGROUND)
	if err != nil {
		log.Fatalf("failed to report app state to foreground (active): %e", err)
	}

	
	err = s.client.SyncManager.EnsureSyncedSocket([]int64{
		1,
	})

	if err != nil {
		log.Fatalf("EnsureSyncedSocket failed to sync db 1: %e", err)
	}

	data.client = s.client
	s.client.eventHandler(data.Finish())
	go s.startHandshakeInterval()
}

func (s *Socket) handleACKEvent(ackData AckEvent) {
	packetId := ackData.GetPacketId()
	err := s.responseHandler.updatePacketChannel(uint16(packetId), ackData)
	if err != nil {
		s.client.Logger.Err(err).Any("data", ackData).Any("packetId", packetId).Msg("failed to handle ack event")
		return
	}
}

func (s *Socket) handleErrorEvent(err error) {
	errEvent := &Event_Error{Err: err}
	s.client.eventHandler(errEvent)
}

func (s *Socket) handlePublishResponseEvent(resp *Event_PublishResponse) {
	packetId := resp.Data.RequestID
	hasPacket := s.responseHandler.hasPacket(uint16(packetId))
	// s.client.Logger.Debug().Any("packetId", packetId).Any("resp", resp).Msg("got response!")
	switch resp.Topic {
		case string(LS_RESP):
			resp.Finish()
			if hasPacket {
				err := s.responseHandler.updateRequestChannel(uint16(packetId), resp)
				if err != nil {
					s.handleErrorEvent(err)
					return
				}
				return
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
				return
			}
			// s.client.Logger.Info().Any("packetId", packetId).Any("data", resp).Msg("Got publish response but was not expecting it for specific packet identifier.")
		default:
			s.client.Logger.Info().Any("packetId", packetId).Any("topic", resp.Topic).Any("data", resp.Data).Msg("Got unknown publish response topic!")
	}
}

type Event_PingResp struct {}
func (pr *Event_PingResp) SetIdentifier(identifier int16) {}
func (e *Event_PingResp) Finish() ResponseData {return e}
// Event_Ready represents the CONNACK packet's response.
//
// The library provides the raw parsed data, so handle connection codes as needed for your application.
type Event_Ready struct {
	client *Client
	IsNewSession   bool
	ConnectionCode ConnectionCode
	CurrentUser types.AccountInfo `skip:"1"`
	Table *table.LSTable
	//Threads []table.LSDeleteThenInsertThread `skip:"1"`
	//Messages []table.LSUpsertMessage `skip:"1"`
	//Contacts []table.LSVerifyContactRowExists `skip:"1"`
}

func (pb *Event_Ready) SetIdentifier(identifier int16) {}


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

// Event_Error is emitted whenever the library encounters/receives an error.
//
// These errors can be for example: failed to send data, failed to read response data and so on.
type Event_Error struct {
	Err error
}

func (pb *Event_Error) SetIdentifier(identifier int16) {}

func (e *Event_Error) Finish() ResponseData {
	return e
}

// Event_SocketClosed is emitted whenever the websockets CloseHandler() is called.
//
// This provides great flexability because the user can then decide whether the client should reconnect or not.
type Event_SocketClosed struct {
	Code int
	Text string
}

func (pb *Event_SocketClosed) SetIdentifier(identifier int16) {}

func (e *Event_SocketClosed) Finish() ResponseData {
	return e
}

type AckEvent interface{
	GetPacketId() uint16
}

// Event_PublishACK is never emitted, it only handles the acknowledgement after a PUBLISH packet has been sent.
type Event_PublishACK struct {
	PacketId uint16
}

func (pb *Event_PublishACK) GetPacketId() uint16 {
	return pb.PacketId
}

func (pb *Event_PublishACK) SetIdentifier(identifier int16) {}

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

func (pb *Event_SubscribeACK) SetIdentifier(identifier int16) {}

func (pb *Event_SubscribeACK) Finish() ResponseData {
	return pb
}

// Event_PublishResponse is emitted if the packetId/requestId from the websocket is 0 or nil
//
// It will also be used for handling the responses after calling a function like GetContacts through the requestId
type Event_PublishResponse struct {
	Topic string `lengthType:"uint16" endian:"big"`
	Data PublishResponseData `jsonString:"1"`
	Table table.LSTable
	MessageIdentifier int16
}

type PublishResponseData struct {
	RequestID int64 `json:"request_id,omitempty"`
	Payload   string `json:"payload,omitempty"`
	Sp     []string `json:"sp,omitempty"` // dependencies
	Target int      `json:"target,omitempty"`
}

func (pb *Event_PublishResponse) SetIdentifier(identifier int16) {
	pb.MessageIdentifier = identifier
}

func (pb *Event_PublishResponse) Finish() ResponseData {
	pb.Table = table.LSTable{}
	var lsData *lightspeed.LightSpeedData
	err := json.Unmarshal([]byte(pb.Data.Payload), &lsData)
	if err != nil {
		log.Println("failed to unmarshal PublishResponseData JSON payload into lightspeed.LightSpeedData struct: %e", err)
		return pb
	}

	dependencies := table.SPToDepMap(pb.Data.Sp)
	decoder := lightspeed.NewLightSpeedDecoder(dependencies, &pb.Table)
	decoder.Decode(lsData.Steps)
	return pb
}