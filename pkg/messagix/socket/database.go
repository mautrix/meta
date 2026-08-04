package socket

const BusinessInboxFetchThreadsLabel = "313"

type SyncChannel int64

const (
	MailBox SyncChannel = 1
	Contact SyncChannel = 2
)

type QueryMetadata struct {
	DatabaseID        int64
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

func (t *FetchThreadsTask) Create() (any, string) {
	return t, "trq"
}

// FetchBusinessInboxThreadsTask is the thread-range request used by Meta
// Business Suite. It is intentionally separate from FetchThreadsTask: the
// latter addresses the personal Messenger mailbox (sync groups 1 and 95),
// while Business Suite routes Page and linked Instagram threads through
// business inbox sync groups with channel-specific secondary filters.
type FetchBusinessInboxThreadsTask struct {
	Cursor                     string `json:"cursor"`
	Filter                     int    `json:"filter"`
	FilterValue                string `json:"filter_value"`
	IsAfter                    int    `json:"is_after"`
	ParentThreadKey            int64  `json:"parent_thread_key"`
	ReferenceActivityTimestamp int64  `json:"reference_activity_timestamp"`
	ReferenceThreadKey         int64  `json:"reference_thread_key"`
	SecondaryFilter            int    `json:"secondary_filter"`
	SyncGroup                  int    `json:"sync_group"`
}

func (t *FetchBusinessInboxThreadsTask) GetLabel() string {
	return BusinessInboxFetchThreadsLabel
}

func (t *FetchBusinessInboxThreadsTask) Create() (any, string) {
	return t, "trq"
}
