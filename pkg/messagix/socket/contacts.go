package socket

import (
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

type GetContactsTask struct {
	Limit int64 `json:"limit,omitempty"`
}

func (t *GetContactsTask) GetLabel() string {
	return TaskLabels["GetContactsTask"]
}

func (t *GetContactsTask) Create() (interface{}, interface{}, bool) {
	queueName := []string{"search_contacts", methods.GenerateTimestampString()}
	return t, queueName, false
}

type GetContactsFullTask struct {
	ContactID int64 `json:"contact_id"`
}

func (t *GetContactsFullTask) GetLabel() string {
	return TaskLabels["GetContactsFullTask"]
}

func (t *GetContactsFullTask) Create() (interface{}, interface{}, bool) {
	queueName := "cpq_v2"
	return t, queueName, false
}

type SearchUserTask struct {
	Query                string             `json:"query"`
	SupportedTypes       []table.SearchType `json:"supported_types"`       // [1,3,4,2,6,7,8,9] on instagram, also 13 on messenger
	SessionID            any                `json:"session_id"`            // null
	SurfaceType          int64              `json:"surface_type"`          // 15
	SelectedParticipants []int64            `json:"selected_participants"` // null
	GroupID              *int64             `json:"group_id"`              // null
	CommunityID          *int64             `json:"community_id"`          // null
	QueryID              *int64             `json:"query_id"`              // null

	Secondary bool `json:"-"`
}

func (t *SearchUserTask) GetLabel() string {
	if t.Secondary {
		return TaskLabels["SearchUserSecondaryTask"]
	}
	return TaskLabels["SearchUserTask"]
}

func (t *SearchUserTask) Create() (interface{}, interface{}, bool) {
	if t.Secondary {
		return t, "search_secondary", false
	}
	return t, []any{"search_primary", time.Now().UnixMilli()}, true
}
