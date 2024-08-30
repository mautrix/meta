package socket

import (
	"encoding/json"
)

type TaskPayload struct {
	EpochId     int64      `json:"epoch_id"`
	DataTraceId string     `json:"data_trace_id,omitempty"`
	Tasks       []TaskData `json:"tasks,omitempty"`
	VersionId   string     `json:"version_id"`
}

type DatabaseQuery struct {
	Database          int64       `json:"database"`
	LastAppliedCursor *string     `json:"last_applied_cursor"`
	SyncParams        *string     `json:"sync_params"`
	EpochId           int64       `json:"epoch_id"`
	DataTraceId       string      `json:"data_trace_id,omitempty"`
	Version           json.Number `json:"version"`
	FailureCount      interface{} `json:"failure_count"`
}

type SyncParams struct {
	Locale string `json:"locale"`
}
