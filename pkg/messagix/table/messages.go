package table

/*
Instructs the client to clear pinned messages (delete by ThreadKey)
*/
type LSClearPinnedMessages struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertMessage struct {
	Text                            string                     `index:"0" json:",omitempty"`
	SubscriptErrorMessage           string                     `index:"1" json:",omitempty"`
	AuthorityLevel                  int64                      `index:"2" json:",omitempty"`
	ThreadKey                       int64                      `index:"3" json:",omitempty"`
	TimestampMs                     int64                      `index:"5" json:",omitempty"`
	PrimarySortKey                  int64                      `index:"6" json:",omitempty"`
	SecondarySortKey                int64                      `index:"7" json:",omitempty"`
	MessageId                       string                     `index:"8" json:",omitempty"`
	OfflineThreadingId              string                     `index:"9" json:",omitempty"`
	SenderId                        int64                      `index:"10" json:",omitempty"`
	StickerId                       int64                      `index:"11" json:",omitempty"`
	IsAdminMessage                  bool                       `index:"12" json:",omitempty"`
	MessageRenderingType            int64                      `index:"13" json:",omitempty"`
	SendStatus                      int64                      `index:"15" json:",omitempty"`
	SendStatusV2                    int64                      `index:"16" json:",omitempty"`
	IsUnsent                        bool                       `index:"17" json:",omitempty"`
	UnsentTimestampMs               int64                      `index:"18" json:",omitempty"`
	MentionOffsets                  string                     `index:"19" json:",omitempty"`
	MentionLengths                  string                     `index:"20" json:",omitempty"`
	MentionIds                      string                     `index:"21" json:",omitempty"`
	MentionTypes                    string                     `index:"22" json:",omitempty"`
	ReplySourceId                   string                     `index:"23" json:",omitempty"`
	ReplySourceType                 int64                      `index:"24" json:",omitempty"`
	ReplySourceTypeV2               ReplySourceTypeV2          `index:"25" json:",omitempty"`
	ReplyStatus                     int64                      `index:"26" json:",omitempty"`
	ReplySnippet                    string                     `index:"27" json:",omitempty"`
	ReplyMessageText                string                     `index:"28" json:",omitempty"`
	ReplyToUserId                   int64                      `index:"29" json:",omitempty"`
	ReplyMediaExpirationTimestampMs int64                      `index:"30" json:",omitempty"`
	ReplyMediaUrl                   string                     `index:"31" json:",omitempty"`
	ReplyMediaUnknownTimestampS     int64                      `index:"32" json:",omitempty"`
	ReplyMediaPreviewWidth          int64                      `index:"33" json:",omitempty"`
	ReplyMediaPreviewHeight         int64                      `index:"34" json:",omitempty"`
	ReplyMediaUrlMimeType           string                     `index:"35" json:",omitempty"`
	ReplyMediaUrlFallback           string                     `index:"36" json:",omitempty"`
	ReplyCtaId                      int64                      `index:"37" json:",omitempty"`
	ReplyCtaTitle                   string                     `index:"38" json:",omitempty"`
	ReplyAttachmentType             AttachmentType             `index:"39" json:",omitempty"`
	ReplyAttachmentId               int64                      `index:"40" json:",omitempty"`
	ReplyAttachmentExtra            string                     `index:"41" json:",omitempty"`
	ReplyType                       int64                      `index:"42" json:",omitempty"`
	IsForwarded                     bool                       `index:"43" json:",omitempty"`
	ForwardScore                    int64                      `index:"44" json:",omitempty"`
	HasQuickReplies                 bool                       `index:"45" json:",omitempty"`
	AdminMsgCtaId                   int64                      `index:"46" json:",omitempty"`
	AdminMsgCtaTitle                string                     `index:"47" json:",omitempty"`
	AdminMsgCtaType                 string                     `index:"48" json:",omitempty"`
	CannotUnsendReason              MessageUnsendabilityStatus `index:"49" json:",omitempty"`
	TextHasLinks                    bool                       `index:"50" json:",omitempty"`
	ViewFlags                       int64                      `index:"51" json:",omitempty"`
	DisplayedContentTypes           DisplayedContentTypes      `index:"52" json:",omitempty"`
	ViewedPluginKey                 int64                      `index:"53" json:",omitempty"`
	ViewedPluginContext             int64                      `index:"54" json:",omitempty"`
	QuickReplyType                  int64                      `index:"55" json:",omitempty"`
	HotEmojiSize                    int64                      `index:"56" json:",omitempty"`
	PlatformXmdEncoded              any                        `index:"57" json:",omitempty"`
	SendAttributionXmd              any                        `index:"58" json:",omitempty"`
	ReplySourceTimestampMs          int64                      `index:"59" json:",omitempty"`
	EphemeralDurationInSec          int64                      `index:"60" json:",omitempty"`
	MsUntilExpirationTs             int64                      `index:"61" json:",omitempty"`
	EphemeralExpirationTs           int64                      `index:"62" json:",omitempty"`
	TakedownState                   any                        `index:"63" json:",omitempty"` // bool?
	IsCollapsed                     bool                       `index:"64" json:",omitempty"`
	SubthreadKey                    int64                      `index:"65" json:",omitempty"`
	BotResponseID                   int64                      `index:"66" json:",omitempty"`
	MetadataDataclass               any                        `index:"67" json:",omitempty"` // int?
	EditCount                       int64                      `index:"68" json:",omitempty"`
	IsPaidPartnership               any                        `index:"69" json:",omitempty"`
	MessageHiddenState              any                        `index:"70" json:",omitempty"`
	AdminSignatureName              any                        `index:"71" json:",omitempty"`
	AdminSignatureProfileURL        string                     `index:"72" json:",omitempty"`
	AdminSignatureCreatorType       any                        `index:"73" json:",omitempty"`
	ReplyMessageHiddenState         any                        `index:"74" json:",omitempty"`
	TranslatedText                  string                     `index:"75" json:",omitempty"`
	TextDialect                     any                        `index:"76" json:",omitempty"`
	TranslatedTextDialect           any                        `index:"77" json:",omitempty"`
	ScheduledTimestamp              int64                      `index:"78" json:",omitempty"`
	IsVideoQuickSend                bool                       `index:"79" json:",omitempty"`
	SubthreadParentMessageID        string                     `index:"80" json:",omitempty"`
	IsExclusiveToSubthread          bool                       `index:"81" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (um *LSUpsertMessage) ToInsert() *LSInsertMessage {
	return &LSInsertMessage{
		Text:                            um.Text,
		SubscriptErrorMessage:           um.SubscriptErrorMessage,
		AuthorityLevel:                  um.AuthorityLevel,
		ThreadKey:                       um.ThreadKey,
		TimestampMs:                     um.TimestampMs,
		PrimarySortKey:                  um.PrimarySortKey,
		SecondarySortKey:                um.SecondarySortKey,
		MessageId:                       um.MessageId,
		OfflineThreadingId:              um.OfflineThreadingId,
		SenderId:                        um.SenderId,
		StickerId:                       um.StickerId,
		IsAdminMessage:                  um.IsAdminMessage,
		MessageRenderingType:            um.MessageRenderingType,
		SendStatus:                      um.SendStatus,
		SendStatusV2:                    um.SendStatusV2,
		IsUnsent:                        um.IsUnsent,
		UnsentTimestampMs:               um.UnsentTimestampMs,
		MentionOffsets:                  um.MentionOffsets,
		MentionLengths:                  um.MentionLengths,
		MentionIds:                      um.MentionIds,
		MentionTypes:                    um.MentionTypes,
		ReplySourceId:                   um.ReplySourceId,
		ReplySourceType:                 um.ReplySourceType,
		ReplySourceTypeV2:               um.ReplySourceTypeV2,
		ReplyStatus:                     um.ReplyStatus,
		ReplySnippet:                    um.ReplySnippet,
		ReplyMessageText:                um.ReplyMessageText,
		ReplyToUserId:                   um.ReplyToUserId,
		ReplyMediaExpirationTimestampMs: um.ReplyMediaExpirationTimestampMs,
		ReplyMediaUrl:                   um.ReplyMediaUrl,
		ReplyMediaUnknownTimestampS:     um.ReplyMediaUnknownTimestampS,
		ReplyMediaPreviewWidth:          um.ReplyMediaPreviewWidth,
		ReplyMediaPreviewHeight:         um.ReplyMediaPreviewHeight,
		ReplyMediaUrlMimeType:           um.ReplyMediaUrlMimeType,
		ReplyMediaUrlFallback:           um.ReplyMediaUrlFallback,
		ReplyCtaId:                      um.ReplyCtaId,
		ReplyCtaTitle:                   um.ReplyCtaTitle,
		ReplyAttachmentType:             um.ReplyAttachmentType,
		ReplyAttachmentId:               um.ReplyAttachmentId,
		ReplyAttachmentExtra:            um.ReplyAttachmentExtra,
		ReplyType:                       um.ReplyType,
		IsForwarded:                     um.IsForwarded,
		ForwardScore:                    um.ForwardScore,
		HasQuickReplies:                 um.HasQuickReplies,
		AdminMsgCtaId:                   um.AdminMsgCtaId,
		AdminMsgCtaTitle:                um.AdminMsgCtaTitle,
		AdminMsgCtaType:                 um.AdminMsgCtaType,
		CannotUnsendReason:              um.CannotUnsendReason,
		TextHasLinks:                    um.TextHasLinks,
		ViewFlags:                       um.ViewFlags,
		DisplayedContentTypes:           um.DisplayedContentTypes,
		ViewedPluginKey:                 um.ViewedPluginKey,
		ViewedPluginContext:             um.ViewedPluginContext,
		QuickReplyType:                  um.QuickReplyType,
		HotEmojiSize:                    um.HotEmojiSize,
		PlatformXmdEncoded:              um.PlatformXmdEncoded,
		ReplySourceTimestampMs:          um.ReplySourceTimestampMs,
		EphemeralDurationInSec:          um.EphemeralDurationInSec,
		MsUntilExpirationTs:             um.MsUntilExpirationTs,
		EphemeralExpirationTs:           um.EphemeralExpirationTs,
		TakedownState:                   um.TakedownState,
		IsCollapsed:                     um.IsCollapsed,
		SubthreadKey:                    um.SubthreadKey,
		BotResponseID:                   um.BotResponseID,
		EditCount:                       um.EditCount,
		IsPaidPartnership:               um.IsPaidPartnership,
		AdminSignatureName:              um.AdminSignatureName,
		AdminSignatureProfileURL:        um.AdminSignatureProfileURL,
		AdminSignatureCreatorType:       um.AdminSignatureCreatorType,
		ReplyMessageHiddenState:         um.ReplyMessageHiddenState,
		TranslatedText:                  um.TranslatedText,
		TextDialect:                     um.TextDialect,
		TranslatedTextDialect:           um.TranslatedTextDialect,
		ScheduledTimestamp:              um.ScheduledTimestamp,
		IsVideoQuickSend:                um.IsVideoQuickSend,
		SubthreadParentMessageID:        um.SubthreadParentMessageID,
		IsExclusiveToSubthread:          um.IsExclusiveToSubthread,
	}
}

type LSDeleteMessage struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	MessageId string `index:"1" json:",omitempty"`
}

func (ls *LSDeleteMessage) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSHandleRepliesOnRemove struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	MessageId string `index:"1" json:",omitempty"`
}

type LSRefreshLastActivityTimestamp struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
}

type LSSetPinnedMessage struct {
	ThreadKey         int64  `index:"0" json:",omitempty"`
	MessageId         string `index:"1" json:",omitempty"`
	PinnedTimestampMs int64  `index:"2" json:",omitempty"`
	AuthorityLevel    int64  `index:"3" json:",omitempty"`
}

type LSSetForwardScore struct {
	ThreadKey    int64  `index:"0" json:",omitempty"`
	MessageId    string `index:"1" json:",omitempty"`
	TimestampMs  int64  `index:"2" json:",omitempty"`
	ForwardScore int64  `index:"3" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSSetMessageDisplayedContentTypes struct {
	ThreadKey   int64  `index:"0" json:",omitempty"`
	MessageId   string `index:"1" json:",omitempty"`
	TimestampMs int64  `index:"2" json:",omitempty"`
	Text        string `index:"3" json:",omitempty"`
	Calc1       bool   `index:"4" json:",omitempty"`
	Calc2       bool   `index:"5" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSInsertNewMessageRange struct {
	ThreadKey              int64       `index:"0" json:",omitempty"`
	MinTimestampMsTemplate int64       `index:"1" json:",omitempty"`
	MaxTimestampMsTemplate int64       `index:"2" json:",omitempty"`
	MinMessageId           string      `index:"3" json:",omitempty"`
	MaxMessageId           string      `index:"4" json:",omitempty"`
	MinTimestampMs         int64       `index:"5" json:",omitempty"`
	MaxTimestampMs         int64       `index:"6" json:",omitempty"`
	HasMoreBefore          bool        `index:"7" json:",omitempty"`
	HasMoreAfter           bool        `index:"8" json:",omitempty"`
	Unknown                interface{} `index:"9" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateExistingMessageRange struct {
	ThreadKey   int64 `index:"0" json:",omitempty"`
	TimestampMS int64 `index:"1" json:",omitempty"`

	UnknownBool2 bool `index:"2" json:",omitempty"`
	UnknownBool3 bool `index:"3" json:",omitempty"`

	// if bool 2 && !3 then clear "has more after" else clear "has more before"
}

func (ls *LSUpdateExistingMessageRange) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSDeleteExistingMessageRanges struct {
	ConsistentThreadFbid int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSCheckAuthoritativeMessageExists struct {
	ThreadKey          int64  `index:"0" json:",omitempty"`
	OfflineThreadingId string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateParticipantLastMessageSendTimestamp struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	SenderId  int64 `index:"1" json:",omitempty"`
	Timestamp int64 `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSInsertMessage struct {
	Text                            string                     `index:"0" json:",omitempty"`
	SubscriptErrorMessage           string                     `index:"1" json:",omitempty"`
	AuthorityLevel                  int64                      `index:"2" json:",omitempty"`
	ThreadKey                       int64                      `index:"3" json:",omitempty"`
	TimestampMs                     int64                      `index:"5" json:",omitempty"`
	PrimarySortKey                  int64                      `index:"6" json:",omitempty"`
	SecondarySortKey                int64                      `index:"7" json:",omitempty"`
	MessageId                       string                     `index:"8" json:",omitempty"`
	OfflineThreadingId              string                     `index:"9" json:",omitempty"`
	SenderId                        int64                      `index:"10" json:",omitempty"`
	StickerId                       int64                      `index:"11" json:",omitempty"`
	IsAdminMessage                  bool                       `index:"12" json:",omitempty"`
	MessageRenderingType            int64                      `index:"13" json:",omitempty"`
	SendStatus                      int64                      `index:"15" json:",omitempty"`
	SendStatusV2                    int64                      `index:"16" json:",omitempty"`
	IsUnsent                        bool                       `index:"17" json:",omitempty"`
	UnsentTimestampMs               int64                      `index:"18" json:",omitempty"`
	MentionOffsets                  string                     `index:"19" json:",omitempty"`
	MentionLengths                  string                     `index:"20" json:",omitempty"`
	MentionIds                      string                     `index:"21" json:",omitempty"`
	MentionTypes                    string                     `index:"22" json:",omitempty"`
	ReplySourceId                   string                     `index:"23" json:",omitempty"`
	ReplySourceType                 int64                      `index:"24" json:",omitempty"`
	ReplySourceTypeV2               ReplySourceTypeV2          `index:"25" json:",omitempty"`
	ReplyStatus                     int64                      `index:"26" json:",omitempty"`
	ReplySnippet                    string                     `index:"27" json:",omitempty"`
	ReplyMessageText                string                     `index:"28" json:",omitempty"`
	ReplyToUserId                   int64                      `index:"29" json:",omitempty"`
	ReplyMediaExpirationTimestampMs int64                      `index:"30" json:",omitempty"`
	ReplyMediaUrl                   string                     `index:"31" json:",omitempty"`
	ReplyMediaUnknownTimestampS     int64                      `index:"32" json:",omitempty"`
	ReplyMediaPreviewWidth          int64                      `index:"33" json:",omitempty"`
	ReplyMediaPreviewHeight         int64                      `index:"34" json:",omitempty"`
	ReplyMediaUrlMimeType           string                     `index:"35" json:",omitempty"`
	ReplyMediaUrlFallback           string                     `index:"36" json:",omitempty"`
	ReplyCtaId                      int64                      `index:"37" json:",omitempty"`
	ReplyCtaTitle                   string                     `index:"38" json:",omitempty"`
	ReplyAttachmentType             AttachmentType             `index:"39" json:",omitempty"`
	ReplyAttachmentId               int64                      `index:"40" json:",omitempty"`
	ReplyAttachmentExtra            string                     `index:"41" json:",omitempty"`
	ReplyType                       any                        `index:"42" json:",omitempty"`
	IsForwarded                     bool                       `index:"43" json:",omitempty"`
	ForwardScore                    int64                      `index:"44" json:",omitempty"`
	HasQuickReplies                 bool                       `index:"45" json:",omitempty"`
	AdminMsgCtaId                   int64                      `index:"46" json:",omitempty"`
	AdminMsgCtaTitle                string                     `index:"47" json:",omitempty"`
	AdminMsgCtaType                 string                     `index:"48" json:",omitempty"`
	CannotUnsendReason              MessageUnsendabilityStatus `index:"49" json:",omitempty"`
	TextHasLinks                    bool                       `index:"50" json:",omitempty"`
	ViewFlags                       int64                      `index:"51" json:",omitempty"`
	DisplayedContentTypes           DisplayedContentTypes      `index:"52" json:",omitempty"`
	ViewedPluginKey                 int64                      `index:"53" json:",omitempty"`
	ViewedPluginContext             int64                      `index:"54" json:",omitempty"`
	QuickReplyType                  int64                      `index:"55" json:",omitempty"`
	HotEmojiSize                    int64                      `index:"56" json:",omitempty"`
	PlatformXmdEncoded              any                        `index:"57" json:",omitempty"`
	SendAttributionXmd              any                        `index:"58" json:",omitempty"`
	ReplySourceTimestampMs          int64                      `index:"59" json:",omitempty"`
	EphemeralDurationInSec          int64                      `index:"60" json:",omitempty"`
	MsUntilExpirationTs             int64                      `index:"61" json:",omitempty"`
	EphemeralExpirationTs           int64                      `index:"62" json:",omitempty"`
	TakedownState                   any                        `index:"63" json:",omitempty"`
	IsCollapsed                     bool                       `index:"64" json:",omitempty"`
	SubthreadKey                    int64                      `index:"65" json:",omitempty"`
	BotResponseID                   int64                      `index:"66" json:",omitempty"`
	MetadataDataclass               any                        `index:"67" json:",omitempty"`
	EditCount                       int64                      `index:"68" json:",omitempty"`
	IsPaidPartnership               any                        `index:"69" json:",omitempty"`
	AdminSignatureName              any                        `index:"70" json:",omitempty"`
	AdminSignatureProfileURL        string                     `index:"71" json:",omitempty"`
	AdminSignatureCreatorType       any                        `index:"72" json:",omitempty"`
	ReplyMessageHiddenState         any                        `index:"73" json:",omitempty"`
	TranslatedText                  string                     `index:"74" json:",omitempty"`
	TextDialect                     any                        `index:"75" json:",omitempty"`
	TranslatedTextDialect           any                        `index:"76" json:",omitempty"`
	ScheduledTimestamp              int64                      `index:"77" json:",omitempty"`
	IsVideoQuickSend                bool                       `index:"78" json:",omitempty"`
	SubthreadParentMessageID        string                     `index:"79" json:",omitempty"`
	IsExclusiveToSubthread          bool                       `index:"80" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSInsertMessage) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSUpsertReaction struct {
	ThreadKey      int64  `index:"0" json:",omitempty"`
	TimestampMs    int64  `index:"1" json:",omitempty"`
	MessageId      string `index:"2" json:",omitempty"`
	ActorId        int64  `index:"3" json:",omitempty"`
	Reaction       string `index:"4" json:",omitempty"` // unicode str
	AuthorityLevel int64  `index:"5" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSUpsertReaction) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSUpdateOrInsertReactionV2 struct {
	ThreadKey                 int64  `index:"0" json:",omitempty"`
	MessageID                 string `index:"1" json:",omitempty"`
	ReactionFBID              int64  `index:"2" json:",omitempty"`
	ReactionLiteral           string `index:"3" json:",omitempty"`
	Count                     int64  `index:"4" json:",omitempty"`
	AuthorityLevel            int64  `index:"5" json:",omitempty"`
	ViewerIsReactor           bool   `index:"6" json:",omitempty"`
	ViewerReactionTimestampMS int64  `index:"7" json:",omitempty"`
	LastUpdatedTimestampMS    int64  `index:"8" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSUpdateOrInsertReactionV2) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSDeleteReactionV2 struct {
	ThreadKey              int64  `index:"0" json:",omitempty"`
	MessageID              string `index:"1" json:",omitempty"`
	ReactionFBID           int64  `index:"2" json:",omitempty"`
	LastUpdatedTimestampMS int64  `index:"3" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSDeleteReactionV2) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSDeleteThenInsertReactionsV2Detail struct {
	ThreadID                               int64  `index:"0" json:",omitempty"`
	MessageID                              string `index:"1" json:",omitempty"`
	ReactorID                              int64  `index:"2" json:",omitempty"`
	ReactionFBID                           int64  `index:"3" json:",omitempty"`
	FullName                               string `index:"4" json:",omitempty"`
	ProfilePictureURL                      string `index:"5" json:",omitempty"`
	ProfilePictureFallbackURL              string `index:"6" json:",omitempty"`
	ProfilePictureURLExpirationTimestampMS int64  `index:"7" json:",omitempty"`
	TimestampMS                            int64  `index:"8" json:",omitempty"`
	SecondaryName                          string `index:"9" json:",omitempty"`
}

func (ls *LSDeleteThenInsertReactionsV2Detail) GetThreadKey() int64 {
	return ls.ThreadID
}

type LSDeleteReaction struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	MessageId string `index:"1" json:",omitempty"`
	ActorId   int64  `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSDeleteReaction) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSHandleRepliesOnUnsend struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	MessageId string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSHandleRepliesOnMessageEdit struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	MessageId string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateForRollCallMessageDeleted struct {
	MessageId     string `index:"0" json:",omitempty"`
	ContributorId int64  `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateUnsentMessageCollapsedStatus struct {
	ThreadKey   int64  `index:"0" json:",omitempty"`
	MessageId   string `index:"1" json:",omitempty"`
	TimestampMs int64  `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateOrInsertEditMessageHistory struct {
	OriginalMessageID             string `index:"0" json:",omitempty"`
	ThreadKey                     int64  `index:"1" json:",omitempty"`
	ServerAdjustedEditTimestampMS int64  `index:"2" json:",omitempty"`
	MessageContent                string `index:"3" json:",omitempty"`
	SendStatus                    int64  `index:"4" json:",omitempty"`
}

type LSDeleteThenInsertMessage struct {
	Text                            string                     `index:"0" json:",omitempty"`
	SubscriptErrorMessage           string                     `index:"1" json:",omitempty"`
	AuthorityLevel                  int64                      `index:"2" json:",omitempty"`
	ThreadKey                       int64                      `index:"3" json:",omitempty"`
	TimestampMs                     int64                      `index:"5" json:",omitempty"`
	PrimarySortKey                  int64                      `index:"6" json:",omitempty"`
	SecondarySortKey                int64                      `index:"7" json:",omitempty"`
	MessageId                       string                     `index:"8" json:",omitempty"`
	OfflineThreadingId              string                     `index:"9" json:",omitempty"`
	SenderId                        int64                      `index:"10" json:",omitempty"`
	StickerId                       int64                      `index:"11" json:",omitempty"`
	IsAdminMessage                  bool                       `index:"12" json:",omitempty"`
	MessageRenderingType            int64                      `index:"13" json:",omitempty"`
	SendStatus                      int64                      `index:"15" json:",omitempty"`
	SendStatusV2                    int64                      `index:"16" json:",omitempty"`
	IsUnsent                        bool                       `index:"17" json:",omitempty"`
	UnsentTimestampMs               int64                      `index:"18" json:",omitempty"`
	MentionOffsets                  string                     `index:"19" json:",omitempty"`
	MentionLengths                  string                     `index:"20" json:",omitempty"`
	MentionIds                      string                     `index:"21" json:",omitempty"`
	MentionTypes                    string                     `index:"22" json:",omitempty"`
	ReplySourceId                   string                     `index:"23" json:",omitempty"`
	ReplySourceType                 int64                      `index:"24" json:",omitempty"`
	ReplySourceTypeV2               ReplySourceTypeV2          `index:"25" json:",omitempty"`
	ReplyStatus                     int64                      `index:"26" json:",omitempty"`
	ReplySnippet                    string                     `index:"27" json:",omitempty"`
	ReplyMessageText                string                     `index:"28" json:",omitempty"`
	ReplyToUserId                   int64                      `index:"29" json:",omitempty"`
	ReplyMediaExpirationTimestampMs int64                      `index:"30" json:",omitempty"`
	ReplyMediaUrl                   string                     `index:"31" json:",omitempty"`
	ReplyMediaUnknownTimestampS     int64                      `index:"32" json:",omitempty"`
	ReplyMediaPreviewWidth          int64                      `index:"33" json:",omitempty"`
	ReplyMediaPreviewHeight         int64                      `index:"34" json:",omitempty"`
	ReplyMediaUrlMimeType           string                     `index:"35" json:",omitempty"`
	ReplyMediaUrlFallback           string                     `index:"36" json:",omitempty"`
	ReplyCtaId                      int64                      `index:"37" json:",omitempty"`
	ReplyCtaTitle                   string                     `index:"38" json:",omitempty"`
	ReplyAttachmentType             AttachmentType             `index:"39" json:",omitempty"`
	ReplyAttachmentId               int64                      `index:"40" json:",omitempty"`
	ReplyAttachmentExtra            string                     `index:"41" json:",omitempty"`
	IsForwarded                     bool                       `index:"42" json:",omitempty"`
	ForwardScore                    int64                      `index:"43" json:",omitempty"`
	HasQuickReplies                 bool                       `index:"44" json:",omitempty"`
	AdminMsgCtaId                   int64                      `index:"45" json:",omitempty"`
	AdminMsgCtaTitle                string                     `index:"46" json:",omitempty"`
	AdminMsgCtaType                 string                     `index:"47" json:",omitempty"`
	CannotUnsendReason              MessageUnsendabilityStatus `index:"48" json:",omitempty"`
	TextHasLinks                    bool                       `index:"49" json:",omitempty"`
	ViewFlags                       int64                      `index:"50" json:",omitempty"`
	DisplayedContentTypes           DisplayedContentTypes      `index:"51" json:",omitempty"`
	ViewedPluginKey                 int64                      `index:"52" json:",omitempty"`
	ViewedPluginContext             int64                      `index:"53" json:",omitempty"`
	QuickReplyType                  int64                      `index:"54" json:",omitempty"`
	HotEmojiSize                    int64                      `index:"55" json:",omitempty"`
	PlatformXmdEncoded              any                        `index:"56" json:",omitempty"`
	SendAttributionXmd              any                        `index:"57" json:",omitempty"`
	ReplySourceTimestampMs          int64                      `index:"58" json:",omitempty"`
	EphemeralDurationInSec          int64                      `index:"59" json:",omitempty"`
	MsUntilExpirationTs             int64                      `index:"60" json:",omitempty"`
	EphemeralExpirationTs           int64                      `index:"61" json:",omitempty"`
	TakedownState                   any                        `index:"62" json:",omitempty"`
	IsCollapsed                     bool                       `index:"63" json:",omitempty"`
	SubthreadKey                    int64                      `index:"64" json:",omitempty"`
	BotResponseID                   int64                      `index:"65" json:",omitempty"`
	MetadataDataclass               any                        `index:"66" json:",omitempty"`
	EditCount                       int64                      `index:"67" json:",omitempty"`
	IsPaidPartnership               any                        `index:"68" json:",omitempty"`
	AdminSignatureName              any                        `index:"69" json:",omitempty"`
	AdminSignatureProfileURL        string                     `index:"70" json:",omitempty"`
	AdminSignatureCreatorType       any                        `index:"71" json:",omitempty"`
	ReplyMessageHiddenState         any                        `index:"72" json:",omitempty"`
	TranslatedText                  string                     `index:"73" json:",omitempty"`
	TextDialect                     any                        `index:"74" json:",omitempty"`
	TranslatedTextDialect           any                        `index:"75" json:",omitempty"`
	ScheduledTimestamp              int64                      `index:"76" json:",omitempty"`
	IsVideoQuickSend                bool                       `index:"77" json:",omitempty"`
	SubthreadParentMessageID        string                     `index:"78" json:",omitempty"`
	IsExclusiveToSubthread          bool                       `index:"79" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSDeleteThenInsertMessage) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSEditMessage struct {
	MessageID      string `index:"0" json:",omitempty"`
	AuthorityLevel int64  `index:"1" json:",omitempty"`
	Text           string `index:"2" json:",omitempty"`
	EditCount      int64  `index:"3" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

// can't spell ?
type LSReplaceOptimsiticMessage struct {
	OfflineThreadingId string `index:"0" json:",omitempty"`
	MessageId          string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSSetMessageTextHasLinks struct {
	ThreadKey   int64  `index:"0" json:",omitempty"`
	MessageId   string `index:"1" json:",omitempty"`
	TimestampMs int64  `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateMessagesOptimisticContext struct {
	Unrecognized map[int]any `json:",omitempty"`
}

type LSReplaceOptimisticReaction struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	ActorId   int64  `index:"1" json:",omitempty"`
	MessageId string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteThenInsertMessageRequest struct {
	ThreadKey            int64 `index:"0" json:",omitempty"`
	Unknown              int64 `index:"1" json:",omitempty"`
	MessageRequestStatus int64 `index:"2" json:",omitempty"` // make enum ?

	Unrecognized map[int]any `json:",omitempty"`
}

type LSMarkOptimisticMessageFailed struct {
	OTID    string `index:"0" json:",omitempty"`
	Message string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSHandleFailedTask struct {
	TaskID  int64  `index:"0" json:",omitempty"`
	OTID    string `index:"1" json:",omitempty"`
	Message string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateSubscriptErrorMessage struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	OTID      string `index:"1" json:",omitempty"`
	Message   string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateSubThreadXMA struct {
	ParentThreadKey int64 `index:"0" json:",omitempty"`
	ThreadKey       int64 `index:"1" json:",omitempty"`
	ReplyCount      int64 `index:"2" json:",omitempty"`
}

type LSSetNumUnreadSubthreads struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	NumUnread int64 `index:"1" json:",omitempty"`
}
