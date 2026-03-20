package table

type LSUpsertFolder struct {
	ParentThreadKey int64      `index:"0" json:",omitempty"`
	ThreadType      ThreadType `index:"1" json:",omitempty"`
	FolderName      string     `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertFolderSeenTimestamp struct {
	ParentThreadKey            int64 `index:"0" json:",omitempty"`
	LastSeenRequestTimestampMs int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}
