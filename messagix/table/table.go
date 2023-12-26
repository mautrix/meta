package table

import (
	"fmt"
	"log"
)

/*
	Unknown fields = I don't know what type it is supposed to be, because I only see 9 which is undefined
	Trial and error works ^ check console for failed conversation within the decoder
*/

type LSTable struct {
	LSMciTraceLog []LSMciTraceLog
	LSExecuteFirstBlockForSyncTransaction []LSExecuteFirstBlockForSyncTransaction
	LSTruncateMetadataThreads []LSTruncateMetadataThreads
	LSTruncateThreadRangeTablesForSyncGroup []LSTruncateThreadRangeTablesForSyncGroup
	LSUpsertSyncGroupThreadsRange []LSUpsertSyncGroupThreadsRange
	LSUpsertInboxThreadsRange []LSUpsertInboxThreadsRange
	LSUpdateThreadsRangesV2 []LSUpdateThreadsRangesV2
	LSUpsertFolderSeenTimestamp []LSUpsertFolderSeenTimestamp
	LSSetHMPSStatus []LSSetHMPSStatus
	LSTruncateTablesForSyncGroup []LSTruncateTablesForSyncGroup
	LSDeleteThenInsertThread []LSDeleteThenInsertThread
	LSAddParticipantIdToGroupThread []LSAddParticipantIdToGroupThread
	LSClearPinnedMessages []LSClearPinnedMessages
	LSWriteThreadCapabilities []LSWriteThreadCapabilities
	LSUpsertMessage []LSUpsertMessage
	LSSetForwardScore []LSSetForwardScore
	LSSetMessageDisplayedContentTypes []LSSetMessageDisplayedContentTypes
	LSUpdateReadReceipt []LSUpdateReadReceipt
	LSInsertNewMessageRange []LSInsertNewMessageRange
	LSDeleteExistingMessageRanges []LSDeleteExistingMessageRanges
	LSUpsertSequenceId []LSUpsertSequenceId
	LSVerifyContactRowExists []LSVerifyContactRowExists
	LSThreadsRangesQuery []LSThreadsRangesQuery
	LSSetRegionHint []LSSetRegionHint
	LSExecuteFinallyBlockForSyncTransaction []LSExecuteFinallyBlockForSyncTransaction
	LSRemoveTask []LSRemoveTask
	LSTaskExists []LSTaskExists
	LSDeleteThenInsertContact []LSDeleteThenInsertContact
	LSUpdateTypingIndicator []LSUpdateTypingIndicator
	LSCheckAuthoritativeMessageExists []LSCheckAuthoritativeMessageExists
	LSMoveThreadToInboxAndUpdateParent []LSMoveThreadToInboxAndUpdateParent
	LSUpdateThreadSnippet []LSUpdateThreadSnippet
	LSVerifyThreadExists []LSVerifyThreadExists
	LSBumpThread []LSBumpThread
	LSUpdateParticipantLastMessageSendTimestamp []LSUpdateParticipantLastMessageSendTimestamp
	LSInsertMessage []LSInsertMessage
	LSUpsertGradientColor []LSUpsertGradientColor
	LSUpsertTheme []LSUpsertTheme
	LSInsertStickerAttachment []LSInsertStickerAttachment
	LSUpsertReaction []LSUpsertReaction
	LSDeleteReaction []LSDeleteReaction
	LSHandleRepliesOnUnsend []LSHandleRepliesOnUnsend
	LSInsertXmaAttachment []LSInsertXmaAttachment
	LSUpdateUnsentMessageCollapsedStatus []LSUpdateUnsentMessageCollapsedStatus
	LSDeleteThenInsertMessage []LSDeleteThenInsertMessage
	LSUpdateThreadSnippetFromLastMessage []LSUpdateThreadSnippetFromLastMessage
	LSUpdateForRollCallMessageDeleted []LSUpdateForRollCallMessageDeleted
	LSInsertBlobAttachment []LSInsertBlobAttachment
	LSDeleteBannersByIds []LSDeleteBannersByIds
	LSUpdateDeliveryReceipt []LSUpdateDeliveryReceipt
	LSUpdateTaskQueueName []LSUpdateTaskQueueName
	LSUpdateTaskValue []LSUpdateTaskValue
	LSReplaceOptimsiticMessage []LSReplaceOptimsiticMessage
	LSUpdateOptimisticContextThreadKeys []LSUpdateOptimisticContextThreadKeys
	LSReplaceOptimisticThread []LSReplaceOptimisticThread
	LSApplyNewGroupThread []LSApplyNewGroupThread
	LSRemoveAllParticipantsForThread []LSRemoveAllParticipantsForThread
	LSAppendDataTraceAddon []LSAppendDataTraceAddon
	LSUpdateThreadInviteLinksInfo []LSUpdateThreadInviteLinksInfo
	LSUpdateThreadParticipantAdminStatus []LSUpdateThreadParticipantAdminStatus
	LSUpdateParticipantSubscribeSourceText []LSUpdateParticipantSubscribeSourceText
	LSOverwriteAllThreadParticipantsAdminStatus []LSOverwriteAllThreadParticipantsAdminStatus
	LSUpdateParticipantCapabilities []LSUpdateParticipantCapabilities
	LSChangeViewerStatus []LSChangeViewerStatus
	LSUpdateSearchQueryStatus []LSUpdateSearchQueryStatus
	LSInsertSearchResult []LSInsertSearchResult
	LSInsertSearchSection []LSInsertSearchSection
	LSSyncUpdateThreadName []LSSyncUpdateThreadName
	LSSetMessageTextHasLinks []LSSetMessageTextHasLinks
	LSUpdateMessagesOptimisticContext []LSUpdateMessagesOptimisticContext
	LSMailboxTaskCompletionApiOnTaskCompletion []LSMailboxTaskCompletionApiOnTaskCompletion
	LSWriteCTAIdToThreadsTable []LSWriteCTAIdToThreadsTable
	LSQueryAdditionalGroupThreads []LSQueryAdditionalGroupThreads
	LSReplaceOptimisticReaction []LSReplaceOptimisticReaction
	LSDeleteThenInsertMessageRequest []LSDeleteThenInsertMessageRequest
	LSDeleteThenInsertIgThreadInfo []LSDeleteThenInsertIgThreadInfo
	LSDeleteThenInsertContactPresence []LSDeleteThenInsertContactPresence
	LSTruncatePresenceDatabase []LSTruncatePresenceDatabase
	LSMarkThreadRead []LSMarkThreadRead
	LSUpdateParentFolderReadWatermark []LSUpdateParentFolderReadWatermark
	LSInsertAttachmentItem []LSInsertAttachmentItem
	LSGetFirstAvailableAttachmentCTAID []LSGetFirstAvailableAttachmentCTAID
	LSInsertAttachmentCta []LSInsertAttachmentCta
	LSUpdateAttachmentItemCtaAtIndex []LSUpdateAttachmentItemCtaAtIndex
	LSUpdateAttachmentCtaAtIndexIgnoringAuthority []LSUpdateAttachmentCtaAtIndexIgnoringAuthority
	LSHasMatchingAttachmentCTA []LSHasMatchingAttachmentCTA
	LSDeleteThenInsertIGContactInfo []LSDeleteThenInsertIGContactInfo
	LSIssueNewTask []LSIssueNewTask
	LSUpdateOrInsertThread []LSUpdateOrInsertThread
	LSSetThreadCannotUnsendReason []LSSetThreadCannotUnsendReason
	LSClearLocalThreadPictureUrl []LSClearLocalThreadPictureUrl
	LSUpdateInviterId []LSUpdateInviterId
	LSAddToMemberCount []LSAddToMemberCount
	LSMoveThreadToArchivedFolder []LSMoveThreadToArchivedFolder
	LSRemoveParticipantFromThread []LSRemoveParticipantFromThread
	LSDeleteRtcRoomOnThread []LSDeleteRtcRoomOnThread
	LSUpdateThreadTheme []LSUpdateThreadTheme
	LSUpdateThreadApprovalMode []LSUpdateThreadApprovalMode
	LSRemoveAllRequestsFromAdminApprovalQueue []LSRemoveAllRequestsFromAdminApprovalQueue
}

var SPTable = map[string]string{
	"removeAllRequestsFromAdminApprovalQueue": "LSRemoveAllRequestsFromAdminApprovalQueue",
	"updateThreadApprovalMode": "LSUpdateThreadApprovalMode",
	"updateThreadTheme": "LSUpdateThreadTheme",
	"deleteRtcRoomOnThread": "LSDeleteRtcRoomOnThread",
	"removeParticipantFromThread": "LSRemoveParticipantFromThread",
	"moveThreadToArchivedFolder": "LSMoveThreadToArchivedFolder",
	"setThreadCannotUnsendReason": "LSSetThreadCannotUnsendReason",
	"clearLocalThreadPictureUrl": "LSClearLocalThreadPictureUrl",
	"updateInviterId": "LSUpdateInviterId",
	"addToMemberCount": "LSAddToMemberCount",
	"updateOrInsertThread": "LSUpdateOrInsertThread",
	"issueNewTask": "LSIssueNewTask",
	"deleteThenInsertIGContactInfo": "LSDeleteThenInsertIGContactInfo",
	"hasMatchingAttachmentCTA": "LSHasMatchingAttachmentCTA",
	"updateAttachmentCtaAtIndexIgnoringAuthority": "LSUpdateAttachmentCtaAtIndexIgnoringAuthority",
	"updateAttachmentItemCtaAtIndex": "LSUpdateAttachmentItemCtaAtIndex",
	"insertAttachmentCta": "LSInsertAttachmentCta",
	"getFirstAvailableAttachmentCTAID": "LSGetFirstAvailableAttachmentCTAID",
	"insertAttachmentItem": "LSInsertAttachmentItem",
	"updateParentFolderReadWatermark": "LSUpdateParentFolderReadWatermark",
	"markThreadRead": "LSMarkThreadRead",
	"truncatePresenceDatabase": "LSTruncatePresenceDatabase",
	"deleteThenInsertContactPresence": "LSDeleteThenInsertContactPresence",
	"deleteThenInsertIgThreadInfo": "LSDeleteThenInsertIgThreadInfo",
	"deleteThenInsertMessageRequest": "LSDeleteThenInsertMessageRequest",
	"replaceOptimisticReaction": "LSReplaceOptimisticReaction",
	"queryAdditionalGroupThreads": "LSQueryAdditionalGroupThreads",
	"writeCTAIdToThreadsTable": "LSWriteCTAIdToThreadsTable",
	"mailboxTaskCompletionApiOnTaskCompletion": "LSMailboxTaskCompletionApiOnTaskCompletion",
	"updateMessagesOptimisticContext": "LSUpdateMessagesOptimisticContext",
	"setMessageTextHasLinks": "LSSetMessageTextHasLinks",
	"syncUpdateThreadName": "LSSyncUpdateThreadName",
	"insertSearchSection": "LSInsertSearchSection",
	"insertSearchResult": "LSInsertSearchResult",
	"updateSearchQueryStatus": "LSUpdateSearchQueryStatus",
	"changeViewerStatus": "LSChangeViewerStatus",
	"updateParticipantCapabilities": "LSUpdateParticipantCapabilities",
	"overwriteAllThreadParticipantsAdminStatus": "LSOverwriteAllThreadParticipantsAdminStatus",
	"updateParticipantSubscribeSourceText": "LSUpdateParticipantSubscribeSourceText",
	"updateThreadParticipantAdminStatus": "LSUpdateThreadParticipantAdminStatus",
	"updateThreadInviteLinksInfo": "LSUpdateThreadInviteLinksInfo",
	"appendDataTraceAddon": "LSAppendDataTraceAddon",
	"removeAllParticipantsForThread": "LSRemoveAllParticipantsForThread",
	"applyNewGroupThread": "LSApplyNewGroupThread",
	"replaceOptimisticThread": "LSReplaceOptimisticThread",
	"updateOptimisticContextThreadKeys": "LSUpdateOptimisticContextThreadKeys",
	"replaceOptimsiticMessage": "LSReplaceOptimsiticMessage",
	"updateTaskQueueName": "LSUpdateTaskQueueName",
	"updateTaskValue": "LSUpdateTaskValue",
	"updateDeliveryReceipt": "LSUpdateDeliveryReceipt",
	"deleteBannersByIds": "LSDeleteBannersByIds",
	"truncateTablesForSyncGroup": "LSTruncateTablesForSyncGroup",
	"insertXmaAttachment": "LSInsertXmaAttachment",
	"insertNewMessageRange": "LSInsertNewMessageRange",
	"threadsRangesQuery": "LSThreadsRangesQuery",
	"updateThreadSnippetFromLastMessage": "LSUpdateThreadSnippetFromLastMessage",
	"upsertInboxThreadsRange": "LSUpsertInboxThreadsRange",
	"deleteThenInsertThread": "LSDeleteThenInsertThread",
	"addParticipantIdToGroupThread": "LSAddParticipantIdToGroupThread",
	"upsertMessage": "LSUpsertMessage",
	"clearPinnedMessages": "LSClearPinnedMessages",
	"mciTraceLog": "LSMciTraceLog",
	"insertBlobAttachment": "LSInsertBlobAttachment",
	"updateUnsentMessageCollapsedStatus": "LSUpdateUnsentMessageCollapsedStatus",
	"executeFirstBlockForSyncTransaction": "LSExecuteFirstBlockForSyncTransaction",
	"updateThreadsRangesV2": "LSUpdateThreadsRangesV2",
	"upsertSyncGroupThreadsRange": "LSUpsertSyncGroupThreadsRange",
	"upsertFolderSeenTimestamp": "LSUpsertFolderSeenTimestamp",
	"setHMPSStatus": "LSSetHMPSStatus",
	"handleRepliesOnUnsend": "LSHandleRepliesOnUnsend",
	"deleteExistingMessageRanges": "LSDeleteExistingMessageRanges",
	"writeThreadCapabilities": "LSWriteThreadCapabilities",
	"upsertSequenceId": "LSUpsertSequenceId",
	"executeFinallyBlockForSyncTransaction": "LSExecuteFinallyBlockForSyncTransaction",
	"verifyContactRowExists": "LSVerifyContactRowExists",
	"taskExists": "LSTaskExists",
	"removeTask": "LSRemoveTask",
	"deleteThenInsertMessage": "LSDeleteThenInsertMessage",
	"deleteThenInsertContact": "LSDeleteThenInsertContact",
	"updateTypingIndicator": "LSUpdateTypingIndicator",
	"checkAuthoritativeMessageExists": "LSCheckAuthoritativeMessageExists",
	"moveThreadToInboxAndUpdateParent": "LSMoveThreadToInboxAndUpdateParent",
	"updateThreadSnippet": "LSUpdateThreadSnippet",
	"setMessageDisplayedContentTypes": "LSSetMessageDisplayedContentTypes",
	"verifyThreadExists": "LSVerifyThreadExists",
	"updateReadReceipt": "LSUpdateReadReceipt",
	"setForwardScore": "LSSetForwardScore",
	"upsertReaction": "LSUpsertReaction",
	"bumpThread": "LSBumpThread",
	"updateParticipantLastMessageSendTimestamp": "LSUpdateParticipantLastMessageSendTimestamp",
	"insertMessage": "LSInsertMessage",
	"upsertTheme": "LSUpsertTheme",
	"upsertGradientColor": "LSUpsertGradientColor",
	"insertStickerAttachment": "LSInsertStickerAttachment",
	"updateForRollCallMessageDeleted": "LSUpdateForRollCallMessageDeleted",
}

func SPToDepMap(sp []string) map[string]string {
	m := make(map[string]string, 0)
	for _, d := range sp {
		depName, ok := SPTable[d]
		if !ok {
			log.Println(fmt.Sprintf("can't convert sp %s to dependency name because it wasn't found in the SPTable", d))
			continue
		}
		m[d] = depName
	}
	return m
}