package table

type DisplayedContentTypes int64

const (
	TEXT_MSG DisplayedContentTypes = 1
)

type ContactIDType int64

const (
	UnknownContactIDType ContactIDType = iota
	ContactIDTypeFBID
	ContactIDTypeSMSLocalID
)

type Gender int64

const (
	NOT_A_PERSON          Gender = 0
	FEMALE_SINGULAR       Gender = 1
	MALE_SINGULAR         Gender = 2
	FEMALE_SINGULAR_GUESS Gender = 3
	MALE_SINGULAR_GUESS   Gender = 4
	MIXED_UNKNOWN         Gender = 5
	NEUTER_SINGULAR       Gender = 6
	UNKNOWN_SINGULAR      Gender = 7
	FEMALE_PLURAL         Gender = 8
	MALE_PLURAL           Gender = 9
	NEUTER_PLURAL         Gender = 10
	UNKNOWN_PLURAL        Gender = 11
)

type ContactViewerRelationship int64

const (
	UNKNOWN_RELATIONSHIP ContactViewerRelationship = 0
	NOT_CONTACT          ContactViewerRelationship = 1
	CONTACT_OF_VIEWER    ContactViewerRelationship = 2
	FACEBOOK_FRIEND      ContactViewerRelationship = 3
	SOFT_CONTACT         ContactViewerRelationship = 4
)

type ThreadSourceType int64

const (
	/* communityFriendsDialog, pagesHomeFriendsDialog, mutualFriendsDialog, birthday, groupMembers, fundraiserSupportersList, memories, feedPoll, reactorList, friendsList, pagesPrivateReply, timeline, feedOrganicPost */
	FB_FEED_ORGANIC_POST ThreadSourceType = 1572865
	/* inboxPendingRequests */
	MESSENGER_INBOX_PENDING_REQUESTS ThreadSourceType = 65546
	/* fullscreenChat */
	MESSENGER_COMMUNITY_MESSAGING_FULLSCREEN_CHAT ThreadSourceType = 2293762
	/* sidebarGroupsList */
	SIDEBAR_CONTACTS_GROUPS ThreadSourceType = 2228227
	/* jewel */
	JEWEL_THREAD_LIST ThreadSourceType = 2097153
	/* shop */
	MINI_SHOP_VIEW_MENU_BUTTON ThreadSourceType = 2818048
	/* chatheadsOverflow */
	CHATHEADS_OVERFLOW ThreadSourceType = 2162690
	/* hovercard, feedDynamicHoverCard */
	FB_FEED_DYNAMIC_HOVER_CARD ThreadSourceType = 1572868
	/* search, messengerUniversalSearch */
	MESSENGER_UNIVERSAL_SEARCH ThreadSourceType = 131072
	/* story, storyAggregatedUsers, storySeenByList */
	FB_STORY ThreadSourceType = 1310720
	/* pageAboutCard */
	FB_PAGE_ABOUT_CARD ThreadSourceType = 786433
	/* inboxInThread */
	MESSENGER_INBOX_IN_THREAD ThreadSourceType = 65537
	/* notificationInThreadReply */
	MESSENGER_NOTIFICATION_IN_THREAD_REPLY ThreadSourceType = 524289
	/* archieve */
	MESSENGER_ARCHIVED_THREADS ThreadSourceType = 2031616
	/* storyViewerSheetRow */
	FB_STORY_VIEWER_SHEET_ROW ThreadSourceType = 1310722
	/* chatheadsNewMessage */
	CHATHEADS_NEW_MESSAGE ThreadSourceType = 2162691
	/* event */
	FB_EVENT ThreadSourceType = 1703936
	/* jewelSearch */
	JEWEL_SEARCH ThreadSourceType = 2097154
	/* inboxSpam, inboxThreadList, inboxRestricted */
	MESSENGER_INBOX ThreadSourceType = 65536
	/* jewelNewMessage */
	JEWEL_NEW_MESSAGE ThreadSourceType = 2097155
	/* pendingRequests */
	MESSENGER_PENDING_REQUESTS_INBOX_THREAD_LIST ThreadSourceType = 327681
	/* sidebarSearch */
	SIDEBAR_CONTACTS_SEARCH ThreadSourceType = 2228226
	/* inboxRemainingThreads */
	MESSENGER_INBOX_REMAINING_THREADS ThreadSourceType = 65541
	/* pagesHeader */
	FB_PAGE_PROFILE_HEADER_MESSAGE_BUTTON ThreadSourceType = 786434
	/* inboxRecentThreads */
	MESSENGER_INBOX_RECENT_THREADS ThreadSourceType = 65540
	/* chatheads */
	CHATHEADS ThreadSourceType = 2162688
	/* pageResponsivenessCard */
	FB_PAGE_RESPONSIVENESS_CONTEXT_CARD ThreadSourceType = 786437
	/* inboxSearch */
	MESSENGER_INBOX_MESSAGE_SEARCH ThreadSourceType = 65542
	/* jewelNestedFolder */
	JEWEL_NESTED_FOLDER ThreadSourceType = 2097156
	/* marketplace */
	MARKETPLACE_SEND_MESSAGE ThreadSourceType = 1245186
	/* feedOrganicPostViewAndMessage */
	FB_FEED_ORGANIC_POST_VIEW_AND_MESSAGE ThreadSourceType = 1572866
	/* adsCta */
	CLICK_TO_MESSENGER_AD_SEND_MESSAGE_CTA ThreadSourceType = 589826
	/* chatInThread */
	MESSENGER_CHAT_IN_THREAD ThreadSourceType = 1966082
	/* payments */
	PAYMENTS ThreadSourceType = 655360
	/* inboxFolder */
	MESSENGER_INBOX_NESTED_FOLDER ThreadSourceType = 65539
	/* inboxArchived */
	MESSENGER_INBOX_ARCHIVED_THREADS ThreadSourceType = 65545
	/* inboxActiveContacts */
	MESSENGER_INBOX_ACTIVE_CONTACTS ThreadSourceType = 65547
	/* sidebarContactsList */
	SIDEBAR_CONTACTS_LIST ThreadSourceType = 2228225
	/* sidebarCommunityChatsList */
	SIDEBAR_CONTACTS_COMMUNITY_CHATS ThreadSourceType = 2228228
	/* None */
	UNKNOWN_THREAD_SOURCE_TYPE ThreadSourceType = 0
)

type AttachmentType int64

const (
	AttachmentTypeNone AttachmentType = iota
	AttachmentTypeSticker
	AttachmentTypeImage
	AttachmentTypeAnimatedImage
	AttachmentTypeVideo
	AttachmentTypeAudio
	AttachmentTypeFile
	AttachmentTypeXMA
	AttachmentTypeEphemeralImage
	AttachmentTypeEphemeralVideo
	AttachmentTypeSelfieSticker
	_ // 11 is unknown
	AttachmentTypeSoundBite
	AttachmentTypeCatalogItem
	AttachmentTypePowerUp
	AttachmentTypeThirdPartySticker
)

type SendType int64

const (
	UNKNOWN_SEND_TYPE SendType = 0
	TEXT              SendType = 1
	STICKER           SendType = 2
	MEDIA             SendType = 3
	FORWARD           SendType = 5
	EXTERNAL_MEDIA    SendType = 7
)

type InitiatingSource int64

const (
	FACEBOOK_CHAT       InitiatingSource = 0
	FACEBOOK_INBOX      InitiatingSource = 1
	ROOMS_SIDE_CHAT     InitiatingSource = 2
	FACEBOOK_FULLSCREEN InitiatingSource = 3
)

type MessageUnsendabilityStatus int64

const (
	CAN_UNSEND                                      MessageUnsendabilityStatus = 0
	DENY_LOG_MESSAGE                                MessageUnsendabilityStatus = 1
	DENY_TOMBSTONE_MESSAGE                          MessageUnsendabilityStatus = 2
	DENY_FOR_NON_SENDER                             MessageUnsendabilityStatus = 3
	DENY_P2P_PAYMENT                                MessageUnsendabilityStatus = 4
	DENY_STORY_REACTION                             MessageUnsendabilityStatus = 5
	DENY_BLOB_ATTACHMENT                            MessageUnsendabilityStatus = 6
	DENY_MESSAGE_NOT_FOUND                          MessageUnsendabilityStatus = 7
	DENY_MESSAGE_INSTAGRAM_DIRECT_WRITE_RESTRICTION MessageUnsendabilityStatus = 8
)

type AppState int64

const (
	BACKGROUND AppState = 0 // not active
	FOREGROUND AppState = 1 // active
)

type ReactionStyle int64

const (
	UNKNOWN_REACTION_STYLE      ReactionStyle = 0
	BASIC_SUPER_REACT_ANIMATION ReactionStyle = 1
)

type RestrictionType int64

const (
	RestrictionTypeNone RestrictionType = iota
	RestrictionTypeDataPrivacy
	RestrictionTypeEncryptedThread
)

type SearchType int64

const (
	SearchTypeUnknown SearchType = iota
	SearchTypeContact
	SearchTypeNonContact
	SearchTypeGroup
	SearchTypePage
	SearchTypeIntegratedMessageSearchThread
	SearchTypeIGContactFollowing
	SearchTypeIGContactNonFollowing
	SearchTypeIGNonContactFollowing
	SearchTypeIGNonContactNonFollowing
	SearchTypeIGBusiness
	SearchTypeTAMContact
	SearchTypeTAMThread
	SearchTypeCommunityMessagingThread
	SearchTypePublicChannel
	SearchTypeSectionHeader
	SearchTypeAIBot
	SearchTypeCommunity
	SearchTypeMedia
	SearchTypeAttachment
	SearchTypeLink
	SearchTypeLocation
)

type ThreadType int64

func (tt ThreadType) IsOneToOne() bool {
	switch tt {
	case ONE_TO_ONE, TINCAN_ONE_TO_ONE, CARRIER_MESSAGING_ONE_TO_ONE, TINCAN_ONE_TO_ONE_DISAPPEARING, ENCRYPTED_OVER_WA_ONE_TO_ONE, AI_BOT:
		return true
	default:
		return false
	}
}

func (tt ThreadType) IsWhatsApp() bool {
	switch tt {
	case ENCRYPTED_OVER_WA_GROUP, ENCRYPTED_OVER_WA_ONE_TO_ONE:
		return true
	default:
		return false
	}
}

const (
	UNKNOWN_THREAD_TYPE                         ThreadType = 0
	ONE_TO_ONE                                  ThreadType = 1
	GROUP_THREAD                                ThreadType = 2
	ROOM                                        ThreadType = 3
	MONTAGE                                     ThreadType = 4
	MARKETPLACE                                 ThreadType = 5
	FOLDER                                      ThreadType = 6
	TINCAN_ONE_TO_ONE                           ThreadType = 7
	TINCAN_GROUP_DISAPPEARING                   ThreadType = 8
	CARRIER_MESSAGING_ONE_TO_ONE                ThreadType = 10
	CARRIER_MESSAGING_GROUP                     ThreadType = 11
	TINCAN_ONE_TO_ONE_DISAPPEARING              ThreadType = 13
	PAGE_FOLLOW_UP                              ThreadType = 14
	ENCRYPTED_OVER_WA_ONE_TO_ONE                ThreadType = 15
	ENCRYPTED_OVER_WA_GROUP                     ThreadType = 16
	COMMUNITY_FOLDER                            ThreadType = 17
	COMMUNITY_GROUP                             ThreadType = 18
	COMMUNITY_GROUP_UNJOINED                    ThreadType = 19
	COMMUNITY_CHANNEL_CATEGORY                  ThreadType = 20
	COMMUNITY_PRIVATE_HIDDEN_JOINED_THREAD      ThreadType = 21
	COMMUNITY_PRIVATE_HIDDEN_UNJOINED_THREAD    ThreadType = 22
	COMMUNITY_BROADCAST_JOINED_THREAD           ThreadType = 23
	COMMUNITY_BROADCAST_UNJOINED_THREAD         ThreadType = 24
	COMMUNITY_GROUP_INVITED_UNJOINED            ThreadType = 25
	COMMUNITY_SUB_THREAD                        ThreadType = 26
	PINNED                                      ThreadType = 101
	LWG                                         ThreadType = 102
	DISCOVERABLE_PUBLIC_CHAT                    ThreadType = 150
	DISCOVERABLE_PUBLIC_CHAT_UNJOINED           ThreadType = 151
	DISCOVERABLE_PUBLIC_BROADCAST_CHAT          ThreadType = 152
	DISCOVERABLE_PUBLIC_BROADCAST_CHAT_UNJOINED ThreadType = 153
	DISCOVERABLE_PUBLIC_CHAT_V2                 ThreadType = 154
	DISCOVERABLE_PUBLIC_CHAT_V2_UNJOINED        ThreadType = 155
	XAC_GROUP                                   ThreadType = 200
	AI_BOT                                      ThreadType = 201
)

type FolderType int64

const (
	INBOX    FolderType = 0
	PENDING  FolderType = 1
	OTHER    FolderType = 2
	SPAM     FolderType = 3
	ARCHIVED FolderType = 4
	HIDDEN   FolderType = 5
)

type ThreadBumpStatus int64

const (
	UNKNOWN_BUMP_STATUS ThreadBumpStatus = 0
	ACTIVITY_AND_READ   ThreadBumpStatus = 1
	ACTIVITY            ThreadBumpStatus = 2
	ACTIVITY_AND_READ_2 ThreadBumpStatus = 3
)

type ReplySourceTypeV2 int64

const (
	ReplySourceTypeNone ReplySourceTypeV2 = iota
	ReplySourceTypeMessage
	ReplySourceTypeStory
	ReplySourceTypeForward
	ReplySourceTypeFBStoryShare
	ReplySourceTypeIGStoryShare
	ReplySourceTypeStoryBase64Encoded
	ReplySourceTypeLightweightStatus
	ReplySourceTypeCloseFriends
	ReplySourceTypeXMA
	ReplySourceTypeIGNote
	ReplySourceTypeCloseFriendsNoteReply
	ReplySourceTypeLightweightStatusReaction
	ReplySourceTypeFBFeedPost
	ReplySourceTypeHighlightsTabPostReply
	ReplySourceTypeHighlightsTabLocalEventReply
	ReplySourceTypeSharedAlbum
	ReplySourceTypeAvatarDetail
)

type EphemeralMediaViewMode int64

const (
	EphemeralMediaViewOnce EphemeralMediaViewMode = iota
	EphemeralMediaReplayable
	EphemeralMediaPermanent
)

type EphemeralMediaState int64

const (
	EphemeralMediaStatePermanent EphemeralMediaState = iota
	EphemeralMediaStateUnseen
	EphemeralMediaStateSeen
	EphemeralMediaStateReplayed
	EphemeralMediaStateExpired
)
