package endpoints

import "fmt"

const (
	facebookHost    = "facebook.com"
	messengerHost   = "messenger.com"
	facebookTorHost = "facebookwkhpilnemxj7asaniu7vnjjbiltxjqhye3mhbshg7kx5tfyd.onion"
)

var FacebookEndpoints = makeFacebookEndpoints(facebookHost)
var MessengerEndpoints = makeFacebookEndpoints(messengerHost)
var FacebookTorEndpoints = makeFacebookEndpoints(facebookTorHost)
var MessengerLiteIOSEndpoints = makeMessengerLiteEndpoints(facebookHost, "graph.facebook.com", false)
var MessengerLiteAndroidEndpoints = makeMessengerLiteEndpoints(facebookHost, "b-graph.facebook.com", true)

func makeFacebookEndpoints(host string) map[string]string {
	wwwHost := "www." + host
	baseURL := "https://" + wwwHost
	dgwBase := "https://gateway." + host
	urls := map[string]string{
		"host":           wwwHost,
		"base_url":       baseURL,
		"messages":       baseURL + "/messages",
		"thread":         baseURL + "/messages/t/",
		"cookie_consent": baseURL + "/cookie/consent/",
		"graphql":        baseURL + "/api/graphql/",
		"media_upload":   baseURL + "/ajax/mercury/upload.php?",
		"web_push":       baseURL + "/push/register/service_worker/",

		"dgw_lightspeed": dgwBase + "/ws/lightspeed",

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

func makeMessengerLiteEndpoints(host string, graph string, doubleSlash bool) map[string]string {
	slash := "/"
	if doubleSlash {
		slash = "//"
	}
	endpoints := makeFacebookEndpoints(host)
	endpoints["graph_graphql"] = fmt.Sprintf("https://%s/graphql", graph)
	endpoints["pwd_key"] = fmt.Sprintf("https://%s%spwd_key_fetch", graph, slash)
	endpoints["v2.10"] = "https://graph.facebook.com/v2.10"
	endpoints["cat"] = "https://web.facebook.com/messaging/lightspeed/cat"
	endpoints["icdc_fetch"] = "https://v.whatsapp.net/v2/fb_icdc_fetch"
	endpoints["icdc_register"] = "https://v.whatsapp.net/v2/fb_register_v2"

	return endpoints
}
