package socket

import (
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

type SendReactionTask struct {
	ThreadKey       int64                  `json:"thread_key,omitempty"`
	TimestampMs     int64                  `json:"timestamp_ms"`
	MessageID       string                 `json:"message_id"`
	ActorID         int64                  `json:"actor_id"`
	Reaction        string                 `json:"reaction"` // unicode emoji (empty reaction to remove)
	ReactionStyle   interface{}            `json:"reaction_style"`
	SyncGroup       int                    `json:"sync_group"`
	SendAttribution table.ThreadSourceType `json:"send_attribution"`
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
	ThreadKey int64  `json:"thread_key,omitempty"`
	MessageId string `json:"message_id"`
}

func (t *DeleteMessageMeOnlyTask) GetLabel() string {
	return TaskLabels["DeleteMessageMeOnlyTask"]
}

func (t *DeleteMessageMeOnlyTask) Create() (interface{}, interface{}, bool) {
	queueName := "155"
	return t, queueName, false
}

type FetchReactionsV2UserList struct {
	ThreadID     int64   `json:"thread_id"`
	MessageID    string  `json:"message_id"`
	ReactionFBID *int64  `json:"reaction_fbid"`
	Cursor       *string `json:"cursor"`
	SyncGroup    int64   `json:"sync_group"`
}

func (t *FetchReactionsV2UserList) GetLabel() string {
	return TaskLabels["FetchReactionsV2UserList"]
}

func (t *FetchReactionsV2UserList) Create() (any, any, bool) {
	return t, "fetch_reactions_v2_details_users_list", false
}

type SendReactionV2Task struct {
	ThreadID         int64  `json:"thread_id"`
	MessageID        string `json:"message_id"`
	MessageTimestamp int64  `json:"message_timestamp"`
	ActorID          int64  `json:"actor_id"`
	ReactionFBID     int64  `json:"reaction_fbid"`
	ReactionStyle    int    `json:"reaction_style"` // 1
	CurrentCount     int    `json:"current_count"`
	ViewerIsReactor  int    `json:"viewer_is_reactor"` // 1 if adding reaction, 0 if removing reaction
	Operation        int    `json:"operation"`         // 1 for add, 3 for remove
	ReactionLiteral  string `json:"reaction_literal"`  // unicode emoji
	EntryPoint       any    `json:"entry_point"`       // null
	SyncGroup        int    `json:"sync_group"`        // 104
}

func (t *SendReactionV2Task) GetLabel() string {
	return TaskLabels["SendReactionV2"]
}

func (t *SendReactionV2Task) Create() (any, any, bool) {
	return t, []string{"reaction_v2", t.MessageID}, true
}
