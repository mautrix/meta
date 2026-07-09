package graphql

type GraphQLDoc struct {
	DocID        string
	ClientDocID  string
	CallerClass  string
	FriendlyName string

	Jsessw string
}

const IGDSlideDeltaProcessorQuery = "28239922265610241"
const IGDTypingIndicatorClientSubscription = "27563068933278040"

var GraphQLDocs = map[string]GraphQLDoc{
	"LSGraphQLRequest": {
		DocID:        "7357432314358409",
		FriendlyName: "LSPlatformGraphQLLightspeedRequestQuery",
	},
	"LSGraphQLRequestIG": {
		DocID:        "6195354443842040",
		FriendlyName: "LSPlatformGraphQLLightspeedRequestForIGDQuery",
	},
	"MAWCatQuery": {
		DocID:        "23999698219677129",
		FriendlyName: "MAWCatQuery",
		Jsessw:       "1",
	},
	"IGDEditThreadNameDialogOffMsysMutation": {
		DocID:        "26508340268868683",
		FriendlyName: "IGDEditThreadNameDialogOffMsysMutation",
	},
	"useIGDirectAcceptMessageRequestMutation": {
		DocID:        "36571001125823973",
		FriendlyName: "useIGDirectAcceptMessageRequestMutation",
	},
	"PolarisDirectMessageRequestQuery": {
		DocID:        "27512223021750545",
		FriendlyName: "PolarisDirectMessageRequestQuery",
	},
	"IGDirectUpdateThreadImageMutation": {
		ClientDocID:  "5576567352987267181917649770",
		FriendlyName: "IGDirectUpdateThreadImageMutation",
	},
	"IGDirectRemoveThreadImageMutation": {
		ClientDocID:  "50027745118339199321503686240",
		FriendlyName: "IGDirectRemoveThreadImageMutation",
	},
	"PolarisDirectInboxQuery": {
		DocID:        "27262915580045003",
		FriendlyName: "PolarisDirectInboxQuery",
	},
	"useIGDSystemFolderUnreadThreadCountQuery": {
		DocID:        "26619714737686638",
		FriendlyName: "useIGDSystemFolderUnreadThreadCountQuery",
	},
	"IGDThreadDetailQuery": {
		DocID:        "37117700834487428",
		FriendlyName: "IGDThreadDetailQuery",
	},
	"IGDSlideAsyncFetchAndInsertIGDViewerThreadQuery": {
		DocID:        "27257464393915989",
		FriendlyName: "IGDSlideAsyncFetchAndInsertIGDViewerThreadQuery",
	},
	"IGDirectReactionSendMutation": {
		DocID:        "24374451552236906",
		FriendlyName: "IGDirectReactionSendMutation",
	},
	"IGDirectTextSendMutation": {
		DocID:        "26911679871773184",
		FriendlyName: "IGDirectTextSendMutation",
	},
	"IGDirectMediaSendMutation": {
		DocID:        "25766288509716264",
		FriendlyName: "IGDirectMediaSendMutation",
	},
	"IGDirectEditMessageMutation": {
		DocID:        "32480262318254796",
		FriendlyName: "IGDirectEditMessageMutation",
	},
	"IGDMessageListOffMsysQuery": {
		DocID:        "27502152406082940",
		FriendlyName: "IGDMessageListOffMsysQuery",
	},
	"IGDMessageUnsendDialogOffMsysMutation": {
		DocID:        "26948700068153789",
		FriendlyName: "IGDMessageUnsendDialogOffMsysMutation",
	},
	"IGDRemoveFromGroupDialogItemOffMsysMutation": {
		DocID:        "26749775594683932",
		FriendlyName: "IGDRemoveFromGroupDialogItemOffMsysMutation",
	},
	"IGDAddAdminDialogItemOffMsysMutation": {
		DocID:        "35563011113312213",
		FriendlyName: "IGDAddAdminDialogItemOffMsysMutation",
	},
	"IGDRemoveAdminDialogItemOffMsysMutation": {
		DocID:        "26688907887428172",
		FriendlyName: "IGDRemoveAdminDialogItemOffMsysMutation",
	},
	"IGDInboxInfoDeleteThreadDialogOffMsysMutation": {
		DocID:        "35352443081068612",
		FriendlyName: "IGDInboxInfoDeleteThreadDialogOffMsysMutation",
	},
	"useIGDMarkThreadAsReadMutation": {
		DocID:        "27356881703909995",
		FriendlyName: "useIGDMarkThreadAsReadMutation",
	},
	"useIGDMarkThreadAsReadValidationMutation": {
		DocID:        "35211594988486314",
		FriendlyName: "useIGDMarkThreadAsReadValidationMutation",
	},
	"PolarisProfilePageContentQuery": {
		DocID:        "26672929172408668",
		FriendlyName: "PolarisProfilePageContentQuery",
	},
	"IGDInboxInfoMuteToggleOffMsysMutation": {
		DocID:        "26360506043651125",
		FriendlyName: "IGDInboxInfoMuteToggleOffMsysMutation",
	},
	"useIGDPinThreadMutation": {
		DocID:        "26491513390471063",
		FriendlyName: "useIGDPinThreadMutation",
	},
	"useIGDPinMessageOffMsysMutation": {
		DocID:        "27027218840244998",
		FriendlyName: "useIGDPinMessageOffMsysMutation",
	},
	"useIGDUnpinMessageOffMsysMutation": {
		DocID:        "26919110827778778",
		FriendlyName: "useIGDUnpinMessageOffMsysMutation",
	},
	"IGDThreadListOffMsysPaginationQuery": {
		DocID:        "27712934271665380",
		FriendlyName: "IGDThreadListOffMsysPaginationQuery",
	},
	"IGDOmniPickerSearchResultsListQuery": {
		DocID:        "27248216181454285",
		FriendlyName: "IGDOmniPickerSearchResultsListQuery",
	},
	"useCreateOpenGroupThreadOffMsysMutation": {
		DocID:        "27529949946640544",
		FriendlyName: "useCreateOpenGroupThreadOffMsysMutation",
	},
}

type IGDeleteThreadGraphQLRequestPayload struct {
	ThreadID                       string `json:"thread_fbid"`
	ShouldMoveFutureRequestsToSpam bool   `json:"should_move_future_requests_to_spam"`
}

type IGEditGroupTitleGraphQLRequestPayload struct {
	ThreadID string `json:"thread_fbid"`
	NewTitle string `json:"new_title"`
}

type IGAcceptMessageRequestGraphQLRequestPayload struct {
	ThreadID           string  `json:"thread_fbid"`
	IGInboxFolder      *string `json:"ig_inbox_folder"`
	OfflineThreadingID string  `json:"offline_threading_id"`
}

type IGListMessageRequestsGraphQLRequestPayload struct {
	DeviceIDForIrisSubscription                  string `json:"device_id_for_iris_subscription"`
	EnablePendingThreadsList                     bool   `json:"enable_pending_threads_list"`
	IGD30DayAgoTimestampMsRelayProvider          string `json:"__relay_internal__pv__IGD30DayAgoTimestampMsrelayprovider"`
	IGDPinnedThreadsRenderEnabledGKRelayProvider bool   `json:"__relay_internal__pv__IGDPinnedThreadsRenderEnabledGKrelayprovider"`
	IGDMaxUnreadMessagesCountRelayProvider       int    `json:"__relay_internal__pv__IGDMaxUnreadMessagesCountrelayprovider"`
	IGDThreadListActionsEnabledGKRelayProvider   bool   `json:"__relay_internal__pv__IGDThreadListActionsEnabledGKrelayprovider"`
}

type IGEditGroupAvatarGraphQLRequestPayload struct {
	ThreadID           string `json:"ig_thread_igid"`
	OfflineThreadingID string `json:"offline_threading_id"`
	AttachmentFBID     string `json:"attachment_fbid,omitempty"`
}

type LSPlatformGraphQLLightspeedRequestPayload struct {
	DeviceID              string `json:"deviceId,omitempty"`
	IncludeChatVisibility bool   `json:"includeChatVisibility"`
	RequestID             int    `json:"requestId"`
	RequestPayload        string `json:"requestPayload,omitempty"`
	RequestType           int    `json:"requestType"`
}

type LSPlatformGraphQLLightspeedVariables struct {
	Database          int         `json:"database,omitempty"`
	EpochID           int64       `json:"epoch_id"`
	SyncParams        interface{} `json:"sync_params,omitempty"`
	LastAppliedCursor any         `json:"last_applied_cursor"`
	Version           int64       `json:"version,omitempty"`
}
