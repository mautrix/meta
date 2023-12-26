package messagix

import (
	"encoding/json"
	"strconv"

	"github.com/0xzer/messagix/methods"
	"github.com/0xzer/messagix/socket"
)

type TaskManager struct {
	client *Client
	currTasks []socket.TaskData
	traceId string
}

func (c *Client) NewTaskManager() *TaskManager {
	return &TaskManager{
		client: c,
		currTasks: make([]socket.TaskData, 0),
		traceId: "",
	}
}

func (tm *TaskManager) FinalizePayload() ([]byte, error) {
	p := &socket.TaskPayload{
		EpochId: methods.GenerateEpochId(),
		Tasks: tm.currTasks,
		DataTraceId: tm.traceId,
		VersionId: strconv.Itoa(int(tm.client.configs.VersionId)),
	}
	tm.currTasks = make([]socket.TaskData, 0)
	return json.Marshal(p)
}

func (tm *TaskManager) setTraceId(traceId string) {
	tm.traceId = traceId
}

func (tm *TaskManager) AddNewTask(task socket.Task) {
	payload, queueName, marshalQueueName := task.Create()
	label := task.GetLabel()

	payloadMarshalled, err := json.Marshal(payload)
	if err != nil {
		tm.client.Logger.Err(err).Any("label", label).Msg("failed to marshal task payload")
		return
	}

	
	if marshalQueueName {
		queueName, err = json.Marshal(queueName)
		if err != nil {
			tm.client.Logger.Err(err).Any("label", label).Msg("failed to marshal queueName information for task")
			return
		}
		queueName = string(queueName.([]byte))
	}
	
	taskData := socket.TaskData{
		FailureCount: nil,
		Label: label,
		Payload: string(payloadMarshalled),
		QueueName: queueName,
		TaskId: tm.GetTaskId(),
	}
	tm.client.Logger.Debug().Any("label", label).Any("payload", payload).Any("queueName", queueName).Any("taskId", taskData.TaskId).Msg("Creating task")

	tm.currTasks = append(tm.currTasks, taskData)
}

func (tm *TaskManager) GetTaskId() int64 {
	return int64(tm.client.GetTaskId())
}