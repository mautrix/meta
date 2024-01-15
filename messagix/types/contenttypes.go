package types

type ContentType string

const (
	NONE ContentType = ""
	JSON ContentType = "application/json"
	FORM ContentType = "application/x-www-form-urlencoded"
)
