package messagix

import (
	"fmt"

	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
)

func (c *Client) ExecuteTasks(tasks ...socket.Task) (*table.LSTable, error) {
	tskm := c.NewTaskManager()
	for _, task := range tasks {
		tskm.AddNewTask(task)
	}
	//tskm.setTraceId(methods.GenerateTraceId())

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
	} else if resp.Topic == "" {
		return nil, fmt.Errorf("request timed out")
	}
	resp.Finish()

	return resp.Table, nil
}
