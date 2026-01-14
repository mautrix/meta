package bloks

type BloksDoc struct {
	ClientDocID string
	AppID       string // used in inner layer
}

var (
	BloksDocLoginHome BloksDoc = BloksDoc{
		ClientDocID: "28114594638751287741908354449",
		AppID:       "com.bloks.www.caa.login.login_homepage",
	}

	BloksDocProcessClientDataAndRedirect BloksDoc = BloksDoc{
		ClientDocID: "15577570885164679244236938926",
		AppID:       "com.bloks.www.bloks.caa.login.process_client_data_and_redirect",
	}

	BloksDocSendLoginRequest BloksDoc = BloksDoc{
		ClientDocID: "155775708812630868437451274928",
		AppID:       "com.bloks.www.bloks.caa.login.async.send_login_request",
	}

	BloksDocTwoStepVerificationEntrypoint BloksDoc = BloksDoc{
		ClientDocID: "28114594639880354457944446921",
		AppID:       "com.bloks.www.two_step_verification.entrypoint",
	}

	BloksDocVerifyCode BloksDoc = BloksDoc{
		ClientDocID: "15577570885164679244236938926",
		AppID:       "com.bloks.www.two_step_verification.verify_code.async",
	}
)
