package socket

type SyncChannel int64

const (
	MailBox SyncChannel = 1
	Contact SyncChannel = 2
)

type QueryMetadata struct {
	DatabaseId        int64
	SendSyncParams    bool
	LastAppliedCursor *string
	SyncParams        interface{}
	SyncChannel
}

type KeyStoreData struct {
	ParentThreadKey            int64
	MinLastActivityTimestampMs int64
	HasMoreBefore              bool
	MinThreadKey               int64
}

type FetchThreadsTask struct {
	IsAfter                    int         `json:"is_after"`
	ParentThreadKey            int64       `json:"parent_thread_key"`
	ReferenceThreadKey         int64       `json:"reference_thread_key"`
	ReferenceActivityTimestamp int64       `json:"reference_activity_timestamp"`
	AdditionalPagesToFetch     int         `json:"additional_pages_to_fetch"`
	Cursor                     interface{} `json:"cursor"`
	MessagingTag               interface{} `json:"messaging_tag"`
	SyncGroup                  int         `json:"sync_group"`
}

func (t *FetchThreadsTask) GetLabel() string {
	return TaskLabels["FetchThreadsTask"]
}

func (t *FetchThreadsTask) Create() (interface{}, interface{}, bool) {
	queueName := "trq"
	return t, queueName, false
}
