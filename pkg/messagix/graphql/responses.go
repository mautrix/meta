package graphql

import (
	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type LSPlatformGraphQLLightspeedRequestQuery struct {
	types.ErrorResponse
	Data *struct {
		Viewer struct {
			LightspeedWebRequest *LightspeedWebRequest `json:"lightspeed_web_request,omitempty"`
		} `json:"viewer,omitempty"`
		LightspeedWebRequestForIG *LightspeedWebRequest `json:"lightspeed_web_request_for_igd,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type LightspeedWebRequest struct {
	Dependencies lightspeed.DependencyList `json:"dependencies,omitempty"`
	Experiments  any                       `json:"experiments,omitempty"`
	Payload      string                    `json:"payload,omitempty"`
	Target       string                    `json:"target,omitempty"`
}
