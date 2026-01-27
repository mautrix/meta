package bloks

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/random"
	"maunium.net/go/mautrix/bridgev2"
)

func (bb *BloksBundle) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	return bb.Layout.Payload.Tree.FindDescendant(pred)
}

func (bb *BloksBundle) FindDescendants(pred func(*BloksTreeComponent) bool) []*BloksTreeComponent {
	return bb.Layout.Payload.Tree.FindDescendants(pred)
}

func (btn *BloksTreeNode) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
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

func FilterByAttribute(compid BloksComponentID, attr string, value string) func(comp *BloksTreeComponent) bool {
	return func(comp *BloksTreeComponent) bool {
		if comp.ComponentID != compid {
			return false
		}
		return comp.GetAttribute(attr) == value
	}
}

func (comp *BloksTreeComponent) GetAttribute(name string) string {
	attr := comp.Attributes[BloksAttributeID(name)]
	if attr == nil {
		return ""
	}
	value, ok := attr.BloksTreeNodeContent.(*BloksTreeLiteral)
	if !ok {
		return ""
	}
	str, ok := value.BloksJavascriptValue.(string)
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
const (
	StateUnknown                    BrowserState = ""
	StateInitial                    BrowserState = "initial"
	StateRedirectToLoginAction      BrowserState = "redirect-to-login-action"
	StateEmailPasswordPage          BrowserState = "enter-email-and-password-page"
	StateEnteredEmailPasswordAction BrowserState = "entered-email-and-password-action"
	StateEmailCodePage              BrowserState = "enter-code-from-email-page"
	StateMFALandingPage             BrowserState = "mfa-landing-page"
	StateChooseMFAPage              BrowserState = "choose-mfa-type-page"
	StateChosenMFAAction            BrowserState = "chosen-mfa-type-action"
	StateAFADPage                   BrowserState = "afad-page"
	StateAFADAction                 BrowserState = "afad-action"
	StateAFADWait                   BrowserState = "afad-waiting"
	StateAFADCompleteAction         BrowserState = "afad-complete-action"
	StateTOTPPage                   BrowserState = "totp-page"
	StateEnteredTOTPAction          BrowserState = "entered-totp-action"
	StateSuccess                    BrowserState = "success"
)

type BrowserConfig struct {
	EncryptPassword  func(string) (string, error)
	MakeBloksRequest func(context.Context, *BloksDoc, *BloksRequestOuter) (*BloksBundle, error)
}

type Browser struct {
	State         BrowserState
	CurrentPage   *BloksBundle
	CurrentAction *BloksBundle

	Config *BrowserConfig
	Bridge *InterpBridge

	AFADNotification string
	AFADInterval     time.Duration
	AFADCallback     func(*BloksScriptLiteral) error
	AFADDisplayed    bool
	LoginData        string
}

func NewBrowser(ctx context.Context, cfg *BrowserConfig) *Browser {
	log := zerolog.Ctx(ctx)
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
		DoRPC: func(name string, params map[string]string, isPage bool, callback func(result *BloksScriptLiteral) error) error {
			log.Debug().Str("state", string(b.State)).Str("rpc", name).Msg("Invoking RPC from Bloks")
			transitions := map[BrowserState]BrowserState{}
			switch name {
			case "com.bloks.www.bloks.caa.login.async.send_login_request":
				transitions[StateEmailPasswordPage] = StateEnteredEmailPasswordAction
			case "com.bloks.www.two_step_verification.entrypoint":
				transitions[StateEnteredEmailPasswordAction] = StateMFALandingPage
			case "com.bloks.www.two_step_verification.verify_code.async":
				transitions[StateTOTPPage] = StateEnteredTOTPAction
			case "com.bloks.www.two_step_verification.method_picker":
				transitions[StateMFALandingPage] = StateChooseMFAPage
			case "com.bloks.www.two_step_verification.method_picker.navigation.async":
				transitions[StateChooseMFAPage] = StateChosenMFAAction
			case "com.bloks.www.two_factor_login.enter_totp_code":
				transitions[StateChosenMFAAction] = StateTOTPPage
			case "com.bloks.www.two_step_verification.approve_from_another_device":
				transitions[StateChosenMFAAction] = StateAFADPage
			case "com.bloks.www.two_step_verification.afad_state.async":
				transitions[StateAFADPage] = StateAFADAction
			case "com.bloks.www.two_step_verification.afad_complete.async":
				transitions[StateAFADAction] = StateAFADCompleteAction
			default:
				return fmt.Errorf("unexpected rpc %s", name)
			}
			if transitions[b.State] == StateUnknown {
				return fmt.Errorf("can't handle rpc %s in state %s", name, b.State)
			}

			var paramsInner BloksParamsInner
			err := json.Unmarshal([]byte(params["params"]), &paramsInner)
			if err != nil {
				return fmt.Errorf("parsing %s params: %w", name, err)
			}

			var doc *BloksDoc
			if isPage {
				doc = &BloksAppDoc
			} else {
				doc = &BloksActionDoc
			}

			pageOrAction, err := cfg.MakeBloksRequest(ctx, doc, NewBloksRequest(name, paramsInner))
			if err != nil {
				return fmt.Errorf("rpc %s: %w", name, err)
			}

			err = pageOrAction.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter())
			if err != nil {
				return err
			}

			if isPage {
				b.CurrentPage = pageOrAction
			} else {
				b.CurrentAction = pageOrAction
			}

			b.State = transitions[b.State]

			if b.State == StateAFADAction {
				b.AFADCallback = callback
			}

			return nil
		},
		DisplayNewScreen: func(name string, page *BloksBundle) error {
			log.Debug().Str("state", string(b.State)).Str("screen", name).Msg("Displaying new screen from Bloks")
			transitions := map[BrowserState]BrowserState{}
			switch name {
			case "com.bloks.www.caa.login.login_homepage":
				transitions[StateRedirectToLoginAction] = StateEmailPasswordPage
			case "com.bloks.www.caa.ar.code_entry":
				transitions[StateEnteredEmailPasswordAction] = StateEmailCodePage
			default:
				return fmt.Errorf("unexpected new screen %s", name)
			}
			if transitions[b.State] == StateUnknown {
				return fmt.Errorf("can't handle new screen %s in state %s", name, b.State)
			}

			err := page.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter())
			if err != nil {
				return err
			}

			b.CurrentPage = page
			b.State = transitions[b.State]
			return nil
		},
		HandleLoginResponse: func(data string) error {
			log.Debug().Str("state", string(b.State)).Msg("Handling login response from Bloks")
			transitions := map[BrowserState]BrowserState{}
			transitions[StateEnteredEmailPasswordAction] = StateSuccess
			transitions[StateEnteredTOTPAction] = StateSuccess
			transitions[StateAFADCompleteAction] = StateSuccess
			if transitions[b.State] == StateUnknown {
				return fmt.Errorf("can't handle login response in state %s", b.State)
			}

			b.LoginData = data
			b.State = transitions[b.State]
			return nil
		},
		StartTimer: func(name string, interval time.Duration, callback func() error) error {
			switch name {
			case "approve_from_another_device_polling_timer":
				// The callback just re-runs the same on_appear logic, so for now
				// we'll just re-load the page instead of actually triggering the
				// callback logic in a loop.
				b.AFADInterval = interval
			default:
				return fmt.Errorf("unexpected timer %s", name)
			}
			return nil
		},
	}
	return &b
}

var definitelyNotPhoneNumberRegexp = regexp.MustCompile(`^.*[@a-zA-Z].*$`)

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

		err = action.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter())
		if err != nil {
			return nil, fmt.Errorf("setup %s interpreter: %w", b.State, err)
		}

		b.CurrentAction = action
		b.State = StateRedirectToLoginAction
	case StateRedirectToLoginAction, StateEnteredEmailPasswordAction, StateChosenMFAAction, StateEnteredTOTPAction, StateAFADAction, StateAFADCompleteAction:
		result, err := b.CurrentAction.Interpreter.Evaluate(ctx, b.CurrentAction.Action())
		if err != nil {
			return nil, fmt.Errorf("execute %s: %w", b.State, err)
		}

		// Most actions are done by now, however in the case of AFAD, we do some special
		// handling to invoke its callback. That callback will trigger finalizing the flow,
		// or it will do nothing in which case we should wait and try again later at the
		// timer interval.

		if b.State != StateAFADAction {
			break
		}

		err = b.AFADCallback(result)
		if err != nil {
			return nil, fmt.Errorf("execute AFAD callback: %w", err)
		}

		if b.State != StateAFADAction {
			break
		}

		if b.AFADInterval <= 0 {
			return nil, fmt.Errorf("no AFAD timer scheduled")
		}

		b.State = StateAFADWait

		// Only display the login step once, keep polling in background
		if !b.AFADDisplayed {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeDisplayAndWait,
				StepID:       "fi.mau.meta.messengerlite.afad_wait",
				Instructions: b.AFADNotification,
				DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
					Type: bridgev2.LoginDisplayTypeNothing,
				},
			}
			b.AFADDisplayed = true
		}
	case StateEmailPasswordPage:
		if userInput["username"] == "" || userInput["password"] == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.email_password",
				Instructions: "Enter your Messenger credentials",
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "username", Name: "Username or email address", Type: bridgev2.LoginInputFieldTypeUsername},
						{ID: "password", Name: "Password", Type: bridgev2.LoginInputFieldTypePassword},
					},
				},
			}
			break
		}

		if !definitelyNotPhoneNumberRegexp.MatchString(userInput["username"]) {
			return nil, fmt.Errorf("only username or email login is allowed, not phone number")
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.components.TextInput", "html_name", "email")).
			FillInput(ctx, b.CurrentPage.Interpreter, userInput["username"])
		if err != nil {
			return nil, fmt.Errorf("filling email input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.components.TextInput", "html_name", "password")).
			FillInput(ctx, b.CurrentPage.Interpreter, userInput["password"])
		if err != nil {
			return nil, fmt.Errorf("filling password input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Log in")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping login button: %w", err)
		}
	case StateEmailCodePage:
		// XXX this entire switch case is completely blind guesswork since I don't have a
		// network trace of what the page actually looks like when you trigger the email
		// code fallback, but if we're lucky this would work on first try
		if userInput["email_code"] == "" {
			step = &bridgev2.LoginStep{
				Type:   bridgev2.LoginStepTypeUserInput,
				StepID: "fi.mau.meta.messengerlite.email_code",
				// TODO look up the message that Facebook displays on the page, and
				// show that as the instructions
				Instructions: "Enter the six-digit code sent to your email",
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "email_code", Name: "Code from email", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, userInput["email_code"])
		if err != nil {
			return nil, fmt.Errorf("filling email code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue: %w", err)
		}
	case StateMFALandingPage:
		err := b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Try another way")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping method selection button: %w", err)
		}
	case StateChooseMFAPage:
		possibleMethods := []string{
			"Authentication app",
			"Notification on another device",
		}

		foundMethods := map[string]*BloksTreeComponent{}
		for _, methodName := range possibleMethods {
			elem := b.CurrentPage.FindDescendant(FilterByAttribute(
				"bk.data.TextSpan", "text", methodName,
			))
			if elem != nil {
				foundMethods[methodName] = elem
			}
		}

		if len(foundMethods) == 0 {
			return nil, fmt.Errorf("couldn't find any allowed mfa types")
		}

		filteredMethods := []string{}
		for _, method := range possibleMethods {
			if foundMethods[method] != nil {
				filteredMethods = append(filteredMethods, method)
			}
		}

		chosenMethod := userInput["mfatype"]
		if chosenMethod == "" && len(filteredMethods) == 1 {
			chosenMethod = filteredMethods[0]
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
							Options: filteredMethods,
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
		if userInput["totp_code"] == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.totp",
				Instructions: "Enter a six-digit code from your authenticator app",
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "totp_code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		err := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, userInput["totp_code"])
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
			return strings.HasPrefix(comp.GetAttribute("text"), "We sent a notification")
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
	case StateAFADWait:
		time.Sleep(b.AFADInterval)
		b.State = StateAFADPage
	default:
		return nil, fmt.Errorf("unexpected state %s", b.State)
	}
	if b.State == prevState {
		if step == nil {
			return nil, fmt.Errorf("handling %s failed to advance flow", prevState)
		} else {
			log.Debug().Str("cur_state", string(b.State)).Any("steps", step).Msg("Requested user input")
		}
	} else {
		log.Debug().Str("old_state", string(prevState)).Str("new_state", string(b.State)).Msg("Transitioned login step")
	}
	return step, nil
}
