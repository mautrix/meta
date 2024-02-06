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
		return nil, fmt.Errorf("failed to finalize payload: %v", err)
	}

	resp, err := c.socket.makeLSRequest(payload, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to send LS request: %v", err)
	}

	resp.Finish()

	return resp.Table, nil
}
