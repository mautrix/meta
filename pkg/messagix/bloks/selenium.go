package bloks

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/random"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

var (
	ErrLoginPhoneNumber = bridgev2.RespError{ErrCode: "FI.MAU.META_PHONE_NUMBER", Err: "Phone number login is not supported, please try email address or username", StatusCode: http.StatusBadRequest}
)

// This error is returned in cases where we have observed Meta returning an error that is
// not due to anything the bridge or user has done wrong, and will cause login to fail for
// this account even in the official Messenger iOS app.
//
// Using an IP address with a low reputation for Meta makes it more likely that Meta will
// block logins for an undisclosed reason, but such blocks are account-specific and other
// accounts can still be logged into.
//
// Account logins are still sometimes blocked even when using a high-reputation residential
// IP address. It's possible that account configuration plays a role, for example which MFA
// methods are enabled, but the details are not known.
func ErrLoginUninformative(callsite string) bridgev2.RespError {
	return bridgev2.RespError{
		ErrCode:       "FI.MAU.META_UNINFORMATIVE_ERROR",
		Err:           "Facebook rejected the login without providing a reason, please try again",
		StatusCode:    http.StatusBadRequest,
		InternalError: "Uninformative login rejection at callsite: " + callsite,
	}
}

func (bb *BloksBundle) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	return bb.Layout.Payload.Tree.FindDescendant(pred)
}

func (bb *BloksBundle) FindDescendants(pred func(*BloksTreeComponent) bool) []*BloksTreeComponent {
	return bb.Layout.Payload.Tree.FindDescendants(pred)
}

func (btn *BloksTreeNode) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	if btn == nil {
		return nil
	}
	if comp, ok := btn.BloksTreeNodeContent.(*BloksTreeComponent); ok {
		return comp.FindDescendant(pred)
	}
	if comps, ok := btn.BloksTreeNodeContent.(*BloksTreeComponentList); ok {
		for _, comp := range *comps {
			if match := comp.FindDescendant(pred); match != nil {
				return match
			}
		}
	}
	return nil
}

func (btn *BloksTreeNode) FindDescendants(pred func(*BloksTreeComponent) bool) []*BloksTreeComponent {
	if btn == nil {
		return nil
	}
	if comp, ok := btn.BloksTreeNodeContent.(*BloksTreeComponent); ok {
		return comp.FindDescendants(pred)
	}
	if comps, ok := btn.BloksTreeNodeContent.(*BloksTreeComponentList); ok {
		matches := []*BloksTreeComponent{}
		for _, comp := range *comps {
			matches = append(matches, comp.FindDescendants(pred)...)
		}
		return matches
	}
	return nil
}

func (comp *BloksTreeComponent) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	if pred(comp) {
		return comp
	}
	for _, subnode := range comp.Attributes {
		if match := subnode.FindDescendant(pred); match != nil {
			return match
		}
	}
	return nil
}

func (comp *BloksTreeComponent) FindDescendants(pred func(*BloksTreeComponent) bool) []*BloksTreeComponent {
	if pred(comp) {
		return []*BloksTreeComponent{comp}
	}
	matches := []*BloksTreeComponent{}
	for _, subnode := range comp.Attributes {
		matches = append(matches, subnode.FindDescendants(pred)...)
	}
	return matches
}

func (comp *BloksTreeComponent) FindAncestor(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	for comp != nil {
		if pred(comp) {
			return comp
		}
		comp = comp.parent
	}
	return nil
}

func (comp *BloksTreeComponent) FindCousin(pred func(*BloksTreeComponent) bool) (found *BloksTreeComponent) {
	comp.FindAncestor(func(comp *BloksTreeComponent) bool {
		found = comp.FindDescendant(pred)
		return found != nil
	})
	return
}

func (comp *BloksTreeComponent) FindContainingButton() *BloksTreeComponent {
	return comp.FindCousin(func(comp *BloksTreeComponent) bool {
		for _, prop := range []BloksAttributeID{"on_click", "on_touch_down", "on_touch_up"} {
			if comp.Attributes[prop] != nil {
				return true
			}
		}
		return false
	})
}

func FilterByComponent(compid BloksComponentID) func(comp *BloksTreeComponent) bool {
	return func(comp *BloksTreeComponent) bool {
		return comp.ComponentID == compid
	}
}

func FilterByAttribute(compid BloksComponentID, attr BloksAttributeID, value string) func(comp *BloksTreeComponent) bool {
	return func(comp *BloksTreeComponent) bool {
		if comp.ComponentID != compid {
			return false
		}
		return comp.GetAttribute(attr) == value
	}
}

func (comp *BloksTreeComponent) GetAttribute(name BloksAttributeID) string {
	if comp == nil {
		return ""
	}
	attr := comp.Attributes[name]
	if attr == nil {
		return ""
	}
	value, ok := attr.BloksTreeNodeContent.(*BloksTreeLiteral)
	if !ok {
		return ""
	}
	str, ok := value.BloksJavaScriptValue.(string)
	if !ok {
		return ""
	}
	return str
}

func (input *BloksTreeComponent) FillInput(ctx context.Context, interp *Interpreter, text string) error {
	if input == nil {
		return fmt.Errorf("no such input")
	}
	err := input.SetTextContent(text)
	if err != nil {
		return err
	}
	onChanged, ok := input.Attributes["on_text_change"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("no on_text_change script")
	}
	_, err = interp.Evaluate(InterpBindThis(ctx, input), &onChanged.AST)
	if err != nil {
		return fmt.Errorf("on_text_change: %w", err)
	}
	return err
}

func (comp *BloksTreeComponent) GetScript(name BloksAttributeID) *BloksTreeScript {
	if comp == nil {
		return nil
	}
	elem, ok := comp.Attributes[name]
	if !ok {
		return nil
	}
	script, ok := elem.BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return nil
	}
	return script
}

func (comp *BloksTreeComponent) GetChildren(name BloksAttributeID) []*BloksTreeComponent {
	if comp == nil {
		return nil
	}
	elem, ok := comp.Attributes[name]
	if !ok {
		return nil
	}
	list, ok := elem.BloksTreeNodeContent.(*BloksTreeComponentList)
	if !ok {
		return nil
	}
	return *list
}

func (comp *BloksTreeComponent) GetDynamicAttribute(ctx context.Context, interp *Interpreter, name BloksAttributeID) string {
	if val := comp.GetAttribute(name); val != "" {
		return val
	}
	bind := comp.Attributes["on_bind"]
	if bind == nil {
		return ""
	}
	scripts, ok := bind.BloksTreeNodeContent.(*BloksTreeScriptSet)
	if !ok {
		return ""
	}
	script, ok := scripts.Scripts[name]
	if !ok {
		return ""
	}
	val, err := evalAs[string](ctx, interp, &script.AST, fmt.Sprintf("on_bind.%s", name))
	if err != nil {
		return ""
	}
	return val
}

func (button *BloksTreeComponent) TapButton(ctx context.Context, interp *Interpreter) error {
	if button == nil {
		return fmt.Errorf("no such button")
	}
	// First try on_click, if that's missing, try the on_touch handlers
	onClick := button.GetScript("on_click")
	if onClick != nil {
		_, err := interp.Evaluate(InterpBindThis(ctx, button), &onClick.AST)
		if err != nil {
			return fmt.Errorf("on_click: %w", err)
		}
		return nil
	}
	onTouchDown := button.GetScript("on_touch_down")
	onTouchUp := button.GetScript("on_touch_up")
	if onTouchDown != nil && onTouchUp != nil {
		_, err := interp.Evaluate(InterpBindThis(ctx, button), &onTouchDown.AST)
		if err != nil {
			return fmt.Errorf("on_touch_down: %w", err)
		}
		_, err = interp.Evaluate(InterpBindThis(ctx, button), &onTouchUp.AST)
		if err != nil {
			return fmt.Errorf("on_touch_up: %w", err)
		}
		return nil
	}
	return fmt.Errorf("couldn't find any event handlers on button")
}

type BrowserState string

// AFAD = Approve From Another Device
// TOTP = Time-Based One-Time Passcode
// MFA = Multi-Factor Authentication
// AP = Authentication Platform
const (
	StateUnknown               BrowserState = ""
	StateTestCaptcha           BrowserState = "test-captcha"
	StateInitial               BrowserState = "initial"
	StateEmailPasswordPage     BrowserState = "enter-email-and-password-page"
	StateCodeEntryPage         BrowserState = "enter-code-page"
	StateCaptchaPage           BrowserState = "captcha-page"
	StateMFALandingPage        BrowserState = "mfa-landing-page"
	StateChooseMFAPage         BrowserState = "choose-mfa-type-page"
	StateAFADPage              BrowserState = "afad-page"
	StateAFADPageWaiting       BrowserState = "afad-waiting"
	StateTOTPPage              BrowserState = "totp-page"
	StateOAuthPage             BrowserState = "oauth-page"
	StateSMSPage               BrowserState = "sms-page"
	StateSMSPageAfterSend      BrowserState = "sms-page-after-send"
	StateBackupCodePage        BrowserState = "backup-code-page"
	StateChooseNumberPage      BrowserState = "choose-number-page"
	StateWhatsAppPage          BrowserState = "whatsapp-page"
	StateWhatsAppPageAfterSend BrowserState = "whatsapp-page-after-send"
	StateSuccess               BrowserState = "success"
)

type BrowserConfig struct {
	EncryptPassword  func(context.Context, string) (string, error)
	MakeBloksRequest func(context.Context, *BloksDoc, *BloksRequestOuter) (*BloksBundle, error)
}

type Browser struct {
	State       BrowserState
	CurrentPage *BloksBundle

	Config *BrowserConfig
	Bridge *InterpBridge

	AFADNotification string
	AFADInterval     time.Duration
	AFADCallback     func() error
	LoginData        string
	DisplayedURL     string

	LastError string
}

// You will want an explanation of how to maintain this code.
//
// The problem being solved is tricky because Facebook wants to shake their frontend around like a
// dog with a stick, and you never really know which way things will get yanked. State changes
// happen in all kinds of callbacks. But at the same time we kind of need to keep track of where we
// are at least a bit, so we know what page we're on and whether there was an error we need to
// report to the user. Despite not being able to write down the whole flow graph explicitly.
//
// The current implementation of the page state graph works as follows.
//
// There is a single b.State variable that keeps track of what page we're on. This is associated
// with a single Bloks bundle stored in b.CurrentPage. Only one page can be displayed/active at a
// time. Now note that there are also Bloks bundles that represent actions. But these aren't
// incorporated into the state graph, unlike in previous versions of this code. Instead, when we get
// an action bundle, we execute it immediately, and just catch up later to see if it did what we
// were hoping it would.
//
// The state starts off in StateInitial, which kicks things off by making a Bloks request manually
// and executing the action that it gets back. When an action is executed, it can trigger further
// action RPC calls, which can execute further actions. Or it can trigger page RPC calls, which lead
// to the interpreter invoking DisplayNewScreen, which updates the page state.
//
// Now, depending on the page state, we have a big switch statement that tells us what actions to
// undertake. This maps reasonably to the human interpretation of "which page am I looking at, and
// therefore what buttons should I try to tap". We expect that executing the logic for a given page
// will do some sequence of actions that navigates us to another page, and if it doesn't, we crash.
// The tricky part comes in when we want to incorporate user input and recoverable errors into that
// flow.
//
// User input: Our implementation here is driven in part by how bridgev2 handles user input. Which
// is that you return a list of input fields, then get called back later with the values for those
// input fields, and must then return another list of input fields, and so on until eventually you
// return success. To provide that interface on top of our big switch statement, we put the switch
// statement in a loop, and give each switch case the capability to return user input fields. Then
// the loop keeps running the switch statement to transition through pages until reaching one where
// the implementation says "this page needs some inputs to complete". Bridgev2 calls us back later
// with the values for those inputs, and the same switch case sees that it has now been passed
// values, so it skips over returning the user input list, and instead uses those values to complete
// the page logic.
//
// Recoverable errors: This is handled by a single b.LastError variable, which is intended
// exclusively for recoverable errors that occur within a single page (i.e., an error that occurs by
// redirecting to a separate error page would not be handled by this mechanism). We have at least
// two different ways we can get a recoverable error, which Facebook chooses between based on
// planetary alignment and the phase of the moon.
//
// One is that executing the page interactions can trigger a Bloks error popup, which the
// interpreter translates into returning a Golang error object. In cases where this might happen due
// to bad user input, we check the error return value, see if it's an error message from Facebook
// that we understand, and if so, assign it to b.LastError. (Otherwise we just return it as fatal.)
//
// The second case is that executing the page interactions doesn't throw an error, but does update a
// page variable, which is intended to display the error message inline. Rather than try to parse
// out those inline error messages from the actual page contents, which is tricky given Facebook's
// disgusting lack of proper CSS selector or other navigability/accessibility features, we just use
// the interpreter to hook onto those variables, and update b.LastError when one of them gets set.
//
// In summary, we execute the page logic, maybe catch certain errors, and at the end of it, either
// we are on a new page (in which case b.LastError is reset), or we are still on the same page and
// b.LastError is set to something (in which case we loop back, display the error to the user, and
// ask for input again), or we are still on the same page and there is no b.LastError (in which case
// there has been a logic error and we crash).
//
// To make the above happen as described, we follow this pattern for all switch cases that take user
// input:
//
// ```
// case StateAskingForXYZPage:
//   xyz := userInput["xyz"]
//   if xyz == "" {
//     step = &bridgev2.LoginStep{ ... }
//     break
//   }
//
//   delete(userInput, "xyz")
//   b.LastError = "Some generic message about how XYZ was rejected"
//
//   ... try to submit xyz, maybe overwrite b.LastError ...
// ```
//
// If the submission logic works, and we end up on a new page, b.LastError is thrown away. If it
// doesn't work, we have a placeholder error message that can be shown to the user next time.
// (Include it into the instructions in the returned LoginStep if it's set.) If our variable watches
// or error checking work properly, we will get a more specific b.LastError that can be used
// instead. And finally, note that deleting the field from userInput ensures that if we end up in an
// error state, then we'll re-prompt the user for input, rather than reusing what they gave last
// time.

func NewBrowser(cfg *BrowserConfig) *Browser {
	b := Browser{
		State:  StateInitial,
		Config: cfg,
	}
	b.Bridge = &InterpBridge{
		DeviceID:       strings.ToUpper(uuid.New().String()),
		FamilyDeviceID: strings.ToUpper(uuid.New().String()),
		// Note: machine_id is set to an empty string the first time the user ever logs in
		// to any account on a given physical device. After a successful login, the login
		// response payload contains a new machine_id that is stored in shady locations that
		// the user can never normally clear even after uninstalling all their apps, and
		// used for all subsequent login attempts to enable persistent tracking across
		// multiple accounts on the same physical device.
		//
		// We do not replicate the second part of that behavior. However, doing so means
		// phone number login does not work, as phone number logins are rejected without a
		// valid machine_id. Note that this implies that the official app is unable to do
		// phone number login, either, unless you've previously logged in a different way
		// (to any account) on the same device. Yes, I tested that.
		//
		// The machine_id would generally be a 24 character alphanumeric string. However it
		// cannot be generated on the client side so this fact is purely informational.
		MachineID:       "",
		EncryptPassword: cfg.EncryptPassword,
		DoPageRPC: func(ctx context.Context, name string, params map[string]string) (*BloksBundle, error) {
			log := zerolog.Ctx(ctx)
			log.Debug().Str("state", string(b.State)).Str("rpc", name).Str("rpc_type", "page").Msg("Invoking RPC from Bloks")
			var paramsInner BloksParamsInner
			err := json.Unmarshal([]byte(params["params"]), &paramsInner)
			if err != nil {
				return nil, fmt.Errorf("parsing %s params: %w", name, err)
			}
			bundle, err := cfg.MakeBloksRequest(ctx, &BloksAppDoc, NewBloksRequest(name, paramsInner))
			if err != nil {
				return nil, fmt.Errorf("rpc %s: %w", name, err)
			}
			return bundle, nil
		},
		DoActionRPC: func(ctx context.Context, name string, params map[string]string) (*BloksScriptNode, error) {
			log := zerolog.Ctx(ctx)
			log.Debug().Str("state", string(b.State)).Str("rpc", name).Str("rpc_type", "action").Msg("Invoking RPC from Bloks")
			var paramsInner BloksParamsInner
			err := json.Unmarshal([]byte(params["params"]), &paramsInner)
			if err != nil {
				return nil, fmt.Errorf("parsing %s params: %w", name, err)
			}
			bundle, err := cfg.MakeBloksRequest(ctx, &BloksActionDoc, NewBloksRequest(name, paramsInner))
			if err != nil {
				return nil, fmt.Errorf("rpc %s: %w", name, err)
			}
			action := bundle.Action()
			if action == nil {
				// This is a super weird action that appears to be handled by the Bloks runtime in
				// an unusual way. It is basically an action, but formatted as a page with a
				// component inside it that has the action code, but then the page itself is tagged
				// as an action.
				script := bundle.FindDescendant(FilterByComponent("action")).GetScript("on_load")
				if script == nil {
					return nil, fmt.Errorf("AP action from rpc %s did not contain script", name)
				}
				action = &script.AST
			}
			// Action payload doesn't include a new page, but it might include some
			// extra payloads or scripts, we need to merge those in.
			//
			// NB: Terrible bug happens if you re-assign b.CurrentPage.Interpreter here,
			// because the calling code still has a reference to the old interpreter and
			// any variable updates in the callback will be lost.
			err = b.CurrentPage.Interpreter.MergeActionBundle(ctx, bundle)
			if err != nil {
				return nil, fmt.Errorf("merging interpreter with new action")
			}
			return action, nil
		},
		DisplayNewScreen: func(ctx context.Context, name string, page *BloksBundle) error {
			log := zerolog.Ctx(ctx)
			log.Debug().Str("state", string(b.State)).Str("screen", name).Msg("Displaying new screen from Bloks")
			newState := StateUnknown
			switch name {
			case "com.bloks.www.caa.login.login_homepage":
				newState = StateEmailPasswordPage
			case "com.bloks.www.caa.ar.code_entry",
				"com.bloks.www.ap.two_step_verification.code_entry":
				newState = StateCodeEntryPage
			case "com.bloks.www.two_step_verification.entrypoint":
				newState = StateMFALandingPage
			case "com.bloks.www.two_step_verification.enter_text_captcha_code":
				newState = StateCaptchaPage
			case "com.bloks.www.ap.two_step_verification.approve_from_another_device",
				"com.bloks.www.two_step_verification.approve_from_another_device":
				// Meta tends to send you here by default and we need to treat it as
				// a landing page that we then navigate to the MFA method picker
				// from. But in case we already went to the method picker and picked
				// AFAD, then we will end up back here and we want to actually do
				// AFAD, not redirect back to the picker again.
				if b.State == StateChooseMFAPage {
					newState = StateAFADPage
				} else {
					newState = StateMFALandingPage
				}
			case "com.bloks.www.ap.two_step_verification.challenge_picker",
				"com.bloks.www.two_step_verification.method_picker",
				"com.bloks.www.caa.ar.auth_method":
				newState = StateChooseMFAPage
			case "com.bloks.www.two_factor_login.enter_totp_code":
				newState = StateTOTPPage
			case "com.bloks.www.ap.two_step_verification.login_with_third_party":
				newState = StateOAuthPage
			case "com.bloks.www.two_step_verification.enter_sms_code":
				newState = StateSMSPage
			case "com.bloks.www.two_factor_login.enter_backup_code":
				newState = StateBackupCodePage
			case "com.bloks.www.ap.two_step_verification.contactpoint_chooser":
				newState = StateChooseNumberPage
			case "com.bloks.www.two_step_verification.enter_whatsapp_code":
				newState = StateWhatsAppPage
			default:
				return fmt.Errorf("unexpected new screen %s", name)
			}
			if newState == StateUnknown {
				return fmt.Errorf("can't handle new screen %s in state %s", name, b.State)
			}

			err := page.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter(), true)
			if err != nil {
				return err
			}

			b.CurrentPage = page
			b.State = newState
			return nil
		},
		HandleLoginResponse: func(ctx context.Context, data string) error {
			log := zerolog.Ctx(ctx)
			log.Debug().Str("state", string(b.State)).Msg("Handling login response from Bloks")
			b.LoginData = data
			b.State = StateSuccess
			return nil
		},
		StartTimer: func(name string, interval time.Duration, callback func() error) error {
			switch name {
			case "approve_from_another_device_polling_timer":
				b.AFADInterval = interval
				b.AFADCallback = callback
			default:
				return fmt.Errorf("unexpected timer %s", name)
			}
			return nil
		},
		OpenURL: func(url string) error {
			b.DisplayedURL = url
			return nil
		},
		HandleVariableChange: func(ctx context.Context, name string, value *BloksScriptLiteral) error {
			switch name {
			case "BLOKS_TWO_STEP_VERIFICATION_ENTER_CODE:error_message":
				switch b.State {
				case StateTOTPPage, StateSMSPage, StateWhatsAppPage, StateBackupCodePage:
				default:
					return nil
				}
				msg, ok := value.Value().(string)
				if !ok {
					return fmt.Errorf("non-string code error: %T", value.Value())
				}
				b.LastError = msg
			case "BLOKS_AUTH_PLATFORM_ENTER_CODE:error_message":
				if b.State != StateCodeEntryPage {
					break
				}
				msg, ok := value.Value().(string)
				if !ok {
					return fmt.Errorf("non-string email code error: %T", value.Value())
				}
				b.LastError = msg
			}
			return nil
		},
	}
	return &b
}

var definitelyNotPhoneNumberRegexp = regexp.MustCompile(`^.*[@a-zA-Z].*$`)

func (b *Browser) getCodeInstructions() string {
	return b.CurrentPage.
		FindDescendant(func(comp *BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			for _, prefix := range []string{
				"Enter the code",
				"We sent a code",
			} {
				if strings.HasPrefix(comp.GetAttribute("text"), prefix) {
					return true
				}
			}
			return false
		}).
		GetAttribute("text")
}

func (b *Browser) getContactNumberInstructions() string {
	return b.CurrentPage.
		FindDescendant(func(comp *BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			return strings.HasPrefix(comp.GetAttribute("text"), "Which number")
		}).
		GetAttribute("text")
}

func (b *Browser) DoLoginStep(ctx context.Context, userInput map[string]string) (step *bridgev2.LoginStep, err error) {
	log := zerolog.Ctx(ctx)
	{
		fields := []string{}
		for field := range userInput {
			fields = append(fields, field)
		}
		log.Debug().Str("cur_state", string(b.State)).Strs("user_input", fields).Msg("Executing login step")
	}
	prevState := b.State
	switch b.State {

	case StateTestCaptcha:
		if userInput["captcha_code"] == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.captcha",
				Instructions: "Here is a test captcha",
				UserInputParams: &bridgev2.LoginUserInputParams{
					Attachments: []*bridgev2.LoginUserInputAttachment{
						{
							Type:     event.MsgImage,
							FileName: "captcha.png",
							Content:  debugImageCaptcha,
							Info: bridgev2.LoginUserInputAttachmentInfo{
								MimeType: "image/png",
								Width:    280,
								Height:   70,
								Size:     len(debugImageCaptcha),
							},
						}, {
							Type:     event.MsgAudio,
							FileName: "captcha.ogg",
							Content:  debugAudioCaptcha,
							Info: bridgev2.LoginUserInputAttachmentInfo{
								MimeType: "audio/ogg",
								Size:     len(debugAudioCaptcha),
							},
						},
					},
					Fields: []bridgev2.LoginInputDataField{
						{ID: "captcha_code", Name: "Captcha code", Type: bridgev2.LoginInputFieldTypeCaptchaCode},
					},
				},
			}
			break
		}
		b.State = StateInitial

	case StateInitial:
		rpc := "com.bloks.www.bloks.caa.login.process_client_data_and_redirect"
		action, err := b.Config.MakeBloksRequest(ctx, &BloksActionDoc, NewBloksRequest(rpc, map[string]any{
			"blocked_uid":                               []any{},
			"offline_experiment_group":                  "caa_iteration_v2_perf_ls_ios_test_1",
			"family_device_id":                          b.Bridge.FamilyDeviceID,
			"use_auto_login_interstitial":               true,
			"layered_homepage_experiment_group":         "not_in_experiment",
			"disable_recursive_auto_login_interstitial": true,
			"show_internal_settings":                    false,
			"waterfall_id":                              hex.EncodeToString(random.Bytes(16)),
			"account_list":                              []any{},
			"disable_auto_login":                        false,
			"is_from_logged_in_switcher":                false,
			"auto_login_interstitial_experiment_group":  "",
			"device_id":                                 b.Bridge.DeviceID,
			"machine_id":                                b.Bridge.MachineID,
		}))
		if err != nil {
			return nil, fmt.Errorf("rpc %s: %w", rpc, err)
		}

		// Set up the action bundle as if it's the "current page", just so we have this
		// variable non-null and can reference it later. Even though it's really an action,
		// not a page.
		b.CurrentPage = action

		err = action.SetupInterpreter(ctx, b.Bridge, nil, true)
		if err != nil {
			return nil, fmt.Errorf("setup %s interpreter: %w", b.State, err)
		}

		_, err = action.Interpreter.Evaluate(ctx, action.Action())
		if err != nil {
			return nil, fmt.Errorf("initial action: %w", err)
		}

	case StateEmailPasswordPage:
		username := userInput["username"]
		password := userInput["password"]
		if username == "" || password == "" {
			instructions := "Enter your Facebook credentials. The Messenger network will only work with Facebook accounts."
			if b.LastError != "" {
				instructions = fmt.Sprintf("%s. %s", b.LastError, instructions)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.email_password",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "username", Name: "Username or email address", Type: bridgev2.LoginInputFieldTypeUsername},
						{ID: "password", Name: "Password", Type: bridgev2.LoginInputFieldTypePassword},
					},
				},
			}
			break
		}

		if !definitelyNotPhoneNumberRegexp.MatchString(username) {
			return nil, ErrLoginPhoneNumber
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "username")
		delete(userInput, "password")
		b.LastError = "Facebook rejected that login"

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.components.TextInput", "html_name", "email")).
			FillInput(ctx, b.CurrentPage.Interpreter, username)
		if err != nil {
			return nil, fmt.Errorf("filling email input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.components.TextInput", "html_name", "password")).
			FillInput(ctx, b.CurrentPage.Interpreter, password)
		if err != nil {
			return nil, fmt.Errorf("filling password input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Log in")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			log.Debug().Err(err).Msg("Got error from username/password submission")
			if strings.Contains(err.Error(), "Invalid username or password") {
				b.LastError = "Invalid username or password"
			} else if strings.Contains(err.Error(), "isn't connected to an account") {
				thing := "username"
				if strings.Contains("@", username) {
					thing = "email address"
				}
				b.LastError = fmt.Sprintf("That %s is not connected to a Messenger account", thing)
			} else if strings.Contains(err.Error(), "com.bloks.www.caa.assistive_login_confirmation") {
				// Facebook tries to send us to this screen when they think we are
				// demonstrating substantial incompetence at entering an email
				// address, like not putting a domain after the at-sign. It really
				// just means the email address isn't valid though so let's report
				// it like that.
				//
				// Technically we don't know that's the ONLY case where this screen
				// comes up, but it's the only one sighted thus far. Update this if
				// something new is discovered.
				b.LastError = "Invalid email address"
			} else {
				return nil, fmt.Errorf("tapping login button: %w", err)
			}
		}

	case StateCodeEntryPage:
		otpCode := userInput["otp_code"]
		if otpCode == "" {
			instructions := b.getCodeInstructions()
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}

			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.otp_code",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "otp_code", Name: "One-time code sent to you", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "otp_code")
		b.LastError = "Facebook rejected that code"

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Enter code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, otpCode)
		if err != nil {
			return nil, fmt.Errorf("filling otp code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			log.Debug().Err(err).Msg("Got error from OTP code submission")
			if strings.Contains(err.Error(), "Please re-enter") {
				// retry
			} else {
				return nil, fmt.Errorf("tapping continue: %w", err)
			}
		}

	case StateBackupCodePage:
		backupCode := userInput["backup_code"]
		if backupCode == "" {
			instructions := "Enter one of your two-factor backup codes."
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}

			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.backup_code",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "backup_code", Name: "Backup code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "backup_code")
		b.LastError = "Facebook rejected that code"

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, backupCode)
		if err != nil {
			return nil, fmt.Errorf("filling backup code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue: %w", err)
		}

	case StateCaptchaPage:
		captchaCode := userInput["captcha_code"]
		if captchaCode == "" {
			img := b.CurrentPage.FindDescendant(FilterByAttribute("bk.components.Image", "unique_id", "i:com.bloks.www.two_step_verification.enter_text_captcha_code/p:captcha_image"))
			if img == nil {
				return nil, fmt.Errorf("can't find captcha image")
			}
			imageURL := img.GetDynamicAttribute(ctx, b.CurrentPage.Interpreter, "url")
			if imageURL == "" {
				return nil, fmt.Errorf("captcha image has no url")
			}
			log.Trace().Str("image_url", imageURL).Msg("Found image captcha")

			audio := b.CurrentPage.FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "play audio"))
			if audio == nil {
				return nil, fmt.Errorf("can't find audio text")
			}
			clickable := audio.FindDescendant(FilterByComponent("bk.style.textspan.ClickableStyle"))
			if clickable == nil {
				return nil, fmt.Errorf("audio text is not clickable")
			}
			onClick := clickable.GetScript("on_click")
			if onClick == nil {
				return nil, fmt.Errorf("no on_click on audio text")
			}
			_, err := b.CurrentPage.Interpreter.Evaluate(ctx, &onClick.AST)
			if err != nil {
				return nil, fmt.Errorf("clicking on audio text: %w", err)
			}
			if b.DisplayedURL == "" {
				return nil, fmt.Errorf("clicking on audio text failed to open url")
			}
			audioURL := strings.Replace(b.DisplayedURL, "/player/", "/", 1)
			log.Trace().Str("audio_url", audioURL).Msg("Found audio captcha")

			imageResp, err := http.Get(imageURL)
			if err != nil {
				return nil, fmt.Errorf("error fetching image response: %w", err)
			}
			defer imageResp.Body.Close()
			imageBytes, err := io.ReadAll(imageResp.Body)
			if err != nil {
				return nil, fmt.Errorf("error reading image response body: %w", err)
			}
			imageMime := imageResp.Header.Get("content-type")
			if !strings.HasPrefix(imageMime, "image/") {
				return nil, fmt.Errorf("bad image captcha mime type %s", imageMime)
			}
			imageFilename := "captcha" + exmime.ExtensionFromMimetype(imageMime)

			audioResp, err := http.Get(audioURL)
			if err != nil {
				return nil, fmt.Errorf("error fetching audio response: %w", err)
			}
			defer audioResp.Body.Close()
			audioBytes, err := io.ReadAll(audioResp.Body)
			if err != nil {
				return nil, fmt.Errorf("error reading audio response body: %w", err)
			}
			audioMime := audioResp.Header.Get("content-type")
			if !strings.HasPrefix(audioMime, "audio/") {
				return nil, fmt.Errorf("bad audio captcha mime type %s", audioMime)
			}
			audioFilename := "captcha" + exmime.ExtensionFromMimetype(audioMime)

			var imageWidth, imageHeight int
			imageMeta, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
			if err == nil {
				imageWidth = imageMeta.Width
				imageHeight = imageMeta.Height
			}

			instructions := "Facebook requires solving a captcha"
			if b.LastError != "" {
				instructions = b.LastError
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.captcha",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Attachments: []*bridgev2.LoginUserInputAttachment{
						{
							Type:     event.MsgImage,
							FileName: imageFilename,
							Content:  imageBytes,
							Info: bridgev2.LoginUserInputAttachmentInfo{
								MimeType: imageMime,
								Width:    imageWidth,
								Height:   imageHeight,
								Size:     len(imageBytes),
							},
						},
						{
							Type:     event.MsgAudio,
							FileName: audioFilename,
							Content:  audioBytes,
							Info: bridgev2.LoginUserInputAttachmentInfo{
								MimeType: audioMime,
								Size:     len(audioBytes),
							},
						},
					},
					Fields: []bridgev2.LoginInputDataField{
						{ID: "captcha_code", Name: "Captcha code", Type: bridgev2.LoginInputFieldTypeCaptchaCode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "captcha_code")
		b.LastError = "Facebook rejected that captcha solution"

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Enter characters",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, captchaCode)
		if err != nil {
			return nil, fmt.Errorf("filling captcha code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			// The entrypoint_async RPC is sometimes rejected with a generic "server
			// error field_exception" response when transitioning from the captcha page
			// to the MFA landing page. The error can be reproduced on the official
			// Messenger iOS app so it is not a bridge issue. Entering the wrong captcha
			// produces a different, non-error response - so we know there is nothing
			// the user could do to cause this, it is purely Meta's fault.
			if strings.Contains(err.Error(), "Query Error") {
				return nil, ErrLoginUninformative("captcha submit query error")
			}
			// Sometimes just for spice, they will throw you a "Wrong Credentials" /
			// "Invalid username or password" error here, even though what you submitted
			// was a captcha rather than a username or password. And of course this
			// happens even if you gave the correct password. If you actually gave a
			// wrong password, it would have errored out at the password step, if we get
			// the same error here, it means the Zuck says no.
			if strings.Contains(err.Error(), "Invalid username or password") {
				return nil, ErrLoginUninformative("captcha submit invalid username/password")
			}
			// Another kind of lie that we can get from Facebook.
			if strings.Contains(err.Error(), "An unexpected error occurred") {
				return nil, ErrLoginUninformative("captcha submit unexpected error")
			}
			return nil, fmt.Errorf("tapping continue: %w", err)
		}

	case StateMFALandingPage:
		btn := b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Try another way")).
			FindContainingButton()
		// The scuffed AFAD page may also act as an MFA landing page instead, but we can't
		// tell until we see whether or not there is a button that would take us to the MFA
		// method selection page. If there is, we'll follow it like in the non-AP case,
		// otherwise we'll just treat this as a mandatory AFAD page.
		if btn == nil {
			b.State = StateAFADPage
			break
		}
		err := btn.TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping method selection button: %w", err)
		}

	case StateChooseMFAPage:
		foundMethods := map[string]*BloksTreeComponent{}
		methodNames := []string{}

		knownMethods := map[string]bool{
			"Notification on another device": true,
			"Authentication app":             true,
			"Email":                          true,
			"Text message":                   true,
			"Backup code":                    true,
			"WhatsApp":                       true,
			"Verify with Google":             false,
		}

		listItems := b.CurrentPage.FindDescendant(FilterByAttribute(
			"bk.data.TextSpan", "text", "Choose a way to confirm it’s you",
		)).
			FindAncestor(FilterByComponent("bk.components.Collection")).
			FindDescendant(FilterByAttribute("bk.components.BoxDecoration", "border_width", "1dp")).
			FindAncestor(FilterByComponent("bk.components.Flexbox")).
			GetChildren("children")

		for _, item := range listItems {
			span := item.
				FindDescendant(FilterByComponent("bk.components.RichText")).
				GetChildren("spans")[0].
				FindDescendant(FilterByComponent("bk.data.TextSpan"))
			method := span.GetAttribute("text")
			if !knownMethods[method] {
				log.Warn().Str("mfa_method", method).Msg("Ignoring unsupported MFA method")
				continue
			}
			foundMethods[method] = span
			methodNames = append(methodNames, method)
		}

		if len(foundMethods) == 0 {
			return nil, fmt.Errorf("couldn't find any allowed mfa types")
		}

		chosenMethod := userInput["mfatype"]
		if chosenMethod == "" && len(foundMethods) == 1 {
			chosenMethod = methodNames[0]
		}
		if chosenMethod == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.mfa_type",
				Instructions: "Choose how to finish signing in",
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{
							ID: "mfatype", Name: "Login method", Type: bridgev2.LoginInputFieldTypeSelect,
							Options: methodNames,
						},
					},
				},
			}
			break
		}

		if foundMethods[chosenMethod] == nil {
			return nil, fmt.Errorf("not a valid mfa method: %s", chosenMethod)
		}

		err := foundMethods[chosenMethod].
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping %q button: %w", chosenMethod, err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue button: %w", err)
		}

	case StateTOTPPage:
		totpCode := userInput["totp_code"]
		if totpCode == "" {
			instructions := "Enter a six-digit code from your authenticator app"
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.totp",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "totp_code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "totp_code")
		b.LastError = "Facebook rejected that code"

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, totpCode)
		if err != nil {
			return nil, fmt.Errorf("filling mfa code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue: %w", err)
		}

	case StateAFADPage:
		notif := b.CurrentPage.FindDescendant(func(comp *BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			for _, prefix := range []string{
				"We sent a notification",
				"Open the notification",
			} {
				if strings.HasPrefix(comp.GetAttribute("text"), prefix) {
					return true
				}
			}
			return false
		})
		if notif == nil {
			return nil, fmt.Errorf("couldn't find AFAD notification info")
		}
		b.AFADNotification = notif.GetAttribute("text")

		for _, comp := range b.CurrentPage.FindDescendants(func(comp *BloksTreeComponent) bool {
			if comp.ComponentID != "bk.components.VisibilityExtension" {
				return false
			}
			return comp.GetScript("on_appear") != nil
		}) {
			script := comp.GetScript("on_appear")
			_, err := b.CurrentPage.Interpreter.Evaluate(ctx, &script.AST)
			if err != nil {
				return nil, fmt.Errorf("on_appear: %w", err)
			}
		}

		if b.State != StateAFADPage {
			break
		}

		if b.AFADInterval <= 0 {
			return nil, fmt.Errorf("no AFAD timer scheduled")
		}

		// Only display the login step once, keep polling in background
		step = &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeDisplayAndWait,
			StepID:       "fi.mau.meta.messengerlite.afad_wait",
			Instructions: b.AFADNotification,
			DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
				Type: bridgev2.LoginDisplayTypeNothing,
			},
		}
		b.State = StateAFADPageWaiting

	case StateAFADPageWaiting:
		for b.State == StateAFADPageWaiting {
			time.Sleep(b.AFADInterval)
			err := b.AFADCallback()
			if err != nil {
				return nil, fmt.Errorf("AFAD callback: %w", err)
			}
		}

	case StateOAuthPage:
		return nil, fmt.Errorf("can't handle Google OAuth yet")

	case StateSMSPage:
		for _, mount := range b.CurrentPage.FindDescendants(FilterByComponent("bk.components.OnMount")) {
			script := mount.GetScript("on_first_mount")
			if script == nil {
				continue
			}
			_, err := b.CurrentPage.Interpreter.Evaluate(ctx, &script.AST)
			if err != nil {
				return nil, fmt.Errorf("sms on_mount script: %w", err)
			}
		}

		// Running the on_mount handlers should have triggered a code to be sent.
		b.State = StateSMSPageAfterSend

	case StateSMSPageAfterSend:
		smsCode := userInput["sms_code"]
		if smsCode == "" {
			instructions := b.getCodeInstructions()
			if instructions == "" {
				instructions = "Enter the SMS code sent to your phone number"
			}
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.sms",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "sms_code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "sms_code")
		b.LastError = "Facebook rejected that code"

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, smsCode)
		if err != nil {
			return nil, fmt.Errorf("filling sms code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue: %w", err)
		}

	case StateChooseNumberPage:
		buttons := b.CurrentPage.
			FindDescendants(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.AccessibilityExtension" {
					return false
				}
				if !strings.HasPrefix(comp.GetAttribute("label"), "+") {
					return false
				}
				return true
			})

		foundNumbers := map[string]*BloksTreeComponent{}
		numberNames := []string{}
		for _, btn := range buttons {
			number := btn.GetAttribute("label")
			foundNumbers[number] = btn
			numberNames = append(numberNames, number)
		}

		contactNumber := userInput["contact_number"]
		if contactNumber == "" && len(foundNumbers) == 1 {
			contactNumber = numberNames[0]
		}
		if contactNumber == "" {
			instructions := b.getContactNumberInstructions()
			if instructions == "" {
				instructions = "Choose the phone number to receive an MFA code"
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.choose_number",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{
							ID: "contact_number", Name: "Phone number", Type: bridgev2.LoginInputFieldTypeSelect,
							Options: numberNames,
						},
					},
				},
			}
			break
		}

		if foundNumbers[contactNumber] == nil {
			return nil, fmt.Errorf("not a valid contact number: %s", contactNumber)
		}

		err := foundNumbers[contactNumber].FindContainingButton().TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tap selected number: %w", err)
		}
		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue button: %w", err)
		}

	case StateWhatsAppPage:
		for _, mount := range b.CurrentPage.FindDescendants(FilterByComponent("bk.components.OnMount")) {
			script := mount.GetScript("on_first_mount")
			if script == nil {
				continue
			}
			_, err := b.CurrentPage.Interpreter.Evaluate(ctx, &script.AST)
			if err != nil {
				return nil, fmt.Errorf("whatsapp on_mount script: %w", err)
			}
		}

		// Running the on_mount handlers should have triggered a code to be sent.
		b.State = StateWhatsAppPageAfterSend

	case StateWhatsAppPageAfterSend:
		whatsAppCode := userInput["whatsapp_code"]
		if whatsAppCode == "" {
			instructions := b.getCodeInstructions()
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.whatsapp",
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "whatsapp_code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "whatsapp_code")
		b.LastError = "Facebook rejected that code"

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, whatsAppCode)
		if err != nil {
			return nil, fmt.Errorf("filling whatsapp code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue: %w", err)
		}

	default:
		return nil, fmt.Errorf("unexpected state %s", b.State)
	}
	if b.State == prevState {
		if step != nil {
			log.Debug().Str("cur_state", string(b.State)).Any("steps", step).Msg("Requested user input")
		} else if b.LastError != "" {
			log.Debug().Str("cur_state", string(b.State)).Str("last_error", b.LastError).Msg("Got intra-screen error, remaining in current state")
		} else {
			return nil, fmt.Errorf("handling %s failed to advance flow", prevState)
		}
	} else {
		log.Debug().Str("old_state", string(prevState)).Str("new_state", string(b.State)).Msg("Transitioned login step")

		// Ignore LastError, which is only used for signaling an error within the current
		// page and can be ignored once we move to a new page.
		b.LastError = ""
	}
	return step, nil
}
