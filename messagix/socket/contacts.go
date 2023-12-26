package socket

import "github.com/0xzer/messagix/methods"

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
	ContactId int64 `json:"contact_id"`
}

func (t *GetContactsFullTask) GetLabel() string {
	return TaskLabels["GetContactsFullTask"]
}

func (t *GetContactsFullTask) Create() (interface{}, interface{}, bool) {
	queueName := "cpq_v2"
	return t, queueName, false
}