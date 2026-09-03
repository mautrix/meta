package loginerrors

import (
	"net/http"

	"maunium.net/go/mautrix/bridgev2"
)

var (
	MissingCookies   = bridgev2.RespError{ErrCode: "FI.MAU.META_MISSING_COOKIES", Err: "Missing cookies", StatusCode: http.StatusBadRequest}
	Challenge        = bridgev2.RespError{ErrCode: "FI.MAU.META_CHALLENGE_ERROR", Err: "Challenge required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	Consent          = bridgev2.RespError{ErrCode: "FI.MAU.META_CONSENT_ERROR", Err: "Consent required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	Checkpoint       = bridgev2.RespError{ErrCode: "FI.MAU.META_CHECKPOINT_ERROR", Err: "Checkpoint required, please check the official website or app and then try again", StatusCode: http.StatusBadRequest}
	TokenInvalidated = bridgev2.RespError{ErrCode: "FI.MAU.META_TOKEN_ERROR", Err: "Got logged out immediately", StatusCode: http.StatusBadRequest}
	Unknown          = bridgev2.RespError{ErrCode: "M_UNKNOWN", Err: "Internal error logging in", StatusCode: http.StatusInternalServerError}

	PhoneNumber      = bridgev2.RespError{ErrCode: "FI.MAU.META_PHONE_NUMBER", Err: "Phone number login is not supported, please try email address or username", StatusCode: http.StatusBadRequest}
	InvalidUsername  = bridgev2.RespError{ErrCode: "FI.MAU.META_MATRIX_ID", Err: "That doesn't look like a valid username, please enter your Facebook email address or username", StatusCode: http.StatusBadRequest}
	AFADStopped      = bridgev2.RespError{ErrCode: "FI.MAU.META_AFAD_STOPPED", Err: "The approval request expired or was denied, please try logging in again", StatusCode: http.StatusBadRequest}
	MandatoryOAuth   = bridgev2.RespError{ErrCode: "FI.MAU.META_OAUTH_MANDATORY", Err: "Meta is requiring Google sign-in which is not supported. Please try adding a different MFA method to your Facebook account from the official app/website", StatusCode: http.StatusBadRequest}
	NoSupportedMFA   = bridgev2.RespError{ErrCode: "FI.MAU.META_NO_SUPPORTED_MFA", Err: "None of the available MFA methods are supported. Please try adding a different MFA method to your Facebook account from the official app/website", StatusCode: http.StatusBadRequest}
	ReCaptcha        = bridgev2.RespError{ErrCode: "FI.MAU.META_GOOGLE_RECAPTCHA", Err: "Meta is requiring Google reCAPTCHA authentication which is not supported. It may help to try again, log in from the official app/website first, or change MFA settings for your Facebook account"}
	NoSMSAvailable   = bridgev2.RespError{ErrCode: "FI.MAU.META_NO_SMS_AVAILABLE", Err: "Meta is refusing to send SMS codes right now. Try again later, or use/add a different MFA method for your Facebook account"}
	MandatoryPasskey = bridgev2.RespError{ErrCode: "FI.MAU.META_PASSKEY_MANDATORY", Err: "Meta is requiring passkey sign-in which is not supported. Please try adding a different MFA method to your Facebook account from the official app/website", StatusCode: http.StatusBadRequest}
	TokenExchange    = bridgev2.RespError{ErrCode: "FI.MAU.META_TOKEN_EXCHANGE_FAILED", Err: "Meta returned a temporary credential after login which could not be exchanged for a usable session. It may help to try again, or to log in from the official app/website first"}
	RateLimited      = bridgev2.RespError{ErrCode: "FI.MAU.META_RATE_LIMITED", Err: "Meta is temporarily rate-limiting login attempts from this network. Please wait a few minutes and try again", StatusCode: http.StatusTooManyRequests}
	AccountSuspended = bridgev2.RespError{ErrCode: "FI.MAU.META_ACCOUNT_SUSPENDED", Err: "Instagram reports that this account is suspended. Open Instagram to review or appeal the suspension before retrying", StatusCode: http.StatusForbidden}
)

func WithMessage(respError bridgev2.RespError, message string) bridgev2.RespError {
	respError.Err = message
	return respError
}

func Uninformative(message, callsite string) bridgev2.RespError {
	return bridgev2.RespError{
		ErrCode:       "FI.MAU.META_UNINFORMATIVE_ERROR",
		Err:           message,
		StatusCode:    http.StatusBadRequest,
		InternalError: "Uninformative login rejection at callsite: " + callsite,
	}
}

func AccountRecovery(service string) bridgev2.RespError {
	return bridgev2.RespError{
		ErrCode:    "FI.MAU.META_ACCOUNT_RECOVERY_REQUIRED",
		Err:        service + " requires Account Recovery for this sign-in. This is not a two-factor code. Complete the recovery check in the official app or website, then start a new bridge login",
		StatusCode: http.StatusBadRequest,
	}
}
