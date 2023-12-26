package table

type LSUpsertFolderSeenTimestamp struct {
	ParentThreadKey int64 `index:"0"`
	LastSeenRequestTimestampMs int64 `index:"1"`
}