package endpoints

const (
	instaBaseHost    = "instagram.com"
	instaWWWHost     = "www." + instaBaseHost
	instaBaseUrl     = "https://" + instaWWWHost
	instaWebApiV1Url = instaBaseUrl + "/api/v1/web"
	instaApiV1Url    = instaBaseUrl + "/api/v1"
	instaDGWBase     = "https://gateway." + instaBaseHost
)

var InstagramEndpoints = map[string]string{
	"host":            instaWWWHost,
	"base_url":        instaBaseUrl, //+ "/",
	"messages":        instaBaseUrl + "/direct/inbox/",
	"thread":          instaBaseUrl + "/direct/t/",
	"graphql":         instaBaseUrl + "/api/graphql",
	"cookie_consent":  "https://graphql.instagram.com/graphql/",
	"default_graphql": "https://graphql.instagram.com/graphql/",

	"dgw_lightspeed":       instaDGWBase + "/ws/lightspeed",
	"dgw_mqttbypass":       instaDGWBase + "/ws/mqttbypass",
	"dgw_streamcontroller": instaDGWBase + "/ws/streamcontroller",

	"e2ee_ws_url":   "wss://web-chat-e2ee.instagram.com/ws/chat",
	"icdc_fetch":    "https://reg-e2ee.instagram.com/v2/fb_icdc_fetch",
	"icdc_register": "https://reg-e2ee.instagram.com/v2/fb_register_v2",

	"media_upload":     instaBaseUrl + "/ajax/mercury/upload.php?",
	"rupload_ig":       "https://rupload.facebook.com/messenger_image/",
	"i_graphql":        "https://i.instagram.com/graphql_www",
	"route_definition": instaBaseUrl + "/ajax/route-definition/",

	"bulk_route_definition": instaBaseUrl + "/ajax/bulk-route-definitions/",

	"web_profile_info": instaApiV1Url + "/users/web_profile_info/?",
	"reels_media":      instaApiV1Url + "/feed/reels_media/?",
	"media_info":       instaApiV1Url + "/media/%s/info/",
	"web_push":         instaWebApiV1Url + "/push/register/",
}
