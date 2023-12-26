package graphql

type GraphQLDoc struct {
	DocId string
	CallerClass string
	FriendlyName string
}

var GraphQLDocs = map[string]GraphQLDoc{
	"LSGraphQLRequest": {
		DocId: "7357432314358409",
		CallerClass: "RelayModern",
		FriendlyName: "LSPlatformGraphQLLightspeedRequestQuery",
	},
	"LSGraphQLRequestIG": {
		DocId: "6195354443842040",
		CallerClass: "RelayModern",
		FriendlyName: "LSPlatformGraphQLLightspeedRequestForIGDQuery",
	},
}

type LSPlatformGraphQLLightspeedRequestPayload struct {
	DeviceID              string `json:"deviceId,omitempty"`
	IncludeChatVisibility bool   `json:"includeChatVisibility"`
	RequestID             int    `json:"requestId"`
	RequestPayload        string `json:"requestPayload,omitempty"`
	RequestType           int    `json:"requestType"`
}

type LSPlatformGraphQLLightspeedVariables struct {
	Database          int `json:"database,omitempty"`
	EpochID           int64 `json:"epoch_id"`
	SyncParams interface{} `json:"sync_params,omitempty"`
	LastAppliedCursor any `json:"last_applied_cursor"`
	Version int64 `json:"version,omitempty"`
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