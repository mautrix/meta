package connector

import (
	"go.mau.fi/util/l10n"
)

var L = l10n.NewNamespace("mautrix-meta")

var (
	msgFlowFacebookName                    = L.Define("login.flow.facebook.name", "facebook.com")
	msgFlowFacebookDescription             = L.Define("login.flow.facebook.description", "Login using cookies from facebook.com")
	msgFlowMessengerName                   = L.Define("login.flow.messenger.name", "messenger.com")
	msgFlowMessengerDescription            = L.Define("login.flow.messenger.description", "Login using cookies from messenger.com")
	msgFlowMessengerLiteIOSName            = L.Define("login.flow.messenger-lite.name", "Messenger iOS")
	msgFlowMessengerLiteIOSDescription     = L.Define("login.flow.messenger-lite.description", "Login with username/password using Messenger iOS API")
	msgFlowMessengerLiteAndroidName        = L.Define("login.flow.messenger-lite-android.name", "Messenger Android")
	msgFlowMessengerLiteAndroidDescription = L.Define("login.flow.messenger-lite-android.description", "Login with username/password using Messenger Android API")

	msgStepCookiesInstructions      = L.Define("login.step.cookies.instructions", "Enter a JSON object with your cookies, or a cURL command copied from browser devtools.")
	msgStepPhoneCookiesInstructions = L.Define("login.step.phone_cookies.instructions", "Sign in to Facebook in the browser to continue with your phone number.")
	msgStepCompleteInstructions     = L.Define(
		"login.step.complete.instructions", "Logged in as {name} ({id})",
		l10n.WithComment("name is the Facebook account's display name, id is its numeric user ID"),
	)

	msgStepHistoryPINInstructions              = L.Define("login.step.history_pin.instructions", "Enter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.")
	msgStepHistoryPINCheckFailedInstructions   = L.Define("login.step.history_pin_check_failed.instructions", "Couldn't check your saved history. Enter your PIN to try again, or leave it empty to continue.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.")
	msgStepHistoryPINInvalidInstructions       = L.Define("login.step.history_pin_invalid.instructions", "Enter your six-digit PIN, or leave it empty to continue without restoring history.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.")
	msgStepHistoryPINRestoreFailedInstructions = L.Define("login.step.history_pin_restore_failed.instructions", "Couldn't restore encrypted history. You can leave the PIN empty to finish signing in.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.")
	msgStepHistoryPINWrongInstructions         = L.DefinePlural(
		"login.step.history_pin_wrong.instructions", "attempts",
		"Couldn't verify your PIN. Messenger reports {attempts} attempt remaining.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.",
		"Couldn't verify your PIN. Messenger reports {attempts} attempts remaining.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.",
	)
	msgStepHistoryPINUnconfirmedInstructions = L.Define("login.step.history_pin_unconfirmed.instructions", "Messenger hasn't confirmed history setup. Leave the PIN empty to finish signing in.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.")
	msgStepHistoryPINSaveFailedInstructions  = L.Define("login.step.history_pin_save_failed.instructions", "Couldn't save history setup. Leave the PIN empty to retry saving it.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.")
	msgStepHistoryPINQueueFailedInstructions = L.Define("login.step.history_pin_queue_failed.instructions", "Couldn't queue your saved history. Leave the PIN empty to try again.\n\nEnter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history.")
	msgFieldPINName                          = L.Define("login.field.pin.name", "Secure storage PIN")
)
