package messagix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/google/go-querystring/query"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/data/responses"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// specific methods for insta api, not socket related
type InstagramMethods struct {
	client *Client
}

func (ig *InstagramMethods) FetchProfile(ctx context.Context, username string) (*responses.ProfileInfoResponse, error) {
	h := ig.client.buildHeaders(true, false)
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", ig.client.GetEndpoint("base_url")+username+"/")
	reqUrl := ig.client.GetEndpoint("web_profile_info") + "username=" + username

	resp, respBody, err := ig.client.MakeRequest(ctx, reqUrl, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the profile by username @%s: %w", username, err)
	}

	ig.client.cookies.UpdateFromResponse(resp)

	var profileInfo *responses.ProfileInfoResponse
	err = json.Unmarshal(respBody, &profileInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.ProfileInfoResponse (statusCode=%d): %w", resp.StatusCode, err)
	}

	return profileInfo, nil
}

func (ig *InstagramMethods) FetchMedia(ctx context.Context, mediaID, mediaShortcode string) (*responses.FetchMediaResponse, error) {
	h := ig.client.buildHeaders(true, false)
	h.Set("x-requested-with", "XMLHttpRequest")
	referer := ig.client.GetEndpoint("base_url")
	if mediaShortcode != "" {
		referer = fmt.Sprintf("%s/p/%s/", referer, mediaShortcode)
	}
	h.Set("referer", referer)
	h.Set("Accept", "*/*")
	reqUrl := fmt.Sprintf(ig.client.GetEndpoint("media_info"), mediaID)

	resp, respBody, err := ig.client.MakeRequest(ctx, reqUrl, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the media by id %s: %w", mediaID, err)
	}

	ig.client.cookies.UpdateFromResponse(resp)

	var mediaInfo *responses.FetchMediaResponse
	err = json.Unmarshal(respBody, &mediaInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.FetchMediaResponse (statusCode=%d): %w", resp.StatusCode, err)
	}

	return mediaInfo, nil
}

func (ig *InstagramMethods) FetchReel(ctx context.Context, reelIDs []string, mediaID string) (*responses.ReelInfoResponse, error) {
	h := ig.client.buildHeaders(true, false)
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", ig.client.GetEndpoint("base_url"))
	h.Set("Accept", "*/*")
	query := url.Values{}
	if mediaID != "" {
		query.Add("media_id", mediaID)
	}
	for _, id := range reelIDs {
		query.Add("reel_ids", id)
	}

	reqUrl := ig.client.GetEndpoint("reels_media") + query.Encode()
	resp, respBody, err := ig.client.MakeRequest(ctx, reqUrl, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reels by ids %v: %w", reelIDs, err)
	}

	ig.client.cookies.UpdateFromResponse(resp)

	var reelInfo *responses.ReelInfoResponse
	err = json.Unmarshal(respBody, &reelInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.ReelInfoResponse (statusCode=%d): %w", resp.StatusCode, err)
	}

	return reelInfo, nil
}

// # NOTE:
//
// Hightlight IDs are different, they come in the format: "highlight:17913397615055292"
func (ig *InstagramMethods) FetchHighlights(ctx context.Context, highlightIDs []string) (*responses.ReelInfoResponse, error) {
	return ig.FetchReel(ctx, highlightIDs, "")
}

func (ig *InstagramMethods) RegisterPushNotifications(ctx context.Context, endpoint string, keys PushKeys) error {
	c := ig.client

	jsonKeys, err := json.Marshal(&keys)
	if err != nil {
		c.Logger.Err(err).Msg("failed to encode push keys to json")
		return err
	}

	u := uuid.New()
	payload := c.newHTTPQuery()
	payload.Mid = u.String()
	payload.DeviceType = "web_vapid"
	payload.DeviceToken = endpoint
	payload.SubscriptionKeys = string(jsonKeys)

	form, err := query.Values(payload)
	if err != nil {
		return err
	}

	payloadBytes := []byte(form.Encode())

	headers := c.buildHeaders(true, false)
	headers.Set("x-requested-with", "XMLHttpRequest")
	headers.Set("Referer", c.GetEndpoint("base_url"))
	headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")

	url := c.GetEndpoint("web_push")
	resp, body, err := c.MakeRequest(ctx, url, "POST", headers, payloadBytes, types.FORM)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	resBody := &struct {
		Status string `json:"status"`
	}{}

	err = json.Unmarshal(body, resBody)
	if err != nil {
		return errors.New("failed to decode response payload, not subscribed to push notifications")
	}

	return nil
}

func (ig *InstagramMethods) ExtractFBID(currentUser types.UserInfo, tbl *table.LSTable) (int64, error) {
	var newFBID int64

	for _, row := range tbl.LSVerifyContactRowExists {
		if row.IsSelf && row.ContactId != newFBID {
			if newFBID != 0 {
				// Hopefully this won't happen
				ig.client.Logger.Warn().Int64("prev_fbid", newFBID).Int64("new_fbid", row.ContactId).Msg("Got multiple fbids for self")
			} else {
				ig.client.Logger.Debug().Int64("fbid", row.ContactId).Msg("Found own fbid")
			}
			newFBID = row.ContactId
		}
	}
	if newFBID == 0 {
		newFBID = currentUser.GetFBID()
		cuid := ig.client.configs.BrowserConfigTable.CurrentUserInitialData
		if strconv.FormatInt(newFBID, 10) == cuid.NonFacebookUserID && cuid.IGUserEIMU != "" {
			newFBID, _ = strconv.ParseInt(cuid.IGUserEIMU, 10, 64)
		}
		ig.client.Logger.Debug().
			Int64("fbid", newFBID).
			Str("non_facebook_user_id", cuid.NonFacebookUserID).
			Str("ig_user_eimu", cuid.IGUserEIMU).
			Str("init_data_user_id", ig.client.configs.BrowserConfigTable.MessengerWebInitData.UserID.String()).
			Msg("Own contact entry not found, falling back to fbid in current user object")
	}
	if newFBID == 0 {
		return 0, fmt.Errorf("failed to extract fbid")
	}

	return newFBID, nil
}

func (ig *InstagramMethods) fetchRouteDefinition(ctx context.Context, threadID string) (id string, err error) {
	payload := ig.client.newHTTPQuery()
	payload.ClientPreviousActorID = "17841477657023246"
	payload.RouteURL = fmt.Sprintf("/direct/t/%s/", threadID)
	if ig.client.configs.RoutingNamespace == "" {
		return "", fmt.Errorf("routing namespace is empty, cannot fetch route definition for thread %s", threadID)
	}
	payload.RoutingNamespace = ig.client.configs.RoutingNamespace
	payload.Crn = "comet.igweb.PolarisDirectInboxRoute"

	form, err := query.Values(&payload)
	if err != nil {
		return "", err
	}
	form.Add("trace_policy", "")
	payloadBytes := []byte(form.Encode())

	headers := ig.client.buildHeaders(true, false)
	headers.Set("accept", "*/*")
	headers.Set("origin", ig.client.GetEndpoint("base_url"))
	headers.Set("accept", "*/*")
	headers.Set("referer", ig.client.GetEndpoint("messages"))
	headers.Set("priority", "u=1, i")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")

	url := ig.client.GetEndpoint("route_definition")
	resp, body, err := ig.client.MakeRequest(ctx, url, "POST", headers, payloadBytes, types.FORM)
	if err != nil {
		return "", fmt.Errorf("failed to fetch route definition for thread %s: %w", threadID, err)
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return "", fmt.Errorf("bad status code when fetching route definition for thread %s: %d", threadID, resp.StatusCode)
	}

	parts := bytes.Split(body, []byte("\r\n"))
	if len(parts) < 1 {
		return "", fmt.Errorf("invalid route definition response for thread %s", threadID)
	}

	responseBody := bytes.TrimPrefix(parts[0], antiJSPrefix)

	var routeDefResp fetchRouteDefinitionResponse
	err = json.Unmarshal(responseBody, &routeDefResp)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal route definition response for thread %s: %w", threadID, err)
	}

	if routeDefResp.Payload.Error {
		return "", fmt.Errorf("route definition response returned error for thread %s", threadID)
	}
	threadFBID := routeDefResp.Payload.Result.Exports.RootView.Props.ThreadFBID
	if threadFBID == "" {
		return "", fmt.Errorf("thread_fbid not found in route definition response for thread %s", threadID)
	}

	zerolog.Ctx(ctx).Info().
		Str("thread_id", threadID).
		Str("thread_fbid", threadFBID).
		Msg("Successfully fetched route definition")

	return threadFBID, nil
}

func (ig *InstagramMethods) DeleteThread(ctx context.Context, threadID string) error {
	id, err := ig.fetchRouteDefinition(ctx, threadID)
	if err != nil {
		return fmt.Errorf("failed to fetch route definition for thread %s: %w", threadID, err)
	}
	igVariables := &graphql.IGDeleteThreadGraphQLRequestPayload{
		ThreadID:                       id,
		ShouldMoveFutureRequestsToSpam: false,
	}
	resp, _, err := ig.client.makeGraphQLRequest(ctx, "IGDeleteThread", &igVariables)
	if err != nil {
		return fmt.Errorf("failed to delete thread %s: %w", threadID, err)
	}
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("failed to delete thread with bad status code %d", resp.StatusCode)
	}
	return nil
}

type fetchRouteDefinitionResponsePayload struct {
	Error  bool `json:"error"`
	Result struct {
		Exports struct {
			RootView struct {
				Props struct {
					ThreadFBID string `json:"thread_fbid"`
				} `json:"props"`
			} `json:"rootView"`
		} `json:"exports"`
	} `json:"result"`
}

type fetchRouteDefinitionResponse struct {
	Payload fetchRouteDefinitionResponsePayload `json:"payload"`
}

func (ig *InstagramMethods) EditGroupTitle(ctx context.Context, threadID, newTitle string) error {
	igVariables := &graphql.IGEditGroupTitleGraphQLRequestPayload{
		ThreadID: threadID,
		NewTitle: newTitle,
	}
	resp, _, err := ig.client.makeGraphQLRequest(ctx, "IGEditGroupTitle", &igVariables)
	if err != nil {
		return fmt.Errorf("failed to edit group title for thread %s: %w", threadID, err)
	}
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("failed to edit group title with bad status code %d", resp.StatusCode)
	}
	return nil
}
