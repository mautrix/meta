package table

/*
	Instructs the client to clear pinned messages (delete by ThreadKey)
*/
type LSClearPinnedMessages struct {
	ThreadKey int64 `index:"0"`
}

type LSUpsertMessage struct {
    Text string `index:"0"`
    SubscriptErrorMessage string `index:"1"`
    AuthorityLevel int64 `index:"2"`
    ThreadKey int64 `index:"3"`
    TimestampMs int64 `index:"5"`
    PrimarySortKey int64 `index:"6"`
    SecondarySortKey int64 `index:"7"`
    MessageId string `index:"8"`
    OfflineThreadingId string `index:"9"`
    SenderId int64 `index:"10"`
    StickerId int64 `index:"11"`
    IsAdminMessage bool `index:"12"`
    MessageRenderingType int64 `index:"13"`
    SendStatus int64 `index:"15"`
    SendStatusV2 int64 `index:"16"`
    IsUnsent bool `index:"17"`
    UnsentTimestampMs int64 `index:"18"`
    MentionOffsets string `index:"19"`
    MentionLengths string `index:"20"`
    MentionIds string `index:"21"`
    MentionTypes string `index:"22"`
    ReplySourceId string `index:"23"`
    ReplySourceType int64 `index:"24"`
    ReplySourceTypeV2 int64 `index:"25"`
    ReplyStatus int64 `index:"26"`
    ReplySnippet string `index:"27"`
    ReplyMessageText string `index:"28"`
    ReplyToUserId int64 `index:"29"`
    ReplyMediaExpirationTimestampMs int64 `index:"30"`
    ReplyMediaUrl string `index:"31"`
    ReplyMediaPreviewWidth int64 `index:"33"`
    ReplyMediaPreviewHeight int64 `index:"34"`
    ReplyMediaUrlMimeType string `index:"35"`
    ReplyMediaUrlFallback string `index:"36"`
    ReplyCtaId int64 `index:"37"`
    ReplyCtaTitle string `index:"38"`
    ReplyAttachmentType int64 `index:"39"`
    ReplyAttachmentId int64 `index:"40"`
    ReplyAttachmentExtra int64 `index:"41"`
    ReplyType int64 `index:"42"`
    IsForwarded bool `index:"43"`
    ForwardScore int64 `index:"44"`
    HasQuickReplies bool `index:"45"`
    AdminMsgCtaId int64 `index:"46"`
    AdminMsgCtaTitle int64 `index:"47"`
    AdminMsgCtaType int64 `index:"48"`
    CannotUnsendReason MessageUnsendabilityStatus `index:"49"`
    TextHasLinks bool `index:"50"`
    ViewFlags int64 `index:"51"`
    DisplayedContentTypes DisplayedContentTypes `index:"52"`
    ViewedPluginKey int64 `index:"53"`
    ViewedPluginContext int64 `index:"54"`
    QuickReplyType int64 `index:"55"`
    HotEmojiSize int64 `index:"56"`
    ReplySourceTimestampMs int64 `index:"57"`
    EphemeralDurationInSec int64 `index:"58"`
    MsUntilExpirationTs int64 `index:"59"`
    EphemeralExpirationTs int64 `index:"60"`
    TakedownState int64 `index:"61"`
    IsCollapsed bool `index:"62"`
    SubthreadKey int64 `index:"63"`
}

type LSSetForwardScore struct {
	ThreadKey int64 `index:"0"`
	MessageId string `index:"1"`
	TimestampMs int64 `index:"2"`
	ForwardScore int64 `index:"3"`
}

type LSSetMessageDisplayedContentTypes struct {
	ThreadKey int64 `index:"0"`
	MessageId string `index:"1"`
	TimestampMs int64 `index:"2"`
	Text string `index:"3"`
	Calc1 bool `index:"4"`
	Calc2 bool `index:"5"`
}

type LSInsertNewMessageRange struct {
	ThreadKey int64 `index:"0"`
	MinTimestampMsTemplate int64 `index:"1"`
	MaxTimestampMsTemplate int64 `index:"2"`
	MinMessageId string `index:"3"`
	MaxMessageId string `index:"4"`
	MaxTimestampMs int64 `index:"5"`
	MinTimestampMs int64 `index:"6"`
	HasMoreBefore bool `index:"7"`
	HasMoreAfter bool `index:"8"`
	Unknown interface{} `index:"9"`
}

type LSDeleteExistingMessageRanges struct {
	ConsistentThreadFbid int64 `index:"0"`
}

type LSCheckAuthoritativeMessageExists struct {
    ThreadKey int64 `index:"0"`
    OfflineThreadingId string `index:"1"`
}

type LSUpdateParticipantLastMessageSendTimestamp struct {
    ThreadKey int64 `index:"0"`
    SenderId int64 `index:"1"`
    Timestamp int64 `index:"2"`
}

type LSInsertMessage struct {
    Text string `index:"0"`
    SubscriptErrorMessage string `index:"1"`
    AuthorityLevel int64 `index:"2"`
    ThreadKey int64 `index:"3"`
    TimestampMs int64 `index:"5"`
    PrimarySortKey int64 `index:"6"`
    SecondarySortKey int64 `index:"7"`
    MessageId string `index:"8"`
    OfflineThreadingId string `index:"9"`
    SenderId int64 `index:"10"`
    StickerId int64 `index:"11"`
    IsAdminMessage bool `index:"12"`
    MessageRenderingType int64 `index:"13"`
    SendStatus int64 `index:"15"`
    SendStatusV2 int64 `index:"16"`
    IsUnsent bool `index:"17"`
    UnsentTimestampMs int64 `index:"18"`
    MentionOffsets int64 `index:"19"`
    MentionLengths int64 `index:"20"`
    MentionIds int64 `index:"21"`
    MentionTypes int64 `index:"22"`
    ReplySourceId int64 `index:"23"`
    ReplySourceType int64 `index:"24"`
    ReplySourceTypeV2 int64 `index:"25"`
    ReplyStatus int64 `index:"26"`
    ReplySnippet string `index:"27"`
    ReplyMessageText string `index:"28"`
    ReplyToUserId int64 `index:"29"`
    ReplyMediaExpirationTimestampMs int64 `index:"30"`
    ReplyMediaUrl string `index:"31"`
    ReplyMediaPreviewWidth int64 `index:"33"`
    ReplyMediaPreviewHeight int64 `index:"34"`
    ReplyMediaUrlMimeType int64 `index:"35"`
    ReplyMediaUrlFallback string `index:"36"`
    ReplyCtaId int64 `index:"37"`
    ReplyCtaTitle string `index:"38"`
    ReplyAttachmentType int64 `index:"39"`
    ReplyAttachmentId int64 `index:"40"`
    ReplyAttachmentExtra string `index:"41"`
    IsForwarded bool `index:"42"`
    ForwardScore int64 `index:"43"`
    HasQuickReplies bool `index:"44"`
    AdminMsgCtaId int64 `index:"45"`
    AdminMsgCtaTitle string `index:"46"`
    AdminMsgCtaType int64 `index:"47"`
    CannotUnsendReason MessageUnsendabilityStatus `index:"48"`
    TextHasLinks bool `index:"49"`
    ViewFlags int64 `index:"50"`
    DisplayedContentTypes DisplayedContentTypes `index:"51"`
    ViewedPluginKey int64 `index:"52"`
    ViewedPluginContext int64 `index:"53"`
    QuickReplyType int64 `index:"54"`
    HotEmojiSize int64 `index:"55"`
    ReplySourceTimestampMs int64 `index:"56"`
    EphemeralDurationInSec int64 `index:"57"`
    MsUntilExpirationTs int64 `index:"58"`
    EphemeralExpirationTs int64 `index:"59"`
    TakedownState int64 `index:"60"`
    IsCollapsed bool `index:"61"`
    SubthreadKey int64 `index:"62"`
    IsPaidPartnership bool `index:"63"`
}

type LSUpsertReaction struct {
    ThreadKey int64 `index:"0"`
    TimestampMs int64 `index:"1"`
    MessageId string `index:"2"`
    ActorId int64 `index:"3"`
    Reaction string `index:"4"` // unicode str
    AuthorityLevel int64 `index:"5"`
}

type LSDeleteReaction struct {
    ThreadKey int64 `index:"0"`
    MessageId string `index:"1"`
    ActorId int64 `index:"2"`
}

type LSHandleRepliesOnUnsend struct {
    ThreadKey int64 `index:"0"`
    MessageId string `index:"1"`
}

type LSUpdateForRollCallMessageDeleted struct {
    MessageId string `index:"0"`
    ContributorId int64 `index:"1"`
}

type LSUpdateUnsentMessageCollapsedStatus struct {
    ThreadKey int64 `index:"0"`
    MessageId string `index:"1"`
    TimestampMs int64 `index:"2"`
}

type LSDeleteThenInsertMessage struct {
    Text string `index:"0"`
    SubscriptErrorMessage int64 `index:"1"`
    AuthorityLevel int64 `index:"2"`
    ThreadKey int64 `index:"3"`
    TimestampMs int64 `index:"5"`
    PrimarySortKey int64 `index:"6"`
    SecondarySortKey int64 `index:"7"`
    MessageId string `index:"8"`
    OfflineThreadingId string `index:"9"`
    SenderId int64 `index:"10"`
    StickerId int64 `index:"11"`
    IsAdminMessage bool `index:"12"`
    MessageRenderingType int64 `index:"13"`
    SendStatus int64 `index:"15"`
    SendStatusV2 int64 `index:"16"`
    IsUnsent bool `index:"17"`
    UnsentTimestampMs int64 `index:"18"`
    MentionOffsets int64 `index:"19"`
    MentionLengths int64 `index:"20"`
    MentionIds int64 `index:"21"`
    MentionTypes int64 `index:"22"`
    ReplySourceId int64 `index:"23"`
    ReplySourceType int64 `index:"24"`
    ReplySourceTypeV2 int64 `index:"25"`
    ReplyStatus int64 `index:"26"`
    ReplySnippet int64 `index:"27"`
    ReplyMessageText int64 `index:"28"`
    ReplyToUserId int64 `index:"29"`
    ReplyMediaExpirationTimestampMs int64 `index:"30"`
    ReplyMediaUrl int64 `index:"31"`
    ReplyMediaPreviewWidth int64 `index:"33"`
    ReplyMediaPreviewHeight int64 `index:"34"`
    ReplyMediaUrlMimeType int64 `index:"35"`
    ReplyMediaUrlFallback int64 `index:"36"`
    ReplyCtaId int64 `index:"37"`
    ReplyCtaTitle int64 `index:"38"`
    ReplyAttachmentType int64 `index:"39"`
    ReplyAttachmentId int64 `index:"40"`
    ReplyAttachmentExtra int64 `index:"41"`
    IsForwarded bool `index:"42"`
    ForwardScore int64 `index:"43"`
    HasQuickReplies bool `index:"44"`
    AdminMsgCtaId int64 `index:"45"`
    AdminMsgCtaTitle int64 `index:"46"`
    AdminMsgCtaType int64 `index:"47"`
    CannotUnsendReason int64 `index:"48"`
    TextHasLinks bool `index:"49"`
    ViewFlags int64 `index:"50"`
    DisplayedContentTypes int64 `index:"51"`
    ViewedPluginKey int64 `index:"52"`
    ViewedPluginContext int64 `index:"53"`
    QuickReplyType int64 `index:"54"`
    HotEmojiSize int64 `index:"55"`
    ReplySourceTimestampMs int64 `index:"56"`
    EphemeralDurationInSec int64 `index:"57"`
    MsUntilExpirationTs int64 `index:"58"`
    EphemeralExpirationTs int64 `index:"59"`
    TakedownState int64 `index:"60"`
    IsCollapsed bool `index:"61"`
    SubthreadKey int64 `index:"62"`
    BotResponseId int64 `index:"63"`
    IsPaidPartnership int64 `index:"64"`
}

// can't spell ?
type LSReplaceOptimsiticMessage struct {
    OfflineThreadingId string `index:"0"`
    MessageId string `index:"1"`
}

type LSSetMessageTextHasLinks struct {
    ThreadKey int64 `index:"0"`
    MessageId string `index:"1"`
    TimestampMs int64 `index:"2"`
}

type LSUpdateMessagesOptimisticContext struct {}

type LSReplaceOptimisticReaction struct {
    ThreadKey int64 `index:"0"`
    ActorId int64 `index:"1"`
    MessageId string `index:"2"`
}

type LSDeleteThenInsertMessageRequest struct {
    ThreadKey int64 `index:"0"`
    Unknown int64 `index:"1"`
    MessageRequestStatus int64 `index:"2"` // make enum ?
}