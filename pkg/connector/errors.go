package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.mau.fi/whatsmeow"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// wrapConversionError turns a failure to convert a Matrix event into a message status. Nothing has been
// sent to Meta at that point, so the failure is certain.
func wrapConversionError(err error) error {
	if err == nil || errors.As(err, &bridgev2.MessageStatus{}) {
		return err
	}
	return bridgev2.WrapErrorInStatus(err).
		WithMessage("failed to convert message for Facebook").
		WithIsCertain(true).
		WithSendNotice(true)
}

func isMetaAccountError(err error) bool {
	return errors.Is(err, httpclient.ErrChallengeRequired) ||
		errors.Is(err, httpclient.ErrCheckpointRequired) ||
		errors.Is(err, httpclient.ErrConsentRequired) ||
		errors.Is(err, httpclient.ErrAccountSuspended)
}

// wrapMetaError turns an error from a messagix or whatsmeow (encrypted chat) request made while handling
// a Matrix event into a message status.
func wrapMetaError(err error) error {
	if err == nil || errors.As(err, &bridgev2.MessageStatus{}) {
		return err
	}
	var lsErr *types.ErrorResponse
	var gqlErr *types.GraphQLError
	var iqErr *whatsmeow.IQError
	var disconnectErr *whatsmeow.DisconnectedError
	switch {
	case errors.Is(err, messagix.ErrClientIsNil), errors.Is(err, dgw.ErrSocketNotOpen),
		errors.Is(err, whatsmeow.ErrNotConnected):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("not connected to Facebook").
			WithErrorReason(event.MessageStatusNetworkError).
			WithIsCertain(true).
			WithSendNotice(true)
	case errors.Is(err, dgw.ErrAckTimeout), errors.Is(err, dgw.ErrReceiveTimeout),
		errors.Is(err, dgw.ErrClosedBeforeTimeout), errors.Is(err, whatsmeow.ErrMessageTimedOut),
		errors.Is(err, whatsmeow.ErrIQTimedOut), errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &disconnectErr):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("no response from Facebook").
			WithErrorReason(event.MessageStatusNetworkError).
			WithIsCertain(false).
			WithSendNotice(true)
	case errors.Is(err, httpclient.ErrRateLimited), errors.Is(err, whatsmeow.ErrIQRateOverLimit):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("rate limited by Facebook").
			WithErrorReason(event.MessageStatusNetworkError).
			WithIsCertain(true).
			WithSendNotice(true)
	case errors.Is(err, httpclient.ErrTokenInvalidated):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("logged out of Facebook").
			WithIsCertain(true).
			WithSendNotice(true)
	case isMetaAccountError(err):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("Facebook requires you to check your account on the website or app").
			WithIsCertain(true).
			WithSendNotice(true)
	case errors.Is(err, whatsmeow.ErrIQForbidden), errors.Is(err, whatsmeow.ErrIQNotAuthorized):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("you don't have permission to do that on Facebook").
			WithErrorReason(event.MessageStatusNoPermission).
			WithStatus(event.MessageStatusFail).
			WithIsCertain(true).
			WithSendNotice(true)
	case errors.Is(err, httpclient.ErrRequestFailed):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("failed to reach Facebook").
			WithErrorReason(event.MessageStatusNetworkError).
			WithIsCertain(false).
			WithSendNotice(true)
	case errors.As(err, &lsErr), errors.As(err, &gqlErr), errors.Is(err, httpclient.ErrUnexpectedError),
		errors.Is(err, whatsmeow.ErrServerReturnedError), errors.As(err, &iqErr):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("Facebook returned an error").
			WithIsCertain(true).
			WithSendNotice(true)
	default:
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("unexpected error from Facebook").
			WithSendNotice(true)
	}
}

// wrapMetaRespError turns an error from a messagix request made during login or provisioning into an
// HTTP error, keeping the original error in the chain. Clients retry 429 and 502-504 automatically,
// which isn't wanted for provisioning requests, so those statuses aren't used.
func wrapMetaRespError(err error) error {
	var mapped bridgev2.RespError
	var lsErr *types.ErrorResponse
	var gqlErr *types.GraphQLError
	switch {
	case errors.Is(err, httpclient.ErrRateLimited):
		mapped = bridgev2.RespError(mautrix.MLimitExceeded.WithMessage("Rate limited by Facebook. Try again later.").WithStatus(http.StatusBadRequest))
	case errors.Is(err, messagix.ErrClientIsNil), errors.Is(err, dgw.ErrSocketNotOpen):
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Not connected to Facebook"))
	case errors.Is(err, dgw.ErrAckTimeout), errors.Is(err, dgw.ErrReceiveTimeout),
		errors.Is(err, dgw.ErrClosedBeforeTimeout), errors.Is(err, context.DeadlineExceeded):
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Facebook didn't respond in time"))
	case errors.Is(err, httpclient.ErrTokenInvalidated):
		mapped = bridgev2.RespError(mautrix.MForbidden.WithMessage("Logged out of Facebook"))
	case isMetaAccountError(err):
		mapped = bridgev2.RespError(mautrix.MForbidden.WithMessage("Facebook requires you to check your account on the website or app"))
	case errors.As(err, &lsErr), errors.As(err, &gqlErr), errors.Is(err, httpclient.ErrUnexpectedError):
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Facebook returned an error"))
	default:
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Request to Facebook failed"))
	}
	return fmt.Errorf("%w: %w", mapped, err)
}
