package endpoints

const (
	instaHost = "www.instagram.com"
	instaBaseUrl = "https://" + instaHost
	instaWebApiV1Url = instaBaseUrl + "/api/v1/web"
	instaApiV1Url = instaBaseUrl + "/api/v1"
)

var InstagramEndpoints = map[string]string{
	"host": instaHost,
	"base_url": instaBaseUrl, //+ "/",
	"login_page": instaBaseUrl + "/accounts/login/",
	"messages": instaBaseUrl + "/direct/inbox/",
	"graphql": instaBaseUrl + "/api/graphql",
	"cookie_consent": "https://graphql.instagram.com/graphql/",
	"default_graphql": "https://graphql.instagram.com/graphql/",

	"media_upload": instaBaseUrl + "/ajax/mercury/upload.php?",

	"web_login_page_v1": instaWebApiV1Url + "/login_page/",
	"web_shared_data_v1": instaWebApiV1Url + "/data/shared_data/",
	"web_login_ajax_v1": instaWebApiV1Url + "/accounts/login/ajax/",
	
	"web_profile_info": instaApiV1Url + "/users/web_profile_info/?",
	"reels_media": instaApiV1Url + "/feed/reels_media/?",
	"media_info": instaApiV1Url + "/media/%s/info/",
}