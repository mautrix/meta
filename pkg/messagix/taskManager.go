package messagix

import (
	"encoding/json"
	"strconv"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
)

type TaskManager struct {
	client    *Client
	currTasks []socket.TaskData
	traceId   string
}

func (c *Client) newTaskManager() *TaskManager {
	return &TaskManager{
		client:    c,
		currTasks: make([]socket.TaskData, 0),
		traceId:   "",
	}
}

func (tm *TaskManager) FinalizePayload() ([]byte, error) {
	p := &socket.TaskPayload{
		EpochId:     methods.GenerateEpochID(),
		Tasks:       tm.currTasks,
		DataTraceId: tm.traceId,
		VersionId:   strconv.FormatInt(tm.client.configs.VersionID, 10),
	}
	tm.currTasks = make([]socket.TaskData, 0)
	return json.Marshal(p)
}

//lint:ignore U1000 -
func (tm *TaskManager) setTraceId(traceId string) {
	tm.traceId = traceId
}

func (tm *TaskManager) AddNewTask(task socket.Task) {
	payload, queueName, marshalQueueName := task.Create()
	label := task.GetLabel()
	if queueName == nil {
		tm.client.Logger.Error().Str("label", label).Msg("no queue name provided for task")
		return
	}

	payloadMarshalled, err := json.Marshal(payload)
	if err != nil {
		tm.client.Logger.Err(err).Str("label", label).Msg("failed to marshal task payload")
		return
	}

	if marshalQueueName {
		queueName, err = json.Marshal(queueName)
		if err != nil {
			tm.client.Logger.Err(err).Str("label", label).Msg("failed to marshal queueName information for task")
			return
		}
		queueName = string(queueName.([]byte))
	}

	taskData := socket.TaskData{
		FailureCount: nil,
		Label:        label,
		Payload:      string(payloadMarshalled),
		QueueName:    queueName,
		TaskId:       tm.GetTaskID(),
	}
	tm.client.Logger.Trace().Str("label", label).Any("payload", payload).Any("queueName", queueName).Int64("taskId", taskData.TaskId).Msg("Creating task")

	tm.currTasks = append(tm.currTasks, taskData)
}

func (tm *TaskManager) GetTaskID() int64 {
	return int64(tm.client.getTaskID())
}
