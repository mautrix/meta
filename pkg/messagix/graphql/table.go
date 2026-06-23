package graphql

import (
	"encoding/json"
)

type GraphQLPreloader struct {
	ActorID     any             `json:"actorID,omitempty"`
	PreloaderID string          `json:"preloaderID,omitempty"`
	QueryID     string          `json:"queryID,omitempty"`
	Variables   json.RawMessage `json:"variables,omitempty"`
}

func (gqp *GraphQLPreloader) ParseVariables() (vars Variables, err error) {
	err = json.Unmarshal(gqp.Variables, &vars)
	return
}

type Variables struct {
	DeviceID              string `json:"deviceId,omitempty"`
	IncludeChatVisibility bool   `json:"includeChatVisibility,omitempty"`
	RequestID             int    `json:"requestId,omitempty"`
	RequestPayload        string `json:"requestPayload,omitempty"`
	RequestType           int    `json:"requestType,omitempty"`
}
