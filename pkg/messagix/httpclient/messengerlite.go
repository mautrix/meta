package httpclient

import (
	"encoding/json"
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type NetworkTags struct {
	Product         string `json:"product"`
	Purpose         string `json:"purpose,omitempty"`
	RequestCategory string `json:"request_category,omitempty"`
	RetryAttempt    string `json:"retry_attempt"`
}

type RequestAnalytics struct {
	NetworkTags NetworkTags `json:"network_tags"`
}

func MakeRequestAnalyticsHeader() (string, error) {
	anal := RequestAnalytics{
		NetworkTags: NetworkTags{
			Product:         useragent.MessengerLiteAppID,
			RequestCategory: "graphql",
			Purpose:         "fetch",
			RetryAttempt:    "0",
		},
	}
	hdr, err := json.Marshal(anal)
	if err != nil {
		return "", fmt.Errorf("make analytics header: %w", err)
	}
	return string(hdr), nil
}
