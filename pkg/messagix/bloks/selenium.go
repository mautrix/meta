package bloks

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/random"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func (bb *BloksBundle) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	return bb.Layout.Payload.Tree.FindDescendant(pred)
}

func (bb *BloksBundle) FindDescendantIncludingEmbedded(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	res := bb.Layout.Payload.Tree.FindDescendant(pred)
	if res != nil {
		return res
	}
	for _, embedded := range bb.Layout.Payload.Embedded {
		res := embedded.Contents.FindDescendant(pred)
		if res != nil {
			return res
		}
	}
	return nil
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
	if comp == nil {
		return nil
	}
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
	if comp == nil {
		return nil
	}
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
	StateUnknown                BrowserState = ""
	StateTestCaptcha            BrowserState = "test-captcha"
	StateInitialMessenger       BrowserState = "initial-messenger"
	StateInitialInstagram       BrowserState = "initial-instagram"
	StateLandingPage            BrowserState = "landing-page"
	StateEmailPasswordPage      BrowserState = "enter-email-and-password-page"
	StateAuthenticationConfirm  BrowserState = "authentication-confirmation-page"
	StateDialog                 BrowserState = "dialog"
	StateAccountSelectionPage   BrowserState = "account-selection-page"
	StateAccountRecoveryPage    BrowserState = "account-recovery-page"
	StatePasswordFormPage       BrowserState = "password-form-page"
	StateCodeEntryPage          BrowserState = "enter-code-page"
	StateCaptchaPage            BrowserState = "captcha-page"
	StateReCaptchaPage          BrowserState = "recaptcha-page"
	StateMFALandingPage         BrowserState = "mfa-landing-page"
	StateChooseMFAPage          BrowserState = "choose-mfa-type-page"
	StateAFADPage               BrowserState = "afad-page"
	StateAFADPageWaiting        BrowserState = "afad-waiting"
	StateTOTPPage               BrowserState = "totp-page"
	StateOAuthPage              BrowserState = "oauth-page"
	StateSMSPage                BrowserState = "sms-page"
	StateSMSPageAfterSend       BrowserState = "sms-page-after-send"
	StateBackupCodePage         BrowserState = "backup-code-page"
	StateChooseContactPointPage BrowserState = "choose-contact-point-page"
	StateWhatsAppPage           BrowserState = "whatsapp-page"
	StateWhatsAppPageAfterSend  BrowserState = "whatsapp-page-after-send"
	StatePasskeyPage            BrowserState = "passkey"
	StateSilentCaptchaPage      BrowserState = "noop-captcha"
	StateSuggestedAccountPage   BrowserState = "suggested-account-page"
	StateSuccess                BrowserState = "success"
)

type BrowserConfig struct {
	Platform         types.Platform
	EncryptPassword  func(context.Context, string) (string, error)
	MakeBloksRequest func(context.Context, *BloksDoc, string, BloksParamsInner, string, string) (*BloksBundle, error)
	FetchAsset       func(ctx context.Context, url string) ([]byte, string, error)
}

type Browser struct {
	State         BrowserState
	PreviousState BrowserState

	CurrentPage       *BloksBundle
	PreviousPage      *BloksBundle
	PreviousPageState BrowserState

	Config  *BrowserConfig
	Bridge  *InterpBridge
	profile browserPlatformProfile

	AFADNotification string
	AFADInterval     time.Duration
	AFADCallback     func() error
	MFACanGoBack     bool

	LoginData    string
	DisplayedURL string

	PendingDialog       *BloksDialog
	DialogPreviousState BrowserState

	LastError           string
	PageTransitionCount uint64
	ActionRPCCount      uint64
}

func (b *Browser) uninformativeLoginError(callsite string) bridgev2.RespError {
	return loginerrors.Uninformative(b.profile.uninformativeMessage, callsite)
}

var instagramLoginSafeDiagnosticPattern = regexp.MustCompile(
	`(?i)(unimplemented function|unexpected new screen|can't handle new screen)\s+([a-z0-9_.-]+)`,
)

func instagramLoginSubmissionErrorDiagnostic(err error) (kind, detail string) {
	if err == nil {
		return "none", ""
	}
	var checkpointErr CheckpointError
	if errors.As(err, &checkpointErr) {
		return "server_rejection", ""
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "Invalid username or password"):
		return "invalid_credentials", ""
	case strings.Contains(message, "isn’t connected to an account") ||
		strings.Contains(message, "isn't connected to an account"):
		return "unlinked_identifier", ""
	case strings.Contains(message, "com.bloks.www.caa.assistive_login_confirmation"):
		return "invalid_identifier", ""
	case strings.Contains(message, "unexpected HTTP status 429"):
		return "rate_limited", ""
	}
	match := instagramLoginSafeDiagnosticPattern.FindStringSubmatch(message)
	if len(match) == 3 {
		switch strings.ToLower(match[1]) {
		case "unimplemented function":
			return "unimplemented_function", match[2]
		case "unexpected new screen", "can't handle new screen":
			return "unexpected_screen", match[2]
		}
	}
	return "other", ""
}

func (b *Browser) stepID(name string) string {
	return b.profile.stepIDPrefix + "." + name
}

func (b *Browser) rejectedCodeMessage() string {
	return b.profile.serviceName + " rejected that code"
}

func (b *Browser) codeNotSentMessage() string {
	return "That code was not sent to " + b.profile.serviceName + "; please try again"
}

func collectPendingVariableIDs(node *BloksScriptNode, ids map[BloksVariableID]struct{}) {
	if node == nil {
		return
	}
	call, ok := node.Content.(*BloksScriptFuncall)
	if !ok {
		return
	}
	switch call.Function {
	case "bk.action.bloks.GetVariable2", "bk.action.bloks.GetVariableWithScope":
		if len(call.Args) > 0 {
			literal, ok := call.Args[0].Content.(*BloksScriptLiteral)
			if ok {
				name, ok := literal.Value().(string)
				if ok && strings.Contains(strings.ToLower(name), "pending") {
					ids[BloksVariableID(name)] = struct{}{}
				}
			}
		}
	}
	for idx := range call.Args {
		collectPendingVariableIDs(&call.Args[idx], ids)
	}
}

func resetPendingCodeSubmissionFlags(button *BloksTreeComponent, interp *Interpreter) int {
	if button == nil || interp == nil {
		return 0
	}
	ids := map[BloksVariableID]struct{}{}
	for _, name := range []BloksAttributeID{"on_click", "on_touch_down", "on_touch_up"} {
		script := button.GetScript(name)
		if script != nil {
			collectPendingVariableIDs(&script.AST, ids)
		}
	}
	reset := 0
	for id := range ids {
		if _, ok := interp.LocalVars[id]; ok {
			interp.LocalVars[id] = BloksLiteralOf(false)
			reset++
		}
		if _, ok := interp.GlobalVars[id]; ok {
			interp.GlobalVars[id] = BloksLiteralOf(false)
			reset++
		}
	}
	return reset
}

var genericDeviceNetworkInfo = map[string]any{
	"active_subscriptions_info": nil,
	"default_subscription_info": map[string]any{
		"network_type":           18,
		"is_data_roaming":        1,
		"is_esim":                nil,
		"is_gsm_roaming":         0,
		"is_sim_sms_capable":     nil,
		"is_mobile_data_enabled": 0,
		"sim_carrier_id":         2578,
		"sim_carrier_id_name":    "Tello",
		"sim_state":              5,
		"sim_operator":           "310240",
		"sim_operator_name":      "Tello",
		"signal_strength":        2,
		"group_id_level_1":       nil,
		"network_operator":       "310260",
	},
	"is_airplane_mode":           0,
	"is_active_network_cellular": 0,
	"is_device_sms_capable":      1,
	"sim_count":                  2,
	"is_wifi":                    1,
}

func (b *Browser) initialLoginParams() (BloksParamsInner, error) {
	params := BloksParamsInner{
		"account_list":           []any{},
		"blocked_uid":            []any{},
		"device_id":              b.Bridge.DeviceID,
		"disable_auto_login":     false,
		"family_device_id":       b.Bridge.FamilyDeviceID,
		"show_internal_settings": false,
		"waterfall_id":           hex.EncodeToString(random.Bytes(16)),
	}
	switch b.Config.Platform {
	case types.MessengerLiteIOS:
		params["auto_login_interstitial_experiment_group"] = ""
		params["disable_recursive_auto_login_interstitial"] = true
		params["is_from_logged_in_switcher"] = false
		params["layered_homepage_experiment_group"] = "not_in_experiment"
		params["machine_id"] = b.Bridge.MachineID
		params["offline_experiment_group"] = "caa_iteration_v2_perf_ls_ios_test_1"
		params["use_auto_login_interstitial"] = true
	case types.MessengerLiteAndroid:
		params["INTERNAL_INFRA_THEME"] = "THREE_NEUTRAL_GRAY"
		params["device_emails"] = []any{}
		params["offline_experiment_group"] = "caa_iteration_v3_perf_msg_6"
		params["openid_tokens"] = map[string]any{}
		params["spectra_guardian_token"] = ""
	default:
		return nil, fmt.Errorf("no initial bloks params for platform %s", b.Config.Platform.String())
	}
	return params, nil
}

func (b *Browser) instagramDirectLoginParams() BloksParamsInner {
	return BloksParamsInner{
		"device_id":                  b.Bridge.AndroidDeviceID,
		"disable_auto_login":         false,
		"family_device_id":           b.Bridge.FamilyDeviceID,
		"is_caa_perf_enabled":        true,
		"is_from_logged_in_switcher": false,
		"is_from_logged_out":         true,
		"last_auto_login_time":       int64(0),
		"logged_out_user":            "",
		"logout_source":              "",
		"offline_experiment_group":   "caa_iteration_v3_perf_ig_4",
		"qe_device_id":               b.Bridge.DeviceID,
		"qpl_join_id":                nil,
		"show_internal_settings":     false,
		"waterfall_id":               uuid.NewString(),
	}
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
// The state starts off in the platform-specific initial state, which kicks things off by making a Bloks request manually
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

func NewBrowser(cfg *BrowserConfig) (*Browser, error) {
	profile := browserProfile(cfg.Platform)
	b := Browser{
		State:   profile.initialState,
		Config:  cfg,
		profile: profile,
	}
	attestationKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate attestation key: %w", err)
	}
	attestationPublicKey, err := x509.MarshalPKIXPublicKey(&attestationKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("get attestation public key: %w", err)
	}
	attestationKeyHash := sha256.Sum256(attestationPublicKey)
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
		MachineID:         "",
		DeviceNetworkInfo: genericDeviceNetworkInfo,
		EncryptPassword:   cfg.EncryptPassword,
		SignRequestData: func(ctx context.Context, data any) (any, error) {
			payload, err := json.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("marshal request data: %w", err)
			}
			hash := sha256.Sum256(payload)
			sig, err := ecdsa.SignASN1(rand.Reader, attestationKey, hash[:])
			if err != nil {
				return nil, fmt.Errorf("sign request data: %w", err)
			}
			return map[string]any{
				"keyHash":   hex.EncodeToString(attestationKeyHash[:]),
				"data":      base64.StdEncoding.EncodeToString(payload),
				"signature": base64.StdEncoding.EncodeToString(sig),
			}, nil
		},
		DoPageRPC: func(ctx context.Context, name string, params map[string]string) (*BloksBundle, error) {
			log := zerolog.Ctx(ctx)
			log.Debug().Str("state", string(b.State)).Str("rpc", name).Str("rpc_type", "page").Msg("Invoking RPC from Bloks")
			var paramsInner BloksParamsInner
			err := json.Unmarshal([]byte(params["params"]), &paramsInner)
			if err != nil {
				return nil, fmt.Errorf("parsing %s params: %w", name, err)
			}
			appDoc, err := GetBloksAppDoc(cfg.Platform)
			if err != nil {
				return nil, fmt.Errorf("rpc %s: %w", name, err)
			}
			bundle, err := cfg.MakeBloksRequest(ctx, appDoc, name, paramsInner, b.Bridge.DeviceID, b.Bridge.FamilyDeviceID)
			if err != nil {
				return nil, fmt.Errorf("rpc %s: %w", name, err)
			}
			return bundle, nil
		},
		DoActionRPC: func(ctx context.Context, name string, params map[string]string) (*BloksScriptNode, error) {
			b.ActionRPCCount++
			log := zerolog.Ctx(ctx)
			log.Debug().Str("state", string(b.State)).Str("rpc", name).Str("rpc_type", "action").Msg("Invoking RPC from Bloks")
			var paramsInner BloksParamsInner
			err := json.Unmarshal([]byte(params["params"]), &paramsInner)
			if err != nil {
				return nil, fmt.Errorf("parsing %s params: %w", name, err)
			}
			actionDoc, err := GetBloksActionDoc(cfg.Platform)
			if err != nil {
				return nil, fmt.Errorf("rpc %s: %w", name, err)
			}
			bundle, err := cfg.MakeBloksRequest(ctx, actionDoc, name, paramsInner, b.Bridge.DeviceID, b.Bridge.FamilyDeviceID)
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
				return nil, fmt.Errorf("merging interpreter with new action: %w", err)
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
			case "com.bloks.www.caa.login.landing_screen":
				newState = StateLandingPage
			case "com.bloks.www.caa.ar.authentication_confirmation":
				newState = authenticationConfirmationPageState(page)
			case "com.bloks.www.caa.ar.select_account",
				"com.bloks.www.caa.login.aymh_multiple_profiles_screen_entry":
				newState = StateAccountSelectionPage
			case "com.bloks.www.caa.login.aymh_single_profile_screen_entry":
				newState = StateAuthenticationConfirm
			case "com.bloks.www.caa.ar.code_entry":
				newState = b.profile.caaCodeEntryState
			case "com.bloks.www.ap.two_step_verification.code_entry":
				newState = StateCodeEntryPage
			case "com.bloks.www.two_step_verification.entrypoint":
				newState = b.profile.twoStepEntrypointState(page)
				b.profile.logTwoStepEntrypoint(log, page, newState)
			case "com.bloks.www.two_step_verification.enter_text_captcha_code",
				"com.bloks.www.caa.ar.sms_captcha":
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
			case "com.bloks.www.ap.two_step_verification.limbo_proactive":
				newState = StateAFADPage
			case "com.bloks.www.ap.two_step_verification.challenge_picker",
				"com.bloks.www.two_step_verification.method_picker",
				"com.bloks.www.caa.ar.initiate_view":
				newState = StateChooseMFAPage
			case "com.bloks.www.caa.ar.auth_method":
				newState = b.profile.authMethodState
			case "com.bloks.www.ap.two_step_verification.google_oauth":
				newState = StateMFALandingPage
			case "com.bloks.www.two_factor_login.enter_totp_code",
				"com.bloks.www.two_step_verification.enter_totp_code",
				"com.bloks.www.ap.two_step_verification.enter_totp_code":
				newState = StateTOTPPage
			case "com.bloks.www.ap.two_step_verification.login_with_third_party":
				newState = StateOAuthPage
			case "com.bloks.www.two_step_verification.enter_sms_code":
				newState = StateSMSPage
			case "com.bloks.www.two_factor_login.enter_backup_code",
				"com.bloks.www.two_step_verification.enter_backup_code",
				"com.bloks.www.ap.two_step_verification.enter_backup_code":
				newState = StateBackupCodePage
			case "com.bloks.www.ap.two_step_verification.contactpoint_chooser",
				"com.bloks.www.two_step_verification.contactpoint_chooser":
				newState = StateChooseContactPointPage
			case "com.bloks.www.approve_from_another_device.xmds.challenged_device_denied":
				return loginerrors.AFADStopped
			case "com.bloks.www.two_step_verification.enter_whatsapp_code":
				newState = StateWhatsAppPage
			case "com.bloks.www.ap.passkey_auth":
				newState = StatePasskeyPage
			case "com.bloks.www.two_step_verification.no_op_captcha":
				newState = StateSilentCaptchaPage
			case "com.bloks.www.two_step_verification.google_recaptcha":
				newState = StateReCaptchaPage
			case "com.bloks.www.caa.login.password_as_id_confirmation":
				newState = StateSuggestedAccountPage
			case "com.bloks.www.caa.ar.password_form":
				newState = StatePasswordFormPage
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

			b.PreviousPage = b.CurrentPage
			b.PreviousPageState = b.State
			b.CurrentPage = page
			b.State = newState
			b.PageTransitionCount += 1
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
			case "notif_delivery_status_polling":
				if !b.profile.supportsNotificationTimer {
					return fmt.Errorf("unexpected timer %s", name)
				}
				// Instagram schedules this after sending a phone login
				// notification. It is delivery-status polling, not the
				// authentication continuation itself, so there is no work for
				// the bridge to schedule.
			default:
				return fmt.Errorf("unexpected timer %s", name)
			}
			return nil
		},
		CancelTimer: func(name string) error {
			switch name {
			case "approve_from_another_device_polling_timer":
				b.AFADInterval = 0
				b.AFADCallback = nil
			case "notif_delivery_status_polling":
				if !b.profile.supportsNotificationTimer {
					return fmt.Errorf("unexpected timer cancel %s", name)
				}
			default:
				return fmt.Errorf("unexpected timer cancel %s", name)
			}
			return nil
		},
		OpenURL: func(url string) error {
			b.DisplayedURL = url
			return nil
		},
		OpenDialog: func(ctx context.Context, dialog *BloksDialog) error {
			if dialog == nil {
				return fmt.Errorf("can't open a nil dialog")
			}
			if b.PendingDialog != nil {
				return fmt.Errorf("can't open a second dialog while one is pending")
			}
			zerolog.Ctx(ctx).Debug().
				Str("state", string(b.State)).
				Int("button_count", len(dialog.Buttons)).
				Msg("Displaying native Bloks dialog")
			b.DialogPreviousState = b.State
			b.PendingDialog = dialog
			b.State = StateDialog
			return nil
		},
		PopScreen: func(ctx context.Context, style string) error {
			if b.PreviousPage == nil || b.PreviousPageState == StateUnknown {
				return fmt.Errorf("can't pop screen without page history")
			}
			zerolog.Ctx(ctx).Debug().
				Str("state", string(b.State)).
				Str("style", style).
				Msg("Popping native Bloks screen")
			b.CurrentPage, b.PreviousPage = b.PreviousPage, b.CurrentPage
			b.State, b.PreviousPageState = b.PreviousPageState, b.State
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
				if msg == "" {
					return nil
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
				// Sometimes Facebook will set an empty string to this variable,
				// resetting it, just before making a request that might fail.
				// Except, we already take care of resetting LastError before we
				// trigger Bloks code, and we might have already set up LastError
				// with a default value that we don't want to have reset. So,
				// require a non-empty string.
				if msg == "" {
					return nil
				}
				b.LastError = msg
			}
			return nil
		},
	}
	return &b, nil
}

var definitelyNotPhoneNumberRegexp = regexp.MustCompile(`^.*[@a-zA-Z].*$`)

var mfaPickerMethods = map[string]bool{
	"Notification on another device": true,
	"Authentication app":             true,
	"Email":                          true,
	"Text message":                   true,
	"Backup code":                    true,
	"WhatsApp":                       true,
	"Verify with Google":             false,
}

// caa.ar.initiate_view renders the same picker with its own labels, and puts a
// profile photo behind the same 1dp border as the rows.
var initiateViewMethods = map[string]string{
	"Get code or link via WhatsApp": "WhatsApp",
	"Get code via email":            "Email",
	"Get code via SMS":              "Text message",
	"Enter password to log in":      "Re-enter password",
	"Log into another account":      "",
}

// FindMFAMethods returns the supported methods on an MFA picker page keyed by
// canonical name, the names in display order, and how many recognised but
// unsupported methods were skipped.
func (bb *BloksBundle) FindMFAMethods(log *zerolog.Logger) (map[string]*BloksTreeComponent, []string, int) {
	if bb.FindDescendant(func(comp *BloksTreeComponent) bool {
		if comp.ComponentID != "bk.data.TextSpan" {
			return false
		}
		return strings.HasPrefix(comp.GetAttribute("text"), "Your session expired")
	}) != nil {
		log.Error().Msg("Got session expired warning on MFA screen")
	}

	foundMethods := map[string]*BloksTreeComponent{}
	methodNames := []string{}
	numIgnored := 0

	if bb.FindDescendant(func(comp *BloksTreeComponent) bool {
		if comp.ComponentID != "bk.data.TextSpan" {
			return false
		}
		return strings.HasPrefix(comp.GetAttribute("text"), "Choose a way to log in")
	}) != nil {
		for _, span := range bb.FindDescendants(FilterByComponent("bk.data.TextSpan")) {
			label := span.GetAttribute("text")
			method, isMethod := initiateViewMethods[label]
			if !isMethod {
				continue
			} else if method == "" {
				log.Warn().Str("mfa_method", label).Msg("Ignoring unsupported MFA method")
				numIgnored += 1
				continue
			} else if foundMethods[method] != nil {
				continue
			}
			foundMethods[method] = span
			methodNames = append(methodNames, method)
		}
		return foundMethods, methodNames, numIgnored
	}

	listItems := bb.FindDescendant(FilterByAttribute(
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
		if !mfaPickerMethods[method] {
			log.Warn().Str("mfa_method", method).Msg("Ignoring unsupported MFA method")
			numIgnored += 1
			continue
		}
		foundMethods[method] = span
		methodNames = append(methodNames, method)
	}
	return foundMethods, methodNames, numIgnored
}

func (b *Browser) getCodeInstructions() string {
	return b.profile.codeInstructions(b.CurrentPage)
}

func (b *Browser) getContactPointInstructions() string {
	return b.CurrentPage.
		FindDescendant(func(comp *BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			// "Which number" or "Which email"
			return strings.HasPrefix(comp.GetAttribute("text"), "Which ")
		}).
		GetAttribute("text")
}

const dialogActionFieldID = "dialog_action"

func pendingDialogInstructions(dialog *BloksDialog, service string) string {
	if dialog == nil {
		return service + " requires a confirmation."
	}
	parts := make([]string, 0, 2)
	if title := strings.TrimSpace(dialog.Title); title != "" {
		parts = append(parts, title)
	}
	if message := strings.TrimSpace(dialog.Message); message != "" {
		parts = append(parts, message)
	}
	if len(parts) == 0 {
		return service + " requires a confirmation."
	}
	return strings.Join(parts, "\n\n")
}

func pendingDialogOptions(dialog *BloksDialog) ([]string, map[string]*BloksDialogButton) {
	options := make([]string, 0, len(dialog.Buttons))
	buttons := make(map[string]*BloksDialogButton, len(dialog.Buttons))
	for idx := range dialog.Buttons {
		button := &dialog.Buttons[idx]
		label := strings.TrimSpace(button.Label)
		if label == "" {
			switch button.Role {
			case "positive":
				label = "Continue"
			case "negative":
				label = "Cancel"
			default:
				label = "Other"
			}
		}
		option := label
		if _, exists := buttons[option]; exists {
			option = fmt.Sprintf("%s (%s)", label, button.Role)
		}
		options = append(options, option)
		buttons[option] = button
	}
	return options, buttons
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
	prevPageTransitionCount := b.PageTransitionCount
	switch b.State {

	case StateDialog:
		dialog := b.PendingDialog
		if dialog == nil {
			return nil, fmt.Errorf("dialog state has no pending dialog")
		}
		options, buttons := pendingDialogOptions(dialog)
		if len(options) == 0 {
			return nil, fmt.Errorf("native dialog has no actions")
		}
		selected := userInput[dialogActionFieldID]
		if selected == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("dialog"),
				Instructions: pendingDialogInstructions(dialog, b.profile.serviceName),
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{{
						ID:           dialogActionFieldID,
						Name:         "Continue login",
						Type:         bridgev2.LoginInputFieldTypeSelect,
						DefaultValue: options[0],
						Options:      options,
					}},
				},
			}
			break
		}
		log.Info().Str("dialog_action", selected).Msg("Picked option from dialog")
		button := buttons[selected]
		if button == nil {
			return nil, fmt.Errorf("unknown dialog action")
		}
		delete(userInput, dialogActionFieldID)
		b.PendingDialog = nil
		b.State = b.DialogPreviousState
		b.DialogPreviousState = StateUnknown
		if button.Callback != nil {
			if err = button.Callback(ctx); err != nil {
				return nil, fmt.Errorf("executing dialog action: %w", err)
			}
		}

	case StateTestCaptcha:
		if userInput["captcha_code"] == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("captcha"),
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
		b.State = b.profile.initialState

	case StateInitialInstagram:
		rpc := "com.bloks.www.caa.login.login_homepage"
		appDoc, err := GetBloksAppDoc(b.Config.Platform)
		if err != nil {
			return nil, fmt.Errorf("initial request: %w", err)
		}
		page, err := b.Config.MakeBloksRequest(
			ctx,
			appDoc,
			rpc,
			b.instagramDirectLoginParams(),
			b.Bridge.DeviceID,
			b.Bridge.FamilyDeviceID,
		)
		if err != nil {
			return nil, fmt.Errorf("rpc %s: %w", rpc, err)
		}
		if err = b.Bridge.DisplayNewScreen(ctx, rpc, page); err != nil {
			return nil, fmt.Errorf("displaying initial Instagram login screen: %w", err)
		}

	case StateInitialMessenger:
		rpc := "com.bloks.www.bloks.caa.login.process_client_data_and_redirect"
		actionDoc, err := GetBloksActionDoc(b.Config.Platform)
		if err != nil {
			return nil, fmt.Errorf("initial request: %w", err)
		}
		params, err := b.initialLoginParams()
		if err != nil {
			return nil, fmt.Errorf("initial request: %w", err)
		}
		action, err := b.Config.MakeBloksRequest(ctx, actionDoc, rpc, params, b.Bridge.DeviceID, b.Bridge.FamilyDeviceID)
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

	case StateLandingPage:
		existingProfile := b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "I already have a profile")).
			FindContainingButton()
		if existingProfile == nil {
			return nil, errors.New("couldn't find the existing profile button")
		}
		err = existingProfile.TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping existing Instagram profile button: %w", err)
		}

	case StateEmailPasswordPage:
		username := userInput["username"]
		password := userInput["password"]
		if username == "" || password == "" {
			instructions := b.profile.credentialsInstruction
			stepID := b.stepID("email_password")
			if b.LastError != "" {
				instructions = fmt.Sprintf("%s. %s", b.LastError, instructions)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       stepID,
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
			return nil, loginerrors.PhoneNumber
		}
		if strings.Contains(username, ":") { // covers MXIDs
			return nil, b.profile.errors.InvalidUsername
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "username")
		delete(userInput, "password")
		b.LastError = b.profile.loginRejected

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
			errorKind, safeDetail := instagramLoginSubmissionErrorDiagnostic(err)
			b.profile.logCredentialSubmissionError(log, err, errorKind, safeDetail)
			if b.profile.isCheckpointRejection(err) {
				b.LastError = b.profile.loginRejected
			} else if strings.Contains(err.Error(), "Invalid username or password") {
				b.LastError = "Invalid username or password"
			} else if strings.Contains(err.Error(), "isn’t connected to an account") || strings.Contains(err.Error(), "isn't connected to an account") {
				// Note matching both unicode + ASCII apostrophe - unicode appears to be what Meta uses
				thing := "username"
				if strings.Contains(username, "@") {
					thing = "email address"
				}
				b.LastError = fmt.Sprintf("That %s is not connected to a %s account", thing, b.profile.serviceName)
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
			} else if errorKind == "rate_limited" {
				return nil, loginerrors.RateLimited
			} else {
				return nil, b.profile.unhandledCredentialError(err, errorKind, safeDetail)
			}
		}

	case StateAuthenticationConfirm:
		if authenticationConfirmationPageState(b.CurrentPage) == StateAccountRecoveryPage {
			b.State = StateAccountRecoveryPage
			break
		}
		btn, clickableTextCount := findAuthenticationConfirmationButton(b.CurrentPage)
		if btn == nil {
			return nil, fmt.Errorf(
				"couldn't find authentication confirmation action (clickable_text_count=%d)",
				clickableTextCount,
			)
		}
		err = btn.TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping authentication confirmation button: %w", err)
		}

	case StateAccountSelectionPage:
		foundAccounts, accountNames, clickableTextCount := findAccountSelectionOptions(b.CurrentPage)
		if len(accountNames) == 0 {
			return nil, fmt.Errorf(
				"couldn't find accounts on account selection page (clickable_text_count=%d)",
				clickableTextCount,
			)
		}

		selectedAccount := userInput["account"]
		if selectedAccount == "" && len(accountNames) == 1 {
			selectedAccount = accountNames[0]
		}
		if selectedAccount == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("account_selection"),
				Instructions: b.profile.accountSelection,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{
							ID: "account", Name: "Instagram account", Type: bridgev2.LoginInputFieldTypeSelect,
							Options: accountNames,
						},
					},
				},
			}
			break
		}
		log.Info().Str("account", selectedAccount).Msg("Picked account from account selection page")

		selectedButton := foundAccounts[selectedAccount]
		if selectedButton == nil {
			return nil, errors.New("invalid account selection")
		}
		delete(userInput, "account")
		err = selectedButton.TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping selected account: %w", err)
		}

	case StateAccountRecoveryPage:
		return nil, b.profile.errors.AccountRecovery

	case StatePasswordFormPage:
		password := userInput["password"]
		if password == "" {
			instructions := fmt.Sprintf("Re-enter your %s password.", b.profile.serviceName)
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("password"),
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{ID: "password", Name: "Password", Type: bridgev2.LoginInputFieldTypePassword},
					},
				},
			}
			break
		}

		delete(userInput, "password")
		b.LastError = b.profile.loginRejected

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
			if strings.Contains(err.Error(), "Invalid username or password") {
				b.LastError = "The password you entered is incorrect"
			} else {
				return nil, fmt.Errorf("tapping password form log in button: %w", err)
			}
		}

	case StateCodeEntryPage:
		otpCode := userInput["otp_code"]
		if otpCode == "" {
			instructions := b.getCodeInstructions()
			if instructions == "" {
				instructions = b.profile.codeInstruction
			}
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("otp_code"),
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					CanCancel: b.MFACanGoBack,
					Fields: []bridgev2.LoginInputDataField{
						{
							ID:   "otp_code",
							Name: b.profile.codeFieldName,
							Type: bridgev2.LoginInputFieldType2FACode,
						},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "otp_code")
		b.LastError = b.rejectedCodeMessage()

		input := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Enter code",
				)) != nil
			})
		input = b.profile.fallbackCodeInput(b.CurrentPage, input)

		continueButton := b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton()
		b.profile.prepareCodeSubmission(b, continueButton, log, "code-submission")

		err := input.FillInput(ctx, b.CurrentPage.Interpreter, otpCode)
		if err != nil {
			return nil, fmt.Errorf("filling otp code input: %w", err)
		}

		actionRPCCountBefore := b.ActionRPCCount
		err = continueButton.TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			log.Debug().Err(err).Msg("Got error from OTP code submission")
			if strings.Contains(err.Error(), "Please re-enter") {
				// retry
			} else if strings.Contains(err.Error(), "An unexpected error occurred") {
				return nil, b.uninformativeLoginError("otp submit unexpected error")
			} else {
				return nil, fmt.Errorf("tapping continue: %w", err)
			}
		}
		b.profile.finishCodeSubmission(b, StateCodeEntryPage, actionRPCCountBefore)

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
				StepID:       b.stepID("backup_code"),
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					CanCancel: b.MFACanGoBack,
					Fields: []bridgev2.LoginInputDataField{
						{ID: "backup_code", Name: "Backup code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "backup_code")
		b.LastError = b.rejectedCodeMessage()

		input := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			})
		if input == nil {
			input = b.CurrentPage.FindDescendant(FilterByComponent("bk.components.TextInput"))
		}
		err := input.FillInput(ctx, b.CurrentPage.Interpreter, backupCode)
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
				img = b.CurrentPage.FindDescendant(FilterByAttribute("bk.components.Image", "scale_type", "stretch"))
			}
			if img == nil {
				return nil, fmt.Errorf("can't find captcha image")
			}
			imageURL := img.GetDynamicAttribute(ctx, b.CurrentPage.Interpreter, "url")
			if imageURL == "" {
				return nil, fmt.Errorf("captcha image has no url")
			}
			log.Trace().Str("image_url", imageURL).Msg("Found image captcha")

			audio := b.CurrentPage.FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.data.TextSpan" {
					return false
				}
				return strings.EqualFold(comp.GetAttribute("text"), "play audio")
			})
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
			b.DisplayedURL = ""
			_, err := b.CurrentPage.Interpreter.Evaluate(ctx, &onClick.AST)
			if err != nil {
				return nil, fmt.Errorf("clicking on audio text: %w", err)
			}
			if b.DisplayedURL == "" {
				return nil, fmt.Errorf("clicking on audio text failed to open url")
			}
			audioURL := strings.Replace(b.DisplayedURL, "/player/", "/", 1)
			log.Trace().Str("audio_url", audioURL).Msg("Found audio captcha")

			imageBytes, imageMime, err := b.Config.FetchAsset(ctx, imageURL)
			if err != nil {
				return nil, fmt.Errorf("error fetching image response: %w", err)
			}
			if !strings.HasPrefix(imageMime, "image/") {
				return nil, fmt.Errorf("bad image captcha mime type %s", imageMime)
			}
			imageFilename := "captcha" + exmime.ExtensionFromMimetype(imageMime)

			audioBytes, audioMime, err := b.Config.FetchAsset(ctx, audioURL)
			if err != nil {
				return nil, fmt.Errorf("error fetching audio response: %w", err)
			}
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

			instructions := b.profile.serviceName + " requires solving a captcha"
			if b.LastError != "" {
				instructions = b.LastError
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("captcha"),
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
		b.LastError = b.profile.serviceName + " rejected that captcha solution"

		input := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Enter characters",
				)) != nil
			})
		if input == nil {
			input = b.CurrentPage.FindDescendant(FilterByComponent("bk.components.TextInput"))
		}
		err := input.
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
				return nil, b.uninformativeLoginError("captcha submit query error")
			}
			// Sometimes just for spice, they will throw you a "Wrong Credentials" /
			// "Invalid username or password" error here, even though what you submitted
			// was a captcha rather than a username or password. And of course this
			// happens even if you gave the correct password. If you actually gave a
			// wrong password, it would have errored out at the password step, if we get
			// the same error here, it means the Zuck says no.
			if strings.Contains(err.Error(), "Invalid username or password") {
				return nil, b.uninformativeLoginError("captcha submit invalid username/password")
			}
			// Another kind of lie that we can get from Facebook.
			if strings.Contains(err.Error(), "An unexpected error occurred") {
				return nil, b.uninformativeLoginError("captcha submit unexpected error")
			}
			return nil, fmt.Errorf("tapping continue: %w", err)
		}

	case StateReCaptchaPage:
		webview := b.CurrentPage.FindDescendantIncludingEmbedded(FilterByComponent("webview"))
		if webview == nil {
			return nil, fmt.Errorf("can't find reCAPTCHA webview")
		}
		token := userInput["recaptcha_token"]
		if token == "" {
			url := webview.GetDynamicAttribute(ctx, b.CurrentPage.Interpreter, "url")
			if url == "" {
				return nil, fmt.Errorf("reCAPTCHA webview has no URL")
			}
			pageBytes, _, err := b.Config.FetchAsset(ctx, url)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to fetch recaptcha webview html")
			} else {
				log.Debug().Bytes("resp", pageBytes).Msg("Fetching recaptcha webview html")
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeCookies,
				StepID:       "fi.mau.meta.messengerlite.recaptcha",
				Instructions: "Complete the Google reCAPTCHA challenge.",
				CookiesParams: &bridgev2.LoginCookiesParams{
					URL: url,
					Fields: []bridgev2.LoginCookieField{{
						ID:       "recaptcha_token",
						Required: true,
						Sources:  []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeSpecial, Name: "recaptcha_token"}},
					}},
					ExtractJS: `new Promise((resolve, reject) => {
						window.FbLoginRecaptcha = {
							onRecaptcha: data => {
								try {
									resolve(JSON.parse(data)["g-recaptcha-response"]);
								} catch (err) {
									reject(err);
								}
							}
						}
					})`,
				},
			}
			break
		}
		log.Debug().Str("recaptcha_token", token).Msg("Got recaptcha token from webview")
		callback := webview.GetScript("callback")
		if callback == nil {
			return nil, fmt.Errorf("reCAPTCHA webview has no callback")
		}
		delete(userInput, "recaptcha_token")
		if _, err = b.CurrentPage.Interpreter.Evaluate(InterpBindArgs(ctx, token), &callback.AST); err != nil {
			return nil, fmt.Errorf("submitting reCAPTCHA token: %w", err)
		}

	case StateMFALandingPage:
		if b.profile.hasMFAMethodsOnLanding(b.CurrentPage) {
			b.State = StateChooseMFAPage
			break
		}
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
		foundMethods, methodNames, numIgnored := b.profile.findMFAMethods(b.CurrentPage, log)
		b.MFACanGoBack = false

		if len(foundMethods) == 0 {
			if numIgnored == 0 {
				return nil, fmt.Errorf("couldn't find any mfa types at all")
			}
			return nil, b.profile.errors.NoSupportedMFA
		}

		chosenMethod := userInput["mfatype"]
		if chosenMethod == "" && len(foundMethods) == 1 {
			chosenMethod = methodNames[0]
		}
		if chosenMethod == "" {
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("mfa_type"),
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
		log.Info().Str("mfatype", chosenMethod).Msg("Picked MFA method from MFA selection page")

		if foundMethods[chosenMethod] == nil {
			return nil, b.profile.invalidMFAMethodError(chosenMethod)
		}

		err := foundMethods[chosenMethod].
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, b.profile.mfaMethodTapError(chosenMethod, err)
		}
		b.MFACanGoBack = len(foundMethods) > 1
		if !b.profile.shouldContinueAfterMFAMethod(b.State) {
			break
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
				StepID:       b.stepID("totp"),
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					CanCancel: b.MFACanGoBack,
					Fields: []bridgev2.LoginInputDataField{
						{ID: "totp_code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "totp_code")
		b.LastError = b.rejectedCodeMessage()

		input := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			})

		if input == nil {
			input = b.CurrentPage.
				FindDescendant(func(comp *BloksTreeComponent) bool {
					if comp.ComponentID != "bk.components.TextInput" {
						return false
					}
					return comp.GetAttribute("type") == "number"
				})
		}
		input = b.profile.fallbackCodeInput(b.CurrentPage, input)

		continueButton := b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Continue")).
			FindContainingButton()
		b.profile.prepareCodeSubmission(b, continueButton, log, "TOTP-submission")

		err := input.FillInput(ctx, b.CurrentPage.Interpreter, totpCode)
		if err != nil {
			return nil, fmt.Errorf("filling mfa code input: %w", err)
		}

		actionRPCCountBefore := b.ActionRPCCount
		err = continueButton.TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping continue: %w", err)
		}
		b.profile.finishCodeSubmission(b, StateTOTPPage, actionRPCCountBefore)

	case StateAFADPage:
		notif := b.CurrentPage.FindDescendant(func(comp *BloksTreeComponent) bool {
			if comp.ComponentID != "bk.data.TextSpan" {
				return false
			}
			for _, prefix := range []string{
				// Covers both "We sent a notification" and "We sent an Instagram notification"/etc
				"We sent a",
				"Open the notification",
				"You need to sign in on",
				"Check your notifications",
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
			StepID:       b.stepID("afad_wait"),
			Instructions: b.AFADNotification,
			DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
				Type:      bridgev2.LoginDisplayTypeNothing,
				CanCancel: b.MFACanGoBack,
			},
		}
		b.State = StateAFADPageWaiting

	case StateAFADPageWaiting:
		for b.State == StateAFADPageWaiting {
			if b.AFADCallback == nil {
				return nil, loginerrors.AFADStopped
			}
			select {
			case <-time.After(b.AFADInterval):
			case <-ctx.Done():
				if errors.Is(context.Cause(ctx), bridgev2.ErrLoginStepCancelled) {
					return nil, bridgev2.ErrLoginStepCancelled
				}
				return nil, fmt.Errorf("login cancelled while waiting for approval: %w", ctx.Err())
			}
			err := b.AFADCallback()
			if err != nil {
				if errors.Is(context.Cause(ctx), bridgev2.ErrLoginStepCancelled) {
					return nil, bridgev2.ErrLoginStepCancelled
				} else if ctxErr := ctx.Err(); ctxErr != nil {
					return nil, fmt.Errorf("login cancelled while waiting for approval: %w", ctxErr)
				}
				return nil, fmt.Errorf("AFAD callback: %w", err)
			}
		}

	case StateOAuthPage:
		b.DisplayedURL = ""

		err = b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Verify with Google")).
			FindContainingButton().
			TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping verify: %w", err)
		}

		if b.DisplayedURL == "" {
			return nil, fmt.Errorf("oauth button failed to open url")
		}

		step = &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeCookies,
			StepID:       "fi.mau.meta.messengerlite.google_oauth",
			Instructions: "Sign in with your Google account.",
			CookiesParams: &bridgev2.LoginCookiesParams{
				URL: b.DisplayedURL,
				Fields: []bridgev2.LoginCookieField{{
					ID:       "oauth_token",
					Required: true,
					Sources:  []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeSpecial, Name: "oauth_token"}},
				}},
				ExtractJS: `new Promise((resolve, reject) => reject("not implemented yet"))`,
			},
		}

	case StateSMSPage:
		for _, mount := range b.CurrentPage.FindDescendants(FilterByComponent("bk.components.OnMount")) {
			script := mount.GetScript("on_first_mount")
			if script == nil {
				continue
			}
			_, err := b.CurrentPage.Interpreter.Evaluate(InterpBindThis(ctx, mount), &script.AST)
			if err != nil {
				if strings.Contains(err.Error(), "We can't send a code right now") {
					return nil, b.profile.errors.NoSMSAvailable
				}
				return nil, fmt.Errorf("sms on_mount script: %w", err)
			}
		}

		// Running the on_mount handlers should have triggered a code to be sent.
		b.State = StateSMSPageAfterSend

	case StateSilentCaptchaPage:
		// This is handled the same way as the SMS page, it should
		// trigger a network request which hopefully leads to something
		// interesting.
		for _, mount := range b.CurrentPage.FindDescendants(FilterByComponent("bk.components.OnMount")) {
			script := mount.GetScript("on_first_mount")
			if script == nil {
				continue
			}
			_, err := b.CurrentPage.Interpreter.Evaluate(InterpBindThis(ctx, mount), &script.AST)
			if err != nil {
				// Sometimes the email/password page redirects us to the captcha
				// page, which then opens a dialog to give the error message.
				//
				// So it's possible that we are getting an error from the captcha
				// process itself, but it's also possible that we are getting a
				// delayed error that Facebook did not show until after the captcha
				// request. We try to detect the latter.
				//
				// If we end up seeing this happen in cases other than the initial
				// email/password screen then we'd want to generalize this code.
				//
				// Warning: I haven't tested the "return to previous screen" logic.
				log.Debug().Err(err).Msg("Got error from no-op captcha on_mount script")
				if strings.Contains(err.Error(), "Invalid username or password") && b.PreviousState == StateEmailPasswordPage {
					log.Debug().Str("cur_state", string(b.State)).Str("prev_state", string(b.PreviousState)).Msg("Returning to previous Bloks screen")
					b.State = StateEmailPasswordPage
					b.CurrentPage = b.PreviousPage
					b.LastError = "Invalid username or password"
				} else {
					return nil, fmt.Errorf("no-op captcha on_mount script: %w", err)
				}
			}
		}

	case StateSMSPageAfterSend:
		smsCode := userInput["sms_code"]
		if smsCode == "" {
			instructions := b.getCodeInstructions()
			if instructions == "" {
				instructions = b.profile.smsInstruction
			}
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("sms"),
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					CanCancel: b.MFACanGoBack,
					Fields: []bridgev2.LoginInputDataField{
						{ID: "sms_code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "sms_code")
		b.LastError = b.rejectedCodeMessage()

		input := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			})

		if input == nil {
			input = b.CurrentPage.
				FindDescendant(func(comp *BloksTreeComponent) bool {
					if comp.ComponentID != "bk.components.TextInput" {
						return false
					}
					return comp.GetAttribute("type") == "number"
				})
		}

		err := input.FillInput(ctx, b.CurrentPage.Interpreter, smsCode)
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

	case StateChooseContactPointPage:
		buttons := b.CurrentPage.
			FindDescendants(func(comp *BloksTreeComponent) bool {
				switch comp.ComponentID {
				case "bk.components.AccessibilityExtension", "accessibilityExtension":
					return strings.HasPrefix(comp.GetAttribute("label"), "+") || strings.Contains(comp.GetAttribute("label"), "@")
				}
				return false
			})

		foundPoints := map[string]*BloksTreeComponent{}
		pointNames := []string{}
		for _, btn := range buttons {
			point := btn.GetAttribute("label")
			foundPoints[point] = btn
			pointNames = append(pointNames, point)
		}

		if len(pointNames) == 0 {
			return nil, fmt.Errorf("failed to find any contact points on selection page")
		}

		contactPoint := userInput["contact_point"]
		if contactPoint == "" && len(foundPoints) == 1 {
			contactPoint = pointNames[0]
		}
		if contactPoint == "" {
			instructions := b.getContactPointInstructions()
			if instructions == "" {
				instructions = b.profile.contactPointInstruction
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("choose_contact_point"),
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					Fields: []bridgev2.LoginInputDataField{
						{
							ID: "contact_point", Name: "Phone number or email", Type: bridgev2.LoginInputFieldTypeSelect,
							Options: pointNames,
						},
					},
				},
			}
			break
		}
		log.Info().Str("contact_point", contactPoint).Msg("Picked contact point from selection page")

		if foundPoints[contactPoint] == nil {
			return nil, b.profile.invalidContactPointError(contactPoint)
		}

		err := foundPoints[contactPoint].FindContainingButton().TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tap selected point: %w", err)
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
			_, err := b.CurrentPage.Interpreter.Evaluate(InterpBindThis(ctx, mount), &script.AST)
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
			if instructions == "" {
				instructions = b.profile.whatsAppInstruction
			}
			if b.LastError != "" {
				instructions = fmt.Sprintf(
					"%s. %s", strings.TrimSuffix(b.LastError, "."), instructions,
				)
				b.LastError = ""
			}
			step = &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeUserInput,
				StepID:       b.stepID("whatsapp"),
				Instructions: instructions,
				UserInputParams: &bridgev2.LoginUserInputParams{
					CanCancel: b.MFACanGoBack,
					Fields: []bridgev2.LoginInputDataField{
						{ID: "whatsapp_code", Name: "Six-digit code", Type: bridgev2.LoginInputFieldType2FACode},
					},
				},
			}
			break
		}

		// Set up in case we don't navigate to a new page successfully
		delete(userInput, "whatsapp_code")
		b.LastError = b.rejectedCodeMessage()

		input := b.CurrentPage.
			FindDescendant(func(comp *BloksTreeComponent) bool {
				if comp.ComponentID != "bk.components.TextInput" {
					return false
				}
				return comp.FindDescendant(FilterByAttribute(
					"bk.components.AccessibilityExtension", "label", "Code",
				)) != nil
			})

		if input == nil {
			input = b.CurrentPage.
				FindDescendant(func(comp *BloksTreeComponent) bool {
					if comp.ComponentID != "bk.components.TextInput" {
						return false
					}
					return comp.GetAttribute("type") == "number"
				})
		}

		err := input.FillInput(ctx, b.CurrentPage.Interpreter, whatsAppCode)
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

	case StatePasskeyPage:
		btn := b.CurrentPage.
			FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Try another way")).
			FindContainingButton()
		if btn == nil {
			return nil, b.profile.errors.MandatoryPasskey
		}
		err := btn.TapButton(ctx, b.CurrentPage.Interpreter)
		if err != nil {
			return nil, fmt.Errorf("tapping try another way button: %w", err)
		}

	case StateSuggestedAccountPage:
		// "We couldn't find an account matching the login info you entered, but found an
		// account that closely matches based on your login history". Identifying details of
		// the account are shown on screen.
		//
		// For now, assume that if Facebook is recommending a specific account and providing
		// personal information for that account, that they have verified the credentials
		// sufficiently that we can assume they've picked the right account and we should
		// just proceed.
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
			fieldIDs := []string{}
			fieldOptions := []string{}
			if step.UserInputParams != nil {
				for _, field := range step.UserInputParams.Fields {
					fieldIDs = append(fieldIDs, field.ID)
					if field.Type == bridgev2.LoginInputFieldTypeSelect {
						fieldOptions = append(fieldOptions, field.Options...)
					}
				}
			}
			if step.CookiesParams != nil {
				for _, field := range step.CookiesParams.Fields {
					fieldIDs = append(fieldIDs, field.ID)
				}
			}
			log.Debug().
				Str("cur_state", string(b.State)).
				Str("step_id", step.StepID).
				Strs("field_ids", fieldIDs).
				Strs("field_options", fieldOptions).
				Msg("Requested user input")
		} else if b.LastError != "" {
			log.Debug().
				Str("cur_state", string(b.State)).
				Bool("has_last_error", true).
				Msg("Got intra-screen error, remaining in current state")
		} else if b.PageTransitionCount > prevPageTransitionCount {
			// This seems to happen sometimes on the CAA page. If we determine that
			// login is never successful after that happens, we can make this an error
			// again.
			log.Debug().Msg("Redirected explicitly back to same page")
		} else {
			return nil, fmt.Errorf("handling %s failed to advance flow", prevState)
		}
	} else {
		b.PreviousState = prevState
		log.Debug().Str("old_state", string(prevState)).Str("new_state", string(b.State)).Msg("Transitioned login step")

		// Ignore LastError, which is only used for signaling an error within the current
		// page and can be ignored once we move to a new page.
		b.LastError = ""
	}
	return step, nil
}

func (b *Browser) CancelLoginStep(ctx context.Context) error {
	if !b.MFACanGoBack {
		return fmt.Errorf("current login step cannot be cancelled")
	}
	switch b.State {
	case StateCodeEntryPage, StateBackupCodePage, StateTOTPPage,
		StateSMSPageAfterSend, StateWhatsAppPageAfterSend, StateAFADPageWaiting:
	default:
		return fmt.Errorf("current login step cannot be cancelled")
	}
	btn := b.CurrentPage.
		FindDescendant(FilterByAttribute("bk.data.TextSpan", "text", "Try another way")).
		FindContainingButton()
	if btn == nil {
		return fmt.Errorf("couldn't find try another way button")
	}
	if err := btn.TapButton(ctx, b.CurrentPage.Interpreter); err != nil {
		return fmt.Errorf("tapping try another way button: %w", err)
	}
	b.MFACanGoBack = false
	return nil
}

func authenticationConfirmationPageState(page *BloksBundle) BrowserState {
	if page != nil && page.FindDescendant(FilterByComponent("bk.components.TextInput")) != nil {
		return StateAccountRecoveryPage
	}
	return StateAuthenticationConfirm
}

func findAuthenticationConfirmationButton(page *BloksBundle) (*BloksTreeComponent, int) {
	if page == nil {
		return nil, 0
	}
	var fallback *BloksTreeComponent
	clickableTextCount := 0
	for _, comp := range page.FindDescendants(func(comp *BloksTreeComponent) bool {
		switch comp.ComponentID {
		case "bk.data.TextSpan", "bk.components.AccessibilityExtension":
			return true
		default:
			return false
		}
	}) {
		text := comp.GetAttribute("text")
		if text == "" {
			text = comp.GetAttribute("label")
		}
		button := comp.FindContainingButton()
		if text == "" || button == nil {
			continue
		}
		clickableTextCount++
		normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
		switch normalized {
		case "continue":
			return button, clickableTextCount
		case "yes, continue", "yes, it's me", "yes, it’s me", "that's me", "that’s me", "this is me", "confirm":
			if fallback == nil {
				fallback = button
			}
		default:
			if strings.HasPrefix(normalized, "continue as ") && fallback == nil {
				fallback = button
			}
		}
	}
	return fallback, clickableTextCount
}

func findAccountSelectionOptions(page *BloksBundle) (map[string]*BloksTreeComponent, []string, int) {
	found := map[string]*BloksTreeComponent{}
	if page == nil {
		return found, nil, 0
	}

	type buttonLabels struct {
		button        *BloksTreeComponent
		text          []string
		accessibility []string
	}
	byButton := map[*BloksTreeComponent]*buttonLabels{}
	buttonOrder := []*BloksTreeComponent{}
	clickableTextCount := 0
	for _, comp := range page.FindDescendants(func(comp *BloksTreeComponent) bool {
		switch comp.ComponentID {
		case "bk.data.TextSpan", "bk.components.AccessibilityExtension", "accessibilityExtension":
			return true
		default:
			return false
		}
	}) {
		label := comp.GetAttribute("text")
		if label == "" {
			label = comp.GetAttribute("label")
		}
		label = strings.TrimSpace(label)
		button := comp.FindContainingButton()
		if label == "" || button == nil {
			continue
		}
		clickableTextCount++
		group := byButton[button]
		if group == nil {
			group = &buttonLabels{button: button}
			byButton[button] = group
			buttonOrder = append(buttonOrder, button)
		}
		if comp.ComponentID == "bk.data.TextSpan" {
			group.text = appendUniqueString(group.text, label)
		} else {
			group.accessibility = appendUniqueString(group.accessibility, label)
		}
	}

	names := []string{}
	for _, button := range buttonOrder {
		group := byButton[button]
		labels := group.text
		if len(labels) == 0 {
			labels = group.accessibility
		}
		accountLabels := []string{}
		for _, label := range labels {
			if !isAccountSelectionAction(label) {
				accountLabels = append(accountLabels, label)
			}
		}
		if len(accountLabels) == 0 {
			continue
		}
		name := strings.Join(accountLabels, " · ")
		uniqueName := name
		for suffix := 2; found[uniqueName] != nil; suffix++ {
			uniqueName = fmt.Sprintf("%s (%d)", name, suffix)
		}
		found[uniqueName] = group.button
		names = append(names, uniqueName)
	}
	return found, names, clickableTextCount
}

func appendUniqueString(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func isAccountSelectionAction(label string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(label), " "))
	switch normalized {
	case "select an account", "choose an account", "accounts",
		"continue", "next", "back", "cancel", "remove", "edit",
		"try another way", "forgot password?", "forgotten password?",
		"log into another account", "log in to another account",
		"add account", "add another account", "create new account":
		return true
	default:
		return strings.HasPrefix(normalized, "log in to another account") ||
			strings.HasPrefix(normalized, "log into another account")
	}
}

func findMessengerMFAMethodOptions(
	page *BloksBundle,
	log *zerolog.Logger,
) (map[string]*BloksTreeComponent, []string, int) {
	if page == nil {
		return map[string]*BloksTreeComponent{}, nil, 0
	}
	return page.FindMFAMethods(log)
}

func findMFAMethodOptions(page *BloksBundle) (map[string]*BloksTreeComponent, []string, int) {
	found := map[string]*BloksTreeComponent{}
	if page == nil {
		return found, nil, 0
	}
	methodNames := []string{}
	seenButtons := map[*BloksTreeComponent]bool{}
	unsupportedCount := 0
	for _, span := range page.FindDescendants(FilterByComponent("bk.data.TextSpan")) {
		method := strings.TrimSpace(span.GetAttribute("text"))
		button := span.FindAncestor(func(comp *BloksTreeComponent) bool {
			for _, prop := range []BloksAttributeID{"on_click", "on_touch_down", "on_touch_up"} {
				if comp.Attributes[prop] != nil {
					return true
				}
			}
			return false
		})
		if method == "" || button == nil || seenButtons[button] {
			continue
		}
		isMethod, supported := classifyMFAMethod(method)
		if !isMethod {
			continue
		}
		seenButtons[button] = true
		if !supported {
			unsupportedCount++
			continue
		}
		found[method] = span
		methodNames = append(methodNames, method)
	}
	return found, methodNames, unsupportedCount
}

func twoStepVerificationEntrypointState(page *BloksBundle) BrowserState {
	if page == nil {
		return StateMFALandingPage
	}
	if page.FindDescendant(FilterByComponent("bk.components.TextInput")) != nil {
		for _, span := range page.FindDescendants(FilterByComponent("bk.data.TextSpan")) {
			label := strings.ToLower(strings.Join(strings.Fields(span.GetAttribute("text")), " "))
			switch {
			case strings.Contains(label, "authentication app"),
				strings.Contains(label, "authenticator app"),
				strings.Contains(label, "six-digit"),
				strings.Contains(label, "6-digit"):
				return StateTOTPPage
			case strings.Contains(label, "backup code"):
				return StateBackupCodePage
			case strings.Contains(label, "whatsapp"):
				return StateWhatsAppPage
			case strings.Contains(label, "text message"),
				strings.Contains(label, "sms"):
				return StateSMSPage
			}
		}
		return StateCodeEntryPage
	}
	if foundMethods, _, _ := findMFAMethodOptions(page); len(foundMethods) > 0 {
		return StateChooseMFAPage
	}
	return StateMFALandingPage
}

func classifyMFAMethod(label string) (isMethod, supported bool) {
	normalized := strings.ToLower(strings.Join(strings.Fields(label), " "))
	switch {
	case strings.Contains(normalized, "google"), strings.Contains(normalized, "passkey"):
		return true, false
	case strings.Contains(normalized, "authentication app"),
		strings.Contains(normalized, "authenticator app"),
		strings.Contains(normalized, "text message"),
		strings.Contains(normalized, "sms"),
		strings.Contains(normalized, "backup code"),
		strings.Contains(normalized, "whatsapp"),
		strings.Contains(normalized, "another device"),
		strings.Contains(normalized, "notification"),
		normalized == "email",
		strings.Contains(normalized, "email code"):
		return true, true
	default:
		return false, false
	}
}
