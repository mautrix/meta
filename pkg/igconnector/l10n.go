package igconnector

import (
	"go.mau.fi/util/l10n"
)

var L = l10n.NewNamespace("mautrix-meta")

var (
	msgFlowCookiesName              = L.Define("login.flow.instagram.name", "Cookies")
	msgFlowCookiesDescription       = L.Define("login.flow.instagram.description", "Login using cookies from instagram.com")
	msgFlowAndroidNativeName        = L.Define("login.flow.android.name", "Native Android")
	msgFlowAndroidNativeDescription = L.Define("login.flow.android.description", "Log in with your username and password using the Instagram Android API")
	msgFlowWebNativeName            = L.Define("login.flow.instagram-password.name", "Native Web")
	msgFlowWebNativeDescription     = L.Define("login.flow.instagram-password.description", "Log in with your username and password using the Instagram web API")

	msgStepCookiesInstructions  = L.Define("login.step.instagram.cookies.instructions", "Enter a JSON object with your cookies, or a cURL command copied from browser devtools.")
	msgStepCompleteInstructions = L.Define(
		"login.step.instagram.complete.instructions", "Logged in as {name} ({id})",
		l10n.WithComment("name is the Instagram account's display name, id is its numeric user ID"),
	)
	msgStepWebChallengeInstructions = L.Define("login.step.instagram.web_challenge.instructions", "Instagram needs you to finish a verification step. Complete it in the browser (you may need to sign in again), then your login will continue here.")
	msgStepWebLoginInstructions     = L.Define("login.step.instagram.web_login.instructions", "Instagram couldn't sign you in here. Sign in on Instagram's page instead, then your login will continue here.")
	msgStepInboxRetryInstructions   = L.Define("login.step.instagram.inbox_retry.instructions", "Your device couldn't load the Instagram inbox. Check your connection and retry to finish signing in.")
	msgFieldRetryName               = L.Define("login.field.instagram.retry.name", "Finish signing in")
	msgFieldRetryOptionRetry        = L.Define("login.field.instagram.retry.option.retry_loading_inbox", "Retry loading inbox")

	msgStepCredentialsInstructions              = L.Define("login.step.instagram.credentials.instructions", "Enter your Instagram email or username and password.")
	msgStepCredentialsExpiredInstructions       = L.Define("login.step.instagram.credentials_expired.instructions", "This Instagram login session expired. Start the login again.")
	msgStepCredentialsMissingInstructions       = L.Define("login.step.instagram.credentials_missing.instructions", "Enter both your Instagram email or username and password.")
	msgStepCredentialsRetryInstructions         = L.Define("login.step.instagram.credentials_retry.instructions", "The request did not complete on this device. Please try again.")
	msgStepCredentialsLoginRejectedInstructions = L.Define("login.step.instagram.credentials_login_rejected.instructions", "Instagram couldn't sign you in. Check your account in Instagram before trying again.")
	msgStepCredentialsInvalidInstructions       = L.Define("login.step.instagram.credentials_invalid.instructions", "Instagram didn't accept that username or password. Check your credentials and try again.")
	msgStepCredentialsRegistrationInstructions  = L.Define("login.step.instagram.credentials_registration.instructions", "Instagram offered to create a new account for those details instead of signing in. Check that you're using the username or email of your Instagram account.")
	msgStepCredentialsCSRFInstructions          = L.Define("login.step.instagram.credentials_csrf.instructions", "Instagram did not return the security state needed to continue. Please try again.")
	msgStepCredentialsTwoFactorInstructions     = L.Define("login.step.instagram.credentials_two_factor.instructions", "That profile uses two-factor authentication. Log in with its own username and password to continue.")
	msgStepCredentialsSwitchFailedInstructions  = L.Define("login.step.instagram.credentials_switch_failed.instructions", "Instagram didn't let Beeper switch to that profile. Log in with its own username and password to continue.")
	msgFieldUsernameName                        = L.Define("login.field.instagram.username.name", "Email or username")
	msgFieldUsernameDescription                 = L.Define("login.field.instagram.username.description", "The email address or username for your Instagram account.")
	msgFieldPasswordName                        = L.Define("login.field.instagram.password.name", "Password")
	msgFieldPasswordDescription                 = L.Define("login.field.instagram.password.description", "Your Instagram password.")

	msgStepTwoFactorInstructions              = L.Define("login.step.instagram.web_two_factor.instructions", "Enter your Instagram verification code.")
	msgStepTwoFactorTOTPInstructions          = L.Define("login.step.instagram.web_two_factor_totp.instructions", "Enter the verification code from your authenticator app.")
	msgStepTwoFactorEmailInstructions         = L.Define("login.step.instagram.web_two_factor_email.instructions", "Enter the verification code Instagram sent to your email.")
	msgStepTwoFactorSMSOrWhatsAppInstructions = L.Define("login.step.instagram.web_two_factor_sms_or_whatsapp.instructions", "Enter the verification code Instagram sent to your phone by WhatsApp or SMS.")
	msgStepTwoFactorSMSInstructions           = L.Define("login.step.instagram.web_two_factor_sms.instructions", "Enter the verification code Instagram sent to your phone by SMS.")
	msgStepTwoFactorWhatsAppInstructions      = L.Define("login.step.instagram.web_two_factor_whatsapp.instructions", "Enter the verification code Instagram sent to your WhatsApp.")
	msgStepTwoFactorMissingInstructions       = L.Define("login.step.instagram.web_two_factor_missing.instructions", "Enter the verification code to continue creating the Instagram messaging session.")
	msgStepTwoFactorRetryInstructions         = L.Define("login.step.instagram.web_two_factor_retry.instructions", "The request did not complete on this device. Enter a fresh verification code and try again.")
	msgStepTwoFactorResentInstructions        = L.Define("login.step.instagram.web_two_factor_resent.instructions", "Instagram rejected that code. A fresh SMS code was requested. Enter it when it arrives.")
	msgStepTwoFactorRejectedInstructions      = L.Define("login.step.instagram.web_two_factor_rejected.instructions", "Instagram did not accept that code. Check that it is the latest code from Instagram, then try again.")
	msgFieldVerificationCodeName              = L.Define("login.field.instagram.verification_code.name", "Verification code")
	msgFieldVerificationCodeDescription       = L.Define("login.field.instagram.verification_code.description", "The verification code for your Instagram account.")
)
