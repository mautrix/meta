package endpoints

const (
	facebookHost    = "www.facebook.com"
	messengerHost   = "www.messenger.com"
	facebookTorHost = "www.facebookwkhpilnemxj7asaniu7vnjjbiltxjqhye3mhbshg7kx5tfyd.onion"
)

var FacebookEndpoints = makeFacebookEndpoints(facebookHost)
var MessengerEndpoints = makeFacebookEndpoints(messengerHost)
var FacebookTorEndpoints = makeFacebookEndpoints(facebookTorHost)

func makeFacebookEndpoints(host string) map[string]string {
	baseURL := "https://" + host
	urls := map[string]string{
		"host":           host,
		"base_url":       baseURL,
		"login_page":     baseURL + "/login",
		"messages":       baseURL + "/messages",
		"thread":         baseURL + "/t/",
		"cookie_consent": baseURL + "/cookie/consent/",
		"graphql":        baseURL + "/api/graphql/",
		"media_upload":   baseURL + "/ajax/mercury/upload.php?",
		"web_push":       baseURL + "/push/register/service_worker/",

		"icdc_fetch":    "https://reg-e2ee.facebook.com/v2/fb_icdc_fetch",
		"icdc_register": "https://reg-e2ee.facebook.com/v2/fb_register_v2",
	}
	if host == messengerHost {
		urls["messages"] = baseURL + "/"
	}
	return urls
}
