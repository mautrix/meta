package bloks

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type browserPlatformErrors struct {
	InvalidUsername  bridgev2.RespError
	MandatoryOAuth   bridgev2.RespError
	NoSupportedMFA   bridgev2.RespError
	ReCaptcha        bridgev2.RespError
	NoSMSAvailable   bridgev2.RespError
	MandatoryPasskey bridgev2.RespError
	AccountRecovery  bridgev2.RespError
}

type browserPlatformProfile struct {
	isInstagram bool

	serviceName             string
	stepIDPrefix            string
	initialState            BrowserState
	credentialsInstruction  string
	loginRejected           string
	accountSelection        string
	codeFieldName           string
	codeInstruction         string
	smsInstruction          string
	contactPointInstruction string
	whatsAppInstruction     string
	uninformativeMessage    string

	caaCodeEntryState         BrowserState
	authMethodState           BrowserState
	supportsNotificationTimer bool
	twoStepEntrypointState    func(*BloksBundle) BrowserState
	codeInstructions          func(*BloksBundle) string
	findMFAMethods            func(*BloksBundle, *zerolog.Logger) (map[string]*BloksTreeComponent, []string, int)

	errors browserPlatformErrors
}

func browserProfile(platform types.Platform) browserPlatformProfile {
	profile := browserPlatformProfile{
		serviceName:            "Facebook",
		stepIDPrefix:           "fi.mau.meta.messengerlite",
		initialState:           StateInitialMessenger,
		credentialsInstruction: "Enter your Facebook credentials. The Messenger network will only work with Facebook accounts.",
		loginRejected:          "Facebook rejected that login",
		accountSelection:       "Choose the Facebook account to connect",
		codeFieldName:          "One-time code sent to you",
		uninformativeMessage:   "Facebook rejected the login without providing a reason. It may help to try again, log in from the official app/website first, or change MFA settings for your Facebook account",
		caaCodeEntryState:      StateCodeEntryPage,
		authMethodState:        StateMFALandingPage,
		twoStepEntrypointState: func(*BloksBundle) BrowserState { return StateMFALandingPage },
		codeInstructions:       messengerCodeInstructions,
		findMFAMethods:         findMessengerMFAMethodOptions,
	}
	profile.errors = browserPlatformErrors{
		InvalidUsername:  loginerrors.InvalidUsername,
		MandatoryOAuth:   loginerrors.MandatoryOAuth,
		NoSupportedMFA:   loginerrors.NoSupportedMFA,
		ReCaptcha:        loginerrors.ReCaptcha,
		NoSMSAvailable:   loginerrors.NoSMSAvailable,
		MandatoryPasskey: loginerrors.MandatoryPasskey,
		AccountRecovery:  loginerrors.AccountRecovery(profile.serviceName),
	}
	if !platform.IsInstagram() {
		return profile
	}

	profile.isInstagram = true
	profile.serviceName = "Instagram"
	profile.stepIDPrefix = "fi.mau.meta.instagram.caa"
	profile.initialState = StateInitialInstagram
	profile.credentialsInstruction = "Enter your Instagram email or username and password."
	profile.loginRejected = "Instagram rejected that login"
	profile.accountSelection = "Choose the Instagram account to connect"
	profile.codeFieldName = "Instagram verification code"
	profile.codeInstruction = "Enter the security code Instagram sent you"
	profile.smsInstruction = "Enter the SMS code sent to your phone number"
	profile.contactPointInstruction = "Choose where to receive an MFA code"
	profile.whatsAppInstruction = "Enter the code sent to you on WhatsApp"
	profile.uninformativeMessage = "Instagram rejected the login without providing a reason. Try again later or complete the security check in the official app or website"
	profile.caaCodeEntryState = StateAccountRecoveryPage
	profile.authMethodState = StateChooseMFAPage
	profile.supportsNotificationTimer = true
	profile.twoStepEntrypointState = twoStepVerificationEntrypointState
	profile.codeInstructions = instagramCodeInstructions
	profile.findMFAMethods = func(page *BloksBundle, _ *zerolog.Logger) (map[string]*BloksTreeComponent, []string, int) {
		return findMFAMethodOptions(page)
	}
	profile.errors = browserPlatformErrors{
		InvalidUsername: loginerrors.WithMessage(
			loginerrors.InvalidUsername,
			"That doesn't look like a valid Instagram username or email address",
		),
		MandatoryOAuth: loginerrors.WithMessage(
			loginerrors.MandatoryOAuth,
			"Meta is requiring Google sign-in, which is not supported. Please add a different two-factor method in the official app or website",
		),
		NoSupportedMFA: loginerrors.WithMessage(
			loginerrors.NoSupportedMFA,
			"None of the available two-factor methods are supported. Please add a different method in the official app or website",
		),
		ReCaptcha: loginerrors.WithMessage(
			loginerrors.ReCaptcha,
			"Meta is requiring Google reCAPTCHA, which is not supported in this native flow. Try again later or complete the security check in the official app or website",
		),
		NoSMSAvailable: loginerrors.WithMessage(
			loginerrors.NoSMSAvailable,
			"Meta can't send an SMS code right now. Try again later or use a different two-factor method",
		),
		MandatoryPasskey: loginerrors.WithMessage(
			loginerrors.MandatoryPasskey,
			"Meta is requiring a passkey, which is not supported. Please add a different two-factor method in the official app or website",
		),
		AccountRecovery: loginerrors.AccountRecovery(profile.serviceName),
	}
	return profile
}

func (profile browserPlatformProfile) logTwoStepEntrypoint(
	log *zerolog.Logger,
	page *BloksBundle,
	state BrowserState,
) {
	if !profile.isInstagram {
		return
	}
	foundMethods, _, unsupportedMethods := findMFAMethodOptions(page)
	log.Debug().
		Str("entrypoint_state", string(state)).
		Int("text_input_count", len(page.FindDescendants(FilterByComponent("bk.components.TextInput")))).
		Int("supported_method_count", len(foundMethods)).
		Int("unsupported_method_count", unsupportedMethods).
		Msg("Classified two-step verification entrypoint")
}

func (profile browserPlatformProfile) logCredentialSubmissionError(
	log *zerolog.Logger,
	err error,
	errorKind string,
	safeDetail string,
) {
	if !profile.isInstagram {
		log.Debug().Err(err).Msg("Got error from username/password submission")
		return
	}
	event := log.Debug().Str("error_kind", errorKind)
	if safeDetail != "" {
		event = event.Str("bloks_identifier", safeDetail)
	}
	event.Msg("Got error from username/password submission")
}

func (profile browserPlatformProfile) isCheckpointRejection(err error) bool {
	if !profile.isInstagram {
		return false
	}
	var checkpointErr CheckpointError
	return errors.As(err, &checkpointErr)
}

func (profile browserPlatformProfile) unhandledCredentialError(
	err error,
	errorKind string,
	safeDetail string,
) error {
	if !profile.isInstagram {
		return fmt.Errorf("tapping login button: %w", err)
	}
	if safeDetail != "" {
		return fmt.Errorf("tapping instagram login button failed (%s %s)", errorKind, safeDetail)
	}
	return fmt.Errorf("tapping instagram login button failed (%s)", errorKind)
}

func (profile browserPlatformProfile) fallbackCodeInput(
	page *BloksBundle,
	input *BloksTreeComponent,
) *BloksTreeComponent {
	if profile.isInstagram && input == nil {
		return page.FindDescendant(FilterByComponent("bk.components.TextInput"))
	}
	return input
}

func (profile browserPlatformProfile) prepareCodeSubmission(
	browser *Browser,
	button *BloksTreeComponent,
	log *zerolog.Logger,
	description string,
) {
	if !profile.isInstagram {
		return
	}
	browser.LastError = ""
	resetPending := resetPendingCodeSubmissionFlags(button, browser.CurrentPage.Interpreter)
	if resetPending > 0 {
		log.Debug().Int("reset_count", resetPending).Msg("Reset stale " + description + " pending flags")
	}
}

func (profile browserPlatformProfile) finishCodeSubmission(
	browser *Browser,
	expectedState BrowserState,
	actionRPCCountBefore uint64,
) {
	if !profile.isInstagram || browser.State != expectedState {
		return
	}
	if browser.ActionRPCCount == actionRPCCountBefore {
		browser.LastError = browser.codeNotSentMessage()
	} else {
		browser.LastError = browser.rejectedCodeMessage()
	}
}

func (profile browserPlatformProfile) hasMFAMethodsOnLanding(page *BloksBundle) bool {
	if !profile.isInstagram {
		return false
	}
	foundMethods, _, _ := findMFAMethodOptions(page)
	return len(foundMethods) > 0
}

func (profile browserPlatformProfile) invalidMFAMethodError(method string) error {
	if profile.isInstagram {
		return errors.New("invalid two-factor method")
	}
	return fmt.Errorf("not a valid mfa method: %s", method)
}

func (profile browserPlatformProfile) mfaMethodTapError(method string, err error) error {
	if profile.isInstagram {
		return fmt.Errorf("tapping selected two-factor method: %w", err)
	}
	return fmt.Errorf("tapping %q button: %w", method, err)
}

func (profile browserPlatformProfile) shouldContinueAfterMFAMethod(state BrowserState) bool {
	return !profile.isInstagram || state == StateChooseMFAPage
}

func (profile browserPlatformProfile) invalidContactPointError(contactPoint string) error {
	if profile.isInstagram {
		return errors.New("invalid contact point")
	}
	return fmt.Errorf("not a valid contact point: %s", contactPoint)
}

func messengerCodeInstructions(page *BloksBundle) string {
	if page == nil {
		return ""
	}
	return page.
		FindDescendant(func(comp *BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			for _, prefix := range []string{"Enter the code", "We sent a code"} {
				if strings.HasPrefix(comp.GetAttribute("text"), prefix) {
					return true
				}
			}
			return false
		}).
		GetAttribute("text")
}

func instagramCodeInstructions(page *BloksBundle) string {
	if page == nil {
		return ""
	}
	instructions := []string{}
	for _, comp := range page.FindDescendants(FilterByComponent("bk.data.TextSpan")) {
		text := strings.TrimSpace(comp.GetAttribute("text"))
		normalized := strings.ToLower(text)
		if !strings.Contains(normalized, "code") {
			continue
		}
		for _, prefix := range []string{
			"enter ",
			"we sent ",
			"check ",
			"get ",
			"use ",
			"you'll need ",
			"you’ll need ",
		} {
			if strings.HasPrefix(normalized, prefix) {
				instructions = appendUniqueString(instructions, text)
				break
			}
		}
	}
	return strings.Join(instructions, "\n\n")
}
