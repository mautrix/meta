package socket

/*
	type 3 = task
*/

var TaskLabels = map[string]string{
	"GetContactsTask": "452",
	"SendMessageTask": "46",
	"ThreadMarkRead": "21",
	"GetContactsFullTask": "207",
	"ReportAppStateTask": "123",
	"FetchThreadsTask": "145",
	"FetchMessagesTask": "228",
	"SendReactionTask": "29",
	"DeleteMessageTask": "33",
	"DeleteMessageMeOnlyTask": "155",
}

type Task interface {
	GetLabel() string
	Create() (interface{}, interface{}, bool) // payload, queue_name, marshal_queuename
}

type TaskData struct {
	FailureCount interface{} `json:"failure_count"`
	Label string `json:"label,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
	QueueName interface{} `json:"queue_name,omitempty"`
	TaskId int64 `json:"task_id"`
}