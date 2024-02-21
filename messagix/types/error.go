package types

import (
	"errors"
	"fmt"
)

type ErrorResponse struct {
	ErrorCode        int    `json:"error,omitempty"`
	ErrorSummary     string `json:"errorSummary,omitempty"`
	ErrorDescription string `json:"errorDescription,omitempty"`
	RedirectTo       string `json:"redirectTo,omitempty"`
}

func (er *ErrorResponse) Is(other error) bool {
	var otherLS *ErrorResponse
	return errors.As(other, &otherLS) && er.ErrorCode == otherLS.ErrorCode
}

func (er *ErrorResponse) Error() string {
	return fmt.Sprintf("%d: %s", er.ErrorCode, er.ErrorDescription)
}
