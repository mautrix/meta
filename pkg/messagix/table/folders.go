package table

type LSUpsertFolder struct {
	ThreadKey                    int64  `index:"0" json:",omitempty"`
	ThreadName                   string `index:"1" json:",omitempty"`
	ThreadPictureURL             string `index:"2" json:",omitempty"`
	LastActivityTimestampMS      int64  `index:"3" json:",omitempty"`
	LastReadWatermarkTimestampMS int64  `index:"4" json:",omitempty"`
	// 5 might be parent thread last activity
	// 6 is unknown

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertFolderSeenTimestamp struct {
	ParentThreadKey            int64 `index:"0" json:",omitempty"`
	LastSeenRequestTimestampMs int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}
