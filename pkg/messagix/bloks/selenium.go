package bloks

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/random"
	"maunium.net/go/mautrix/bridgev2"
)

func (bb *BloksBundle) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	return bb.Layout.Payload.Tree.FindDescendant(pred)
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
		return comp.ComponentID == "bk.components.FoaTouchExtension"
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

func (button *BloksTreeComponent) TapButton(ctx context.Context, interp *Interpreter) error {
	if button == nil {
		return fmt.Errorf("no such button")
	}
	onTouchDown, ok := button.Attributes["on_touch_down"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("no on_touch_down script")
	}
	onTouchUp, ok := button.Attributes["on_touch_up"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("no on_touch_up script")
	}
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

type BrowserState string

// AFAD = Approve From Another Device
// TOTP = Time-Based One-Time Passcode
// MFA = Multi-Factor Authentication
const (
	StateUnknown                    BrowserState = ""
	StateInitial                                 = "initial"
	StateRedirectToLoginAction                   = "redirect-to-login-action"
	StateEmailPasswordPage                       = "enter-email-and-password-page"
	StateEnteredEmailPasswordAction              = "entered-email-and-password-action"
	StateEmailCodePage                           = "enter-code-from-email-page"
	StateMFALandingPage                          = "mfa-landing-page"
	StateChooseMFAPage                           = "choose-mfa-type-page"
	StateAFADPage                                = "afad-page"
	StateTOTPPage                                = "totp-page"
	StateEnteredTOTPAction                       = "entered-totp-action"
	StateSuccess                                 = "success"
)

type BrowserConfig struct {
	EncryptPassword  func(string) (string, error)
	MakeBloksRequest func(context.Context, *BloksRequestOuter) (*BloksBundle, error)
}

type Browser struct {
	State           BrowserState
	CurrentPage     *BloksBundle
	NewPageOrAction *BloksBundle

	Config *BrowserConfig
	Bridge *InterpBridge

	LoginData string
}

func NewBrowser(ctx context.Context, cfg *BrowserConfig) *Browser {
	log := zerolog.Ctx(ctx)
	b := Browser{
		State:  StateInitial,
		Config: cfg,
	}
	b.Bridge = &InterpBridge{
		DeviceID:        strings.ToUpper(uuid.New().String()),
		FamilyDeviceID:  strings.ToUpper(uuid.New().String()),
		MachineID:       string(random.StringBytes(25)),
		EncryptPassword: cfg.EncryptPassword,
		DoRPC: func(name string, params map[string]string) error {
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

			pageOrAction, err := cfg.MakeBloksRequest(ctx, NewBloksRequest(name, paramsInner))
			if err != nil {
				return fmt.Errorf("rpc %s: %w", name, err)
			}

			b.NewPageOrAction = pageOrAction
			b.State = transitions[b.State]
			return nil
		},
		DisplayNewScreen: func(name string, page *BloksBundle) error {
			log.Debug().Str("state", string(b.State)).Str("screen", name).Msg("Displaying new screen from Bloks")
			transitions := map[BrowserState]BrowserState{}
			switch name {
			case "com.bloks.www.caa.login.login_homepage":
				transitions[StateRedirectToLoginAction] = StateEmailPasswordPage
			case "com.bloks.www.caa.ar.code_entry":
				transitions[StateEnteredEmailPasswordAction] = StateMFALandingPage
			default:
				return fmt.Errorf("unexpected new screen %s", name)
			}
			if transitions[b.State] == StateUnknown {
				return fmt.Errorf("can't handle new screen %s in state %s", name, b.State)
			}

			b.NewPageOrAction = page
			b.State = transitions[b.State]
			return nil
		},
		HandleLoginResponse: func(data string) error {
			log.Debug().Str("state", string(b.State)).Msg("Handling login response from Bloks")
			transitions := map[BrowserState]BrowserState{}
			transitions[StateEnteredEmailPasswordAction] = StateSuccess
			transitions[StateEnteredTOTPAction] = StateSuccess
			if transitions[b.State] == StateUnknown {
				return fmt.Errorf("can't handle login response in state %s", b.State)
			}

			b.LoginData = data
			b.State = transitions[b.State]
			return nil
		},
	}
	return &b
}

func (b *Browser) DoLoginStep(ctx context.Context, userInput map[string]string) (*bridgev2.LoginStep, error) {
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
		action, err := b.Config.MakeBloksRequest(ctx, NewBloksRequest(rpc, map[string]any{
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

		b.NewPageOrAction = action
		b.State = StateRedirectToLoginAction
	case StateRedirectToLoginAction, StateEnteredEmailPasswordAction, StateEnteredTOTPAction:
		err := b.NewPageOrAction.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter())
		if err != nil {
			return nil, fmt.Errorf("setup %s interpreter: %w", b.State, err)
		}
		_, err = b.NewPageOrAction.Interpreter.Evaluate(ctx, b.NewPageOrAction.Action())
		if err != nil {
			return nil, fmt.Errorf("execute %s: %w", b.State, err)
		}
	case StateEmailPasswordPage:
		if userInput == nil {
			return &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.emailpassword",
				Instructions: "Enter your Messenger credentials",
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "username", Name: "Email address", Type: bridgev2.LoginInputFieldTypeEmail},
						{ID: "password", Name: "Password", Type: bridgev2.LoginInputFieldTypePassword},
					},
				},
			}, nil
		}

		err := b.NewPageOrAction.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter())
		if err != nil {
			return nil, err
		}
		b.CurrentPage = b.NewPageOrAction

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
	case StateMFALandingPage:
		err := b.NewPageOrAction.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter())
		if err != nil {
			return nil, err
		}
		b.CurrentPage = b.NewPageOrAction

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Try another way")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping method selection button: %w", err)
		}
	case StateTOTPPage:
		if userInput == nil {
			return &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       "fi.mau.meta.messengerlite.totp",
				Instructions: "Enter a six-digit code from your authenticator app",
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}, nil
		}

		err := b.NewPageOrAction.SetupInterpreter(ctx, b.Bridge, b.CurrentPage.GetInterpreter())
		if err != nil {
			return nil, err
		}
		b.CurrentPage = b.NewPageOrAction

		err = b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			}).
			FillInput(ctx, b.CurrentPage.Interpreter, userInput["code"])
		if err != nil {
			return nil, fmt.Errorf("filling mfa code input: %w", err)
		}

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping login button: %w", err)
		}
	default:
		return nil, fmt.Errorf("unexpected state %s", b.State)
	}
	if b.State == prevState {
		return nil, fmt.Errorf("handling %s failed to advance flow", prevState)
	}
	log.Debug().Str("old_state", string(prevState)).Str("new_state", string(b.State)).Msg("Transitioned login step")
	return nil, nil
}
