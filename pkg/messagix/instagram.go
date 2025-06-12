package messagix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/google/go-querystring/query"
	"github.com/google/uuid"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/data/responses"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// specific methods for insta api, not socket related
type InstagramMethods struct {
	client *Client
}

func (ig *InstagramMethods) Login(ctx context.Context, identifier, password string) (*cookies.Cookies, error) {
	ig.client.loadLoginPage(ctx)
	if _, err := ig.client.configs.SetupConfigs(ctx, nil); err != nil {
		return nil, err
	}
	h := ig.client.buildHeaders(false, false)
	h.Set("x-web-device-id", ig.client.cookies.Get(cookies.IGCookieDeviceID))
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-origin")
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", ig.client.getEndpoint("login_page"))

	login_page_v1 := ig.client.getEndpoint("web_login_page_v1")
	_, _, err := ig.client.MakeRequest(ctx, login_page_v1, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s for instagram login: %w", login_page_v1, err)
	}

	err = ig.client.sendCookieConsent(ctx, "")
	if err != nil {
		return nil, err
	}

	web_shared_data_v1 := ig.client.getEndpoint("web_shared_data_v1")
	req, respBody, err := ig.client.MakeRequest(ctx, web_shared_data_v1, "GET", h, nil, types.NONE) // returns actual machineId you're supposed to use
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s for instagram login: %w", web_shared_data_v1, err)
	}

	ig.client.cookies.UpdateFromResponse(req)

	err = json.Unmarshal(respBody, &ig.client.configs.BrowserConfigTable.XIGSharedData.ConfigData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal web_shared_data_v1 resp body into *XIGSharedData.ConfigData: %w", err)
	}

	encryptionConfig := ig.client.configs.BrowserConfigTable.XIGSharedData.ConfigData.Encryption
	pubKeyId, err := strconv.Atoi(encryptionConfig.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert keyId for instagram password encryption to int: %w", err)
	}

	encryptedPw, err := crypto.EncryptPassword(int(types.Instagram), pubKeyId, encryptionConfig.PublicKey, password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password for instagram: %w", err)
	}

	loginForm := &types.InstagramLoginPayload{
		Password:             encryptedPw,
		OptIntoOneTap:        false,
		QueryParams:          "{}",
		TrustedDeviceRecords: "{}",
		Username:             identifier,
	}

	form, err := query.Values(&loginForm)
	if err != nil {
		return nil, err
	}
	web_login_ajax_v1 := ig.client.getEndpoint("web_login_ajax_v1")
	loginResp, loginBody, err := ig.client.sendLoginRequest(ctx, form, web_login_ajax_v1)
	if err != nil {
		return nil, err
	}

	loginResult := ig.client.processLogin(loginResp, loginBody)
	if loginResult != nil {
		return nil, loginResult
	}

	return ig.client.cookies, nil
}

func (ig *InstagramMethods) FetchProfile(ctx context.Context, username string) (*responses.ProfileInfoResponse, error) {
	h := ig.client.buildHeaders(true, false)
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", ig.client.getEndpoint("base_url")+username+"/")
	reqUrl := ig.client.getEndpoint("web_profile_info") + "username=" + username

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
	referer := ig.client.getEndpoint("base_url")
	if mediaShortcode != "" {
		referer = fmt.Sprintf("%s/p/%s/", referer, mediaShortcode)
	}
	h.Set("referer", referer)
	h.Set("Accept", "*/*")
	reqUrl := fmt.Sprintf(ig.client.getEndpoint("media_info"), mediaID)

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
	h.Set("referer", ig.client.getEndpoint("base_url"))
	h.Set("Accept", "*/*")
	query := url.Values{}
	if mediaID != "" {
		query.Add("media_id", mediaID)
	}
	for _, id := range reelIDs {
		query.Add("reel_ids", id)
	}

	reqUrl := ig.client.getEndpoint("reels_media") + query.Encode()
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
	headers.Set("Referer", c.getEndpoint("base_url"))
	headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")

	url := c.getEndpoint("web_push")
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
