package messagix

type Topic string

const (
	UNKNOWN_TOPIC       Topic = "unknown"
	LS_APP_SETTINGS     Topic = "/ls_app_settings"
	LS_FOREGROUND_STATE Topic = "/ls_foreground_state"
	LS_REQ              Topic = "/ls_req"
	LS_RESP             Topic = "/ls_resp"
)
