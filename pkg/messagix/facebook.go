package messagix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/go-querystring/query"
	"go.mau.fi/util/jsonbytes"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
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

	payload := c.http.NewHTTPQuery()
	payload.AppID = "1443096165982425"
	payload.PushEndpoint = endpoint
	payload.SubscriptionKeys = string(jsonKeys)

	form, err := query.Values(payload)
	if err != nil {
		return err
	}

	payloadBytes := []byte(form.Encode())

	headers := c.http.BuildHeaders(true, false)
	headers.Set("Referer", c.GetEndpoint("base_url"))
	headers.Set("Sec-fetch-site", "same-origin")
	headers.Set("Accept", "*/*")

	url := c.GetEndpoint("web_push")

	_, body, err := c.http.MakeRequest(ctx, url, "POST", headers, payloadBytes, types.FORM)
	if err != nil {
		return err
	}

	body = bytes.TrimPrefix(body, httpclient.AntiJSPrefix)

	var r pushNotificationsResponse
	err = json.Unmarshal(body, &r)
	if err != nil {
		c.Logger.Err(err).Bytes("body", body).Msg("failed to unmarshal response")
		return err
	}

	if !r.Payload.Success {
		c.Logger.Err(err).Bytes("body", body).Msg("non-success push registration response")
		if respErr := r.AsError(); respErr != nil {
			return fmt.Errorf("non-success push registration response: %w", respErr)
		}
		return errors.New("non-success response payload")
	}

	return nil
}

func (fb *FacebookMethods) Logout(ctx context.Context) error {
	c := fb.client
	if c.configs == nil || c.configs.BrowserConfigTable.DTSGInitData.Token == "" {
		return errors.New("no fb_dtsg token loaded")
	}
	form := url.Values{
		"fb_dtsg": {c.configs.BrowserConfigTable.DTSGInitData.Token},
		"jazoest": {c.configs.Jazoest},
	}
	headers := c.http.BuildHeaders(true, true)
	headers.Set("Referer", c.GetEndpoint("base_url")+"/")
	headers.Set("Origin", c.GetEndpoint("base_url"))
	headers.Set("Sec-fetch-site", "same-origin")
	_, _, err := c.http.MakeRequestOnceNoRedirect(ctx, c.GetEndpoint("logout"), "POST", headers, []byte(form.Encode()), types.FORM)
	return err
}

type pushNotificationsResponse struct {
	types.ErrorResponse

	Ar      int `json:"__ar"`
	Payload struct {
		Success bool `json:"success"`
	} `json:"payload"`
	DtsgToken string `json:"dtsgToken"`
	Lid       string `json:"lid"`
}
