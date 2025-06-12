package socket

import "go.mau.fi/mautrix-meta/pkg/messagix/table"

type ReportAppStateTask struct {
	AppState  table.AppState `json:"app_state"`
	RequestID string         `json:"request_id"`
}

func (t *ReportAppStateTask) GetLabel() string {
	return TaskLabels["ReportAppStateTask"]
}

func (t *ReportAppStateTask) Create() (interface{}, interface{}, bool) {
	queueName := "ls_presence_report_app_state"
	return t, queueName, false
}
