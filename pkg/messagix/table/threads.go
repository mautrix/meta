package table

type MinimalThreadInfo interface {
	GetThreadKey() int64
	GetThreadType() ThreadType
}

type ThreadInfo interface {
	MinimalThreadInfo
	GetThreadName() string
	GetLastReadWatermarkTimestampMs() int64
	GetThreadDescription() string
	GetThreadPictureUrl() string
	GetFolderName() string
}

type LSTruncateMetadataThreads struct{}

type LSUpsertInboxThreadsRange struct {
	SyncGroup                  int64 `index:"0" json:",omitempty"`
	MinLastActivityTimestampMs int64 `index:"1" json:",omitempty"`
	HasMoreBefore              bool  `index:"2" json:",omitempty"`
	IsLoadingBefore            bool  `index:"3" json:",omitempty"`
	MinThreadKey               int64 `index:"4" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadsRangesV2 struct {
	FolderName                 string `index:"0" json:",omitempty"`
	ParentThreadKey            int64  `index:"1" json:",omitempty"` /* not sure */
	MinLastActivityTimestampMs int64  `index:"2" json:",omitempty"`
	MinThreadKey               int64  `index:"3" json:",omitempty"`
	IsLoadingBefore            int64  `index:"4" json:",omitempty"` /* not sure */

	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteThenInsertThread struct {
	LastActivityTimestampMs               int64      `index:"0" json:",omitempty"`
	LastReadWatermarkTimestampMs          int64      `index:"1" json:",omitempty"`
	Snippet                               string     `index:"2" json:",omitempty"`
	ThreadName                            string     `index:"3" json:",omitempty"`
	ThreadPictureUrl                      string     `index:"4" json:",omitempty"`
	NeedsAdminApprovalForNewParticipant   bool       `index:"5" json:",omitempty"`
	AuthorityLevel                        int64      `index:"6" json:",omitempty"`
	ThreadKey                             int64      `index:"7" json:",omitempty"`
	MailboxType                           int64      `index:"8" json:",omitempty"`
	ThreadType                            ThreadType `index:"9" json:",omitempty"`
	FolderName                            string     `index:"10" json:",omitempty"`
	ThreadPictureUrlFallback              string     `index:"11" json:",omitempty"`
	ThreadPictureUrlExpirationTimestampMs int64      `index:"12" json:",omitempty"`
	RemoveWatermarkTimestampMs            int64      `index:"13" json:",omitempty"`
	MuteExpireTimeMs                      int64      `index:"14" json:",omitempty"`
	MuteCallsExpireTimeMs                 int64      `index:"15" json:",omitempty"`
	GroupNotificationSettings             int64      `index:"16" json:",omitempty"`
	IsAdminSnippet                        bool       `index:"17" json:",omitempty"`
	SnippetSenderContactId                int64      `index:"18" json:",omitempty"`
	SnippetStringHash                     int64      `index:"21" json:",omitempty"`
	SnippetStringArgument1                int64      `index:"22" json:",omitempty"`
	SnippetAttribution                    int64      `index:"23" json:",omitempty"`
	SnippetAttributionStringHash          int64      `index:"24" json:",omitempty"`
	DisappearingSettingTtl                int64      `index:"25" json:",omitempty"`
	DisappearingSettingUpdatedTs          int64      `index:"26" json:",omitempty"`
	DisappearingSettingUpdatedBy          int64      `index:"27" json:",omitempty"`
	OngoingCallState                      int64      `index:"29" json:",omitempty"`
	CannotReplyReason                     int64      `index:"30" json:",omitempty"`
	CustomEmoji                           string     `index:"31" json:",omitempty"`
	CustomEmojiImageUrl                   string     `index:"32" json:",omitempty"`
	OutgoingBubbleColor                   int64      `index:"33" json:",omitempty"`
	ThemeFbid                             int64      `index:"34" json:",omitempty"`
	ParentThreadKey                       int64      `index:"35" json:",omitempty"`
	NullstateDescriptionText1             string     `index:"36" json:",omitempty"`
	NullstateDescriptionType1             int64      `index:"37" json:",omitempty"`
	NullstateDescriptionText2             string     `index:"38" json:",omitempty"`
	NullstateDescriptionType2             int64      `index:"39" json:",omitempty"`
	NullstateDescriptionText3             string     `index:"40" json:",omitempty"`
	NullstateDescriptionType3             int64      `index:"41" json:",omitempty"`
	SnippetHasEmoji                       bool       `index:"42" json:",omitempty"`
	HasPersistentMenu                     bool       `index:"43" json:",omitempty"`
	DisableComposerInput                  bool       `index:"44" json:",omitempty"`
	CannotUnsendReason                    int64      `index:"45" json:",omitempty"`
	ViewedPluginKey                       int64      `index:"46" json:",omitempty"`
	ViewedPluginContext                   int64      `index:"47" json:",omitempty"`
	ClientThreadKey                       int64      `index:"48" json:",omitempty"`
	Capabilities                          int64      `index:"49" json:",omitempty"`
	ShouldRoundThreadPicture              int64      `index:"50" json:",omitempty"`
	ProactiveWarningDismissTime           int64      `index:"51" json:",omitempty"`
	IsCustomThreadPicture                 bool       `index:"52" json:",omitempty"`
	OtidOfFirstMessage                    int64      `index:"53" json:",omitempty"`
	NormalizedSearchTerms                 string     `index:"54" json:",omitempty"`
	AdditionalThreadContext               string     `index:"55" json:",omitempty"`
	DisappearingThreadKey                 int64      `index:"56" json:",omitempty"`
	IsDisappearingMode                    bool       `index:"57" json:",omitempty"`
	DisappearingModeInitiator             int64      `index:"58" json:",omitempty"`
	UnreadDisappearingMessageCount        int64      `index:"59" json:",omitempty"`
	LastMessageCtaId                      int64      `index:"61" json:",omitempty"`
	LastMessageCtaType                    string     `index:"62" json:",omitempty"`
	ConsistentThreadFbid                  int64      `index:"63" json:",omitempty"`
	ThreadDescription                     string     `index:"64" json:",omitempty"`
	UnsendLimitMs                         int64      `index:"65" json:",omitempty"`
	SyncGroup                             int64      `index:"66" json:",omitempty"`
	ThreadInvitesEnabled                  int64      `index:"67" json:",omitempty"`
	ThreadInviteLink                      string     `index:"68" json:",omitempty"`
	NumUnreadSubthreads                   int64      `index:"69" json:",omitempty"`
	SubthreadCount                        int64      `index:"70" json:",omitempty"`
	ThreadInvitesEnabledV2                int64      `index:"71" json:",omitempty"`
	JoinRequestApprovalSetting            int64      `index:"72" json:",omitempty"`
	PendingJoinRequestsCount              int64      `index:"73" json:",omitempty"`
	EventStartTimestampMs                 int64      `index:"74" json:",omitempty"`
	EventEndTimestampMs                   int64      `index:"75" json:",omitempty"`
	TakedownState                         int64      `index:"76" json:",omitempty"`
	MemberCount                           int64      `index:"77" json:",omitempty"`
	AdmodCount                            int64      `index:"78" json:",omitempty"`
	SecondaryParentThreadKey              int64      `index:"79" json:",omitempty"`
	IgFolder                              int64      `index:"80" json:",omitempty"`
	InviterId                             int64      `index:"81" json:",omitempty"`
	ThreadTags                            int64      `index:"82" json:",omitempty"`
	ReadReceiptsDisabledV2                any        `index:"83" json:",omitempty"`
	ThreadStatus                          int64      `index:"84" json:",omitempty"`
	ThreadSubtype                         int64      `index:"85" json:",omitempty"`
	PauseThreadTimestamp                  int64      `index:"86" json:",omitempty"`
	Metadata                              any        `index:"87" json:",omitempty"`
	SyncSource                            any        `index:"88" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (lsdt *LSDeleteThenInsertThread) GetThreadKey() int64 {
	return lsdt.ThreadKey
}

func (lsdt *LSDeleteThenInsertThread) GetThreadName() string {
	return lsdt.ThreadName
}

func (lsdt *LSDeleteThenInsertThread) GetThreadPictureUrl() string {
	return lsdt.ThreadPictureUrl
}

func (lsdt *LSDeleteThenInsertThread) GetThreadType() ThreadType {
	return lsdt.ThreadType
}

func (lsdt *LSDeleteThenInsertThread) GetLastReadWatermarkTimestampMs() int64 {
	return lsdt.LastReadWatermarkTimestampMs
}

func (lsdt *LSDeleteThenInsertThread) GetThreadDescription() string {
	return lsdt.ThreadDescription
}

func (lsdt *LSDeleteThenInsertThread) GetFolderName() string {
	return lsdt.FolderName
}

type LSAddParticipantIdToGroupThread struct {
	ThreadKey                     int64  `index:"0" json:",omitempty"`
	ContactId                     int64  `index:"1" json:",omitempty"`
	ReadWatermarkTimestampMs      int64  `index:"2" json:",omitempty"`
	ReadActionTimestampMs         int64  `index:"3" json:",omitempty"`
	DeliveredWatermarkTimestampMs int64  `index:"4" json:",omitempty"`
	Nickname                      string `index:"5" json:",omitempty"`
	IsAdmin                       bool   `index:"6" json:",omitempty"`
	SubscribeSource               string `index:"7" json:",omitempty"`
	AuthorityLevel                int64  `index:"9" json:",omitempty"`
	NormalizedSearchTerms         string `index:"10" json:",omitempty"`
	GroupParticipantJoinState     int64  `index:"11" json:",omitempty"`
	IsModerator                   bool   `index:"12" json:",omitempty"`
	IsSuperAdmin                  bool   `index:"13" json:",omitempty"`
	ThreadRoles                   int64  `index:"14" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSAddParticipantIdToGroupThread) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSWriteThreadCapabilities struct {
	ThreadKey     int64 `index:"0" json:",omitempty"`
	Capabilities  int64 `index:"1" json:",omitempty"`
	Capabilities2 int64 `index:"2" json:",omitempty"`
	Capabilities3 int64 `index:"3" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateReadReceipt struct {
	ReadWatermarkTimestampMs int64 `index:"0" json:",omitempty"`
	ThreadKey                int64 `index:"1" json:",omitempty"`
	ContactId                int64 `index:"2" json:",omitempty"`
	ReadActionTimestampMs    int64 `index:"3" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSUpdateReadReceipt) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSThreadsRangesQuery struct {
	ParentThreadKey            int64 `index:"0" json:",omitempty"`
	Unknown1                   bool  `index:"1" json:",omitempty"`
	IsAfter                    bool  `index:"2" json:",omitempty"`
	ReferenceThreadKey         int64 `conditionField:"IsAfter" indexes:"4,3" json:",omitempty"`
	ReferenceActivityTimestamp int64 `conditionField:"IsAfter" indexes:"5,6" json:",omitempty"`
	AdditionalPagesToFetch     int64 `index:"7" json:",omitempty"`
	Unknown8                   bool  `index:"8" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateTypingIndicator struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	SenderId  int64 `index:"1" json:",omitempty"`
	IsTyping  bool  `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSUpdateTypingIndicator) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSMoveThreadToInboxAndUpdateParent struct {
	ThreadKey       int64 `index:"0" json:",omitempty"`
	ParentThreadKey int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadSnippet struct {
	ThreadKey              int64  `index:"0" json:",omitempty"`
	Snippet                string `index:"1" json:",omitempty"`
	IsAdminSnippet         bool   `index:"2" json:",omitempty"`
	SnippetSenderContactId int64  `index:"3" json:",omitempty"`
	SnippetHasEmoji        bool   `index:"4" json:",omitempty"`
	ViewedPluginKey        string `index:"5" json:",omitempty"`
	ViewedPluginContext    string `index:"6" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSVerifyThreadExists struct {
	ThreadKey       int64      `index:"0" json:",omitempty"`
	ThreadType      ThreadType `index:"1" json:",omitempty"`
	FolderName      string     `index:"2" json:",omitempty"`
	ParentThreadKey int64      `index:"3" json:",omitempty"`
	AuthorityLevel  int64      `index:"4" json:",omitempty"`
	SyncGroup       int64      `index:"5" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (lsui *LSVerifyThreadExists) GetThreadKey() int64 {
	return lsui.ThreadKey
}

func (lsui *LSVerifyThreadExists) GetThreadType() ThreadType {
	return lsui.ThreadType
}

func (lsui *LSVerifyThreadExists) GetFolderName() string {
	return lsui.FolderName
}

type LSBumpThread struct {
	LastReadWatermarkTimestampMs int64            `index:"0" json:",omitempty"`
	BumpStatus                   ThreadBumpStatus `index:"1" json:",omitempty"`
	ThreadKey                    int64            `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

// Idk which snippet is the correct, there's like 6 (snippet, snippetStringHash, snippetStringArgument1, snippetAttribution, snippetAttributionStringHash)
type LSUpdateThreadSnippetFromLastMessage struct {
	AccountId      int64  `index:"0" json:",omitempty"`
	ThreadKey      int64  `index:"1" json:",omitempty"`
	Snippet1       string `index:"2" json:",omitempty"`
	Snippet2       string `index:"3" json:",omitempty"`
	Snippet3       string `index:"4" json:",omitempty"`
	Snippet4       string `index:"5" json:",omitempty"`
	Snippet5       string `index:"6" json:",omitempty"`
	Snippet6       string `index:"7" json:",omitempty"`
	Snippet7       string `index:"8" json:",omitempty"`
	Snippet8       string `index:"9" json:",omitempty"`
	Snippet9       string `index:"10" json:",omitempty"`
	IsAdminSnippet bool   `index:"11" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadSnippetFromLastMessageV2 struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteBannersByIds struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateDeliveryReceipt struct {
	DeliveredWatermarkTimestampMs int64 `index:"0" json:",omitempty"`
	ThreadKey                     int64 `index:"1" json:",omitempty"`
	ContactId                     int64 `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateOptimisticContextThreadKeys struct {
	ThreadKey1 int64 `index:"0" json:",omitempty"`
	ThreadKey2 int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSReplaceOptimisticThread struct {
	ThreadKey1 int64 `index:"0" json:",omitempty"`
	ThreadKey2 int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSApplyNewGroupThread struct {
	OtidOfFirstMessage           string     `index:"0" json:",omitempty"`
	ThreadKey                    int64      `index:"1" json:",omitempty"`
	ThreadType                   ThreadType `index:"2" json:",omitempty"`
	FolderName                   string     `index:"3" json:",omitempty"`
	ParentThreadKey              int64      `index:"4" json:",omitempty"`
	ThreadPictureUrlFallback     string     `index:"5" json:",omitempty"`
	LastActivityTimestampMs      int64      `index:"6" json:",omitempty"`
	LastReadWatermarkTimestampMs int64      `index:"6" json:",omitempty"`
	NullstateDescriptionText1    string     `index:"8" json:",omitempty"`
	NullstateDescriptionType1    int64      `index:"9" json:",omitempty"`
	NullstateDescriptionText2    string     `index:"10" json:",omitempty"`
	NullstateDescriptionType2    int64      `index:"11" json:",omitempty"`
	CannotUnsendReason           int64      `index:"12" json:",omitempty"`
	Capabilities                 int64      `index:"13" json:",omitempty"`
	InviterId                    int64      `index:"14" json:",omitempty"`
	IgFolder                     int64      `index:"15" json:",omitempty"`
	ThreadSubtype                int64      `index:"16" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSRemoveAllParticipantsForThread struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadInviteLinksInfo struct {
	ThreadKey            int64  `index:"0" json:",omitempty"`
	ThreadInvitesEnabled int64  `index:"1" json:",omitempty"` // 0 or 1
	ThreadInviteLink     string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadParticipantAdminStatus struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	ContactId int64 `index:"1" json:",omitempty"`
	IsAdmin   bool  `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateParticipantSubscribeSourceText struct {
	ThreadKey       int64  `index:"0" json:",omitempty"`
	ContactId       int64  `index:"1" json:",omitempty"`
	SubscribeSource string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSOverwriteAllThreadParticipantsAdminStatus struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	IsAdmin   bool  `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateParticipantCapabilities struct {
	ContactId int64 `index:"0" json:",omitempty"`
	ThreadKey int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSChangeViewerStatus struct {
	ThreadKey         int64  `index:"0" json:",omitempty"`
	CannotReplyReason string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSSyncUpdateThreadName struct {
	ThreadName  string `index:"0" json:",omitempty"`
	ThreadKey   int64  `index:"1" json:",omitempty"`
	ThreadName1 string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (utn *LSSyncUpdateThreadName) GetThreadKey() int64 {
	return utn.ThreadKey
}

type LSSetThreadImageURL struct {
	ThreadKey                int64  `index:"0" json:",omitempty"`
	ImageURL                 string `index:"1" json:",omitempty"`
	ImageFallbackURL         string `index:"2" json:",omitempty"`
	ImageURLExpiryTimeMS     int64  `index:"3" json:",omitempty"`
	IsCustomThreadPicture    bool   `index:"4" json:",omitempty"`
	ShouldRoundThreadPicture bool   `index:"5" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSSetThreadImageURL) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSWriteCTAIdToThreadsTable struct {
	ThreadKey                 int64  `index:"0" json:",omitempty"`
	LastMessageCtaType        string `index:"1" json:",omitempty"`
	LastMessageCtaTimestampMs int64  `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSQueryAdditionalGroupThreads struct {
	NumThreads             int64 `index:"0" json:",omitempty"`
	NumMessages            int64 `index:"1" json:",omitempty"`
	AdditionalPagesToFetch int64 `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteThenInsertIgThreadInfo struct {
	ThreadKey  int64  `index:"0" json:",omitempty"`
	IgThreadId string `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSMarkThreadRead struct {
	LastReadWatermarkTimestampMs int64 `index:"0" json:",omitempty"`
	ThreadKey                    int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSMarkThreadRead) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSMarkThreadReadV2 struct {
	ThreadKey                    int64 `index:"0" json:",omitempty"`
	LastReadWatermarkTimestampMs int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSMarkThreadReadV2) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSUpdateParentFolderReadWatermark struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	// ShouldUpdate bool `index:"1" json:",omitempty"` // condition ?

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateOrInsertThread struct {
	LastActivityTimestampMs                int64      `index:"0" json:",omitempty"`
	LastReadWatermarkTimestampMs           int64      `index:"1" json:",omitempty"`
	Snippet                                string     `index:"2" json:",omitempty"`
	ThreadName                             string     `index:"3" json:",omitempty"`
	ThreadPictureUrl                       string     `index:"4" json:",omitempty"`
	NeedsAdminApprovalForNewParticipant    bool       `index:"5" json:",omitempty"`
	AuthorityLevel                         int64      `index:"6" json:",omitempty"`
	ThreadKey                              int64      `index:"7" json:",omitempty"`
	MailboxType                            int64      `index:"8" json:",omitempty"`
	ThreadType                             ThreadType `index:"9" json:",omitempty"`
	FolderName                             string     `index:"10" json:",omitempty"`
	ThreadPictureUrlFallback               string     `index:"11" json:",omitempty"`
	ThreadPictureUrlExpirationTimestampMs  int64      `index:"12" json:",omitempty"`
	RemoveWatermarkTimestampMs             int64      `index:"13" json:",omitempty"`
	MuteExpireTimeMs                       int64      `index:"14" json:",omitempty"`
	MuteMentionExpireTimeMs                int64      `index:"15" json:",omitempty"`
	MuteCallsExpireTimeMs                  int64      `index:"16" json:",omitempty"`
	GroupNotificationSettings              int64      `index:"19" json:",omitempty"`
	IsAdminSnippet                         bool       `index:"20" json:",omitempty"`
	SnippetSenderContactId                 int64      `index:"21" json:",omitempty"`
	SnippetStringHash                      int64      `index:"24" json:",omitempty"`
	SnippetStringArgument1                 int64      `index:"25" json:",omitempty"`
	SnippetAttribution                     int64      `index:"26" json:",omitempty"`
	SnippetAttributionStringHash           int64      `index:"27" json:",omitempty"`
	DisappearingSettingTtl                 int64      `index:"28" json:",omitempty"`
	DisappearingSettingUpdatedTs           int64      `index:"29" json:",omitempty"`
	DisappearingSettingUpdatedBy           int64      `index:"30" json:",omitempty"`
	OngoingCallState                       int64      `index:"32" json:",omitempty"`
	CannotReplyReason                      int64      `index:"33" json:",omitempty"`
	CustomEmoji                            string     `index:"34" json:",omitempty"`
	CustomEmojiImageUrl                    string     `index:"35" json:",omitempty"`
	OutgoingBubbleColor                    int64      `index:"36" json:",omitempty"`
	ThemeFbid                              int64      `index:"37" json:",omitempty"`
	ParentThreadKey                        int64      `index:"38" json:",omitempty"`
	NullstateDescriptionText1              string     `index:"39" json:",omitempty"`
	NullstateDescriptionType1              int64      `index:"40" json:",omitempty"`
	NullstateDescriptionText2              string     `index:"41" json:",omitempty"`
	NullstateDescriptionType2              int64      `index:"42" json:",omitempty"`
	NullstateDescriptionText3              string     `index:"43" json:",omitempty"`
	NullstateDescriptionType3              int64      `index:"44" json:",omitempty"`
	SnippetHasEmoji                        bool       `index:"45" json:",omitempty"`
	HasPersistentMenu                      bool       `index:"46" json:",omitempty"`
	DisableComposerInput                   bool       `index:"47" json:",omitempty"`
	CannotUnsendReason                     int64      `index:"48" json:",omitempty"`
	ViewedPluginKey                        int64      `index:"49" json:",omitempty"`
	ViewedPluginContext                    int64      `index:"50" json:",omitempty"`
	ClientThreadKey                        int64      `index:"51" json:",omitempty"`
	Capabilities                           int64      `index:"52" json:",omitempty"`
	ShouldRoundThreadPicture               int64      `index:"53" json:",omitempty"`
	ProactiveWarningDismissTime            int64      `index:"54" json:",omitempty"`
	IsCustomThreadPicture                  bool       `index:"55" json:",omitempty"`
	OtidOfFirstMessage                     int64      `index:"56" json:",omitempty"`
	NormalizedSearchTerms                  string     `index:"57" json:",omitempty"`
	AdditionalThreadContext                string     `index:"58" json:",omitempty"`
	DisappearingThreadKey                  int64      `index:"59" json:",omitempty"`
	IsDisappearingMode                     bool       `index:"60" json:",omitempty"`
	DisappearingModeInitiator              int64      `index:"61" json:",omitempty"`
	UnreadDisappearingMessageCount         int64      `index:"62" json:",omitempty"`
	LastMessageCtaId                       int64      `index:"64" json:",omitempty"`
	LastMessageCtaType                     string     `index:"65" json:",omitempty"`
	LastMessageCtaTimestampMs              int64      `index:"66" json:",omitempty"`
	ConsistentThreadFbid                   int64      `index:"67" json:",omitempty"`
	ThreadDescription                      string     `index:"69" json:",omitempty"`
	UnsendLimitMs                          int64      `index:"70" json:",omitempty"`
	Capabilities2                          int64      `index:"78" json:",omitempty"`
	Capabilities3                          int64      `index:"79" json:",omitempty"`
	SyncGroup                              int64      `index:"82" json:",omitempty"`
	ThreadInvitesEnabled                   int64      `index:"83" json:",omitempty"`
	ThreadInviteLink                       string     `index:"84" json:",omitempty"`
	IsAllUnreadMessageMissedCallXma        bool       `index:"85" json:",omitempty"`
	NumUnreadSubthreads                    int64      `index:"86" json:",omitempty"`
	SubthreadCount                         int64      `index:"87" json:",omitempty"`
	LastNonMissedCallXmaMessageTimestampMs int64      `index:"88" json:",omitempty"`
	ThreadInvitesEnabledV2                 int64      `index:"90" json:",omitempty"`
	JoinRequestApprovalSetting             int64      `index:"93" json:",omitempty"`
	HasPendingInvitation                   int64      `index:"94" json:",omitempty"`
	EventStartTimestampMs                  int64      `index:"95" json:",omitempty"`
	EventEndTimestampMs                    int64      `index:"96" json:",omitempty"`
	TakedownState                          int64      `index:"97" json:",omitempty"`
	SecondaryParentThreadKey               int64      `index:"98" json:",omitempty"`
	IgFolder                               int64      `index:"99" json:",omitempty"`
	InviterId                              int64      `index:"100" json:",omitempty"`
	ThreadTags                             int64      `index:"101" json:",omitempty"`
	IsReadReceiptsDisabled                 bool       `index:"102" json:",omitempty"`
	ReadReceiptsDisabledV2                 int64      `index:"103" json:",omitempty"`
	ThreadStatus                           int64      `index:"104" json:",omitempty"`
	ThreadSubtype                          int64      `index:"105" json:",omitempty"`
	PauseThreadTimestamp                   int64      `index:"106" json:",omitempty"`
	Capabilities4                          string     `index:"107" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (lsui *LSUpdateOrInsertThread) GetThreadKey() int64 {
	return lsui.ThreadKey
}

func (lsui *LSUpdateOrInsertThread) GetThreadName() string {
	return lsui.ThreadName
}

func (lsui *LSUpdateOrInsertThread) GetThreadPictureUrl() string {
	return lsui.ThreadPictureUrl
}

func (lsui *LSUpdateOrInsertThread) GetThreadType() ThreadType {
	return lsui.ThreadType
}

func (lsui *LSUpdateOrInsertThread) GetLastReadWatermarkTimestampMs() int64 {
	return lsui.LastReadWatermarkTimestampMs
}

func (lsui *LSUpdateOrInsertThread) GetThreadDescription() string {
	return lsui.ThreadDescription
}

func (lsui *LSUpdateOrInsertThread) GetFolderName() string {
	return lsui.FolderName
}

type LSSetThreadCannotUnsendReason struct {
	ThreadKey          int64 `index:"0" json:",omitempty"`
	CannotUnsendReason int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSClearLocalThreadPictureUrl struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateInviterId struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	InviterId int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSAddToMemberCount struct {
	ThreadKey      int64 `index:"0" json:",omitempty"`
	IncrementCount int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSMoveThreadToArchivedFolder struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSMoveThreadToE2EECutoverFolder struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSMoveThreadToE2EECutoverFolder) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSRemoveParticipantFromThread struct {
	ThreadKey     int64 `index:"0" json:",omitempty"`
	ParticipantId int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSRemoveParticipantFromThread) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSDeleteRtcRoomOnThread struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadTheme struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	Unknown1  int64 `index:"1" json:",omitempty"`
	Unknown2  int64 `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadApprovalMode struct {
	ThreadKey int64 `index:"0" json:",omitempty"`
	Value     bool  `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSRemoveAllRequestsFromAdminApprovalQueue struct {
	ThreadKey int64 `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteThread struct {
	ThreadKey    int64 `index:"0" json:",omitempty"`
	UnknownBool1 bool  `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSDeleteThread) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSUpdateThreadMuteSetting struct {
	ThreadKey        int64 `index:"0" json:",omitempty"`
	MuteExpireTimeMS int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSUpdateThreadMuteSetting) GetThreadKey() int64 {
	return ls.ThreadKey
}

type LSVerifyHybridThreadExists struct {
	ThreadKey                    int64      `index:"0" json:",omitempty"`
	ThreadJID                    int64      `index:"1" json:",omitempty"`
	ThreadType                   ThreadType `index:"2" json:",omitempty"`
	IsGroupThread                bool       `index:"5" json:",omitempty"`
	LastActivityTimestampMS      int64      `index:"6" json:",omitempty"`
	LastReadWatermarkTimestampMS int64      `index:"7" json:",omitempty"`
	// 8 = unknown bool (true)
	// 9 = unknown int64 (0)
	// 10 = unknown bool (false)

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateThreadAuthorityAndMappingWithOTIDFromJID struct {
	ThreadJID int64 `index:"0" json:",omitempty"`
	ThreadKey int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}
