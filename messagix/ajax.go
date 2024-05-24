package messagix

import (
	"github.com/google/go-querystring/query"
	"go.mau.fi/mautrix-meta/messagix/types"
)

type AjaxClient struct {
	client *Client
}

func (c *Client) newAjaxClient() *AjaxClient {
	return &AjaxClient{
		client: c,
	}
}

func (ac *AjaxClient) SendQMRequest(path string, payload interface{}) error {
	form, err := query.Values(payload)
	if err != nil {
		return err
	}
	payloadBytes := []byte(form.Encode())

	ac.client.Logger.Info().
		Str("url_encoded_payload", string(payloadBytes)).
		Str("url_path", path).
		Msg("Ajax QM Request")

	baseURL := ac.client.getEndpoint("base_url")
	headers := ac.client.buildHeaders(true)
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "no-cors")
	headers.Set("sec-fetch-site", "same-origin")
	headers.Set("origin", baseURL)
	headers.Set("referer", ac.client.getEndpoint("messages")+"/")

	reqUrl := baseURL + path
	resp, respData, err := ac.client.MakeRequest(reqUrl, "POST", headers, payloadBytes, types.FORM)
	if err != nil {
		return err
	}

	ac.client.Logger.Info().Str("response_body", string(respData)).Int("status_code", resp.StatusCode).Msg("Ajax QM Response")
	return nil
}