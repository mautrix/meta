package table

type LSUpsertFolderSeenTimestamp struct {
	ParentThreadKey            int64 `index:"0" json:",omitempty"`
	LastSeenRequestTimestampMs int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}
