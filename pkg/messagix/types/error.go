package types

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"go.mau.fi/util/exslices"
)

type ErrorResponse struct {
	ErrorCode        int    `json:"error,omitempty"`
	ErrorSummary     string `json:"errorSummary,omitempty"`
	ErrorDescription string `json:"errorDescription,omitempty"`
	RedirectTo       string `json:"redirectTo,omitempty"`

	Errors []*GraphQLError `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message         string   `json:"message"`
	Severity        string   `json:"severity"`
	MIDs            []string `json:"mids"`
	DebugLink       string   `json:"debug_link"`
	Code            int      `json:"code"`
	ApiErrorCode    any      `json:"api_error_code"`
	Summary         string   `json:"summary"`
	Description     string   `json:"description"`
	DescriptionHTML any      `json:"description_html"`
	DescriptionRaw  string   `json:"description_raw"`
	IsSilent        bool     `json:"is_silent"`
	IsTransient     bool     `json:"is_transient"`
	IsNotCritical   bool     `json:"is_not_critical"`
	RequiresReauth  bool     `json:"requires_reauth"`
	AllowUserRetry  bool     `json:"allow_user_retry"`
	DebugInfo       any      `json:"debug_info"`
	QueryPath       any      `json:"query_path"`
	FbtraceID       string   `json:"fbtrace_id"`
	WWWRequestID    string   `json:"www_request_id"`
	Path            []any    `json:"path"`
}

func (gqe *GraphQLError) Error() string {
	message := gqe.Message
	if gqe.Summary != "" || gqe.Description != "" {
		message = fmt.Sprintf("%s - %s", gqe.Summary, gqe.Description)
	}
	return fmt.Sprintf("graphql error at %s: %s", strings.Join(exslices.CastFunc(gqe.Path, func(from any) string { return fmt.Sprint(from) }), "."), message)
}

func (gqe *GraphQLError) HumanError() string {
	return gqe.Description
}

var ErrPleaseLogIn = &ErrorResponse{ErrorCode: 1357001}
var ErrPleaseReloadPage = &ErrorResponse{ErrorCode: 1357004}
var ErrContentUnavailable = &ErrorResponse{ErrorCode: 1357031}

func (er *ErrorResponse) Is(other error) bool {
	var otherLS *ErrorResponse
	return errors.As(other, &otherLS) && er.ErrorCode == otherLS.ErrorCode
}

func (er *ErrorResponse) AsError() error {
	if er.ErrorCode != 0 {
		return er
	} else if len(er.Errors) > 0 {
		return er.WrapGQLErrors()
	}
	return nil
}

func (er *ErrorResponse) WrapGQLErrors() error {
	return errors.Join(exslices.CastFunc(er.Errors, func(from *GraphQLError) error {
		return from
	})...)
}

func (er *ErrorResponse) HumanError() string {
	if er.ErrorDescription != "" {
		return er.ErrorDescription
	}
	return er.ErrorSummary
}

func (er *ErrorResponse) RedirectPath() string {
	if er.RedirectTo == "" {
		return ""
	} else if parsed, err := url.Parse(er.RedirectTo); err == nil {
		return parsed.Path
	}
	return "(unparseable)"
}

func (er *ErrorResponse) Error() string {
	if redirectPath := er.RedirectPath(); redirectPath != "" {
		return fmt.Sprintf("%d: %s (redirect to %s)", er.ErrorCode, er.ErrorDescription, redirectPath)
	}
	return fmt.Sprintf("%d: %s", er.ErrorCode, er.ErrorDescription)
}
