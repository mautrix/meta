package messagix

import (
	"fmt"

	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
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

func (c *Client) ExecuteTasks(tasks []socket.Task) (*table.LSTable, error) {
	tskm := c.NewTaskManager()
	for _, task := range tasks {
		tskm.AddNewTask(task)
	}
	tskm.setTraceId(methods.GenerateTraceId())

	payload, err := tskm.FinalizePayload()
	if err != nil {
		return nil, fmt.Errorf("failed to finalize payload for SendMessageTask: %v", err)
	}

	packetId, err := c.socket.makeLSRequest(payload, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to make LS request for SendMessageTask: %v", err)
	}

	resp := c.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("failed to receive response from socket after sending SendMessageTask. packetId: %d", packetId)
	}
	resp.Finish()

	return resp.Table, nil
}
