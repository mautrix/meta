package instameow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/go-querystring/query"
	"go.mau.fi/util/jsonbytes"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type PushKeys struct {
	P256DH jsonbytes.UnpaddedURLBytes `json:"p256dh"`
	Auth   jsonbytes.UnpaddedURLBytes `json:"auth"`
}

type pushRegisterQuery struct {
	DeviceToken      string `url:"device_token,omitempty"`
	DeviceType       string `url:"device_type,omitempty"`
	MID              string `url:"mid,omitempty"`
	SubscriptionKeys string `url:"subscription_keys,omitempty"`
	Jazoest          string `url:"jazoest,omitempty"`
	FBDTSG           string `url:"fb_dtsg,omitempty"`
}

func (c *Client) RegisterPushNotifications(ctx context.Context, endpoint string, keys PushKeys) error {
	jsonKeys, err := json.Marshal(&keys)
	if err != nil {
		return err
	}

	form, err := query.Values(&pushRegisterQuery{
		SubscriptionKeys: string(jsonKeys),
		DeviceType:       "web_vapid",
		DeviceToken:      endpoint,
		Jazoest:          c.configs.Jazoest,
		MID:              c.GetCookies().Get(cookies.IGCookieMachineID),
		FBDTSG:           c.configs.BrowserConfigTable.DTSGInitData.Token,
	})
	if err != nil {
		return err
	}

	headers := c.http.BuildHeaders(true, false)
	headers.Set("x-requested-with", "XMLHttpRequest")
	headers.Set("Referer", c.GetEndpoint("base_url"))

	_, body, err := c.http.MakeRequest(
		ctx, c.GetEndpoint("web_push"), http.MethodPost, headers, []byte(form.Encode()), types.FORM,
	)
	if err != nil {
		return err
	}

	resBody := &struct {
		Status string `json:"status"`
	}{}

	err = json.Unmarshal(body, resBody)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	} else if resBody.Status != "ok" {
		return fmt.Errorf("unexpected response status: %s", resBody.Status)
	}

	return nil
}
