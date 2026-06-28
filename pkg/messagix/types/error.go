package types

import (
	"errors"
	"fmt"
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
	Path            []string `json:"path"`
}

func (gqe *GraphQLError) Error() string {
	message := gqe.Message
	if gqe.Summary != "" || gqe.Description != "" {
		message = fmt.Sprintf("%s - %s", gqe.Summary, gqe.Description)
	}
	return fmt.Sprintf("graphql error at %s: %s", strings.Join(gqe.Path, "."), message)
}

var ErrPleaseReloadPage = &ErrorResponse{ErrorCode: 1357004}

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

func (er *ErrorResponse) Error() string {
	return fmt.Sprintf("%d: %s", er.ErrorCode, er.ErrorDescription)
}
