package types

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorResponse struct {
	ErrorCode        int    `json:"error,omitempty"`
	ErrorSummary     string `json:"errorSummary,omitempty"`
	ErrorDescription string `json:"errorDescription,omitempty"`
	RedirectTo       string `json:"redirectTo,omitempty"`

	Errors []GraphQLError `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message  string   `json:"message"`
	Severity string   `json:"severity"`
	MIDs     []string `json:"mids"`
	Path     []string `json:"path"`
}

func (gqe *GraphQLError) Error() string {
	return fmt.Sprintf("graphql error at %s: %s", strings.Join(gqe.Path, "."), gqe.Message)
}

var ErrPleaseReloadPage = &ErrorResponse{ErrorCode: 1357004}

func (er *ErrorResponse) Is(other error) bool {
	var otherLS *ErrorResponse
	return errors.As(other, &otherLS) && er.ErrorCode == otherLS.ErrorCode
}

func (er *ErrorResponse) Error() string {
	return fmt.Sprintf("%d: %s", er.ErrorCode, er.ErrorDescription)
}
