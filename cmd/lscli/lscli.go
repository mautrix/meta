package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	badGlobalLog "github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
	"github.com/zyedidia/clipboard"
	"go.mau.fi/util/exerrors"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

var taskNames = make(map[string]string)

func init() {
	for name, id := range socket.TaskLabels {
		taskNames[id] = name
	}
}

func main() {
	badGlobalLog.Logger = zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = os.Stderr
		w.TimeFormat = time.RFC3339
	})).With().Timestamp().Logger()

	var data []byte
	if exerrors.Must(os.Stdin.Stat()).Mode()&os.ModeNamedPipe != 0 {
		data = exerrors.Must(io.ReadAll(os.Stdin))
	} else {
		exerrors.PanicIfNotNil(clipboard.Initialize())
		data = []byte(exerrors.Must(clipboard.ReadAll("clipboard")))
	}
	if !json.Valid(data) {
		data, _ = base64.StdEncoding.DecodeString(string(data))
		if bytes.Contains(data[:20], []byte("/ls_req")) || bytes.Contains(data[:20], []byte("/ls_resp")) {
			data = data[bytes.Index(data, []byte(`{"`)):]
		}
		if !json.Valid(data) {
			_, _ = fmt.Fprintln(os.Stderr, "invalid json")
			return
		}
	}

	var lsData *lightspeed.LightSpeedData
	var dependencies map[string]string
	if gjson.GetBytes(data, "payload").Exists() {
		if gjson.GetBytes(data, "app_id").Exists() {
			handleOutgoingRequest(data)
			return
		}
		var pbd *messagix.PublishResponseData
		exerrors.PanicIfNotNil(json.Unmarshal(data, &pbd))
		dependencies = table.SPToDepMap(pbd.Sp)
		exerrors.PanicIfNotNil(json.Unmarshal([]byte(pbd.Payload), &lsData))
	} else if gjson.GetBytes(data, "data.lightspeed_web_request_for_igd").Exists() {

	} else {
		_, _ = fmt.Fprintln(os.Stderr, "unsupported json")
		return
	}
	tbl := table.LSTable{}
	decoder := lightspeed.NewLightSpeedDecoder(dependencies, &tbl)
	decoder.Decode(lsData.Steps)
	exerrors.PanicIfNotNil(json.NewEncoder(os.Stdout).Encode(&tbl))
}

type formattedOutgoingRequest struct {
	AppID       string          `json:"app_id"`
	RequestID   int             `json:"request_id"`
	Type        int             `json:"type"`
	EpochID     int64           `json:"epoch_id"`
	DataTraceID string          `json:"data_trace_id"`
	VersionID   string          `json:"version_id"`
	Tasks       []formattedTask `json:"tasks"`

	DatabaseQuery *socket.DatabaseQuery `json:"database_query,omitempty"`
}

type formattedTask struct {
	FailureCount *int64          `json:"failure_count"`
	Label        string          `json:"label"`
	LabelName    string          `json:"label_name"`
	QueueName    any             `json:"queue_name"`
	TaskID       int64           `json:"task_id"`
	Payload      json.RawMessage `json:"payload"`
}

func handleOutgoingRequest(data json.RawMessage) {
	var payload messagix.SocketLSRequestPayload
	exerrors.PanicIfNotNil(json.Unmarshal(data, &payload))
	var taskPayload socket.TaskPayload
	exerrors.PanicIfNotNil(json.Unmarshal([]byte(payload.Payload), &taskPayload))
	if len(taskPayload.Tasks) == 0 {
		var dbQuery socket.DatabaseQuery
		exerrors.PanicIfNotNil(json.Unmarshal([]byte(payload.Payload), &dbQuery))
		exerrors.PanicIfNotNil(json.NewEncoder(os.Stdout).Encode(&formattedOutgoingRequest{
			AppID:         payload.AppId,
			RequestID:     payload.RequestId,
			Type:          payload.Type,
			EpochID:       taskPayload.EpochId,
			DataTraceID:   taskPayload.DataTraceId,
			VersionID:     taskPayload.VersionId,
			DatabaseQuery: &dbQuery,
		}))
		return
	}
	formattedTasks := make([]formattedTask, len(taskPayload.Tasks))
	for i, task := range taskPayload.Tasks {
		formattedTasks[i] = formattedTask{
			FailureCount: task.FailureCount,
			Label:        task.Label,
			LabelName:    taskNames[task.Label],
			QueueName:    task.QueueName,
			TaskID:       task.TaskId,
			Payload:      json.RawMessage(task.Payload.(string)),
		}
	}
	exerrors.PanicIfNotNil(json.NewEncoder(os.Stdout).Encode(&formattedOutgoingRequest{
		AppID:       payload.AppId,
		RequestID:   payload.RequestId,
		Type:        payload.Type,
		EpochID:     taskPayload.EpochId,
		DataTraceID: taskPayload.DataTraceId,
		VersionID:   taskPayload.VersionId,
		Tasks:       formattedTasks,
	}))
}
