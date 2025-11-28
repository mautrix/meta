package messagix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/go-querystring/query"
	"go.mau.fi/util/jsonbytes"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type FacebookMethods struct {
	client *Client
}

type PushKeys struct {
	P256DH jsonbytes.UnpaddedURLBytes `json:"p256dh"`
	Auth   jsonbytes.UnpaddedURLBytes `json:"auth"`
}

func (fb *FacebookMethods) RegisterPushNotifications(ctx context.Context, endpoint string, keys PushKeys) error {
	c := fb.client
	jsonKeys, err := json.Marshal(&keys)
	if err != nil {
		c.Logger.Err(err).Msg("failed to encode push keys to json")
		return err
	}

	payload := c.newHTTPQuery()
	payload.AppID = "1443096165982425"
	payload.PushEndpoint = endpoint
	payload.SubscriptionKeys = string(jsonKeys)

	form, err := query.Values(payload)
	if err != nil {
		return err
	}

	payloadBytes := []byte(form.Encode())

	headers := c.buildHeaders(true, false)
	headers.Set("Referer", c.GetEndpoint("base_url"))
	headers.Set("Sec-fetch-site", "same-origin")
	headers.Set("Accept", "*/*")

	url := c.GetEndpoint("web_push")

	resp, body, err := c.MakeRequest(ctx, url, "POST", headers, payloadBytes, types.FORM)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	body = bytes.TrimPrefix(body, antiJSPrefix)

	var r pushNotificationsResponse
	err = json.Unmarshal(body, &r)
	if err != nil {
		c.Logger.Err(err).Bytes("body", body).Msg("failed to unmarshal response")
		return err
	}

	if !r.Payload.Success {
		c.Logger.Err(err).Bytes("body", body).Msg("non-success push registration response")
		return errors.New("non-success response payload")
	}

	return nil
}

type pushNotificationsResponse struct {
	Ar        int     `json:"__ar"`
	Payload   payload `json:"payload"`
	DtsgToken string  `json:"dtsgToken"`
	Lid       string  `json:"lid"`
}

type payload struct {
	Success bool `json:"success"`
}
