package bloks

type BloksDoc struct {
	ClientDocId  string
	AppID        string // used in inner layer
	FriendlyName string // usually MSGBloksActionRootQuery-{AppId}
}

var (
	BloksDocLoginHome BloksDoc = BloksDoc{
		ClientDocId: "28114594638751287741908354449",
		AppID:       "com.bloks.www.caa.login.login_homepage",
	}

	BloksDocProcessClientDataAndRedirect BloksDoc = BloksDoc{
		ClientDocId: "15577570885164679244236938926",
		AppID:       "com.bloks.www.bloks.caa.login.process_client_data_and_redirect",
	}

	BloksDocSendLoginRequest BloksDoc = BloksDoc{
		ClientDocId: "155775708812630868437451274928",
		AppID:       "com.bloks.www.bloks.caa.login.async.send_login_request",
	}
)
