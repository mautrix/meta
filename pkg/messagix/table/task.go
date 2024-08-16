package table

import (
	"encoding/json"
	"strconv"
)

type LSTaskExists struct {
	TaskId int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSRemoveTask struct {
	TaskId int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateTaskQueueName struct {
	QueueNameTaskId string `index:"0" json:",omitempty"`
	QueueName       string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateTaskValue struct {
	QueueNameTaskId string `index:"0" json:",omitempty"`
	/*
		b.taskValue.split(a[1]).join(a[2]) // b = curr obj value
	*/
	TaskValue1 string `index:"1" json:",omitempty"`
	TaskValue2 string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSMailboxTaskCompletionApiOnTaskCompletion struct {
	TaskId  int64 `index:"0" json:",omitempty"`
	Success bool  `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSIssueNewTask struct {
	QueueName                string `index:"0" json:",omitempty"`
	Context                  int64  `index:"1" json:",omitempty"`
	TaskValue                string `index:"2" json:",omitempty"`
	HttpUrlOverride          string `index:"3" json:",omitempty"`
	TimeoutTimestampMs       int64  `index:"4" json:",omitempty"`
	PluginType               int64  `index:"5" json:",omitempty"`
	Priority                 int64  `index:"6" json:",omitempty"`
	SyncGroupId              int64  `index:"7" json:",omitempty"`
	TransportKey             int64  `index:"8" json:",omitempty"`
	MinTimeToSyncTimestampMs int64  `index:"9" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (l *LSIssueNewTask) GetLabel() string {
	return strconv.FormatInt(l.Context, 10)
}

func (l *LSIssueNewTask) Create() (any, any, bool) {
	return json.RawMessage(l.TaskValue), l.QueueName, false
}
