package socket

import "github.com/0xzer/messagix/table"

type ReportAppStateTask struct {
	AppState table.AppState `json:"app_state"`
	RequestId string `json:"request_id"`
}

func (t *ReportAppStateTask) GetLabel() string {
	return TaskLabels["ReportAppStateTask"]
}

func (t *ReportAppStateTask) Create() (interface{}, interface{}, bool) {
	queueName := "ls_presence_report_app_state"
	return t, queueName, false
}