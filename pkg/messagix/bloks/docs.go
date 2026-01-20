package bloks

type BloksDoc struct {
	ClientDocID string
	AppID       string // used in inner layer
}

// It should be possible to autogenerate these somehow, not sure how yet
var DocIDs = map[string]string{
	"com.bloks.www.caa.login.login_homepage":                         "28114594638751287741908354449",
	"com.bloks.www.bloks.caa.login.process_client_data_and_redirect": "15577570885164679244236938926",
	"com.bloks.www.bloks.caa.login.async.send_login_request":         "155775708812630868437451274928",
	"com.bloks.www.two_step_verification.entrypoint":                 "28114594639880354457944446921",
	"com.bloks.www.two_step_verification.verify_code.async":          "15577570885164679244236938926",
	"com.bloks.www.two_step_verification.method_picker":              "28114594639880354457944446921",
}
