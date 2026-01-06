package bloks

import (
	"encoding/json"
)

const BloksVersion = "3988ff4cdf5ca5de647ba84aa74b5bd2fcd4ffd768e0faec8adc3e53492f3f87"

type wrappedBloksParams struct {
	ServerParams      map[string]any `json:"server_params"`
	ClientInputParams map[string]any `json:"client_input_params"`
}

type wrappedBloksBkContext struct {
	PixelRatio   float64 `json:"pixel_ratio"`
	BloksVersion string  `json:"bloks_version"`
}

type wrappedBloksOuterParams struct {
	BloksVersioningId string `json:"bloks_versioning_id"`
	AppID             string `json:"app_id"`
	Params            string `json:"params"`
}

type WrappedBloksRequest struct {
	BkContext *wrappedBloksBkContext   `json:"bk_context,omitempty"`
	Params    *wrappedBloksOuterParams `json:"params,omitempty"`
}

func makeWrappedBloksRequest(pixelRatio float64, bloksVersion string, appID string, params wrappedBloksParams) (*WrappedBloksRequest, error) {
	innerInnerParamsJson, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	innerParamsJson, err := json.Marshal(map[string]any{
		"params": string(innerInnerParamsJson),
	})
	if err != nil {
		return nil, err
	}

	wrappedRequest := &WrappedBloksRequest{
		BkContext: &wrappedBloksBkContext{
			PixelRatio:   pixelRatio,
			BloksVersion: bloksVersion,
		},
		Params: &wrappedBloksOuterParams{
			BloksVersioningId: bloksVersion,
			AppID:             appID,
			Params:            string(innerParamsJson),
		},
	}

	return wrappedRequest, nil
}

func MakeWrappedBloksRequest(appID string, serverParams map[string]any, clientParams map[string]any) (*WrappedBloksRequest, error) {
	return makeWrappedBloksRequest(3, BloksVersion, appID, wrappedBloksParams{
		ServerParams:      serverParams,
		ClientInputParams: clientParams,
	})
}

type BloksResponse struct {
	Data map[string]BloksResponseData `json:"data"`
}

type BloksResponseData struct {
	Action BloksAction `json:"action"`
}

type BloksAction struct {
	Bundle BloksActionBundle `json:"action_bundle"`
}

type BloksActionBundle struct {
	BundleAction string `json:"bloks_bundle_action"` // stringified json
}

type BloksBundleAction struct {
	Layout BloksLayout `json:"layout"`
}

type BloksLayout struct {
	Payload BloksPayload `json:"bloks_payload"`
}

type BloksPayload struct {
	Action string `json:"action"` // scuffed lisp
	// ... more fields that we don't use
}
