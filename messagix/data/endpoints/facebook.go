package endpoints

const (
	facebookHost = "www.facebook.com"
	facebookBaseUrl = "https://" + facebookHost
)

var FacebookEndpoints = map[string]string{
	"host": facebookHost,
	"base_url": facebookBaseUrl,
	"login_page": facebookBaseUrl + "/login",
	"messages": facebookBaseUrl + "/messages",
	"cookie_consent": facebookBaseUrl + "/cookie/consent/",
	"graphql": facebookBaseUrl + "/api/graphql/",
	"media_upload": facebookBaseUrl + "/ajax/mercury/upload.php?",
}