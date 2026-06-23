package graphql

import (
	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type LSPlatformGraphQLLightspeedRequestQuery = Response[*struct {
	Viewer struct {
		LightspeedWebRequest *LightspeedWebRequest `json:"lightspeed_web_request,omitempty"`
	} `json:"viewer,omitempty"`
	LightspeedWebRequestForIG *LightspeedWebRequest `json:"lightspeed_web_request_for_igd,omitempty"`
}]

type LightspeedWebRequest struct {
	Dependencies lightspeed.DependencyList `json:"dependencies,omitempty"`
	Experiments  any                       `json:"experiments,omitempty"`
	Payload      string                    `json:"payload,omitempty"`
	Target       string                    `json:"target,omitempty"`
}

type Response[T any] struct {
	Data T `json:"data"`
	types.ErrorResponse
	Extensions Extensions `json:"extensions,omitempty"`
}

type ServerMetadata struct {
	RequestStartTimeMS int64 `json:"request_start_time_ms"`
	TimeAtFlushMS      int64 `json:"time_at_flush_ms"`
}

type Extensions struct {
	ServerMetadata ServerMetadata `json:"server_metadata"`
	IsFinal        bool           `json:"is_final"`
}
