package bloks

import (
	"encoding/json"
)

const BloksVersion = "3988ff4cdf5ca5de647ba84aa74b5bd2fcd4ffd768e0faec8adc3e53492f3f87"

// TODO: Fix analytics
type NetworkTags struct {
	Product         string `json:"product"`
	Purpose         string `json:"purpose,omitempty"`
	RequestCategory string `json:"request_category,omitempty"`
	RetryAttempt    string `json:"retry_attempt"`
}

type RequestAnalytics struct {
	NetworkTags NetworkTags `json:"network_tags"`
}

// WrappedBloks:

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

// WrappedBloks is very cursed
func MakeWrappedBloksRequest(appID string, serverParams map[string]any, clientParams map[string]any) (*WrappedBloksRequest, error) {
	return makeWrappedBloksRequest(3, BloksVersion, appID, wrappedBloksParams{
		ServerParams:      serverParams,
		ClientInputParams: clientParams,
	})
}
