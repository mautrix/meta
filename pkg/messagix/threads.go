package messagix

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func (c *Client) ExecuteTasks(ctx context.Context, tasks ...socket.Task) (*table.LSTable, error) {
	if c == nil {
		return nil, ErrClientIsNil
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
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

	resp, err := c.socket.makeLSRequest(ctx, payload, 3)
	if err != nil {
		return nil, err
	}

	resp.Finish()

	return resp.Table, nil
}

func (c *Client) ExecuteStatelessTask(ctx context.Context, task socket.Task) error {
	if c == nil {
		return ErrClientIsNil
	} else if ctx.Err() != nil {
		return ctx.Err()
	}
	innerPayload, queueName, _ := task.Create()
	label := task.GetLabel()
	if queueName != nil {
		return fmt.Errorf("tried to execute stateful task %s as stateless", label)
	}
	innerPayloadMarshalled, err := json.Marshal(innerPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal inner task %s payload: %w", label, err)
	}
	outerPayload := socket.StatelessTaskData{
		Label:   label,
		Payload: string(innerPayloadMarshalled),
		Version: strconv.FormatInt(c.configs.VersionID, 10),
	}
	outerPayloadMarshalled, err := json.Marshal(outerPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal outer task %s payload: %w", label, err)
	}
	_, err = c.socket.makeLSRequest(ctx, outerPayloadMarshalled, 4)
	return err
}
