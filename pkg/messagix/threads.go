package messagix

import (
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func (c *Client) ExecuteTasks(tasks ...socket.Task) (*table.LSTable, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	tskm := c.newTaskManager()
	for _, task := range tasks {
		tskm.AddNewTask(task)
	}
	//tskm.setTraceId(methods.GenerateTraceID())

	payload, err := tskm.FinalizePayload()
	if err != nil {
		return nil, fmt.Errorf("failed to finalize payload: %w", err)
	}

	resp, err := c.socket.makeLSRequest(payload, 3)
	if err != nil {
		return nil, err
	}

	resp.Finish()

	return resp.Table, nil
}
