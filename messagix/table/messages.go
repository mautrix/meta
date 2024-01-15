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
	ReplySourceTypeV2               int64                      `index:"25" json:",omitempty"`
	ReplyStatus                     int64                      `index:"26" json:",omitempty"`
	ReplySnippet                    string                     `index:"27" json:",omitempty"`
	ReplyMessageText                string                     `index:"28" json:",omitempty"`
	ReplyToUserId                   int64                      `index:"29" json:",omitempty"`
	ReplyMediaExpirationTimestampMs int64                      `index:"30" json:",omitempty"`
	ReplyMediaUrl                   string                     `index:"31" json:",omitempty"`
	ReplyMediaPreviewWidth          int64                      `index:"33" json:",omitempty"`
	ReplyMediaPreviewHeight         int64                      `index:"34" json:",omitempty"`
	ReplyMediaUrlMimeType           string                     `index:"35" json:",omitempty"`
	ReplyMediaUrlFallback           string                     `index:"36" json:",omitempty"`
	ReplyCtaId                      int64                      `index:"37" json:",omitempty"`
	ReplyCtaTitle                   string                     `index:"38" json:",omitempty"`
	ReplyAttachmentType             int64                      `index:"39" json:",omitempty"`
	ReplyAttachmentId               int64                      `index:"40" json:",omitempty"`
	ReplyAttachmentExtra            int64                      `index:"41" json:",omitempty"`
	ReplyType                       int64                      `index:"42" json:",omitempty"`
	IsForwarded                     bool                       `index:"43" json:",omitempty"`
	ForwardScore                    int64                      `index:"44" json:",omitempty"`
	HasQuickReplies                 bool                       `index:"45" json:",omitempty"`
	AdminMsgCtaId                   int64                      `index:"46" json:",omitempty"`
	AdminMsgCtaTitle                int64                      `index:"47" json:",omitempty"`
	AdminMsgCtaType                 int64                      `index:"48" json:",omitempty"`
	CannotUnsendReason              MessageUnsendabilityStatus `index:"49" json:",omitempty"`
	TextHasLinks                    bool                       `index:"50" json:",omitempty"`
	ViewFlags                       int64                      `index:"51" json:",omitempty"`
	DisplayedContentTypes           DisplayedContentTypes      `index:"52" json:",omitempty"`
	ViewedPluginKey                 int64                      `index:"53" json:",omitempty"`
	ViewedPluginContext             int64                      `index:"54" json:",omitempty"`
	QuickReplyType                  int64                      `index:"55" json:",omitempty"`
	HotEmojiSize                    int64                      `index:"56" json:",omitempty"`
	ReplySourceTimestampMs          int64                      `index:"57" json:",omitempty"`
	EphemeralDurationInSec          int64                      `index:"58" json:",omitempty"`
	MsUntilExpirationTs             int64                      `index:"59" json:",omitempty"`
	EphemeralExpirationTs           int64                      `index:"60" json:",omitempty"`
	TakedownState                   int64                      `index:"61" json:",omitempty"`
	IsCollapsed                     bool                       `index:"62" json:",omitempty"`
	SubthreadKey                    int64                      `index:"63" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
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
	MaxTimestampMs         int64       `index:"5" json:",omitempty"`
	MinTimestampMs         int64       `index:"6" json:",omitempty"`
	HasMoreBefore          bool        `index:"7" json:",omitempty"`
	HasMoreAfter           bool        `index:"8" json:",omitempty"`
	Unknown                interface{} `index:"9" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
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
	MentionOffsets                  int64                      `index:"19" json:",omitempty"`
	MentionLengths                  int64                      `index:"20" json:",omitempty"`
	MentionIds                      int64                      `index:"21" json:",omitempty"`
	MentionTypes                    int64                      `index:"22" json:",omitempty"`
	ReplySourceId                   int64                      `index:"23" json:",omitempty"`
	ReplySourceType                 int64                      `index:"24" json:",omitempty"`
	ReplySourceTypeV2               int64                      `index:"25" json:",omitempty"`
	ReplyStatus                     int64                      `index:"26" json:",omitempty"`
	ReplySnippet                    string                     `index:"27" json:",omitempty"`
	ReplyMessageText                string                     `index:"28" json:",omitempty"`
	ReplyToUserId                   int64                      `index:"29" json:",omitempty"`
	ReplyMediaExpirationTimestampMs int64                      `index:"30" json:",omitempty"`
	ReplyMediaUrl                   string                     `index:"31" json:",omitempty"`
	ReplyMediaPreviewWidth          int64                      `index:"33" json:",omitempty"`
	ReplyMediaPreviewHeight         int64                      `index:"34" json:",omitempty"`
	ReplyMediaUrlMimeType           int64                      `index:"35" json:",omitempty"`
	ReplyMediaUrlFallback           string                     `index:"36" json:",omitempty"`
	ReplyCtaId                      int64                      `index:"37" json:",omitempty"`
	ReplyCtaTitle                   string                     `index:"38" json:",omitempty"`
	ReplyAttachmentType             int64                      `index:"39" json:",omitempty"`
	ReplyAttachmentId               int64                      `index:"40" json:",omitempty"`
	ReplyAttachmentExtra            string                     `index:"41" json:",omitempty"`
	IsForwarded                     bool                       `index:"42" json:",omitempty"`
	ForwardScore                    int64                      `index:"43" json:",omitempty"`
	HasQuickReplies                 bool                       `index:"44" json:",omitempty"`
	AdminMsgCtaId                   int64                      `index:"45" json:",omitempty"`
	AdminMsgCtaTitle                string                     `index:"46" json:",omitempty"`
	AdminMsgCtaType                 int64                      `index:"47" json:",omitempty"`
	CannotUnsendReason              MessageUnsendabilityStatus `index:"48" json:",omitempty"`
	TextHasLinks                    bool                       `index:"49" json:",omitempty"`
	ViewFlags                       int64                      `index:"50" json:",omitempty"`
	DisplayedContentTypes           DisplayedContentTypes      `index:"51" json:",omitempty"`
	ViewedPluginKey                 int64                      `index:"52" json:",omitempty"`
	ViewedPluginContext             int64                      `index:"53" json:",omitempty"`
	QuickReplyType                  int64                      `index:"54" json:",omitempty"`
	HotEmojiSize                    int64                      `index:"55" json:",omitempty"`
	ReplySourceTimestampMs          int64                      `index:"56" json:",omitempty"`
	EphemeralDurationInSec          int64                      `index:"57" json:",omitempty"`
	MsUntilExpirationTs             int64                      `index:"58" json:",omitempty"`
	EphemeralExpirationTs           int64                      `index:"59" json:",omitempty"`
	TakedownState                   int64                      `index:"60" json:",omitempty"`
	IsCollapsed                     bool                       `index:"61" json:",omitempty"`
	SubthreadKey                    int64                      `index:"62" json:",omitempty"`
	IsPaidPartnership               bool                       `index:"63" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
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

type LSDeleteReaction struct {
	ThreadKey int64  `index:"0" json:",omitempty"`
	MessageId string `index:"1" json:",omitempty"`
	ActorId   int64  `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSHandleRepliesOnUnsend struct {
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

type LSDeleteThenInsertMessage struct {
	Text                            string `index:"0" json:",omitempty"`
	SubscriptErrorMessage           int64  `index:"1" json:",omitempty"`
	AuthorityLevel                  int64  `index:"2" json:",omitempty"`
	ThreadKey                       int64  `index:"3" json:",omitempty"`
	TimestampMs                     int64  `index:"5" json:",omitempty"`
	PrimarySortKey                  int64  `index:"6" json:",omitempty"`
	SecondarySortKey                int64  `index:"7" json:",omitempty"`
	MessageId                       string `index:"8" json:",omitempty"`
	OfflineThreadingId              string `index:"9" json:",omitempty"`
	SenderId                        int64  `index:"10" json:",omitempty"`
	StickerId                       int64  `index:"11" json:",omitempty"`
	IsAdminMessage                  bool   `index:"12" json:",omitempty"`
	MessageRenderingType            int64  `index:"13" json:",omitempty"`
	SendStatus                      int64  `index:"15" json:",omitempty"`
	SendStatusV2                    int64  `index:"16" json:",omitempty"`
	IsUnsent                        bool   `index:"17" json:",omitempty"`
	UnsentTimestampMs               int64  `index:"18" json:",omitempty"`
	MentionOffsets                  int64  `index:"19" json:",omitempty"`
	MentionLengths                  int64  `index:"20" json:",omitempty"`
	MentionIds                      int64  `index:"21" json:",omitempty"`
	MentionTypes                    int64  `index:"22" json:",omitempty"`
	ReplySourceId                   int64  `index:"23" json:",omitempty"`
	ReplySourceType                 int64  `index:"24" json:",omitempty"`
	ReplySourceTypeV2               int64  `index:"25" json:",omitempty"`
	ReplyStatus                     int64  `index:"26" json:",omitempty"`
	ReplySnippet                    int64  `index:"27" json:",omitempty"`
	ReplyMessageText                int64  `index:"28" json:",omitempty"`
	ReplyToUserId                   int64  `index:"29" json:",omitempty"`
	ReplyMediaExpirationTimestampMs int64  `index:"30" json:",omitempty"`
	ReplyMediaUrl                   int64  `index:"31" json:",omitempty"`
	ReplyMediaPreviewWidth          int64  `index:"33" json:",omitempty"`
	ReplyMediaPreviewHeight         int64  `index:"34" json:",omitempty"`
	ReplyMediaUrlMimeType           int64  `index:"35" json:",omitempty"`
	ReplyMediaUrlFallback           int64  `index:"36" json:",omitempty"`
	ReplyCtaId                      int64  `index:"37" json:",omitempty"`
	ReplyCtaTitle                   int64  `index:"38" json:",omitempty"`
	ReplyAttachmentType             int64  `index:"39" json:",omitempty"`
	ReplyAttachmentId               int64  `index:"40" json:",omitempty"`
	ReplyAttachmentExtra            int64  `index:"41" json:",omitempty"`
	IsForwarded                     bool   `index:"42" json:",omitempty"`
	ForwardScore                    int64  `index:"43" json:",omitempty"`
	HasQuickReplies                 bool   `index:"44" json:",omitempty"`
	AdminMsgCtaId                   int64  `index:"45" json:",omitempty"`
	AdminMsgCtaTitle                int64  `index:"46" json:",omitempty"`
	AdminMsgCtaType                 int64  `index:"47" json:",omitempty"`
	CannotUnsendReason              int64  `index:"48" json:",omitempty"`
	TextHasLinks                    bool   `index:"49" json:",omitempty"`
	ViewFlags                       int64  `index:"50" json:",omitempty"`
	DisplayedContentTypes           int64  `index:"51" json:",omitempty"`
	ViewedPluginKey                 int64  `index:"52" json:",omitempty"`
	ViewedPluginContext             int64  `index:"53" json:",omitempty"`
	QuickReplyType                  int64  `index:"54" json:",omitempty"`
	HotEmojiSize                    int64  `index:"55" json:",omitempty"`
	ReplySourceTimestampMs          int64  `index:"56" json:",omitempty"`
	EphemeralDurationInSec          int64  `index:"57" json:",omitempty"`
	MsUntilExpirationTs             int64  `index:"58" json:",omitempty"`
	EphemeralExpirationTs           int64  `index:"59" json:",omitempty"`
	TakedownState                   int64  `index:"60" json:",omitempty"`
	IsCollapsed                     bool   `index:"61" json:",omitempty"`
	SubthreadKey                    int64  `index:"62" json:",omitempty"`
	BotResponseId                   int64  `index:"63" json:",omitempty"`
	IsPaidPartnership               int64  `index:"64" json:",omitempty"`

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
