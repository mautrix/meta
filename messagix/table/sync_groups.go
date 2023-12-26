package table

type LSTruncateTablesForSyncGroup struct {
	SyncGroup int64 `index:"0"`
}

type LSTruncateThreadRangeTablesForSyncGroup struct {
	ParentThreadKey int64 `index:"0"`
}

type LSUpsertSyncGroupThreadsRange struct {
	SyncGroup int64 `index:"0"`
	ParentThreadKey int64 `index:"1"`
	MinLastActivityTimestampMs int64 `index:"2"`
	HasMoreBefore bool `index:"3"`
	IsLoadingBefore bool `index:"4"`
	MinThreadKey int64 `index:"5"`
}

type LSMciTraceLog struct {
	SomeInt0 int64 `index:"0"`
	MCITraceUnsampledEventTraceId string `index:"1"`
	Unknown2 interface{} `index:"2"`
	SomeInt3 int64 `index:"3"`
	Unknown4 interface{} `index:"4"`
	DatascriptExecute string `index:"5"`
	SomeInt6 int64 `index:"6"`
}

type LSExecuteFirstBlockForSyncTransaction struct {
	DatabaseId int64 `index:"0"`
	EpochId int64 `index:"1"`
    CurrentCursor string `index:"2"`
	NextCursor string `index:"3"`
	SyncStatus int64 `index:"4"`
	SendSyncParams bool `index:"5"`
	MinTimeToSyncTimestampMs int64 `index:"6"` // fix this, use conditionIndex
	CanIgnoreTimestamp bool `index:"7"`
	SyncChannel int64 `index:"8"`
}

type LSExecuteFinallyBlockForSyncTransaction struct {
	ShouldFlush bool `index:"0"` // shouldFlush ? should sync ?
	SyncDatabaseId int64 `index:"1"`
	EpochId int64 `index:"2"`
}

type LSSetHMPSStatus struct {
	AccountId int64 `index:"0"`
	Unknown1 int64 `index:"1"`
	Timestamp int64 `index:"2"`
}

type LSUpsertSequenceId struct {
	LastAppliedMailboxSequenceId int64 `index:"0"`
}

type LSSetRegionHint struct {
	Unknown0 int64 `index:"0"`
	RegionHint string `index:"1"`
}

type LSUpsertGradientColor struct {
	ThemeFbid int64 `index:"0"`
	GradientIndex int64 `index:"1"`
	Color int64 `index:"2"`
	Type int64 `index:"3"`
}

type LSUpsertTheme struct {
    Fbid int64 `index:"0"`
    NormalThemeId int64 `index:"0"`
    ThemeIdx int64 `index:"1"`
    FallbackColor int64 `index:"2"`
    ReverseGradiantsForRadial bool `index:"3"`
    AccessibilityLabel string `index:"4"`
    IconUrl string `index:"5"`
    IconUrlFallback string `index:"6"`
    BackgroundUrl string `index:"8"`
    IsDeprecated bool `index:"11"`
    AppColorMode int64 `index:"13"`
    TitlebarBackgroundColor int64 `index:"14"`
    TitlebarButtonTintColor int64 `index:"15"`
    TitlebarTextColor int64 `index:"16"`
    ComposerTintColor int64 `index:"17"`
    ComposerUnselectedTintColor int64 `index:"18"`
    ComposerInputTextPlaceholderColor int64 `index:"19"`
    ComposerInputBackgroundColor int64 `index:"20"`
    ComposerInputBorderColor int64 `index:"21"`
    ComposerInputBorderWidth int64 `index:"22"`
    ComposerBackgroundColor int64 `index:"23"`
    MessageTextColor int64 `index:"24"`
    MessageBorderColor int64 `index:"25"`
    MessageBorderWidth int64 `index:"26"`
    IncomingMessageTextColor int64 `index:"27"`
    IncomingMessageBorderColor int64 `index:"28"`
    IncomingMessageBorderWidth int64 `index:"29"`
    DeliveryReceiptColor int64 `index:"30"`
    TertiaryTextColor int64 `index:"31"`
    PrimaryButtonBackgroundColor int64 `index:"32"`
    HotLikeColor int64 `index:"33"`
    SecondaryTextColor int64 `index:"34"`
    QuotedIncomingMessageBubbleColor int64 `index:"35"`
    CornerRadius int64 `index:"36"`
    BlurredComposerBackgroundColor int64 `index:"37"`
    ComposerSecondaryButtonColor int64 `index:"38"`
    ComposerPlaceholderTextColor int64 `index:"39"`
}

type LSAppendDataTraceAddon struct {
    TraceId string `index:"0"`
    CheckPointId int64 `index:"1"`
    SyncChannel int64 `index:"2"`
    ErrorMessage string `index:"3"`
    Tags string `index:"4"`
}

type LSTruncatePresenceDatabase struct {
    ShouldTruncate bool `index:"0"`
}