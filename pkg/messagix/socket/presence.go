package socket

type UpdatePresenceTask struct {
	ThreadKey     int64 `json:"thread_key,omitempty"`
	IsGroupThread int64 `json:"is_group_thread"`
	IsTyping      int64 `json:"is_typing"`
	Attribution   int64 `json:"attribution"`
	SyncGroup     int64 `json:"sync_group"`
	ThreadType    int64 `json:"thread_type"`
}

func (t *UpdatePresenceTask) GetLabel() string {
	return TaskLabels["UpdatePresence"]
}

func (t *UpdatePresenceTask) Create() (any, any, bool) {
	return t, nil, false
}
