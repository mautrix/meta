package endpoints

const (
	facebookHost    = "www.facebook.com"
	messengerHost   = "www.messenger.com"
	facebookTorHost = "www.facebookwkhpilnemxj7asaniu7vnjjbiltxjqhye3mhbshg7kx5tfyd.onion"
)

var FacebookEndpoints = makeFacebookEndpoints(facebookHost)
var MessengerEndpoints = makeFacebookEndpoints(messengerHost)
var FacebookTorEndpoints = makeFacebookEndpoints(facebookTorHost)
var MessengerLiteEndpoints = makeMessengerLiteEndpoints(facebookHost)

func makeFacebookEndpoints(host string) map[string]string {
	baseURL := "https://" + host
	urls := map[string]string{
		"host":           host,
		"base_url":       baseURL,
		"messages":       baseURL + "/messages",
		"thread":         baseURL + "/messages/t/",
		"cookie_consent": baseURL + "/cookie/consent/",
		"graphql":        baseURL + "/api/graphql/",
		"media_upload":   baseURL + "/ajax/mercury/upload.php?",
		"web_push":       baseURL + "/push/register/service_worker/",

		"e2ee_ws_url":   "wss://web-chat-e2ee.facebook.com/ws/chat",
		"icdc_fetch":    "https://reg-e2ee.facebook.com/v2/fb_icdc_fetch",
		"icdc_register": "https://reg-e2ee.facebook.com/v2/fb_register_v2",
	}
	if host == messengerHost {
		urls["messages"] = baseURL + "/"
		urls["thread"] = baseURL + "/t/"
	}
	return urls
}

func makeMessengerLiteEndpoints(host string) map[string]string {
	endpoints := makeFacebookEndpoints(host)
	endpoints["graph_graphql"] = "https://graph.facebook.com/graphql"
	endpoints["pwd_key"] = "https://graph.facebook.com/pwd_key_fetch"
	endpoints["v2.10"] = "https://graph.facebook.com/v2.10"
	endpoints["cat"] = "https://web.facebook.com/messaging/lightspeed/cat"
	endpoints["icdc_fetch"] = "https://v.whatsapp.net/v2/fb_icdc_fetch"
	endpoints["icdc_register"] = "https://v.whatsapp.net/v2/fb_register_v2"

	return endpoints
}
