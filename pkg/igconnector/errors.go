// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package igconnector

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// wrapConversionError turns a failure to convert a Matrix event into a message status. Nothing has been
// sent to Instagram at that point, so the failure is certain.
func wrapConversionError(err error) error {
	if err == nil || errors.As(err, &bridgev2.MessageStatus{}) {
		return err
	}
	return bridgev2.WrapErrorInStatus(err).
		WithMessage("failed to convert message for Instagram").
		WithIsCertain(true).
		WithSendNotice(true)
}

func isInstagramAccountError(err error) bool {
	return errors.Is(err, httpclient.ErrChallengeRequired) ||
		errors.Is(err, httpclient.ErrCheckpointRequired) ||
		errors.Is(err, httpclient.ErrConsentRequired) ||
		errors.Is(err, httpclient.ErrAccountSuspended)
}

func isInstagramTimeout(err error) bool {
	return errors.Is(err, dgw.ErrAckTimeout) ||
		errors.Is(err, dgw.ErrReceiveTimeout) ||
		errors.Is(err, dgw.ErrClosedBeforeTimeout) ||
		errors.Is(err, context.DeadlineExceeded)
}

// wrapInstagramError turns an error from an instameow request made while handling a Matrix event into a
// message status.
func wrapInstagramError(err error) error {
	if err == nil || errors.As(err, &bridgev2.MessageStatus{}) {
		return err
	}
	var lsErr *types.ErrorResponse
	var gqlErr *types.GraphQLError
	switch {
	case errors.Is(err, instameow.ErrClientIsNil), errors.Is(err, dgw.ErrSocketNotOpen):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("not connected to Instagram").
			WithIsCertain(true).
			WithSendNotice(true)
	case isInstagramTimeout(err):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("no response from Instagram").
			WithIsCertain(false).
			WithSendNotice(true)
	case errors.Is(err, httpclient.ErrRateLimited):
		// httpclient retries after request errors, so an earlier attempt may have gone through.
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("rate limited by Instagram").
			WithErrorReason(event.MessageStatusNetworkError).
			WithIsCertain(false).
			WithSendNotice(true)
	case errors.Is(err, httpclient.ErrTokenInvalidated):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("logged out of Instagram").
			WithIsCertain(true).
			WithSendNotice(true)
	case isInstagramAccountError(err):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("Instagram requires you to check your account on the website or app").
			WithIsCertain(true).
			WithSendNotice(true)
	case errors.Is(err, instameow.ErrThreadNotFound):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("chat not found on Instagram").
			WithIsCertain(true).
			WithSendNotice(true)
	case errors.Is(err, httpclient.ErrRequestFailed):
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("failed to reach Instagram").
			WithIsCertain(false).
			WithSendNotice(true)
	case errors.As(err, &lsErr), errors.As(err, &gqlErr), errors.Is(err, httpclient.ErrUnexpectedError):
		// httpclient retries after request errors, so an earlier attempt may have gone through.
		st := bridgev2.WrapErrorInStatus(err)
		if st.Message == "" {
			st = st.WithMessage("Instagram returned an error")
		}
		return st.WithIsCertain(false).WithSendNotice(true)
	default:
		return bridgev2.WrapErrorInStatus(err).
			WithMessage("unexpected error from Instagram").
			WithSendNotice(true)
	}
}

// wrapInstagramRespError turns an error from an instameow request made during login or provisioning into
// an HTTP error, keeping the original error in the chain. Clients retry 429 and 502-504 automatically,
// which isn't wanted for provisioning requests, so those statuses aren't used.
func wrapInstagramRespError(err error) error {
	var mapped bridgev2.RespError
	var lsErr *types.ErrorResponse
	var gqlErr *types.GraphQLError
	switch {
	case errors.Is(err, httpclient.ErrRateLimited):
		mapped = bridgev2.RespError(mautrix.MLimitExceeded.WithMessage("Rate limited by Instagram. Try again later.").WithStatus(http.StatusBadRequest))
	case errors.Is(err, instameow.ErrClientIsNil), errors.Is(err, dgw.ErrSocketNotOpen):
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Not connected to Instagram"))
	case isInstagramTimeout(err):
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Instagram didn't respond in time"))
	case errors.Is(err, httpclient.ErrTokenInvalidated):
		mapped = bridgev2.RespError(mautrix.MForbidden.WithMessage("Logged out of Instagram"))
	case isInstagramAccountError(err):
		mapped = bridgev2.RespError(mautrix.MForbidden.WithMessage("Instagram requires you to check your account on the website or app"))
	case errors.Is(err, instameow.ErrThreadNotFound):
		mapped = bridgev2.RespError(mautrix.MNotFound.WithMessage("Chat not found on Instagram"))
	case errors.As(err, &lsErr), errors.As(err, &gqlErr), errors.Is(err, httpclient.ErrUnexpectedError):
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Instagram returned an error"))
	default:
		mapped = bridgev2.RespError(mautrix.MUnknown.WithMessage("Request to Instagram failed"))
	}
	return fmt.Errorf("%w: %w", mapped.WithInternalError(err), err)
}
