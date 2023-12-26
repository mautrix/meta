package socket

import (
	"time"

	"github.com/0xzer/messagix/table"
)

type SendReactionTask struct {
	ThreadKey       int64  `json:"thread_key,omitempty"`
	TimestampMs     int64  `json:"timestamp_ms"`
	MessageID       string `json:"message_id"`
	ActorID         int64  `json:"actor_id"`
	Reaction        string `json:"reaction"` // unicode emoji (empty reaction to remove)
	ReactionStyle   interface{}    `json:"reaction_style"`
	SyncGroup       int    `json:"sync_group"`
	SendAttribution table.ThreadSourceType    `json:"send_attribution"`
}

func (t *SendReactionTask) GetLabel() string {
	return TaskLabels["SendReactionTask"]
}

func (t *SendReactionTask) Create() (interface{}, interface{}, bool) {
	t.TimestampMs = time.Now().UnixMilli()
	t.SyncGroup = 1
	queueName := []string{"reaction", t.MessageID}
	return t, queueName, true
}

type DeleteMessageTask struct {
	MessageId string `json:"message_id"`
}

func (t *DeleteMessageTask) GetLabel() string {
	return TaskLabels["DeleteMessageTask"]
}

func (t *DeleteMessageTask) Create() (interface{}, interface{}, bool) {
	queueName := "unsend_message"
	return t, queueName, false
}

type DeleteMessageMeOnlyTask struct {
	ThreadKey int64 `json:"thread_key,omitempty"`
	MessageId string `json:"message_id"`
}

func (t *DeleteMessageMeOnlyTask) GetLabel() string {
	return TaskLabels["DeleteMessageMeOnlyTask"]
}

func (t *DeleteMessageMeOnlyTask) Create() (interface{}, interface{}, bool) {
	queueName := "155"
	return t, queueName, false
}