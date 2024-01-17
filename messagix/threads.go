package messagix

import (
	"fmt"

	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

type Threads struct {
	client *Client
}

func (t *Threads) FetchMessages(ThreadId int64, ReferenceTimestampMs int64, ReferenceMessageId string, Cursor string) (*table.LSTable, error) {
	tskm := t.client.NewTaskManager()
	tskm.AddNewTask(&socket.FetchMessagesTask{ThreadKey: ThreadId, Direction: 0, ReferenceTimestampMs: ReferenceTimestampMs, ReferenceMessageId: ReferenceMessageId, SyncGroup: 1, Cursor: Cursor})

	payload, err := tskm.FinalizePayload()
	if err != nil {
		return nil, fmt.Errorf("failed to finalize payload: %v", err)
	}

	packetId, err := t.client.socket.makeLSRequest(payload, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	resp := t.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("failed to receive response from socket while trying to fetch messages. (packetId=%d, thread_key=%d, cursor=%s, reference_message_id=%s, reference_timestamp_ms=%d)", packetId, ThreadId, Cursor, ReferenceMessageId, ReferenceTimestampMs)
	}

	return resp.Table, nil
}

type MessageBuilder struct {
	client      *Client
	payload     *socket.SendMessageTask
	readPayload *socket.ThreadMarkReadTask
}

func (t *Threads) NewMessageBuilder(threadId int64) *MessageBuilder {
	return &MessageBuilder{
		client: t.client,
		payload: &socket.SendMessageTask{
			ThreadId:          threadId,
			SkipUrlPreviewGen: 0,
			TextHasLinks:      0,
			AttachmentFBIds:   make([]int64, 0),
		},
		readPayload: &socket.ThreadMarkReadTask{
			ThreadId:  threadId,
			SyncGroup: 1,
		},
	}
}

func (m *MessageBuilder) SetMedias(medias []*types.MercuryUploadResponse) {
	for _, media := range medias {
		data, ok := media.Payload.Metadata.(types.MediaMetadata)
		if ok {
			m.payload.AttachmentFBIds = append(m.payload.AttachmentFBIds, data.GetFbId())
		} else {
			// TODO do something?
		}
	}
}

func (m *MessageBuilder) SetReplyMetadata(replyMetadata *socket.ReplyMetaData) *MessageBuilder {
	m.payload.ReplyMetaData = replyMetadata
	return m
}

func (m *MessageBuilder) SetSource(source table.ThreadSourceType) *MessageBuilder {
	m.payload.Source = source
	return m
}

func (m *MessageBuilder) SetInitiatingSource(initatingSource table.InitiatingSource) *MessageBuilder {
	m.payload.InitiatingSource = initatingSource
	return m
}

func (m *MessageBuilder) SetSyncGroup(syncGroup int64) *MessageBuilder {
	m.payload.SyncGroup = syncGroup
	m.readPayload.SyncGroup = syncGroup
	return m
}

func (m *MessageBuilder) SetSkipUrlPreviewGen() *MessageBuilder {
	m.payload.SkipUrlPreviewGen = 1
	return m
}

func (m *MessageBuilder) SetTextHasLinks() *MessageBuilder {
	m.payload.TextHasLinks = 1
	return m
}

func (m *MessageBuilder) SetText(text string) *MessageBuilder {
	m.payload.Text = text
	return m
}

func (m *MessageBuilder) SetLastReadWatermarkTs(ts int64) *MessageBuilder {
	m.readPayload.LastReadWatermarkTs = ts
	return m
}

func (m *MessageBuilder) SetOfflineThreadingID(otid int64) *MessageBuilder {
	m.payload.Otid = otid
	return m
}

func (m *MessageBuilder) Execute() (*table.LSTable, error) {
	tskm := m.client.NewTaskManager()

	if m.payload.Source == 0 {
		m.payload.Source = table.MESSENGER_INBOX_IN_THREAD
	}

	if m.payload.SyncGroup == 0 {
		m.payload.SyncGroup = 1
	}

	if m.payload.InitiatingSource == 0 {
		m.payload.InitiatingSource = table.FACEBOOK_INBOX
	}

	if m.payload.Otid == 0 {
		m.payload.Otid = methods.GenerateEpochId()
	}

	if m.payload.Text != nil {
		tskm.AddNewTask(&socket.SendMessageTask{
			ThreadId:          m.payload.ThreadId,
			Otid:              m.payload.Otid,
			Source:            m.payload.Source,
			SyncGroup:         m.payload.SyncGroup,
			ReplyMetaData:     m.payload.ReplyMetaData,
			Text:              m.payload.Text,
			InitiatingSource:  m.payload.InitiatingSource,
			SkipUrlPreviewGen: m.payload.SkipUrlPreviewGen,
			TextHasLinks:      m.payload.TextHasLinks,
			SendType:          table.TEXT,
			MultiTabEnv:       0,
		})
	}

	if len(m.payload.AttachmentFBIds) > 0 {
		m.addAttachmentTasks(tskm)
	}

	tskm.AddNewTask(m.readPayload)
	tskm.setTraceId(methods.GenerateTraceId())

	payload, err := tskm.FinalizePayload()
	if err != nil {
		return nil, fmt.Errorf("failed to finalize payload for SendMessageTask: %v", err)
	}

	packetId, err := m.client.socket.makeLSRequest(payload, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to make LS request for SendMessageTask: %v", err)
	}

	resp := m.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("failed to receive response from socket after sending SendMessageTask. packetId: %d", packetId)
	}
	resp.Finish()

	return resp.Table, nil
}

func (m *MessageBuilder) addAttachmentTasks(tskm *TaskManager) {
	if m.client.platform == types.Facebook {
		otid := methods.GenerateEpochId()
		tskm.AddNewTask(&socket.SendMessageTask{
			ThreadId:        m.payload.ThreadId,
			Otid:            otid + 100,
			Source:          m.payload.Source,
			SyncGroup:       m.payload.SyncGroup,
			ReplyMetaData:   m.payload.ReplyMetaData,
			SendType:        table.MEDIA,
			AttachmentFBIds: m.payload.AttachmentFBIds,
		})
	} else {
		for _, mediaId := range m.payload.AttachmentFBIds {
			otid := methods.GenerateEpochId()
			tskm.AddNewTask(&socket.SendMessageTask{
				ThreadId:        m.payload.ThreadId,
				Otid:            otid + 100,
				Source:          m.payload.Source,
				SyncGroup:       m.payload.SyncGroup,
				ReplyMetaData:   m.payload.ReplyMetaData,
				SendType:        table.MEDIA,
				AttachmentFBIds: []int64{mediaId},
			})
		}
	}
}
