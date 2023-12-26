package table

type ThreadInfo interface{
    GetThreadKey() int64
    GetThreadName() string
    GetThreadType() ThreadType
    GetLastReadWatermarkTimestampMs() int64
    GetThreadDescription() string
    GetFolderName() string
}

type LSTruncateMetadataThreads struct {}

type LSUpsertInboxThreadsRange struct {
	SyncGroup int64 `index:"0"`
	MinLastActivityTimestampMs int64 `index:"1"`
	HasMoreBefore bool `index:"2"`
	IsLoadingBefore bool `index:"3"`
	MinThreadKey int64 `index:"4"`
}

type LSUpdateThreadsRangesV2 struct {
	FolderName string `index:"0"`
	ParentThreadKey int64 `index:"1"` /* not sure */
	MinLastActivityTimestampMs int64 `index:"2"`
	MinThreadKey int64 `index:"3"`
	IsLoadingBefore int64 `index:"4"` /* not sure */
}

type LSDeleteThenInsertThread struct {
    LastActivityTimestampMs int64 `index:"0"`
    LastReadWatermarkTimestampMs int64 `index:"1"`
    Snippet string `index:"2"`
    ThreadName string `index:"3"`
    ThreadPictureUrl string `index:"4"`
    NeedsAdminApprovalForNewParticipant bool `index:"5"`
    AuthorityLevel int64 `index:"6"`
    ThreadKey int64 `index:"7"`
    MailboxType int64 `index:"8"`
    ThreadType ThreadType `index:"9"`
    FolderName string `index:"10"`
    ThreadPictureUrlFallback string `index:"11"`
    ThreadPictureUrlExpirationTimestampMs int64 `index:"12"`
    RemoveWatermarkTimestampMs int64 `index:"13"`
    MuteExpireTimeMs int64 `index:"14"`
    MuteCallsExpireTimeMs int64 `index:"15"`
    GroupNotificationSettings int64 `index:"16"`
    IsAdminSnippet bool `index:"17"`
    SnippetSenderContactId int64 `index:"18"`
    SnippetStringHash int64 `index:"21"`
    SnippetStringArgument1 int64 `index:"22"`
    SnippetAttribution int64 `index:"23"`
    SnippetAttributionStringHash int64 `index:"24"`
    DisappearingSettingTtl int64 `index:"25"`
    DisappearingSettingUpdatedTs int64 `index:"26"`
    DisappearingSettingUpdatedBy int64 `index:"27"`
    OngoingCallState int64 `index:"29"`
    CannotReplyReason int64 `index:"30"`
    CustomEmoji int64 `index:"31"`
    CustomEmojiImageUrl int64 `index:"32"`
    OutgoingBubbleColor int64 `index:"33"`
    ThemeFbid int64 `index:"34"`
    ParentThreadKey int64 `index:"35"`
    NullstateDescriptionText1 string `index:"36"`
    NullstateDescriptionType1 int64 `index:"37"`
    NullstateDescriptionText2 string `index:"38"`
    NullstateDescriptionType2 int64 `index:"39"`
    NullstateDescriptionText3 string `index:"40"`
    NullstateDescriptionType3 int64 `index:"41"`
    SnippetHasEmoji bool `index:"42"`
    HasPersistentMenu bool `index:"43"`
    DisableComposerInput bool `index:"44"`
    CannotUnsendReason int64 `index:"45"`
    ViewedPluginKey int64 `index:"46"`
    ViewedPluginContext int64 `index:"47"`
    ClientThreadKey int64 `index:"48"`
    Capabilities int64 `index:"49"`
    ShouldRoundThreadPicture int64 `index:"50"`
    ProactiveWarningDismissTime int64 `index:"51"`
    IsCustomThreadPicture bool `index:"52"`
    OtidOfFirstMessage int64 `index:"53"`
    NormalizedSearchTerms string `index:"54"`
    AdditionalThreadContext int64 `index:"55"`
    DisappearingThreadKey int64 `index:"56"`
    IsDisappearingMode bool `index:"57"`
    DisappearingModeInitiator int64 `index:"58"`
    UnreadDisappearingMessageCount int64 `index:"59"`
    LastMessageCtaId int64 `index:"61"`
    LastMessageCtaType int64 `index:"62"`
    ConsistentThreadFbid int64 `index:"63"`
    ThreadDescription string `index:"64"`
    UnsendLimitMs int64 `index:"65"`
    SyncGroup int64 `index:"66"`
    ThreadInvitesEnabled int64 `index:"67"`
    ThreadInviteLink string `index:"68"`
    NumUnreadSubthreads int64 `index:"69"`
    SubthreadCount int64 `index:"70"`
    ThreadInvitesEnabledV2 int64 `index:"71"`
    EventStartTimestampMs int64 `index:"72"`
    EventEndTimestampMs int64 `index:"73"`
    TakedownState int64 `index:"74"`
    MemberCount int64 `index:"75"`
    SecondaryParentThreadKey int64 `index:"76"`
    IgFolder int64 `index:"77"`
    InviterId int64 `index:"78"`
    ThreadTags int64 `index:"79"`
    ThreadStatus int64 `index:"80"`
    ThreadSubtype int64 `index:"81"`
    PauseThreadTimestamp int64 `index:"82"`
}

func (lsdt LSDeleteThenInsertThread) GetThreadKey() int64 {
    return lsdt.ThreadKey
}

func (lsdt LSDeleteThenInsertThread) GetThreadName() string {
    return lsdt.ThreadName
}

func (lsdt LSDeleteThenInsertThread) GetThreadType() ThreadType {
    return lsdt.ThreadType
}

func (lsdt LSDeleteThenInsertThread) GetLastReadWatermarkTimestampMs() int64 {
    return lsdt.LastReadWatermarkTimestampMs
}

func (lsdt LSDeleteThenInsertThread) GetThreadDescription() string {
    return lsdt.ThreadDescription
}

func (lsdt LSDeleteThenInsertThread) GetFolderName() string {
    return lsdt.FolderName
}

type LSAddParticipantIdToGroupThread struct {
    ThreadKey int64 `index:"0"`
    ContactId int64 `index:"1"`
    ReadWatermarkTimestampMs int64 `index:"2"`
    ReadActionTimestampMs int64 `index:"3"`
    DeliveredWatermarkTimestampMs int64 `index:"4"`
    Nickname string `index:"5"`
    IsAdmin bool `index:"6"`
    SubscribeSource string `index:"7"`
    AuthorityLevel int64 `index:"9"`
    NormalizedSearchTerms string `index:"10"`
    IsSuperAdmin bool `index:"11"`
    ThreadRoles int64 `index:"12"`
}

type LSWriteThreadCapabilities struct {
    ThreadKey int64 `index:"0"`
    Capabilities int64 `index:"1"`
    Capabilities2 int64 `index:"2"`
    Capabilities3 int64 `index:"3"`
}

type LSUpdateReadReceipt struct {
    ReadWatermarkTimestampMs int64 `index:"0"`
    ThreadKey int64 `index:"1"`
    ContactId int64 `index:"2"`
    ReadActionTimestampMs int64 `index:"3"`
}

type LSThreadsRangesQuery struct {
    ParentThreadKey int64 `index:"0"`
    Unknown1 bool `index:"1"`
    IsAfter bool `index:"2"`
    ReferenceThreadKey int64 `conditionField:"IsAfter" indexes:"4,3"`
    ReferenceActivityTimestamp int64 `conditionField:"IsAfter" indexes:"5,6"`
    AdditionalPagesToFetch int64 `index:"7"`
    Unknown8 bool `index:"8"`
}

type LSUpdateTypingIndicator struct {
    ThreadKey int64 `index:"0"`
    SenderId int64 `index:"1"`
    IsTyping bool `index:"2"`
}

type LSMoveThreadToInboxAndUpdateParent struct {
    ThreadKey int64 `index:"0"`
    ParentThreadKey int64 `index:"1"`
}

type LSUpdateThreadSnippet struct {
    ThreadKey int64 `index:"0"`
    Snippet string `index:"1"`
    IsAdminSnippet bool `index:"2"`
    SnippetSenderContactId int64 `index:"3"`
    SnippetHasEmoji bool `index:"4"`
    ViewedPluginKey string `index:"5"`
    ViewedPluginContext string `index:"6"`
}

type LSVerifyThreadExists struct {
    ThreadKey int64 `index:"0"`
    ThreadType ThreadType `index:"1"`
    FolderName string `index:"2"`
    ParentThreadKey int64 `index:"3"`
    AuthorityLevel int64 `index:"4"`
}

func (lsui LSVerifyThreadExists) GetThreadKey() int64 {
    return lsui.ThreadKey
}

func (lsui LSVerifyThreadExists) GetThreadName() string {
    return ""
}

func (lsui LSVerifyThreadExists) GetThreadType() ThreadType {
    return lsui.ThreadType
}

func (lsui LSVerifyThreadExists) GetLastReadWatermarkTimestampMs() int64 {
    return 0
}

func (lsui LSVerifyThreadExists) GetThreadDescription() string {
    return ""
}

func (lsui LSVerifyThreadExists) GetFolderName() string {
    return lsui.FolderName
}

type LSBumpThread struct {
    LastReadWatermarkTimestampMs int64 `index:"0"`
    BumpStatus ThreadBumpStatus `index:"1"`
    ThreadKey int64 `index:"2"`
}

func (lsui LSBumpThread) GetThreadKey() int64 {
    return lsui.ThreadKey
}

func (lsui LSBumpThread) GetThreadName() string {
    return ""
}

func (lsui LSBumpThread) GetThreadType() ThreadType {
    return 0
}

func (lsui LSBumpThread) GetLastReadWatermarkTimestampMs() int64 {
    return lsui.LastReadWatermarkTimestampMs
}

func (lsui LSBumpThread) GetThreadDescription() string {
    return ""
}

func (lsui LSBumpThread) GetFolderName() string {
    return ""
}

// Idk which snippet is the correct, there's like 6 (snippet, snippetStringHash, snippetStringArgument1, snippetAttribution, snippetAttributionStringHash)
type LSUpdateThreadSnippetFromLastMessage struct {
    AccountId int64 `index:"0"`
    ThreadKey int64 `index:"1"`
    Snippet1 string `index:"2"`
    Snippet2 string `index:"3"`
    Snippet3 string `index:"4"`
    Snippet4 string `index:"5"`
    Snippet5 string `index:"6"`
    Snippet6 string `index:"7"`
    Snippet7 string `index:"8"`
    Snippet8 string `index:"9"`
    Snippet9 string `index:"10"`
    IsAdminSnippet bool `index:"11"`
}

type LSDeleteBannersByIds struct {
    ThreadKey int64 `index:"0"`
}

type LSUpdateDeliveryReceipt struct {
    DeliveredWatermarkTimestampMs int64 `index:"0"`
    ThreadKey int64 `index:"1"`
    ContactId int64 `index:"2"`
}

type LSUpdateOptimisticContextThreadKeys struct {
    ThreadKey1 int64 `index:"0"`
    ThreadKey2 int64 `index:"1"`
}

type LSReplaceOptimisticThread struct {
    ThreadKey1 int64 `index:"0"`
    ThreadKey2 int64 `index:"1"`
}

type LSApplyNewGroupThread struct {
    OtidOfFirstMessage string `index:"0"`
    ThreadKey int64 `index:"1"`
    ThreadType ThreadType `index:"2"`
    FolderName string `index:"3"`
    ParentThreadKey int64 `index:"4"`
    ThreadPictureUrlFallback string `index:"5"`
    LastActivityTimestampMs int64 `index:"6"`
    LastReadWatermarkTimestampMs int64 `index:"6"`
    NullstateDescriptionText1 string `index:"8"`
    NullstateDescriptionType1 int64 `index:"9"`
    NullstateDescriptionText2 string `index:"10"`
    NullstateDescriptionType2 int64 `index:"11"`
    CannotUnsendReason int64 `index:"12"`
    Capabilities int64 `index:"13"`
    InviterId int64 `index:"14"`
    IgFolder int64 `index:"15"`
    ThreadSubtype int64 `index:"16"`
}

type LSRemoveAllParticipantsForThread struct {
    ThreadKey int64 `index:"0"`
}

type LSUpdateThreadInviteLinksInfo struct {
    ThreadKey int64 `index:"0"`
    ThreadInvitesEnabled int64 `index:"1"` // 0 or 1
    ThreadInviteLink string `index:"2"`
}

type LSUpdateThreadParticipantAdminStatus struct {
    ThreadKey int64 `index:"0"`
    ContactId int64 `index:"1"`
    IsAdmin bool `index:"2"`
}

type LSUpdateParticipantSubscribeSourceText struct {
    ThreadKey int64 `index:"0"`
    ContactId int64 `index:"1"`
    SubscribeSource string `index:"2"`
}

type LSOverwriteAllThreadParticipantsAdminStatus struct {
    ThreadKey int64 `index:"0"`
    IsAdmin bool `index:"1"`
}

type LSUpdateParticipantCapabilities struct {
    ContactId int64 `index:"0"`
    ThreadKey int64 `index:"1"`
}

type LSChangeViewerStatus struct {
    ThreadKey int64 `index:"0"`
    CannotReplyReason string `index:"1"`
}

type LSSyncUpdateThreadName struct {
    ThreadName string `index:"0"`
    ThreadKey int64 `index:"1"`
    ThreadName1 string `index:"2"`
}

type LSWriteCTAIdToThreadsTable struct {
    ThreadKey int64 `index:"0"`
    LastMessageCtaType int64 `index:"1"`
    LastMessageCtaTimestampMs int64 `index:"2"`
}

type LSQueryAdditionalGroupThreads struct {
    NumThreads int64 `index:"0"`
    NumMessages int64 `index:"1"`
    AdditionalPagesToFetch int64 `index:"2"`
}

type LSDeleteThenInsertIgThreadInfo struct {
    ThreadKey int64 `index:"0"`
    IgThreadId string `index:"1"`
}

type LSMarkThreadRead struct {
    LastReadWatermarkTimestampMs int64 `index:"0"`
    ThreadKey int64 `index:"1"`
}

func (lsui LSMarkThreadRead) GetThreadKey() int64 {
    return lsui.ThreadKey
}

func (lsui LSMarkThreadRead) GetThreadName() string {
    return ""
}

func (lsui LSMarkThreadRead) GetThreadType() ThreadType {
    return 0
}

func (lsui LSMarkThreadRead) GetLastReadWatermarkTimestampMs() int64 {
    return lsui.LastReadWatermarkTimestampMs
}

func (lsui LSMarkThreadRead) GetThreadDescription() string {
    return ""
}

func (lsui LSMarkThreadRead) GetFolderName() string {
    return ""
}

type LSUpdateParentFolderReadWatermark struct {
    ThreadKey int64 `index:"0"`
    // ShouldUpdate bool `index:"1"` // condition ?
}

type LSUpdateOrInsertThread struct {
    LastActivityTimestampMs int64 `index:"0"`
    LastReadWatermarkTimestampMs int64 `index:"1"`
    Snippet string `index:"2"`
    ThreadName string `index:"3"`
    ThreadPictureUrl string `index:"4"`
    NeedsAdminApprovalForNewParticipant bool `index:"5"`
    AuthorityLevel int64 `index:"6"`
    ThreadKey int64 `index:"7"`
    MailboxType int64 `index:"8"`
    ThreadType ThreadType `index:"9"`
    FolderName string `index:"10"`
    ThreadPictureUrlFallback string `index:"11"`
    ThreadPictureUrlExpirationTimestampMs int64 `index:"12"`
    RemoveWatermarkTimestampMs int64 `index:"13"`
    MuteExpireTimeMs int64 `index:"14"`
    MuteMentionExpireTimeMs int64 `index:"15"`
    MuteCallsExpireTimeMs int64 `index:"16"`
    GroupNotificationSettings int64 `index:"19"`
    IsAdminSnippet bool `index:"20"`
    SnippetSenderContactId int64 `index:"21"`
    SnippetStringHash int64 `index:"24"`
    SnippetStringArgument1 int64 `index:"25"`
    SnippetAttribution int64 `index:"26"`
    SnippetAttributionStringHash int64 `index:"27"`
    DisappearingSettingTtl int64 `index:"28"`
    DisappearingSettingUpdatedTs int64 `index:"29"`
    DisappearingSettingUpdatedBy int64 `index:"30"`
    OngoingCallState int64 `index:"32"`
    CannotReplyReason int64 `index:"33"`
    CustomEmoji int64 `index:"34"`
    CustomEmojiImageUrl int64 `index:"35"`
    OutgoingBubbleColor int64 `index:"36"`
    ThemeFbid int64 `index:"37"`
    ParentThreadKey int64 `index:"38"`
    NullstateDescriptionText1 int64 `index:"39"`
    NullstateDescriptionType1 int64 `index:"40"`
    NullstateDescriptionText2 int64 `index:"41"`
    NullstateDescriptionType2 int64 `index:"42"`
    NullstateDescriptionText3 int64 `index:"43"`
    NullstateDescriptionType3 int64 `index:"44"`
    SnippetHasEmoji bool `index:"45"`
    HasPersistentMenu bool `index:"46"`
    DisableComposerInput bool `index:"47"`
    CannotUnsendReason int64 `index:"48"`
    ViewedPluginKey int64 `index:"49"`
    ViewedPluginContext int64 `index:"50"`
    ClientThreadKey int64 `index:"51"`
    Capabilities int64 `index:"52"`
    ShouldRoundThreadPicture int64 `index:"53"`
    ProactiveWarningDismissTime int64 `index:"54"`
    IsCustomThreadPicture bool `index:"55"`
    OtidOfFirstMessage int64 `index:"56"`
    NormalizedSearchTerms string `index:"57"`
    AdditionalThreadContext int64 `index:"58"`
    DisappearingThreadKey int64 `index:"59"`
    IsDisappearingMode bool `index:"60"`
    DisappearingModeInitiator int64 `index:"61"`
    UnreadDisappearingMessageCount int64 `index:"62"`
    LastMessageCtaId int64 `index:"64"`
    LastMessageCtaType int64 `index:"65"`
    LastMessageCtaTimestampMs int64 `index:"66"`
    ConsistentThreadFbid int64 `index:"67"`
    ThreadDescription string `index:"69"`
    UnsendLimitMs int64 `index:"70"`
    Capabilities2 int64 `index:"78"`
    Capabilities3 int64 `index:"79"`
    SyncGroup int64 `index:"82"`
    ThreadInvitesEnabled int64 `index:"83"`
    ThreadInviteLink string `index:"84"`
    IsAllUnreadMessageMissedCallXma bool `index:"85"`
    NumUnreadSubthreads int64 `index:"86"`
    SubthreadCount int64 `index:"87"`
    LastNonMissedCallXmaMessageTimestampMs int64 `index:"88"`
    ThreadInvitesEnabledV2 int64 `index:"90"`
    HasPendingInvitation int64 `index:"93"`
    EventStartTimestampMs int64 `index:"94"`
    EventEndTimestampMs int64 `index:"95"`
    TakedownState int64 `index:"96"`
    SecondaryParentThreadKey int64 `index:"97"`
    IgFolder int64 `index:"98"`
    InviterId int64 `index:"99"`
    ThreadTags int64 `index:"100"`
    ThreadStatus int64 `index:"101"`
    ThreadSubtype int64 `index:"102"`
    PauseThreadTimestamp int64 `index:"103"`
    Capabilities4 int64 `index:"104"`
}

func (lsui LSUpdateOrInsertThread) GetThreadKey() int64 {
    return lsui.ThreadKey
}

func (lsui LSUpdateOrInsertThread) GetThreadName() string {
    return lsui.ThreadName
}

func (lsui LSUpdateOrInsertThread) GetThreadType() ThreadType {
    return lsui.ThreadType
}

func (lsui LSUpdateOrInsertThread) GetLastReadWatermarkTimestampMs() int64 {
    return lsui.LastReadWatermarkTimestampMs
}

func (lsui LSUpdateOrInsertThread) GetThreadDescription() string {
    return lsui.ThreadDescription
}

func (lsui LSUpdateOrInsertThread) GetFolderName() string {
    return lsui.FolderName
}

type LSSetThreadCannotUnsendReason struct {
    ThreadKey int64 `index:"0"`
    CannotUnsendReason int64 `index:"1"`
}

type LSClearLocalThreadPictureUrl struct {
    ThreadKey int64 `index:"0"`
}

type LSUpdateInviterId struct {
    ThreadKey int64 `index:"0"`
    InviterId int64 `index:"1"`
}

type LSAddToMemberCount struct {
    ThreadKey int64 `index:"0"`
    IncrementCount int64 `index:"1"`
}

type LSMoveThreadToArchivedFolder struct {
    ThreadKey int64 `index:"0"`
}

type LSRemoveParticipantFromThread struct {
    ThreadKey int64 `index:"0"`
    ParticipantId int64 `index:"1"`
}

type LSDeleteRtcRoomOnThread struct {
    ThreadKey int64 `index:"0"`
}

type LSUpdateThreadTheme struct {
    ThreadKey int64 `index:"0"`
    Unknown1 int64 `index:"1"`
    Unknown2 int64 `index:"2"`
}

type LSUpdateThreadApprovalMode struct {
    ThreadKey int64 `index:"0"`
    Value bool `index:"1"`
}

type LSRemoveAllRequestsFromAdminApprovalQueue struct {
    ThreadKey int64 `index:"0"`
}