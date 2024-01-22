package messagix

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/go-querystring/query"

	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/crypto"
	"go.mau.fi/mautrix-meta/messagix/data/responses"
	"go.mau.fi/mautrix-meta/messagix/types"
)

// specific methods for insta api, not socket related
type InstagramMethods struct {
	client *Client
}

func (ig *InstagramMethods) Login(identifier, password string) (cookies.Cookies, error) {
	ig.client.loadLoginPage()
	if err := ig.client.configs.SetupConfigs(); err != nil {
		return nil, err
	}
	h := ig.client.buildHeaders(false)
	h.Set("x-web-device-id", ig.client.cookies.GetValue("ig_did"))
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-origin")
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", ig.client.getEndpoint("login_page"))

	login_page_v1 := ig.client.getEndpoint("web_login_page_v1")
	_, _, err := ig.client.MakeRequest(login_page_v1, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s for instagram login: %v", login_page_v1, err)
	}

	err = ig.client.sendCookieConsent("")
	if err != nil {
		return nil, err
	}

	web_shared_data_v1 := ig.client.getEndpoint("web_shared_data_v1")
	req, respBody, err := ig.client.MakeRequest(web_shared_data_v1, "GET", h, nil, types.NONE) // returns actual machineId you're supposed to use
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s for instagram login: %v", web_shared_data_v1, err)
	}

	cookies.UpdateFromResponse(ig.client.cookies, req.Header)

	err = json.Unmarshal(respBody, &ig.client.configs.browserConfigTable.XIGSharedData.ConfigData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal web_shared_data_v1 resp body into *XIGSharedData.ConfigData: %v", err)
	}

	encryptionConfig := ig.client.configs.browserConfigTable.XIGSharedData.ConfigData.Encryption
	pubKeyId, err := strconv.Atoi(encryptionConfig.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert keyId for instagram password encryption to int: %v", err)
	}

	encryptedPw, err := crypto.EncryptPassword(int(types.Instagram), pubKeyId, encryptionConfig.PublicKey, password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password for instagram: %v", err)
	}

	loginForm := &types.InstagramLoginPayload{
		Password:             encryptedPw,
		OptIntoOneTap:        false,
		QueryParams:          "{}",
		TrustedDeviceRecords: "{}",
		Username:             identifier,
	}

	form, err := query.Values(&loginForm)
	web_login_ajax_v1 := ig.client.getEndpoint("web_login_ajax_v1")
	loginResp, loginBody, err := ig.client.Account.sendLoginRequest(form, web_login_ajax_v1)
	if err != nil {
		return nil, err
	}

	loginResult := ig.client.Account.processLogin(loginResp, loginBody)
	if loginResult != nil {
		return nil, loginResult
	}

	return ig.client.cookies, nil
}

func (ig *InstagramMethods) FetchProfile(username string) (*responses.ProfileInfoResponse, error) {
	h := ig.client.buildHeaders(true)
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", ig.client.getEndpoint("base_url")+username+"/")
	reqUrl := ig.client.getEndpoint("web_profile_info") + "username=" + username

	resp, respBody, err := ig.client.MakeRequest(reqUrl, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the profile by username @%s: %v", username, err)
	}

	cookies.UpdateFromResponse(ig.client.cookies, resp.Header)

	var profileInfo *responses.ProfileInfoResponse
	err = json.Unmarshal(respBody, &profileInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.ProfileInfoResponse (statusCode=%d): %v", resp.StatusCode, err)
	}

	return profileInfo, nil
}

func (ig *InstagramMethods) FetchMedia(mediaID, nativeURL string) (*responses.FetchMediaResponse, error) {
	h := ig.client.buildHeaders(true)
	h.Set("x-requested-with", "XMLHttpRequest")
	referer := ig.client.getEndpoint("base_url")
	if strings.HasPrefix(nativeURL, "instagram://media/?shortcode=") {
		referer = fmt.Sprintf("%s/p/%s/", referer, strings.TrimPrefix(nativeURL, "instagram://media/?shortcode="))
	}
	h.Set("referer", referer)
	h.Set("Accept", "*/*")
	reqUrl := fmt.Sprintf(ig.client.getEndpoint("media_info"), mediaID)

	resp, respBody, err := ig.client.MakeRequest(reqUrl, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the media by id %s: %v", mediaID, err)
	}

	cookies.UpdateFromResponse(ig.client.cookies, resp.Header)

	var mediaInfo *responses.FetchMediaResponse
	err = json.Unmarshal(respBody, &mediaInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.FetchMediaResponse (statusCode=%d): %v", resp.StatusCode, err)
	}

	return mediaInfo, nil
}

func (ig *InstagramMethods) FetchReel(reelIds []string, mediaID string) (*responses.ReelInfoResponse, error) {
	h := ig.client.buildHeaders(true)
	h.Set("x-requested-with", "XMLHttpRequest")
	h.Set("referer", ig.client.getEndpoint("base_url"))
	h.Set("Accept", "*/*")
	query := url.Values{}
	if mediaID != "" {
		query.Add("media_id", mediaID)
	}
	for _, id := range reelIds {
		query.Add("reel_ids", id)
	}

	reqUrl := ig.client.getEndpoint("reels_media") + query.Encode()
	resp, respBody, err := ig.client.MakeRequest(reqUrl, "GET", h, nil, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reels by ids %v: %v", reelIds, err)
	}

	cookies.UpdateFromResponse(ig.client.cookies, resp.Header)

	var reelInfo *responses.ReelInfoResponse
	err = json.Unmarshal(respBody, &reelInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response bytes into *responses.ReelInfoResponse (statusCode=%d): %v", resp.StatusCode, err)
	}

	return reelInfo, nil
}

// # NOTE:
//
// Hightlight IDs are different, they come in the format: "highlight:17913397615055292"
func (ig *InstagramMethods) FetchHighlights(highlightIds []string) (*responses.ReelInfoResponse, error) {
	return ig.FetchReel(highlightIds, "")
}
