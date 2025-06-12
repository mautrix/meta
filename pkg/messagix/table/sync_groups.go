package table

type LSTruncateTablesForSyncGroup struct {
	SyncGroup int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSTruncateThreadRangeTablesForSyncGroup struct {
	ParentThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertSyncGroupThreadsRange struct {
	SyncGroup                  int64 `index:"0" json:",omitempty"`
	ParentThreadKey            int64 `index:"1" json:",omitempty"`
	MinLastActivityTimestampMS int64 `index:"2" json:",omitempty"`
	HasMoreBefore              bool  `index:"3" json:",omitempty"`
	IsLoadingBefore            bool  `index:"4" json:",omitempty"`
	MinThreadKey               int64 `index:"5" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSMciTraceLog struct {
	SomeInt0                      int64       `index:"0" json:",omitempty"`
	MCITraceUnsampledEventTraceID string      `index:"1" json:",omitempty"`
	Unknown2                      interface{} `index:"2" json:",omitempty"`
	SomeInt3                      int64       `index:"3" json:",omitempty"`
	Unknown4                      interface{} `index:"4" json:",omitempty"`
	DatascriptExecute             string      `index:"5" json:",omitempty"`
	SomeInt6                      int64       `index:"6" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSExecuteFirstBlockForSyncTransaction struct {
	DatabaseID               int64  `index:"0" json:",omitempty"`
	EpochID                  int64  `index:"1" json:",omitempty"`
	CurrentCursor            string `index:"2" json:",omitempty"`
	NextCursor               string `index:"3" json:",omitempty"`
	SyncStatus               int64  `index:"4" json:",omitempty"`
	SendSyncParams           bool   `index:"5" json:",omitempty"`
	MinTimeToSyncTimestampMS int64  `index:"6" json:",omitempty"` // fix this, use conditionIndex
	CanIgnoreTimestamp       bool   `index:"7" json:",omitempty"`
	SyncChannel              int64  `index:"8" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSExecuteFinallyBlockForSyncTransaction struct {
	ShouldFlush    bool  `index:"0" json:",omitempty"` // shouldFlush ? should sync ?
	SyncDatabaseID int64 `index:"1" json:",omitempty"`
	EpochID        int64 `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSSetHMPSStatus struct {
	AccountId int64 `index:"0" json:",omitempty"`
	Unknown1  int64 `index:"1" json:",omitempty"`
	Timestamp int64 `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertSequenceID struct {
	LastAppliedMailboxSequenceId int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSSetRegionHint struct {
	Unknown0   int64  `index:"0" json:",omitempty"`
	RegionHint string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertGradientColor struct {
	ThemeFbid     int64 `index:"0" json:",omitempty"`
	GradientIndex int64 `index:"1" json:",omitempty"`
	Color         int64 `index:"2" json:",omitempty"`
	Type          int64 `index:"3" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertTheme struct {
	Fbid                              int64  `index:"0" json:",omitempty"`
	NormalThemeId                     int64  `index:"0" json:",omitempty"`
	ThemeIdx                          int64  `index:"1" json:",omitempty"`
	FallbackColor                     int64  `index:"2" json:",omitempty"`
	ReverseGradiantsForRadial         bool   `index:"3" json:",omitempty"`
	AccessibilityLabel                string `index:"4" json:",omitempty"`
	IconUrl                           string `index:"5" json:",omitempty"`
	IconUrlFallback                   string `index:"6" json:",omitempty"`
	BackgroundUrl                     string `index:"8" json:",omitempty"`
	IsDeprecated                      bool   `index:"11" json:",omitempty"`
	AppColorMode                      int64  `index:"13" json:",omitempty"`
	TitlebarBackgroundColor           int64  `index:"14" json:",omitempty"`
	TitlebarButtonTintColor           int64  `index:"15" json:",omitempty"`
	TitlebarTextColor                 int64  `index:"16" json:",omitempty"`
	ComposerTintColor                 int64  `index:"17" json:",omitempty"`
	ComposerUnselectedTintColor       int64  `index:"18" json:",omitempty"`
	ComposerInputTextPlaceholderColor int64  `index:"19" json:",omitempty"`
	ComposerInputBackgroundColor      int64  `index:"20" json:",omitempty"`
	ComposerInputBorderColor          int64  `index:"21" json:",omitempty"`
	ComposerInputBorderWidth          int64  `index:"22" json:",omitempty"`
	ComposerBackgroundColor           int64  `index:"23" json:",omitempty"`
	MessageTextColor                  int64  `index:"24" json:",omitempty"`
	MessageBorderColor                int64  `index:"25" json:",omitempty"`
	MessageBorderWidth                int64  `index:"26" json:",omitempty"`
	IncomingMessageTextColor          int64  `index:"27" json:",omitempty"`
	IncomingMessageBorderColor        int64  `index:"28" json:",omitempty"`
	IncomingMessageBorderWidth        int64  `index:"29" json:",omitempty"`
	DeliveryReceiptColor              int64  `index:"30" json:",omitempty"`
	TertiaryTextColor                 int64  `index:"31" json:",omitempty"`
	PrimaryButtonBackgroundColor      int64  `index:"32" json:",omitempty"`
	HotLikeColor                      int64  `index:"33" json:",omitempty"`
	SecondaryTextColor                int64  `index:"34" json:",omitempty"`
	QuotedIncomingMessageBubbleColor  int64  `index:"35" json:",omitempty"`
	CornerRadius                      int64  `index:"36" json:",omitempty"`
	BlurredComposerBackgroundColor    int64  `index:"37" json:",omitempty"`
	ComposerSecondaryButtonColor      int64  `index:"38" json:",omitempty"`
	ComposerPlaceholderTextColor      int64  `index:"39" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSAppendDataTraceAddon struct {
	TraceId      string `index:"0" json:",omitempty"`
	CheckPointId int64  `index:"1" json:",omitempty"`
	SyncChannel  int64  `index:"2" json:",omitempty"`
	ErrorMessage string `index:"3" json:",omitempty"`
	Tags         string `index:"4" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSTruncatePresenceDatabase struct {
	ShouldTruncate bool `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateLastSyncCompletedTimestampMsToNow struct {
	Unknown0 int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSStoryContactSyncFromBucket struct {
	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteThenInsertBotProfileInfoCategoryV2 struct {
	Category      string  `index:"0" json:",omitempty"`
	BotID         int64   `index:"1" json:",omitempty"`
	UnknownFloat2 float64 `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteThenInsertBotProfileInfoV2 struct {
	BotID             int64  `index:"0" json:",omitempty"`
	IsCreatedByViewer bool   `index:"1" json:",omitempty"`
	TintColor         int64  `index:"2" json:",omitempty"`
	ShortDescription  string `index:"3" json:",omitempty"`
	Description       string `index:"4" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSHandleSyncFailure struct {
	DatabaseID   int64  `index:"0" json:",omitempty"`
	ErrorMessage string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}
