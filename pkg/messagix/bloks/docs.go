package bloks

type BloksDoc struct {
	ClientDocId  string
	AppID        string // used in inner layer
	FriendlyName string // usually MSGBloksActionRootQuery-{AppId}
}

var BloksDocs = map[string]BloksDoc{
	"CAA_LOGIN_HOME_PAGE": {
		ClientDocId:  "28114594638751287741908354449",
		AppID:        "com.bloks.www.caa.login.login_homepage",
		FriendlyName: "MSGBloksActionRootQuery-com.bloks.www.caa.login.login_homepage",
	},
	"CAA_LOGIN_ASYNC_SEND_LOGIN_REQUEST": { // this is a made up name
		ClientDocId:  "155775708812630868437451274928",
		AppID:        "com.bloks.www.bloks.caa.login.async.send_login_request",
		FriendlyName: "MSGBloksActionRootQuery-com.bloks.www.bloks.caa.login.async.send_login_request",
	},
}
