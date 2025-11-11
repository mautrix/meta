package endpoints

const (
	instaHost        = "www.instagram.com"
	instaBaseUrl     = "https://" + instaHost
	instaWebApiV1Url = instaBaseUrl + "/api/v1/web"
	instaApiV1Url    = instaBaseUrl + "/api/v1"
)

var InstagramEndpoints = map[string]string{
	"host":            instaHost,
	"base_url":        instaBaseUrl, //+ "/",
	"login_page":      instaBaseUrl + "/accounts/login/",
	"messages":        instaBaseUrl + "/direct/inbox/",
	"thread":          instaBaseUrl + "/direct/t/",
	"graphql":         instaBaseUrl + "/api/graphql",
	"cookie_consent":  "https://graphql.instagram.com/graphql/",
	"default_graphql": "https://graphql.instagram.com/graphql/",

	"e2ee_ws_url":   "wss://web-chat-e2ee.instagram.com/ws/chat",
	"icdc_fetch":    "https://reg-e2ee.instagram.com/v2/fb_icdc_fetch",
	"icdc_register": "https://reg-e2ee.instagram.com/v2/fb_register_v2",

	"media_upload":     instaBaseUrl + "/ajax/mercury/upload.php?",
	"route_definition": instaBaseUrl + "/ajax/route-definition/",

	"web_login_page_v1":  instaWebApiV1Url + "/login_page/",
	"web_shared_data_v1": instaWebApiV1Url + "/data/shared_data/",
	"web_login_ajax_v1":  instaWebApiV1Url + "/accounts/login/ajax/",

	"web_profile_info": instaApiV1Url + "/users/web_profile_info/?",
	"reels_media":      instaApiV1Url + "/feed/reels_media/?",
	"media_info":       instaApiV1Url + "/media/%s/info/",
	"web_push":         instaWebApiV1Url + "/push/register/",
}
