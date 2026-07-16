package instameow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/go-querystring/query"
	"go.mau.fi/util/jsonbytes"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
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
		c.checkResponseError(err)
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

func (c *Client) FetchMedia(ctx context.Context, mediaID, mediaShortcode string) (*responses.FetchMediaResponse, error) {
	h := c.http.BuildHeaders(true, false)
	h.Set("x-requested-with", "XMLHttpRequest")
	referer := c.GetEndpoint("base_url")
	if mediaShortcode != "" {
		referer = fmt.Sprintf("%s/p/%s/", referer, mediaShortcode)
	}
	h.Set("referer", referer)
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-origin")
	// TODO the web client seems to change this for media requests?
	h.Set("X-Web-Session-ID", c.configs.WebSessionID)
	reqUrl := fmt.Sprintf(c.GetEndpoint("media_info"), mediaID)

	resp, respBody, err := c.http.MakeRequest(ctx, reqUrl, http.MethodGet, h, nil, types.NONE)
	if err != nil {
		c.checkResponseError(err)
		return nil, fmt.Errorf("failed to fetch the media by id %s: %w", mediaID, err)
	}

	c.cookies.UpdateFromResponse(resp)

	var mediaInfo *responses.FetchMediaResponse
	err = json.Unmarshal(respBody, &mediaInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.FetchMediaResponse (statusCode=%d): %w", resp.StatusCode, err)
	}

	return mediaInfo, nil
}

func (c *Client) FetchReel(ctx context.Context, reelIDs []string, mediaID string) (*responses.ReelInfoResponse, error) {
	h := c.http.BuildHeaders(true, false)
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", c.GetEndpoint("base_url"))
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-origin")
	h.Set("X-Web-Session-ID", c.configs.WebSessionID)
	query := url.Values{}
	if mediaID != "" {
		query.Add("media_id", mediaID)
	}
	for _, id := range reelIDs {
		query.Add("reel_ids", id)
	}

	reqURL := c.GetEndpoint("reels_media") + query.Encode()
	resp, respBody, err := c.http.MakeRequest(ctx, reqURL, http.MethodGet, h, nil, types.NONE)
	if err != nil {
		c.checkResponseError(err)
		return nil, fmt.Errorf("failed to fetch reels by ids %v: %w", reelIDs, err)
	}

	c.cookies.UpdateFromResponse(resp)

	var reelInfo *responses.ReelInfoResponse
	err = json.Unmarshal(respBody, &reelInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.ReelInfoResponse (statusCode=%d): %w", resp.StatusCode, err)
	}

	return reelInfo, nil
}
