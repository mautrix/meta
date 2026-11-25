package bloks

import (
	"fmt"

	"go.mau.fi/util/l10n"
)

var L = l10n.NewNamespace("mautrix-meta")

const errorParamComment = "error is a message explaining what went wrong; it may be a localized bridge message or untranslated text from Meta"

var (
	msgStepFBCredentialsInstructions        = L.Define("login.step.facebook.email_password.instructions", "Enter your Facebook credentials. The Messenger network will only work with Facebook accounts.")
	msgStepFBCredentialsErrorInstructions   = L.Define("login.step.facebook.email_password_error.instructions", "{error}. Enter your Facebook credentials. The Messenger network will only work with Facebook accounts.", l10n.WithComment(errorParamComment))
	msgStepIGCredentialsInstructions        = L.Define("login.step.instagram.email_password.instructions", "Enter your Instagram email or username and password.")
	msgStepIGCredentialsErrorInstructions   = L.Define("login.step.instagram.email_password_error.instructions", "{error}. Enter your Instagram email or username and password.", l10n.WithComment(errorParamComment))
	msgStepFBPasswordInstructions           = L.Define("login.step.facebook.password.instructions", "Re-enter your Facebook password.")
	msgStepFBPasswordErrorInstructions      = L.Define("login.step.facebook.password_error.instructions", "{error}. Re-enter your Facebook password.", l10n.WithComment(errorParamComment))
	msgStepIGPasswordInstructions           = L.Define("login.step.instagram.password.instructions", "Re-enter your Instagram password.")
	msgStepIGPasswordErrorInstructions      = L.Define("login.step.instagram.password_error.instructions", "{error}. Re-enter your Instagram password.", l10n.WithComment(errorParamComment))
	msgStepFBAccountSelectionInstructions   = L.Define("login.step.facebook.account_selection.instructions", "Choose the Facebook account to connect")
	msgStepIGAccountSelectionInstructions   = L.Define("login.step.instagram.account_selection.instructions", "Choose the Instagram account to connect")
	msgStepFBCaptchaInstructions            = L.Define("login.step.facebook.captcha.instructions", "Facebook requires solving a captcha")
	msgStepIGCaptchaInstructions            = L.Define("login.step.instagram.captcha.instructions", "Instagram requires solving a captcha")
	msgStepFBDialogInstructions             = L.Define("login.step.facebook.dialog.instructions", "Facebook requires a confirmation.")
	msgStepIGDialogInstructions             = L.Define("login.step.instagram.dialog.instructions", "Instagram requires a confirmation.")
	msgStepIGCodeInstructions               = L.Define("login.step.instagram.otp_code.instructions", "Enter the security code Instagram sent you")
	msgStepIGCodeErrorInstructions          = L.Define("login.step.instagram.otp_code_error.instructions", "{error}. Enter the security code Instagram sent you", l10n.WithComment(errorParamComment))
	msgStepIGSMSInstructions                = L.Define("login.step.instagram.sms.instructions", "Enter the SMS code sent to your phone number")
	msgStepIGSMSErrorInstructions           = L.Define("login.step.instagram.sms_error.instructions", "{error}. Enter the SMS code sent to your phone number", l10n.WithComment(errorParamComment))
	msgStepIGContactPointInstructions       = L.Define("login.step.instagram.choose_contact_point.instructions", "Choose where to receive an MFA code")
	msgStepIGWhatsAppInstructions           = L.Define("login.step.instagram.whatsapp.instructions", "Enter the code sent to you on WhatsApp")
	msgStepIGWhatsAppErrorInstructions      = L.Define("login.step.instagram.whatsapp_error.instructions", "{error}. Enter the code sent to you on WhatsApp", l10n.WithComment(errorParamComment))
	msgStepIGConfirmationInstructions       = L.Define("login.step.instagram.confirmation_action.instructions", "Instagram sent a sign-in link by SMS. To use a different verification method, choose Try another way.")
	msgStepIGConfirmationErrorInstructions  = L.Define("login.step.instagram.confirmation_action_error.instructions", "{error}. Instagram sent a sign-in link by SMS. To use a different verification method, choose Try another way.", l10n.WithComment(errorParamComment))
	msgStepTestCaptchaInstructions          = L.Define("login.step.test_captcha.instructions", "Here is a test captcha")
	msgStepBackupCodeInstructions           = L.Define("login.step.backup_code.instructions", "Enter one of your two-factor backup codes.")
	msgStepBackupCodeErrorInstructions      = L.Define("login.step.backup_code_error.instructions", "{error}. Enter one of your two-factor backup codes.", l10n.WithComment(errorParamComment))
	msgStepReCaptchaInstructions            = L.Define("login.step.recaptcha.instructions", "Complete the Google reCAPTCHA challenge.")
	msgStepMFATypeInstructions              = L.Define("login.step.mfa_type.instructions", "Choose how to finish signing in")
	msgStepTOTPInstructions                 = L.Define("login.step.totp.instructions", "Enter a six-digit code from your authenticator app")
	msgStepTOTPErrorInstructions            = L.Define("login.step.totp_error.instructions", "{error}. Enter a six-digit code from your authenticator app", l10n.WithComment(errorParamComment))
	msgStepGoogleOAuthInstructions          = L.Define("login.step.google_oauth.instructions", "Sign in with your Google account.")
	msgStepGoogleOAuthErrorInstructions     = L.Define("login.step.google_oauth_error.instructions", "{error}. Sign in with your Google account.", l10n.WithComment(errorParamComment))
	msgFieldFBCodeName                      = L.Define("login.field.facebook.otp_code.name", "One-time code sent to you")
	msgFieldIGCodeName                      = L.Define("login.field.instagram.otp_code.name", "Instagram verification code")
	msgFieldDialogActionName                = L.Define("login.field.dialog_action.name", "Continue login")
	msgFieldCaptchaCodeName                 = L.Define("login.field.captcha_code.name", "Captcha code")
	msgFieldUsernameName                    = L.Define("login.field.username.name", "Username or email address")
	msgFieldPasswordName                    = L.Define("login.field.password.name", "Password")
	msgFieldConfirmationActionName          = L.Define("login.field.confirmation_action.name", "Verification method")
	msgFieldConfirmationActionOptionAnother = L.Define("login.field.confirmation_action.option.try_another_way", "Try another way")
	msgFieldAccountName                     = L.Define("login.field.account.name", "Instagram account")
	msgFieldBackupCodeName                  = L.Define("login.field.backup_code.name", "Backup code")
	msgFieldMFATypeName                     = L.Define("login.field.mfatype.name", "Login method")
	msgFieldTOTPCodeName                    = L.Define("login.field.totp_code.name", "Six-digit code")
	msgFieldSMSCodeName                     = L.Define("login.field.sms_code.name", "Six-digit code")
	msgFieldWhatsAppCodeName                = L.Define("login.field.whatsapp_code.name", "Six-digit code")
	msgFieldContactPointName                = L.Define("login.field.contact_point.name", "Phone number or email")
	msgFieldCodeActionName                  = L.Define("login.field.code_action.name", "Verification code")
	msgFieldCodeActionOptionEnter           = L.Define("login.field.code_action.option.enter_code", "Enter code")
	msgFieldCodeActionOptionResend          = L.Define("login.field.code_action.option.get_new_code", "Get a new code")
	msgFieldCodeActionOptionAnother         = L.Define("login.field.code_action.option.try_another_way", "Try another way")

	msgErrInvalidCredentials          = L.Define("login.error.invalid_credentials", "Invalid username or password")
	msgErrIncorrectPassword           = L.Define("login.error.incorrect_password", "The password you entered is incorrect")
	msgErrInvalidEmail                = L.Define("login.error.invalid_email", "Invalid email address")
	msgErrNoAnotherMethod             = L.Define("login.error.no_another_method", "Instagram did not advance to another verification method")
	msgErrCodeResendFailed            = L.Define("login.error.code_resend_failed", "Instagram didn't confirm that it sent a new code. Check for a message before requesting another")
	msgErrCodeResendAccepted          = L.Define("login.error.code_resend_accepted", "Instagram is sending a new code")
	msgErrCodeAnotherFailed           = L.Define("login.error.code_another_failed", "Instagram didn't offer another way to confirm it's you")
	msgErrLoginRejected               = L.Define("login.error.login_rejected", "{service} rejected that login", l10n.WithComment("service is the network name, Facebook or Instagram"))
	msgErrCaptchaRejected             = L.Define("login.error.captcha_rejected", "{service} rejected that captcha solution", l10n.WithComment("service is the network name, Facebook or Instagram"))
	msgErrCodeRejected                = L.Define("login.error.code_rejected", "{service} rejected that code", l10n.WithComment("service is the network name, Facebook or Instagram"))
	msgErrCodeNotSent                 = L.Define("login.error.code_not_sent", "That code was not sent to {service}; please try again", l10n.WithComment("service is the network name, Facebook or Instagram"))
	msgErrUsernameNotConnected        = L.Define("login.error.username_not_connected", "That username is not connected to a {service} account", l10n.WithComment("service is the network name, Facebook or Instagram"))
	msgErrEmailNotConnected           = L.Define("login.error.email_not_connected", "That email address is not connected to a {service} account", l10n.WithComment("service is the network name, Facebook or Instagram"))
	msgErrGoogleVerificationCancelled = L.Define("login.error.google_verification_cancelled", "Google verification was cancelled")
	msgErrGoogleVerificationFailed    = L.Define("login.error.google_verification_failed", "Google verification failed")
)

type instructionTemplates struct {
	plain, withError *l10n.Template
}

// stepInstructions consumes LastError. Text from Meta takes priority over the fallback template.
func (b *Browser) stepInstructions(remoteText string, fallback instructionTemplates) l10n.Message {
	lastError := b.LastError
	b.LastError = l10n.Message{}
	if remoteText == "" && fallback.plain != nil {
		if lastError.IsZero() {
			return fallback.plain.Message()
		}
		return fallback.withError.Render(l10n.Params{"error": lastError})
	}
	if !lastError.IsZero() {
		remoteText = fmt.Sprintf("%s. %s", lastError.Text, remoteText)
	}
	return l10n.Unlocalized(remoteText)
}
