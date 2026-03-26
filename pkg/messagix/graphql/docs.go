package graphql

type GraphQLDoc struct {
	DocID        string
	CallerClass  string
	FriendlyName string

	Jsessw string
}

var GraphQLDocs = map[string]GraphQLDoc{
	"LSGraphQLRequest": {
		DocID:        "7357432314358409",
		CallerClass:  "RelayModern",
		FriendlyName: "LSPlatformGraphQLLightspeedRequestQuery",
	},
	"LSGraphQLRequestIG": {
		DocID:        "6195354443842040",
		CallerClass:  "RelayModern",
		FriendlyName: "LSPlatformGraphQLLightspeedRequestForIGDQuery",
	},
	"MAWCatQuery": {
		DocID:        "23999698219677129",
		CallerClass:  "RelayModern",
		FriendlyName: "MAWCatQuery",
		Jsessw:       "1",
	},
	"IGDeleteThread": {
		DocID:        "23915602751379354",
		CallerClass:  "RelayModern",
		FriendlyName: "IGDInboxInfoDeleteThreadDialogOffMsysMutation",
	},
	"IGEditGroupTitle": {
		DocID:        "29088580780787855",
		CallerClass:  "RelayModern",
		FriendlyName: "IGDEditThreadNameDialogOffMsysMutation",
	},
	"IGAcceptMessageRequest": {
		DocID:        "25093807760274522",
		CallerClass:  "RelayModern",
		FriendlyName: "useIGDirectAcceptMessageRequestMutation",
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

type SyncParams struct {
	FullHeight                int    `json:"full_height,omitempty"`
	Locale                    string `json:"locale,omitempty"`
	PreviewHeight             int    `json:"preview_height,omitempty"`
	PreviewHeightLarge        int    `json:"preview_height_large,omitempty"`
	PreviewWidth              int    `json:"preview_width,omitempty"`
	PreviewWidthLarge         int    `json:"preview_width_large,omitempty"`
	Scale                     int    `json:"scale,omitempty"`
	SnapshotNumThreadsPerPage int    `json:"snapshot_num_threads_per_page,omitempty"`
}
