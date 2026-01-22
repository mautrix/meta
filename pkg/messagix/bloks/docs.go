package bloks

type BloksDoc struct {
	ClientDocID string
	AppID       string // used in inner layer
}

// It should be possible to autogenerate these somehow, not sure how yet
var DocIDs = map[string]string{
	"com.bloks.www.caa.login.login_homepage":                             "28114594638751287741908354449", // ???
	"com.bloks.www.bloks.caa.login.process_client_data_and_redirect":     "15577570885164679244236938926", // MSGBloksActionRootQuery
	"com.bloks.www.bloks.caa.login.async.send_login_request":             "15577570885164679244236938926", // MSGBloksActionRootQuery
	"com.bloks.www.two_step_verification.entrypoint":                     "28114594639880354457944446921", // MSGBloksAppRootQuery
	"com.bloks.www.two_step_verification.verify_code.async":              "15577570885164679244236938926", // MSGBloksActionRootQuery
	"com.bloks.www.two_step_verification.method_picker":                  "28114594639880354457944446921", // MSGBloksAppRootQuery
	"com.bloks.www.two_step_verification.method_picker.navigation.async": "15577570885164679244236938926", // MSGBloksActionRootQuery
	"com.bloks.www.two_factor_login.enter_totp_code":                     "28114594639880354457944446921", // MSGBloksAppRootQuery
	"com.bloks.www.two_step_verification.approve_from_another_device":    "28114594639880354457944446921", // MSGBloksAppRootQuery
	"com.bloks.www.two_step_verification.afad_state.async":               "15577570885164679244236938926", // MSGBloksActionRootQuery
	"com.bloks.www.two_step_verification.afad_complete.async":            "15577570885164679244236938926", // MSGBloksActionRootQuery
}
