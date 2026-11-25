package instameow

import (
	"go.mau.fi/util/l10n"
)

var L = l10n.NewNamespace("mautrix-meta")

const channelParamComment = "channel is an English name of where the code was sent: email, SMS, WhatsApp, or WhatsApp or SMS"

var (
	msgStepAccountManagerInstructions = L.Define("login.step.instagram.account_manager.instructions", "Choose the Instagram profile to connect")
	msgFieldAccountManagerName        = L.Define("login.field.instagram.account.name", "Instagram profile")

	msgStepAPApproveInstructions       = L.Define("login.step.instagram.auth_platform.approve.instructions", "Approve this login in the Instagram app on another device where you're signed in.")
	msgStepAPApproveDeviceInstructions = L.Define(
		"login.step.instagram.auth_platform.approve_device.instructions", "Instagram sent a notification to your {device}. Approve this login in the Instagram app on that device.",
		l10n.WithComment("device is the device name Instagram reported, e.g. a phone model"),
	)
	msgStepAPMethodInstructions = L.Define("login.step.instagram.auth_platform.method.instructions", "Choose one of the verification methods Instagram offered.")
	msgStepAPActionInstructions = L.Define(
		"login.step.instagram.auth_platform.action.instructions", "Instagram requires a verification code via {channel}. Choose an action below.",
		l10n.WithComment(channelParamComment),
	)
	msgStepAPCodeInstructions = L.Define(
		"login.step.instagram.auth_platform.code.instructions", "Enter your Instagram verification code from {channel}.",
		l10n.WithComment(channelParamComment),
	)
	msgStepAPCodeHintInstructions = L.Define(
		"login.step.instagram.auth_platform.code_hint.instructions", "Enter your Instagram verification code from {channel}. If it hasn't arrived, go back and choose Resend code.",
		l10n.WithComment(channelParamComment),
	)
	msgStepAPRequestFailedInstructions     = L.Define("login.step.instagram.auth_platform.request_failed.instructions", "The verification request did not complete on this device. Enter the code again to retry.")
	msgStepAPRequestFailedHintInstructions = L.Define("login.step.instagram.auth_platform.request_failed_hint.instructions", "The verification request did not complete on this device. Enter the code again to retry. If it hasn't arrived, go back and choose Resend code.")
	msgStepAPCodeRejectedInstructions      = L.Define("login.step.instagram.auth_platform.code_rejected.instructions", "Instagram did not accept that code. Check the latest code and try again, or go back to choose another method.")
	msgStepAPCodeRejectedHintInstructions  = L.Define("login.step.instagram.auth_platform.code_rejected_hint.instructions", "Instagram did not accept that code. Check the latest code and try again, or go back to choose another method. If it hasn't arrived, go back and choose Resend code.")
	msgStepAPProblemInstructions           = L.Define("login.step.instagram.auth_platform.problem.instructions", "Instagram reported a problem with verification. Check the code or choose another available method.")
	msgStepAPProblemHintInstructions       = L.Define("login.step.instagram.auth_platform.problem_hint.instructions", "Instagram reported a problem with verification. Check the code or choose another available method. If it hasn't arrived, go back and choose Resend code.")
	msgStepAPResendWaitInstructions        = L.DefinePlural(
		"login.step.instagram.auth_platform.resend_wait.instructions", "seconds",
		"Wait {seconds} second before requesting another code. You can still enter a code you've already received.",
		"Wait {seconds} seconds before requesting another code. You can still enter a code you've already received.",
	)
	msgStepAPRateLimitedInstructions           = L.Define("login.step.instagram.auth_platform.rate_limited.instructions", "Instagram is limiting code requests. You can still enter a code you've received or use another available method.")
	msgStepAPResendUnconfirmedInstructions     = L.Define("login.step.instagram.auth_platform.resend_unconfirmed.instructions", "Couldn't confirm whether Instagram sent another code. Check for a message before requesting another.")
	msgStepAPResendUnconfirmedHintInstructions = L.Define("login.step.instagram.auth_platform.resend_unconfirmed_hint.instructions", "Couldn't confirm whether Instagram sent another code. Check for a message before requesting another. If it hasn't arrived, go back and choose Resend code.")
	msgStepAPResendFailedInstructions          = L.Define("login.step.instagram.auth_platform.resend_failed.instructions", "Instagram couldn't send another code. Wait before trying again, or use another available method.")
	msgStepAPResendFailedHintInstructions      = L.Define("login.step.instagram.auth_platform.resend_failed_hint.instructions", "Instagram couldn't send another code. Wait before trying again, or use another available method. If it hasn't arrived, go back and choose Resend code.")
	msgStepAPResendAcceptedInstructions        = L.Define(
		"login.step.instagram.auth_platform.resend_accepted.instructions", "Instagram accepted the request for another code via {channel}. Enter the latest code when it arrives.",
		l10n.WithComment(channelParamComment),
	)
	msgStepAPResendAcceptedHintInstructions = L.Define(
		"login.step.instagram.auth_platform.resend_accepted_hint.instructions", "Instagram accepted the request for another code via {channel}. Enter the latest code when it arrives. If it hasn't arrived, go back and choose Resend code.",
		l10n.WithComment(channelParamComment),
	)
	msgFieldAPMethodName                  = L.Define("login.field.instagram.auth_platform.method.name", "Verification method")
	msgFieldAPActionName                  = L.Define("login.field.instagram.auth_platform.action.name", "Verification method")
	msgFieldAPActionOptionEnter           = L.Define("login.field.instagram.auth_platform.action.option.enter_code", "Enter code")
	msgFieldAPActionOptionResend          = L.Define("login.field.instagram.auth_platform.action.option.resend_code", "Resend code")
	msgFieldAPActionOptionAnother         = L.Define("login.field.instagram.auth_platform.action.option.try_another_method", "Try another method")
	msgFieldAPVerificationCodeName        = L.Define("login.field.instagram.auth_platform.verification_code.name", "Verification code")
	msgStepWebCaptchaInstructions         = L.Define("login.step.instagram.web_captcha.instructions", "Complete Instagram's CAPTCHA in the embedded browser. Your login will continue here afterwards.")
	msgStepWebCaptchaFreshInstructions    = L.Define("login.step.instagram.web_captcha_fresh.instructions", "Complete a fresh CAPTCHA to continue this login.")
	msgStepWebCaptchaRejectedInstructions = L.Define("login.step.instagram.web_captcha_rejected.instructions", "Instagram rejected or expired that CAPTCHA. Complete a fresh challenge.")
)
