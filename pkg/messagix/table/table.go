package table

import (
	"reflect"

	badGlobalLog "github.com/rs/zerolog/log"
)

/*
	Unknown fields = I don't know what type it is supposed to be, because I only see 9 which is undefined
	Trial and error works ^ check console for failed conversation within the decoder
*/

type LSTable struct {
	LSMciTraceLog                                    []*LSMciTraceLog                                    `json:",omitempty"`
	LSExecuteFirstBlockForSyncTransaction            []*LSExecuteFirstBlockForSyncTransaction            `json:",omitempty"`
	LSTruncateMetadataThreads                        []*LSTruncateMetadataThreads                        `json:",omitempty"`
	LSTruncateThreadRangeTablesForSyncGroup          []*LSTruncateThreadRangeTablesForSyncGroup          `json:",omitempty"`
	LSUpsertSyncGroupThreadsRange                    []*LSUpsertSyncGroupThreadsRange                    `json:",omitempty"`
	LSUpsertInboxThreadsRange                        []*LSUpsertInboxThreadsRange                        `json:",omitempty"`
	LSUpdateThreadsRangesV2                          []*LSUpdateThreadsRangesV2                          `json:",omitempty"`
	LSUpsertFolderSeenTimestamp                      []*LSUpsertFolderSeenTimestamp                      `json:",omitempty"`
	LSSetHMPSStatus                                  []*LSSetHMPSStatus                                  `json:",omitempty"`
	LSTruncateTablesForSyncGroup                     []*LSTruncateTablesForSyncGroup                     `json:",omitempty"`
	LSDeleteThenInsertThread                         []*LSDeleteThenInsertThread                         `json:",omitempty"`
	LSAddParticipantIdToGroupThread                  []*LSAddParticipantIdToGroupThread                  `json:",omitempty"`
	LSClearPinnedMessages                            []*LSClearPinnedMessages                            `json:",omitempty"`
	LSWriteThreadCapabilities                        []*LSWriteThreadCapabilities                        `json:",omitempty"`
	LSUpsertMessage                                  []*LSUpsertMessage                                  `json:",omitempty"`
	LSSetForwardScore                                []*LSSetForwardScore                                `json:",omitempty"`
	LSSetMessageDisplayedContentTypes                []*LSSetMessageDisplayedContentTypes                `json:",omitempty"`
	LSUpdateReadReceipt                              []*LSUpdateReadReceipt                              `json:",omitempty"`
	LSInsertNewMessageRange                          []*LSInsertNewMessageRange                          `json:",omitempty"`
	LSUpdateExistingMessageRange                     []*LSUpdateExistingMessageRange                     `json:",omitempty"`
	LSDeleteExistingMessageRanges                    []*LSDeleteExistingMessageRanges                    `json:",omitempty"`
	LSUpsertSequenceId                               []*LSUpsertSequenceID                               `json:",omitempty"`
	LSVerifyContactRowExists                         []*LSVerifyContactRowExists                         `json:",omitempty"`
	LSThreadsRangesQuery                             []*LSThreadsRangesQuery                             `json:",omitempty"`
	LSSetRegionHint                                  []*LSSetRegionHint                                  `json:",omitempty"`
	LSExecuteFinallyBlockForSyncTransaction          []*LSExecuteFinallyBlockForSyncTransaction          `json:",omitempty"`
	LSRemoveTask                                     []*LSRemoveTask                                     `json:",omitempty"`
	LSTaskExists                                     []*LSTaskExists                                     `json:",omitempty"`
	LSDeleteThenInsertContact                        []*LSDeleteThenInsertContact                        `json:",omitempty"`
	LSUpdateTypingIndicator                          []*LSUpdateTypingIndicator                          `json:",omitempty"`
	LSCheckAuthoritativeMessageExists                []*LSCheckAuthoritativeMessageExists                `json:",omitempty"`
	LSMoveThreadToInboxAndUpdateParent               []*LSMoveThreadToInboxAndUpdateParent               `json:",omitempty"`
	LSUpdateThreadSnippet                            []*LSUpdateThreadSnippet                            `json:",omitempty"`
	LSVerifyThreadExists                             []*LSVerifyThreadExists                             `json:",omitempty"`
	LSBumpThread                                     []*LSBumpThread                                     `json:",omitempty"`
	LSUpdateParticipantLastMessageSendTimestamp      []*LSUpdateParticipantLastMessageSendTimestamp      `json:",omitempty"`
	LSInsertMessage                                  []*LSInsertMessage                                  `json:",omitempty"`
	LSUpsertGradientColor                            []*LSUpsertGradientColor                            `json:",omitempty"`
	LSUpsertTheme                                    []*LSUpsertTheme                                    `json:",omitempty"`
	LSInsertStickerAttachment                        []*LSInsertStickerAttachment                        `json:",omitempty"`
	LSUpsertReaction                                 []*LSUpsertReaction                                 `json:",omitempty"`
	LSDeleteReaction                                 []*LSDeleteReaction                                 `json:",omitempty"`
	LSUpdateOrInsertReactionV2                       []*LSUpdateOrInsertReactionV2                       `json:",omitempty"`
	LSDeleteReactionV2                               []*LSDeleteReactionV2                               `json:",omitempty"`
	LSDeleteThenInsertReactionsV2Detail              []*LSDeleteThenInsertReactionsV2Detail              `json:",omitempty"`
	LSHandleRepliesOnUnsend                          []*LSHandleRepliesOnUnsend                          `json:",omitempty"`
	LSInsertXmaAttachment                            []*LSInsertXmaAttachment                            `json:",omitempty"`
	LSUpdateUnsentMessageCollapsedStatus             []*LSUpdateUnsentMessageCollapsedStatus             `json:",omitempty"`
	LSDeleteThenInsertMessage                        []*LSDeleteThenInsertMessage                        `json:",omitempty"`
	LSUpdateThreadSnippetFromLastMessage             []*LSUpdateThreadSnippetFromLastMessage             `json:",omitempty"`
	LSUpdateForRollCallMessageDeleted                []*LSUpdateForRollCallMessageDeleted                `json:",omitempty"`
	LSInsertBlobAttachment                           []*LSInsertBlobAttachment                           `json:",omitempty"`
	LSDeleteBannersByIds                             []*LSDeleteBannersByIds                             `json:",omitempty"`
	LSUpdateDeliveryReceipt                          []*LSUpdateDeliveryReceipt                          `json:",omitempty"`
	LSUpdateTaskQueueName                            []*LSUpdateTaskQueueName                            `json:",omitempty"`
	LSUpdateTaskValue                                []*LSUpdateTaskValue                                `json:",omitempty"`
	LSReplaceOptimsiticMessage                       []*LSReplaceOptimsiticMessage                       `json:",omitempty"`
	LSUpdateOptimisticContextThreadKeys              []*LSUpdateOptimisticContextThreadKeys              `json:",omitempty"`
	LSReplaceOptimisticThread                        []*LSReplaceOptimisticThread                        `json:",omitempty"`
	LSApplyNewGroupThread                            []*LSApplyNewGroupThread                            `json:",omitempty"`
	LSRemoveAllParticipantsForThread                 []*LSRemoveAllParticipantsForThread                 `json:",omitempty"`
	LSAppendDataTraceAddon                           []*LSAppendDataTraceAddon                           `json:",omitempty"`
	LSUpdateThreadInviteLinksInfo                    []*LSUpdateThreadInviteLinksInfo                    `json:",omitempty"`
	LSUpdateThreadParticipantAdminStatus             []*LSUpdateThreadParticipantAdminStatus             `json:",omitempty"`
	LSUpdateParticipantSubscribeSourceText           []*LSUpdateParticipantSubscribeSourceText           `json:",omitempty"`
	LSOverwriteAllThreadParticipantsAdminStatus      []*LSOverwriteAllThreadParticipantsAdminStatus      `json:",omitempty"`
	LSUpdateParticipantCapabilities                  []*LSUpdateParticipantCapabilities                  `json:",omitempty"`
	LSChangeViewerStatus                             []*LSChangeViewerStatus                             `json:",omitempty"`
	LSUpdateSearchQueryStatus                        []*LSUpdateSearchQueryStatus                        `json:",omitempty"`
	LSInsertSearchResult                             []*LSInsertSearchResult                             `json:",omitempty"`
	LSInsertSearchSection                            []*LSInsertSearchSection                            `json:",omitempty"`
	LSSyncUpdateThreadName                           []*LSSyncUpdateThreadName                           `json:",omitempty"`
	LSSetThreadImageURL                              []*LSSetThreadImageURL                              `json:",omitempty"`
	LSSetMessageTextHasLinks                         []*LSSetMessageTextHasLinks                         `json:",omitempty"`
	LSUpdateMessagesOptimisticContext                []*LSUpdateMessagesOptimisticContext                `json:",omitempty"`
	LSMailboxTaskCompletionApiOnTaskCompletion       []*LSMailboxTaskCompletionApiOnTaskCompletion       `json:",omitempty"`
	LSWriteCTAIdToThreadsTable                       []*LSWriteCTAIdToThreadsTable                       `json:",omitempty"`
	LSQueryAdditionalGroupThreads                    []*LSQueryAdditionalGroupThreads                    `json:",omitempty"`
	LSReplaceOptimisticReaction                      []*LSReplaceOptimisticReaction                      `json:",omitempty"`
	LSDeleteThenInsertMessageRequest                 []*LSDeleteThenInsertMessageRequest                 `json:",omitempty"`
	LSDeleteThenInsertIgThreadInfo                   []*LSDeleteThenInsertIgThreadInfo                   `json:",omitempty"`
	LSDeleteThenInsertContactPresence                []*LSDeleteThenInsertContactPresence                `json:",omitempty"`
	LSTruncatePresenceDatabase                       []*LSTruncatePresenceDatabase                       `json:",omitempty"`
	LSMarkThreadRead                                 []*LSMarkThreadRead                                 `json:",omitempty"`
	LSMarkThreadReadV2                               []*LSMarkThreadReadV2                               `json:",omitempty"`
	LSUpdateParentFolderReadWatermark                []*LSUpdateParentFolderReadWatermark                `json:",omitempty"`
	LSInsertAttachmentItem                           []*LSInsertAttachmentItem                           `json:",omitempty"`
	LSGetFirstAvailableAttachmentCTAID               []*LSGetFirstAvailableAttachmentCTAID               `json:",omitempty"`
	LSInsertAttachmentCta                            []*LSInsertAttachmentCta                            `json:",omitempty"`
	LSUpdateAttachmentItemCtaAtIndex                 []*LSUpdateAttachmentItemCtaAtIndex                 `json:",omitempty"`
	LSUpdateAttachmentCtaAtIndexIgnoringAuthority    []*LSUpdateAttachmentCtaAtIndexIgnoringAuthority    `json:",omitempty"`
	LSHasMatchingAttachmentCTA                       []*LSHasMatchingAttachmentCTA                       `json:",omitempty"`
	LSDeleteThenInsertIGContactInfo                  []*LSDeleteThenInsertIGContactInfo                  `json:",omitempty"`
	LSIssueNewTask                                   []*LSIssueNewTask                                   `json:",omitempty"`
	LSUpdateOrInsertThread                           []*LSUpdateOrInsertThread                           `json:",omitempty"`
	LSSetThreadCannotUnsendReason                    []*LSSetThreadCannotUnsendReason                    `json:",omitempty"`
	LSClearLocalThreadPictureUrl                     []*LSClearLocalThreadPictureUrl                     `json:",omitempty"`
	LSUpdateInviterId                                []*LSUpdateInviterId                                `json:",omitempty"`
	LSAddToMemberCount                               []*LSAddToMemberCount                               `json:",omitempty"`
	LSMoveThreadToArchivedFolder                     []*LSMoveThreadToArchivedFolder                     `json:",omitempty"`
	LSRemoveParticipantFromThread                    []*LSRemoveParticipantFromThread                    `json:",omitempty"`
	LSDeleteRtcRoomOnThread                          []*LSDeleteRtcRoomOnThread                          `json:",omitempty"`
	LSUpdateThreadTheme                              []*LSUpdateThreadTheme                              `json:",omitempty"`
	LSUpdateThreadApprovalMode                       []*LSUpdateThreadApprovalMode                       `json:",omitempty"`
	LSRemoveAllRequestsFromAdminApprovalQueue        []*LSRemoveAllRequestsFromAdminApprovalQueue        `json:",omitempty"`
	LSUpdateLastSyncCompletedTimestampMsToNow        []*LSUpdateLastSyncCompletedTimestampMsToNow        `json:",omitempty"`
	LSDeleteMessage                                  []*LSDeleteMessage                                  `json:",omitempty"`
	LSHandleRepliesOnRemove                          []*LSHandleRepliesOnRemove                          `json:",omitempty"`
	LSRefreshLastActivityTimestamp                   []*LSRefreshLastActivityTimestamp                   `json:",omitempty"`
	LSSetPinnedMessage                               []*LSSetPinnedMessage                               `json:",omitempty"`
	LSStoryContactSyncFromBucket                     []*LSStoryContactSyncFromBucket                     `json:",omitempty"`
	LSUpsertLiveLocationSharer                       []*LSUpsertLiveLocationSharer                       `json:",omitempty"`
	LSDeleteLiveLocationSharer                       []*LSDeleteLiveLocationSharer                       `json:",omitempty"`
	LSUpdateSharedAlbumOnMessageRecall               []*LSUpdateSharedAlbumOnMessageRecall               `json:",omitempty"`
	LSEditMessage                                    []*LSEditMessage                                    `json:",omitempty"`
	LSHandleRepliesOnMessageEdit                     []*LSHandleRepliesOnMessageEdit                     `json:",omitempty"`
	LSUpdateThreadSnippetFromLastMessageV2           []*LSUpdateThreadSnippetFromLastMessageV2           `json:",omitempty"`
	LSMarkOptimisticMessageFailed                    []*LSMarkOptimisticMessageFailed                    `json:",omitempty"`
	LSUpdateSubscriptErrorMessage                    []*LSUpdateSubscriptErrorMessage                    `json:",omitempty"`
	LSDeleteThenInsertBotProfileInfoCategoryV2       []*LSDeleteThenInsertBotProfileInfoCategoryV2       `json:",omitempty"`
	LSDeleteThenInsertBotProfileInfoV2               []*LSDeleteThenInsertBotProfileInfoV2               `json:",omitempty"`
	LSHandleSyncFailure                              []*LSHandleSyncFailure                              `json:",omitempty"`
	LSDeleteThread                                   []*LSDeleteThread                                   `json:",omitempty"`
	LSAddPollForThread                               []*LSAddPollForThread                               `json:",omitempty"`
	LSAddPollOption                                  []*LSAddPollOption                                  `json:",omitempty"`
	LSAddPollOptionV2                                []*LSAddPollOption                                  `json:",omitempty"`
	LSAddPollVote                                    []*LSAddPollVote                                    `json:",omitempty"`
	LSAddPollVoteV2                                  []*LSAddPollVote                                    `json:",omitempty"`
	LSUpdateThreadMuteSetting                        []*LSUpdateThreadMuteSetting                        `json:",omitempty"`
	LSInsertAttachment                               []*LSInsertAttachment                               `json:",omitempty"`
	LSUpdateExtraAttachmentColumns                   []*LSUpdateExtraAttachmentColumns                   `json:",omitempty"`
	LSMoveThreadToE2EECutoverFolder                  []*LSMoveThreadToE2EECutoverFolder                  `json:",omitempty"`
	LSHandleFailedTask                               []*LSHandleFailedTask                               `json:",omitempty"`
	LSUpdateOrInsertEditMessageHistory               []*LSUpdateOrInsertEditMessageHistory               `json:",omitempty"`
	LSVerifyHybridThreadExists                       []*LSVerifyHybridThreadExists                       `json:",omitempty"`
	LSUpdateThreadAuthorityAndMappingWithOTIDFromJID []*LSUpdateThreadAuthorityAndMappingWithOTIDFromJID `json:",omitempty"`
	LSVerifyContactParticipantExist                  []*LSVerifyContactParticipantExist                  `json:",omitempty"`
	LSVerifyCommunityMemberContextualProfileExists   []*LSVerifyCommunityMemberContextualProfileExists   `json:",omitempty"`
	LSInsertCommunityMember                          []*LSInsertCommunityMember                          `json:",omitempty"`
	LSUpdateOrInsertCommunityMember                  []*LSUpdateOrInsertCommunityMember                  `json:",omitempty"`
	LSUpsertCommunityMemberRanges                    []*LSUpsertCommunityMemberRanges                    `json:",omitempty"`
	LSUpdateSubThreadXMA                             []*LSUpdateSubThreadXMA                             `json:",omitempty"`
	LSSetNumUnreadSubthreads                         []*LSSetNumUnreadSubthreads                         `json:",omitempty"`
}

func (t *LSTable) NonNilFields() (fields []string) {
	if t == nil {
		return
	}
	reflectedTable := reflect.ValueOf(t).Elem()
	for _, field := range reflect.VisibleFields(reflectedTable.Type()) {
		if reflectedTable.FieldByName(field.Name).IsNil() {
			continue
		}
		fields = append(fields, field.Name)
	}
	return
}

// TODO replace SPTable with struct tags

var SPTable = map[string]string{
	"removeAllRequestsFromAdminApprovalQueue":        "LSRemoveAllRequestsFromAdminApprovalQueue",
	"updateThreadApprovalMode":                       "LSUpdateThreadApprovalMode",
	"updateThreadTheme":                              "LSUpdateThreadTheme",
	"deleteRtcRoomOnThread":                          "LSDeleteRtcRoomOnThread",
	"removeParticipantFromThread":                    "LSRemoveParticipantFromThread",
	"moveThreadToArchivedFolder":                     "LSMoveThreadToArchivedFolder",
	"setThreadCannotUnsendReason":                    "LSSetThreadCannotUnsendReason",
	"clearLocalThreadPictureUrl":                     "LSClearLocalThreadPictureUrl",
	"updateInviterId":                                "LSUpdateInviterId",
	"addToMemberCount":                               "LSAddToMemberCount",
	"updateOrInsertThread":                           "LSUpdateOrInsertThread",
	"issueNewTask":                                   "LSIssueNewTask",
	"deleteThenInsertIGContactInfo":                  "LSDeleteThenInsertIGContactInfo",
	"hasMatchingAttachmentCTA":                       "LSHasMatchingAttachmentCTA",
	"updateAttachmentCtaAtIndexIgnoringAuthority":    "LSUpdateAttachmentCtaAtIndexIgnoringAuthority",
	"updateAttachmentItemCtaAtIndex":                 "LSUpdateAttachmentItemCtaAtIndex",
	"insertAttachmentCta":                            "LSInsertAttachmentCta",
	"getFirstAvailableAttachmentCTAID":               "LSGetFirstAvailableAttachmentCTAID",
	"insertAttachmentItem":                           "LSInsertAttachmentItem",
	"updateParentFolderReadWatermark":                "LSUpdateParentFolderReadWatermark",
	"markThreadRead":                                 "LSMarkThreadRead",
	"markThreadReadV2":                               "LSMarkThreadReadV2",
	"truncatePresenceDatabase":                       "LSTruncatePresenceDatabase",
	"deleteThenInsertContactPresence":                "LSDeleteThenInsertContactPresence",
	"deleteThenInsertIgThreadInfo":                   "LSDeleteThenInsertIgThreadInfo",
	"deleteThenInsertMessageRequest":                 "LSDeleteThenInsertMessageRequest",
	"replaceOptimisticReaction":                      "LSReplaceOptimisticReaction",
	"queryAdditionalGroupThreads":                    "LSQueryAdditionalGroupThreads",
	"writeCTAIdToThreadsTable":                       "LSWriteCTAIdToThreadsTable",
	"mailboxTaskCompletionApiOnTaskCompletion":       "LSMailboxTaskCompletionApiOnTaskCompletion",
	"updateMessagesOptimisticContext":                "LSUpdateMessagesOptimisticContext",
	"setMessageTextHasLinks":                         "LSSetMessageTextHasLinks",
	"syncUpdateThreadName":                           "LSSyncUpdateThreadName",
	"setThreadImageURL":                              "LSSetThreadImageURL",
	"insertSearchSection":                            "LSInsertSearchSection",
	"insertSearchResult":                             "LSInsertSearchResult",
	"updateSearchQueryStatus":                        "LSUpdateSearchQueryStatus",
	"changeViewerStatus":                             "LSChangeViewerStatus",
	"updateParticipantCapabilities":                  "LSUpdateParticipantCapabilities",
	"overwriteAllThreadParticipantsAdminStatus":      "LSOverwriteAllThreadParticipantsAdminStatus",
	"updateParticipantSubscribeSourceText":           "LSUpdateParticipantSubscribeSourceText",
	"updateThreadParticipantAdminStatus":             "LSUpdateThreadParticipantAdminStatus",
	"updateThreadInviteLinksInfo":                    "LSUpdateThreadInviteLinksInfo",
	"appendDataTraceAddon":                           "LSAppendDataTraceAddon",
	"removeAllParticipantsForThread":                 "LSRemoveAllParticipantsForThread",
	"applyNewGroupThread":                            "LSApplyNewGroupThread",
	"replaceOptimisticThread":                        "LSReplaceOptimisticThread",
	"updateOptimisticContextThreadKeys":              "LSUpdateOptimisticContextThreadKeys",
	"replaceOptimsiticMessage":                       "LSReplaceOptimsiticMessage",
	"updateTaskQueueName":                            "LSUpdateTaskQueueName",
	"updateTaskValue":                                "LSUpdateTaskValue",
	"updateDeliveryReceipt":                          "LSUpdateDeliveryReceipt",
	"deleteBannersByIds":                             "LSDeleteBannersByIds",
	"truncateTablesForSyncGroup":                     "LSTruncateTablesForSyncGroup",
	"insertXmaAttachment":                            "LSInsertXmaAttachment",
	"insertNewMessageRange":                          "LSInsertNewMessageRange",
	"updateExistingMessageRange":                     "LSUpdateExistingMessageRange",
	"threadsRangesQuery":                             "LSThreadsRangesQuery",
	"updateThreadSnippetFromLastMessage":             "LSUpdateThreadSnippetFromLastMessage",
	"upsertInboxThreadsRange":                        "LSUpsertInboxThreadsRange",
	"deleteThenInsertThread":                         "LSDeleteThenInsertThread",
	"addParticipantIdToGroupThread":                  "LSAddParticipantIdToGroupThread",
	"upsertMessage":                                  "LSUpsertMessage",
	"clearPinnedMessages":                            "LSClearPinnedMessages",
	"mciTraceLog":                                    "LSMciTraceLog",
	"insertBlobAttachment":                           "LSInsertBlobAttachment",
	"updateUnsentMessageCollapsedStatus":             "LSUpdateUnsentMessageCollapsedStatus",
	"executeFirstBlockForSyncTransaction":            "LSExecuteFirstBlockForSyncTransaction",
	"updateThreadsRangesV2":                          "LSUpdateThreadsRangesV2",
	"upsertSyncGroupThreadsRange":                    "LSUpsertSyncGroupThreadsRange",
	"upsertFolderSeenTimestamp":                      "LSUpsertFolderSeenTimestamp",
	"setHMPSStatus":                                  "LSSetHMPSStatus",
	"handleRepliesOnUnsend":                          "LSHandleRepliesOnUnsend",
	"deleteExistingMessageRanges":                    "LSDeleteExistingMessageRanges",
	"writeThreadCapabilities":                        "LSWriteThreadCapabilities",
	"upsertSequenceId":                               "LSUpsertSequenceId",
	"executeFinallyBlockForSyncTransaction":          "LSExecuteFinallyBlockForSyncTransaction",
	"verifyContactRowExists":                         "LSVerifyContactRowExists",
	"taskExists":                                     "LSTaskExists",
	"removeTask":                                     "LSRemoveTask",
	"deleteThenInsertMessage":                        "LSDeleteThenInsertMessage",
	"deleteThenInsertContact":                        "LSDeleteThenInsertContact",
	"updateTypingIndicator":                          "LSUpdateTypingIndicator",
	"checkAuthoritativeMessageExists":                "LSCheckAuthoritativeMessageExists",
	"moveThreadToInboxAndUpdateParent":               "LSMoveThreadToInboxAndUpdateParent",
	"updateThreadSnippet":                            "LSUpdateThreadSnippet",
	"setMessageDisplayedContentTypes":                "LSSetMessageDisplayedContentTypes",
	"verifyThreadExists":                             "LSVerifyThreadExists",
	"updateReadReceipt":                              "LSUpdateReadReceipt",
	"setForwardScore":                                "LSSetForwardScore",
	"upsertReaction":                                 "LSUpsertReaction",
	"updateOrInsertReactionV2":                       "LSUpdateOrInsertReactionV2",
	"deleteReactionV2":                               "LSDeleteReactionV2",
	"deleteThenInsertReactionsV2Detail":              "LSDeleteThenInsertReactionsV2Detail",
	"bumpThread":                                     "LSBumpThread",
	"updateParticipantLastMessageSendTimestamp":      "LSUpdateParticipantLastMessageSendTimestamp",
	"insertMessage":                                  "LSInsertMessage",
	"upsertTheme":                                    "LSUpsertTheme",
	"upsertGradientColor":                            "LSUpsertGradientColor",
	"insertStickerAttachment":                        "LSInsertStickerAttachment",
	"updateForRollCallMessageDeleted":                "LSUpdateForRollCallMessageDeleted",
	"updateLastSyncCompletedTimestampMsToNow":        "LSUpdateLastSyncCompletedTimestampMsToNow",
	"deleteMessage":                                  "LSDeleteMessage",
	"handleRepliesOnRemove":                          "LSHandleRepliesOnRemove",
	"refreshLastActivityTimestamp":                   "LSRefreshLastActivityTimestamp",
	"setPinnedMessage":                               "LSSetPinnedMessage",
	"storyContactSyncFromBucket":                     "LSStoryContactSyncFromBucket",
	"upsertLiveLocationSharer":                       "LSUpsertLiveLocationSharer",
	"deleteLiveLocationSharer":                       "LSDeleteLiveLocationSharer",
	"updateSharedAlbumOnMessageRecall":               "LSUpdateSharedAlbumOnMessageRecall",
	"editMessage":                                    "LSEditMessage",
	"handleRepliesOnMessageEdit":                     "LSHandleRepliesOnMessageEdit",
	"updateThreadSnippetFromLastMessageV2":           "LSUpdateThreadSnippetFromLastMessageV2",
	"markOptimisticMessageFailed":                    "LSMarkOptimisticMessageFailed",
	"updateSubscriptErrorMessage":                    "LSUpdateSubscriptErrorMessage",
	"deleteThenInsertBotProfileInfoCategoryV2":       "LSDeleteThenInsertBotProfileInfoCategoryV2",
	"deleteThenInsertBotProfileInfoV2":               "LSDeleteThenInsertBotProfileInfoV2",
	"handleSyncFailure":                              "LSHandleSyncFailure",
	"deleteThread":                                   "LSDeleteThread",
	"addPollOption":                                  "LSAddPollOption",
	"addPollOptionV2":                                "LSAddPollOptionV2",
	"addPollVote":                                    "LSAddPollVote",
	"addPollVoteV2":                                  "LSAddPollVoteV2",
	"addPollForThread":                               "LSAddPollForThread",
	"deleteReaction":                                 "LSDeleteReaction",
	"updateThreadMuteSetting":                        "LSUpdateThreadMuteSetting",
	"insertAttachment":                               "LSInsertAttachment",
	"updateExtraAttachmentColumns":                   "LSUpdateExtraAttachmentColumns",
	"moveThreadToE2EECutoverFolder":                  "LSMoveThreadToE2EECutoverFolder",
	"handleFailedTask":                               "LSHandleFailedTask",
	"updateOrInsertEditMessageHistory":               "LSUpdateOrInsertEditMessageHistory",
	"verifyHybridThreadExists":                       "LSVerifyHybridThreadExists",
	"updateThreadAuthorityAndMappingWithOTIDFromJID": "LSUpdateThreadAuthorityAndMappingWithOTIDFromJID",
	"verifyContactParticipantExist":                  "LSVerifyContactParticipantExist",
	"verifyCommunityMemberContextualProfileExists":   "LSVerifyCommunityMemberContextualProfileExists",
	"insertCommunityMember":                          "LSInsertCommunityMember",
	"updateOrInsertCommunityMember":                  "LSUpdateOrInsertCommunityMember",
	"upsertCommunityMemberRanges":                    "LSUpsertCommunityMemberRanges",
	"updateSubThreadXMA":                             "LSUpdateSubThreadXMA",
	"setNumUnreadSubthreads":                         "LSSetNumUnreadSubthreads",
}

func SPToDepMap(sp []string) map[string]string {
	m := make(map[string]string, 0)
	for _, d := range sp {
		depName, ok := SPTable[d]
		if !ok {
			badGlobalLog.Warn().Str("dependency", d).Msg("Unknown dependency in sp")
			continue
		}
		m[d] = depName
	}
	return m
}
