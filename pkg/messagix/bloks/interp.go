package bloks

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CheckpointError struct {
	error
}

type InterpBridge struct {
	DeviceID               string
	FamilyDeviceID         string
	AndroidDeviceID        string
	MachineID              string
	EncryptPassword        func(context.Context, string) (string, error)
	GetEncryptedMSISDN     func(context.Context, string, bool) (string, error)
	SignRequestData        func(context.Context, any) (any, error)
	SIMPhones              any
	DeviceEmails           any
	DevicePhoneNumber      any
	DeviceNetworkInfo      any
	IsAppInstalled         func(url string, pkgnames ...string) bool
	HasAppPermissions      func(permissions ...string) bool
	GetSecureNonces        func() []string
	GetSecureNoncesForUser func(userKey string) any
	DoPageRPC              func(ctx context.Context, name string, params map[string]string) (*BloksBundle, error)
	DoActionRPC            func(ctx context.Context, name string, params map[string]string) (*BloksScriptNode, error)
	DisplayNewScreen       func(context.Context, string, *BloksBundle) error
	HandleLoginResponse    func(ctx context.Context, data string) error
	StartTimer             func(name string, interval time.Duration, callback func() error) error
	CancelTimer            func(name string) error
	OpenURL                func(url string) error
	OpenDialog             func(context.Context, *BloksDialog) error
	PopScreen              func(context.Context, string) error
	HandleVariableChange   func(ctx context.Context, name string, value *BloksScriptLiteral) error
}

type BloksDialogButton struct {
	Label    string
	Role     string
	Callback func(context.Context) error
}

type BloksDialog struct {
	Title   string
	Message string
	Buttons []BloksDialogButton
}

type Interpreter struct {
	Bridge InterpBridge

	Scripts      map[BloksScriptID]*BloksLambda
	Payloads     map[BloksPayloadID]*BloksBundleRef
	LocalVars    map[BloksVariableID]*BloksScriptLiteral
	GlobalVars   map[BloksVariableID]*BloksScriptLiteral
	SessionStore map[string]*BloksScriptLiteral
}

func NewInterpreter(ctx context.Context, b *BloksBundle, br *InterpBridge, old *Interpreter, clearLocals bool) (*Interpreter, error) {
	p := b.Layout.Payload
	scripts := map[BloksScriptID]*BloksLambda{}
	payloads := map[BloksPayloadID]*BloksBundleRef{}
	globals := map[BloksVariableID]*BloksScriptLiteral{}
	locals := map[BloksVariableID]*BloksScriptLiteral{}
	sessionStore := map[string]*BloksScriptLiteral{}
	if old != nil {
		maps.Copy(scripts, old.Scripts)
		maps.Copy(payloads, old.Payloads)
		maps.Copy(globals, old.GlobalVars)
		maps.Copy(locals, old.LocalVars)
		maps.Copy(sessionStore, old.SessionStore)
	}
	for id, script := range p.Scripts {
		scripts[id] = &BloksLambda{
			Body: &script.AST,
		}
	}
	for _, payload := range p.Embedded {
		payloads[payload.ID] = &BloksBundleRef{
			Bundle: &payload.Contents,
		}
	}
	for _, item := range p.Variables {
		// Deal with the dynamic variables later
		if item.Info.InitialScript != nil {
			continue
		}
		id := BloksVariableID(item.ID)
		switch item.Type {
		case "gs", "bloks_android_system_insets", "bloks_ios_view_insets":
			// Check if global var was already set
			if globals[id] != nil {
				break
			}
			globals[id] = BloksLiteralFromJavaScript(item.Info.Initial)
		case "ls":
			// Local vars do not carry over between screens
			if locals[id] != nil && !clearLocals {
				break
			}
			locals[id] = BloksLiteralFromJavaScript(item.Info.Initial)
		default:
			return nil, fmt.Errorf("unexpected var type %s", item.Type)
		}
	}
	interp := Interpreter{
		Bridge: *br,

		Scripts:      scripts,
		Payloads:     payloads,
		GlobalVars:   globals,
		LocalVars:    locals,
		SessionStore: sessionStore,
	}
	br = &interp.Bridge
	if br.DeviceID == "" {
		br.DeviceID = strings.ToUpper(uuid.New().String())
	}
	if br.FamilyDeviceID == "" {
		// On Android, it appears that FamilyDeviceID is set to the same
		// as regular DeviceID in the initial Bloks request. We may want
		// to replicate that behavior.
		br.FamilyDeviceID = strings.ToUpper(uuid.New().String())
	}
	if br.EncryptPassword == nil {
		br.EncryptPassword = func(ctx context.Context, pw string) (string, error) {
			return fmt.Sprintf(
				"#PWD_LIGHTSPEED_FAKE:%s",
				base64.StdEncoding.EncodeToString(sha256.New().Sum([]byte(pw))),
			), nil
		}
	}
	if br.GetEncryptedMSISDN == nil {
		br.GetEncryptedMSISDN = func(ctx context.Context, name string, flag bool) (string, error) {
			return "", nil
		}
	}
	if br.SignRequestData == nil {
		br.SignRequestData = func(ctx context.Context, data any) (any, error) {
			return nil, nil
		}
	}
	if br.IsAppInstalled == nil {
		br.IsAppInstalled = func(url string, pkgname ...string) bool {
			return false
		}
	}
	if br.HasAppPermissions == nil {
		br.HasAppPermissions = func(permissions ...string) bool {
			return false
		}
	}
	if br.GetSecureNonces == nil {
		br.GetSecureNonces = func() []string {
			return nil
		}
	}
	if br.DoPageRPC == nil {
		br.DoPageRPC = func(ctx context.Context, name string, params map[string]string) (*BloksBundle, error) {
			return nil, fmt.Errorf("unhandled page rpc %s", name)
		}
	}
	if br.DoActionRPC == nil {
		br.DoActionRPC = func(ctx context.Context, name string, params map[string]string) (*BloksScriptNode, error) {
			return nil, fmt.Errorf("unhandled action rpc %s", name)
		}
	}
	if br.DisplayNewScreen == nil {
		br.DisplayNewScreen = func(ctx context.Context, name string, bb *BloksBundle) error {
			return fmt.Errorf("unhandled new screen %s", name)
		}
	}
	if br.HandleLoginResponse == nil {
		br.HandleLoginResponse = func(ctx context.Context, data string) error {
			return fmt.Errorf("unhandled login response")
		}
	}
	if br.StartTimer == nil {
		br.StartTimer = func(name string, interval time.Duration, callback func() error) error {
			return fmt.Errorf("unhandled timer %s", name)
		}
	}
	if br.CancelTimer == nil {
		br.CancelTimer = func(name string) error {
			return fmt.Errorf("unhandled timer cancel %s", name)
		}
	}
	if br.OpenURL == nil {
		br.OpenURL = func(url string) error {
			return fmt.Errorf("unhandled url %s", url)
		}
	}
	if br.OpenDialog == nil {
		br.OpenDialog = func(context.Context, *BloksDialog) error {
			return fmt.Errorf("unhandled dialog")
		}
	}
	if br.PopScreen == nil {
		br.PopScreen = func(context.Context, string) error {
			return fmt.Errorf("unhandled screen pop")
		}
	}
	if br.HandleVariableChange == nil {
		br.HandleVariableChange = func(ctx context.Context, name string, value *BloksScriptLiteral) error {
			return nil
		}
	}
	for _, item := range p.Variables {
		// We already handled the static variables
		if item.Info.InitialScript == nil {
			continue
		}
		// Technically we shouldn't do this evaluation until after we
		// know if the value will be used or not, but as of yet I have
		// not seen a case where it matters.
		value, err := interp.Evaluate(ctx, &item.Info.InitialScript.AST)
		if err != nil {
			return nil, fmt.Errorf("var %s: %w", item.ID, err)
		}
		id := BloksVariableID(item.ID)
		switch item.Type {
		case "gs", "bloks_android_system_insets", "bloks_ios_view_insets":
			if globals[id] != nil {
				break
			}
			globals[id] = value
		case "ls":
			if locals[id] != nil && !clearLocals {
				break
			}
			locals[id] = value
		default:
			return nil, fmt.Errorf("unexpected var type %s", item.Type)
		}
	}
	return &interp, nil
}

func (interp *Interpreter) MergeActionBundle(ctx context.Context, b *BloksBundle) error {
	// Kind of a hack, maybe do this better
	newInterp, err := NewInterpreter(ctx, b, &interp.Bridge, interp, false)
	if err != nil {
		return err
	}
	interp.Scripts = newInterp.Scripts
	interp.Payloads = newInterp.Payloads
	interp.LocalVars = newInterp.LocalVars
	interp.GlobalVars = newInterp.GlobalVars
	return nil
}

type BloksLambda struct {
	Body      *BloksScriptNode
	BoundArgs []*BloksScriptLiteral
}

type BloksElemRef struct {
	Component *BloksTreeComponent
}

type BloksBundleRef struct {
	Bundle *BloksBundle
}

type interpCtx string

const (
	interpCtxArgs interpCtx = "args"
)

func evalAs[T any](ctx context.Context, i *Interpreter, form *BloksScriptNode, where string) (T, error) {
	var zero T
	val, err := i.Evaluate(ctx, form)
	if err != nil {
		return zero, err
	}
	cast, ok := val.Value().(T)
	if !ok {
		return zero, fmt.Errorf("expected %T in %s, got %T", zero, where, val.Value())
	}
	return cast, nil
}

func castFloat(val *BloksScriptLiteral, where string) (float64, error) {
	if cast, ok := val.Value().(float64); ok {
		return cast, nil
	}
	if cast, ok := val.Value().(int64); ok {
		return float64(cast), nil
	}
	return 0, fmt.Errorf("expected int64 or float64 in %s, got %T", where, val.Value())
}

func evalFloat(ctx context.Context, i *Interpreter, form *BloksScriptNode, where string) (float64, error) {
	val, err := i.Evaluate(ctx, form)
	if err != nil {
		return 0, err
	}
	return castFloat(val, where)
}

func literalString(value *BloksScriptLiteral, where string) (string, error) {
	cast, ok := value.Value().(string)
	if !ok {
		return "", fmt.Errorf("expected string in %s, got %T", where, value.Value())
	}
	return cast, nil
}

func evalTreeProp35(ctx context.Context, i *Interpreter, form *BloksScriptNode, where string) (string, error) {
	make, ok := form.Content.(*BloksScriptFuncall)
	if !ok {
		return "", fmt.Errorf("%s non-funcall %T", where, form.Content)
	}
	if make.Function != "bk.action.tree.Make" {
		return "", fmt.Errorf("%s non-tree funcall %s", where, make.Function)
	}
	if len(make.Args)%2 != 1 {
		return "", fmt.Errorf("%s tree.make even number of args %d", where, len(make.Args))
	}
	var lastEvalErr error
	for idx := 1; idx < len(make.Args); idx += 2 {
		attr, err := evalAs[int64](ctx, i, &make.Args[idx], "tree.make")
		if err != nil {
			return "", err
		}
		if attr != 35 && attr != 41 && attr != 43 {
			continue
		}
		data, err := evalAs[string](ctx, i, &make.Args[idx+1], "tree.make")
		if err != nil {
			lastEvalErr = err
			continue
		}
		return data, nil
	}
	return "", fmt.Errorf("no matching string prop in %s tree: %w", where, lastEvalErr)
}

func findTreePropNode(
	ctx context.Context,
	i *Interpreter,
	form *BloksScriptNode,
	attrToFind int64,
	where string,
) (*BloksScriptNode, error) {
	make, ok := form.Content.(*BloksScriptFuncall)
	if !ok {
		return nil, fmt.Errorf("%s non-funcall %T", where, form.Content)
	}
	if make.Function != "bk.action.tree.Make" {
		return nil, fmt.Errorf("%s non-tree funcall %s", where, make.Function)
	}
	if len(make.Args)%2 != 1 {
		return nil, fmt.Errorf("%s tree.make even number of args %d", where, len(make.Args))
	}
	for idx := 1; idx < len(make.Args); idx += 2 {
		attr, err := evalAs[int64](ctx, i, &make.Args[idx], "tree.make")
		if err != nil {
			return nil, err
		}
		if attr == attrToFind {
			return &make.Args[idx+1], nil
		}
	}
	return nil, nil
}

func evalOptionalTreeStringProp(
	ctx context.Context,
	i *Interpreter,
	form *BloksScriptNode,
	attr int64,
	where string,
) (string, error) {
	prop, err := findTreePropNode(ctx, i, form, attr, where)
	if err != nil || prop == nil {
		return "", err
	}
	value, err := i.Evaluate(ctx, prop)
	if err != nil || value.Value() == nil {
		return "", err
	}
	return literalString(value, where)
}

func evalDialogButton(
	ctx context.Context,
	i *Interpreter,
	form *BloksScriptNode,
	attr int64,
	role string,
) (*BloksDialogButton, error) {
	buttonTree, err := findTreePropNode(ctx, i, form, attr, "dialog")
	if err != nil || buttonTree == nil {
		return nil, err
	}
	if literal, ok := buttonTree.Content.(*BloksScriptLiteral); ok && literal.Value() == nil {
		return nil, nil
	}
	label, err := evalOptionalTreeStringProp(ctx, i, buttonTree, 36, "dialog button label")
	if err != nil {
		return nil, err
	}
	callbackNode, err := findTreePropNode(ctx, i, buttonTree, 35, "dialog button")
	if err != nil {
		return nil, err
	}
	button := &BloksDialogButton{
		Label: label,
		Role:  role,
	}
	if callbackNode != nil {
		button.Callback = func(ctx context.Context) error {
			result, err := i.Evaluate(ctx, callbackNode)
			if err != nil {
				return err
			}
			callback, ok := result.Value().(*BloksLambda)
			if !ok {
				return nil
			}
			_, err = i.Evaluate(ctx, &BloksScriptNode{
				Content: &BloksScriptFuncall{
					Function: "bk.action.core.Apply",
					Args: []BloksScriptNode{{
						Content: BloksLiteralOf(callback),
					}},
				},
			})
			return err
		}
	}
	return button, nil
}

func evalInstagramDialog(
	ctx context.Context,
	i *Interpreter,
	form *BloksScriptNode,
) (*BloksDialog, error) {
	title, err := evalOptionalTreeStringProp(ctx, i, form, 40, "dialog title")
	if err != nil {
		return nil, err
	}
	message, err := evalOptionalTreeStringProp(ctx, i, form, 35, "dialog message")
	if err != nil {
		return nil, err
	}
	dialog := &BloksDialog{
		Title:   title,
		Message: message,
	}
	for _, buttonType := range []struct {
		attr int64
		role string
	}{
		{attr: 36, role: "positive"},
		{attr: 38, role: "negative"},
		{attr: 44, role: "neutral"},
	} {
		button, err := evalDialogButton(ctx, i, form, buttonType.attr, buttonType.role)
		if err != nil {
			return nil, err
		}
		if button != nil {
			dialog.Buttons = append(dialog.Buttons, *button)
		}
	}
	if dialog.Title == "" && dialog.Message == "" && len(dialog.Buttons) == 0 {
		return nil, fmt.Errorf("empty Instagram dialog")
	}
	return dialog, nil
}

func evalTreeCallback(ctx context.Context, i *Interpreter, form *BloksScriptNode, where string) (*BloksLambda, error) {
	make, ok := form.Content.(*BloksScriptFuncall)
	if !ok {
		return nil, fmt.Errorf("%s non-funcall %T", where, form.Content)
	}
	if make.Function != "bk.action.tree.Make" {
		return nil, fmt.Errorf("%s non-tree funcall %s", where, make.Function)
	}
	if len(make.Args)%2 != 1 {
		return nil, fmt.Errorf("%s tree.make even number of args %d", where, len(make.Args))
	}
	var lastEvalErr error
	for idx := 1; idx < len(make.Args); idx += 2 {
		attr, err := evalAs[int64](ctx, i, &make.Args[idx], "tree.make")
		if err != nil {
			return nil, err
		}
		// For component 16131, prop 35 is on_failure, prop 36 is on_success_with_result
		if attr != 36 {
			continue
		}
		data, err := evalAs[*BloksLambda](ctx, i, &make.Args[idx+1], "tree.make")
		if err != nil {
			lastEvalErr = err
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("no matching callback prop in %s tree: %w", where, lastEvalErr)
}

const maxInterpArgs = 100

func InterpBindThis(ctx context.Context, this *BloksTreeComponent) context.Context {
	return InterpBindArgs(ctx, &BloksElemRef{this})
}

func InterpBindArgs(ctx context.Context, args ...any) context.Context {
	ambientArgs, ok := ctx.Value(interpCtxArgs).([]*BloksScriptLiteral)
	if !ok {
		ambientArgs = make([]*BloksScriptLiteral, maxInterpArgs)
	}
	for i, arg := range args {
		ambientArgs[i] = BloksLiteralOf(arg)
	}
	return context.WithValue(ctx, interpCtxArgs, ambientArgs)
}

type checkpointsFlowErrorData struct {
	URL                         string `json:"url"`
	FlowID                      int64  `json:"flow_id"`
	UID                         int64  `json:"uid"`
	ShowNativeCheckpoints       bool   `json:"show_native_checkpoints"`
	StartInternalWebviewFromURL bool   `json:"start_internal_webview_from_url"`
}

type checkpointsFlowError struct {
	UID              int64                    `json:"uid"`
	Code             int                      `json:"code"`
	Message          any                      `json:"message"`
	ErrorUserTitle   string                   `json:"error_user_title"`
	ErrorSubcode     int                      `json:"error_subcode"`
	ErrorUserMessage string                   `json:"error_user_msg"`
	ErrorData        checkpointsFlowErrorData `json:"error_data"`
}

type checkpointsFlow struct {
	Error checkpointsFlowError `json:"error"`
}

type bloksPattern struct {
	Value *BloksScriptLiteral
	Body  *BloksScriptNode
}

type bloksDefault struct {
	Body *BloksScriptNode
}

func unwrapLazyBloksBody(node *BloksScriptNode, where string) (*BloksScriptNode, error) {
	call, ok := node.Content.(*BloksScriptFuncall)
	if !ok {
		return nil, fmt.Errorf("%s expected funcall, got %T", where, node.Content)
	}
	if call.Function != "bk.action.core.FuncConst" || len(call.Args) != 1 {
		return nil, fmt.Errorf("%s expected one-argument FuncConst, got %s (%d args)", where, call.Function, len(call.Args))
	}
	return &call.Args[0], nil
}

func getBloksType(lit *BloksScriptLiteral) (int64, error) {
	switch lit.Value().(type) {
	case nil:
		return 0, nil
	case bool:
		return 1, nil
	case string:
		return 2, nil
	case int64:
		return 3, nil
	case float64:
		return 4, nil
	case []*BloksScriptLiteral:
		return 6, nil
	case map[string]*BloksScriptLiteral:
		return 7, nil
	case *BloksLambda:
		return 8, nil
	}
	// Native code would return 5 in this case
	return -1, fmt.Errorf("unexpected bloks typecheck for %T", lit.Value())
}

func (i *Interpreter) Evaluate(ctx context.Context, form *BloksScriptNode) (*BloksScriptLiteral, error) {
	if lit, ok := form.Content.(*BloksScriptLiteral); ok {
		return lit, nil
	}
	ambientArgs, ok := ctx.Value(interpCtxArgs).([]*BloksScriptLiteral)
	if !ok {
		ambientArgs = make([]*BloksScriptLiteral, maxInterpArgs)
	}
	call, ok := form.Content.(*BloksScriptFuncall)
	if !ok {
		return nil, fmt.Errorf("unexpected script node %T", form.Content)
	}
	// Some of the cases in this switch are not needed for any given login. However different
	// functions get pulled in depending on which API you are talking to, so I left in
	// everything that came up at one point or another during testing.
	switch call.Function {
	case "bk.action.core.If":
		cond, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		if cond.IsTruthy() {
			return i.Evaluate(ctx, &call.Args[1])
		}
		return i.Evaluate(ctx, &call.Args[2])
	case "bk.action.bool.Or", "bk.action.core.Coalesce":
		first, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		if first.IsTruthy() {
			return first, nil
		}
		return i.Evaluate(ctx, &call.Args[1])
	case "bk.action.bool.And":
		first, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		if !first.IsTruthy() {
			return first, nil
		}
		return i.Evaluate(ctx, &call.Args[1])
	case "bk.action.core.Let":
		// Lazy-init: return arg0 if non-null, else apply the FuncConst in arg1.
		current, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		if current.IsTruthy() {
			return current, nil
		}
		init, err := evalAs[*BloksLambda](ctx, i, &call.Args[1], "let.init")
		if err != nil {
			return nil, err
		}
		newArgs := make([]*BloksScriptLiteral, maxInterpArgs)
		copy(newArgs, init.BoundArgs)
		ctx = context.WithValue(ctx, interpCtxArgs, newArgs)
		return i.Evaluate(ctx, init.Body)
	case "bk.action.core.Default":
		body, err := unwrapLazyBloksBody(&call.Args[0], "core.default")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(&bloksDefault{Body: body}), nil
	case "bk.action.core.Pattern":
		value, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		body, err := unwrapLazyBloksBody(&call.Args[1], "core.pattern")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(&bloksPattern{Value: value, Body: body}), nil
	case "bk.action.core.Match":
		value, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		patterns, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[1], "core.match patterns")
		if err != nil {
			return nil, err
		}
		for idx, candidate := range patterns {
			pattern, ok := candidate.Value().(*bloksPattern)
			if !ok {
				return nil, fmt.Errorf("core.match pattern %d has type %T", idx, candidate.Value())
			}
			if reflect.DeepEqual(pattern.Value.Value(), value.Value()) {
				return i.Evaluate(ctx, pattern.Body)
			}
		}
		fallback, err := evalAs[*bloksDefault](ctx, i, &call.Args[2], "core.match fallback")
		if err != nil {
			return nil, err
		}
		return i.Evaluate(ctx, fallback.Body)
	case "bk.action.core.GetTemplateArg":
		// The parsed login tree is already expanded by the Bloks response. Template
		// arguments only affect Android-side rendering, which this interpreter does
		// not perform.
		return BloksNull, nil
	case "bk.action.bloks.GetVariable2", "bk.action.bloks.GetVariableWithScope":
		// The second argument to the WithScope variant is an integer that may specify
		// whether to get a local or global variable. For now, ignore.
		varname, err := evalAs[string](ctx, i, &call.Args[0], "getvar2")
		if err != nil {
			return nil, err
		}
		value, ok := i.LocalVars[BloksVariableID(varname)]
		if ok {
			return value, nil
		}
		value, ok = i.GlobalVars[BloksVariableID(varname)]
		if ok {
			return value, nil
		}
		return BloksNull, nil
	case "bk.action.core.TakeLast":
		var result *BloksScriptLiteral
		var err error
		for _, arg := range call.Args {
			result, err = i.Evaluate(ctx, &arg)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	case "bk.action.core.Apply":
		fn, err := evalAs[*BloksLambda](ctx, i, &call.Args[0], "apply")
		if err != nil {
			return nil, err
		}
		newArgs := make([]*BloksScriptLiteral, maxInterpArgs)
		copy(newArgs, fn.BoundArgs)
		for idx := 0; idx < len(call.Args)-1; idx++ {
			result, err := i.Evaluate(ctx, &call.Args[idx+1])
			if err != nil {
				return nil, err
			}
			newArgs[len(fn.BoundArgs)+idx] = result
		}
		ctx := context.WithValue(ctx, interpCtxArgs, newArgs)
		return i.Evaluate(ctx, fn.Body)
	case "bk.action.core.FuncConst":
		return BloksLiteralOf(&BloksLambda{&call.Args[0], nil}), nil
	case "bk.action.core.GetArg":
		idx, err := evalAs[int64](ctx, i, &call.Args[0], "getarg")
		if err != nil {
			return nil, err
		}
		return ambientArgs[idx], nil
	case "bk.action.core.SetArg":
		idx, err := evalAs[int64](ctx, i, &call.Args[0], "setarg")
		if err != nil {
			return nil, err
		}
		value, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		ambientArgs := ctx.Value(interpCtxArgs).([]*BloksScriptLiteral)
		ambientArgs[idx] = value
		return BloksNothing, nil
	case "bk.action.f32.Eq":
		first, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		second, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(first.Value() == second.Value()), nil
	case "bk.action.f32.Lt", "bk.action.i64.Lt":
		first, err := evalAs[int64](ctx, i, &call.Args[0], "lt lhs")
		if err != nil {
			return nil, err
		}
		second, err := evalAs[int64](ctx, i, &call.Args[1], "lt rhs")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(first < second), nil
	case "bk.action.f32.Gt", "bk.action.i64.Gt":
		first, err := evalAs[int64](ctx, i, &call.Args[0], "gt lhs")
		if err != nil {
			return nil, err
		}
		second, err := evalAs[int64](ctx, i, &call.Args[1], "gt rhs")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(first > second), nil
	case "bk.action.f32.Const":
		return i.Evaluate(ctx, &call.Args[0])
	case "bk.action.f32.Add":
		first, err := evalFloat(ctx, i, &call.Args[0], "add lhs")
		if err != nil {
			return nil, err
		}
		second, err := evalFloat(ctx, i, &call.Args[1], "add rhs")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(first + second), nil
	case "jmu", "jn3":
		lhs, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		rhs, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		subtract := call.Function == "jn3"
		lhsInt, lhsIsInt := lhs.Value().(int64)
		rhsInt, rhsIsInt := rhs.Value().(int64)
		if lhsIsInt && rhsIsInt {
			if subtract {
				return BloksLiteralOf(lhsInt - rhsInt), nil
			}
			return BloksLiteralOf(lhsInt + rhsInt), nil
		}
		lhsFloat, err := castFloat(lhs, string(call.Function)+" lhs")
		if err != nil {
			return nil, err
		}
		rhsFloat, err := castFloat(rhs, string(call.Function)+" rhs")
		if err != nil {
			return nil, err
		}
		if subtract {
			return BloksLiteralOf(lhsFloat - rhsFloat), nil
		}
		return BloksLiteralOf(lhsFloat + rhsFloat), nil
	case "bk.action.f32.Sub":
		first, err := evalFloat(ctx, i, &call.Args[0], "sub lhs")
		if err != nil {
			return nil, err
		}
		second, err := evalFloat(ctx, i, &call.Args[1], "sub rhs")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(first - second), nil
	case "bk.action.bloks.GetScript":
		name, err := evalAs[string](ctx, i, &call.Args[0], "getscript")
		if err != nil {
			return nil, err
		}
		script := i.Scripts[BloksScriptID(name)]
		if script == nil {
			return nil, fmt.Errorf("no such script %q", name)
		}
		return BloksLiteralOf(script), nil
	case "bk.action.bloks.WriteLocalState":
		varname, err := evalAs[string](ctx, i, &call.Args[0], "writelocalstate")
		if err != nil {
			return nil, err
		}
		value, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		i.LocalVars[BloksVariableID(varname)] = value
		return BloksNothing, nil
	case "bk.action.bloks.WriteGlobalConsistencyStore":
		varname, err := evalAs[string](ctx, i, &call.Args[0], "writegcs")
		if err != nil {
			return nil, err
		}
		value, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		i.GlobalVars[BloksVariableID(varname)] = value
		err = i.Bridge.HandleVariableChange(ctx, varname, value)
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.array.Make":
		results := []*BloksScriptLiteral{}
		for _, arg := range call.Args {
			result, err := i.Evaluate(ctx, &arg)
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}
		return BloksLiteralOf(results), nil
	case "bk.action.map.Make":
		first, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[0], "map.make")
		if err != nil {
			return nil, err
		}
		second, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[1], "map.make")
		if err != nil {
			return nil, err
		}
		if len(first) != len(second) {
			return nil, fmt.Errorf("mismatching map lengths %d != %d", len(first), len(second))
		}
		result := map[string]*BloksScriptLiteral{}
		for idx := 0; idx < len(first); idx++ {
			key, ok := first[idx].Value().(string)
			if !ok {
				return nil, fmt.Errorf("non-string key %T", first[0].Value())
			}
			result[key] = second[idx]
		}
		return BloksLiteralOf(result), nil
	case "bk.action.caa.login.GetUniqueDeviceId":
		return BloksLiteralOf(i.Bridge.DeviceID), nil
	case "bk.fx.action.GetFamilyDeviceId":
		return BloksLiteralOf(i.Bridge.FamilyDeviceID), nil
	case "bk.action.caa.FetchMachineID":
		return BloksLiteralOf(i.Bridge.MachineID), nil
	case "bk.action.string.EncryptPassword":
		pass, err := evalAs[string](ctx, i, &call.Args[0], "encryptpassword")
		if err != nil {
			return nil, err
		}
		pass, err = i.Bridge.EncryptPassword(ctx, pass)
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(pass), nil
	case "bk.action.fos.headers.GetHeadersSubmitIdentifier":
		name, err := evalAs[string](ctx, i, &call.Args[0], "msisdn")
		if err != nil {
			return nil, err
		}
		flag, err := evalAs[bool](ctx, i, &call.Args[1], "msisdn")
		if err != nil {
			return nil, err
		}
		msisdn, err := i.Bridge.GetEncryptedMSISDN(ctx, name, flag)
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(msisdn), nil
	case "bk.action.caa.attestation.SignRequestDataAndChallengeNonce":
		input, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		attest, err := i.Bridge.SignRequestData(ctx, input.Flatten(false))
		if err != nil {
			return nil, err
		}
		return BloksLiteralFromJavaScript(attest), nil
	case "bk.action.textinput.GetText", "bk.action.caa.GetUsernameText", "bk.action.caa.GetPasswordText":
		ref, err := evalAs[*BloksElemRef](ctx, i, &call.Args[0], "gettext")
		if err != nil {
			return nil, err
		}
		text := ref.Component.textContent
		if text == nil {
			return nil, fmt.Errorf("no text content in referenced element")
		}
		return BloksLiteralOf(*text), nil
	case "bk.action.bool.Not":
		arg, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(!arg.IsTruthy()), nil
	case "h9h":
		arg, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(arg.Value() == nil), nil
	case "bk.action.mins.CallRuntime":
		num, err := evalAs[int64](ctx, i, &call.Args[0], "callruntime")
		if err != nil {
			return nil, err
		}
		if num != 6 {
			return nil, fmt.Errorf("unknown runtime subr %d", num)
		}
		result := map[string]*BloksScriptLiteral{}
		switch len(call.Args) {
		case 1:
			break
		case 3:
			key, err := evalAs[string](ctx, i, &call.Args[1], "callruntime")
			if err != nil {
				return nil, err
			}
			val, err := i.Evaluate(ctx, &call.Args[2])
			if err != nil {
				return nil, err
			}
			result[key] = val
		default:
			return nil, fmt.Errorf("bad arg count %d for runtime subr 6", len(call.Args))
		}
		return BloksLiteralOf(result), nil
	case "bk.action.array.Put", "bk.action.mins.PutByVal":
		dict, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[0], "put")
		if err != nil {
			return nil, err
		}
		key, err := evalAs[string](ctx, i, &call.Args[1], "put")
		if err != nil {
			return nil, err
		}
		val, err := i.Evaluate(ctx, &call.Args[2])
		if err != nil {
			return nil, err
		}
		dict[key] = val
		return BloksNothing, nil
	case "bk.action.array.Length":
		arr, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[0], "array.length")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(int64(len(arr))), nil
	case "bk.action.array.Get":
		mapping, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		switch val := mapping.Value().(type) {
		case map[string]*BloksScriptLiteral:
			key, err := evalAs[string](ctx, i, &call.Args[1], "array.get")
			if err != nil {
				return nil, err
			}
			if val[key] == nil {
				return nil, fmt.Errorf("array key %s not present in map", key)
			}
			return val[key], nil
		case []*BloksScriptLiteral:
			idx, err := evalAs[int64](ctx, i, &call.Args[1], "array.get")
			if err != nil {
				return nil, err
			}
			if idx < 0 || idx >= int64(len(val)) {
				return nil, fmt.Errorf("array index %d out of bounds for length %d", idx, len(val))
			}
			return val[idx], nil
		default:
			return nil, fmt.Errorf("expected array or map in array.get, got %T", mapping.Value())
		}
	case "bk.action.array.Map":
		arr, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[0], "array.map")
		if err != nil {
			return nil, err
		}
		callback, err := evalAs[*BloksLambda](ctx, i, &call.Args[1], "array.map")
		if err != nil {
			return nil, err
		}
		results := []*BloksScriptLiteral{}
		for idx, item := range arr {
			res, err := i.Evaluate(ctx, &BloksScriptNode{
				Content: &BloksScriptFuncall{
					Function: "bk.action.core.Apply",
					Args: []BloksScriptNode{{
						BloksLiteralOf(callback),
					}, {
						BloksLiteralOf(idx),
					}, {
						item,
					}},
				},
			})
			if err != nil {
				return nil, fmt.Errorf("map idx %d: %w", idx, err)
			}
			results = append(results, res)
		}
		return BloksLiteralOf(results), nil
	case "bk.action.array.Filter":
		arr, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[0], "array.filter")
		if err != nil {
			return nil, err
		}
		callback, err := evalAs[*BloksLambda](ctx, i, &call.Args[1], "array.filter")
		if err != nil {
			return nil, err
		}
		results := make([]*BloksScriptLiteral, 0, len(arr))
		for idx, item := range arr {
			include, err := i.Evaluate(ctx, &BloksScriptNode{
				Content: &BloksScriptFuncall{
					Function: "bk.action.core.Apply",
					Args: []BloksScriptNode{{
						Content: BloksLiteralOf(callback),
					}, {
						Content: item,
					}},
				},
			})
			if err != nil {
				return nil, fmt.Errorf("filter idx %d: %w", idx, err)
			}
			if include.IsTruthy() {
				results = append(results, item)
			}
		}
		return BloksLiteralOf(results), nil
	case "bk.action.map.Keys":
		dict, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[0], "map.keys")
		if err != nil {
			return nil, err
		}
		keys := []*BloksScriptLiteral{}
		for key := range dict {
			keys = append(keys, BloksLiteralOf(key))
		}
		return BloksLiteralOf(keys), nil
	case "bk.action.map.Values":
		dict, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[0], "map.keys")
		if err != nil {
			return nil, err
		}
		vals := []*BloksScriptLiteral{}
		for _, val := range dict {
			vals = append(vals, val)
		}
		return BloksLiteralOf(vals), nil
	case "ig.action.IsDarkModeEnabled", "fb.action.IsDarkModeEnabled":
		return BloksLiteralOf(false), nil
	case "bk.action.mins.InByVal":
		dict, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[0], "put")
		if err != nil {
			return nil, err
		}
		key, err := evalAs[string](ctx, i, &call.Args[1], "put")
		if err != nil {
			return nil, err
		}
		_, ok := dict[key]
		return BloksLiteralOf(ok), nil
	case "bk.action.caa.login.GetSimPhones":
		return BloksLiteralOf(i.Bridge.SIMPhones), nil
	case "bk.action.caa.login.GetDeviceEmails":
		return BloksLiteralOf(i.Bridge.DeviceEmails), nil
	case "bk.action.caa.login.GetDevicePhoneNumber":
		return BloksLiteralOf(i.Bridge.DevicePhoneNumber), nil
	case "bk.action.mi.GetDeviceNetworkInfoSync":
		return BloksLiteralOf(i.Bridge.DeviceNetworkInfo), nil
	case "bk.action.bloks.IsAppInstalled":
		url, err := evalAs[string](ctx, i, &call.Args[0], "isappinstalled")
		if err != nil {
			return nil, err
		}
		pkgids, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[1], "isappinstalled")
		if err != nil {
			return nil, err
		}
		strs := []string{}
		for _, perm := range pkgids {
			str, ok := perm.Value().(string)
			if !ok {
				return nil, fmt.Errorf("non-string pkgid %T", perm.Value())
			}
			strs = append(strs, str)
		}
		return BloksLiteralOf(i.Bridge.IsAppInstalled(url, strs...)), nil
	case "bk.action.CheckPermissionStatus":
		perms, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[0], "checkpermissionstatus")
		if err != nil {
			return nil, err
		}
		strs := []string{}
		for _, perm := range perms {
			str, ok := perm.Value().(string)
			if !ok {
				return nil, fmt.Errorf("non-string permission %T", perm.Value())
			}
			strs = append(strs, str)
		}
		return BloksLiteralOf(i.Bridge.HasAppPermissions(strs...)), nil
	case "bk.action.ig.protection.GetSecureNonces":
		userKey, err := evalAs[string](ctx, i, &call.Args[0], "getsecurenonces")
		if err != nil {
			return nil, err
		}
		if i.Bridge.GetSecureNoncesForUser != nil {
			return BloksLiteralFromJavaScript(i.Bridge.GetSecureNoncesForUser(userKey)), nil
		}
		result := []*BloksScriptLiteral{}
		for _, nonce := range i.Bridge.GetSecureNonces() {
			result = append(result, BloksLiteralOf(nonce))
		}
		return BloksLiteralOf(result), nil
	case "bk.action.ref.Make":
		// Technically the way we are handling refs here is totally wrong, since they are
		// supposed to be actual objects and not just transparent macro-like forms, but I
		// was a bit lazy when I wrote this. As long as we never access ref values in a way
		// other than looking them up using getvar, it should work the same.
		return i.Evaluate(ctx, &call.Args[0])
	case "bk.action.ref.Read":
		ref, ok := call.Args[0].Content.(*BloksScriptFuncall)
		if !ok {
			return nil, fmt.Errorf("reading from non-ref %T", call.Args[0].Content)
		}
		if ref.Function != "bk.action.bloks.GetVariable2" && ref.Function != "bk.action.bloks.GetVariableWithScope" {
			return nil, fmt.Errorf("reading from non-ref funcall %s", ref.Function)
		}
		varname, err := evalAs[string](ctx, i, &ref.Args[0], "ref.read")
		if err != nil {
			return nil, err
		}
		value, ok := i.GlobalVars[BloksVariableID(varname)]
		if !ok {
			return BloksNull, nil
		}
		return value, nil
	case "bk.action.ref.Write":
		ref, ok := call.Args[0].Content.(*BloksScriptFuncall)
		if !ok {
			return nil, fmt.Errorf("reading from non-ref %T (for write)", call.Args[0].Content)
		}
		if ref.Function != "bk.action.bloks.GetVariable2" && ref.Function != "bk.action.bloks.GetVariableWithScope" {
			return nil, fmt.Errorf("reading from non-ref funcall %s (for write)", ref.Function)
		}
		varname, err := evalAs[string](ctx, i, &ref.Args[0], "ref.read")
		if err != nil {
			return nil, err
		}
		value, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		i.GlobalVars[BloksVariableID(varname)] = value
		return BloksNothing, nil
	case "bk.action.bloks.AsyncActionWithDataManifestV2":
		name, err := evalAs[string](ctx, i, &call.Args[0], "asyncaction")
		if err != nil {
			return nil, err
		}
		params, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[1], "asyncaction")
		if err != nil {
			return nil, err
		}
		flatParams := map[string]string{}
		for key, val := range params {
			str, ok := val.Value().(string)
			if !ok {
				return nil, fmt.Errorf("non-string param %T for asyncaction", val.Value())
			}
			flatParams[key] = str
		}
		callback, err := evalTreeCallback(ctx, i, &call.Args[2], "asyncaction")
		if err != nil {
			return nil, err
		}
		action, err := i.Bridge.DoActionRPC(ctx, name, flatParams)
		if err != nil {
			return nil, err
		}
		// Evaluate the action and also pass it to the callback
		_, err = i.Evaluate(ctx, &BloksScriptNode{
			Content: &BloksScriptFuncall{
				Function: "bk.action.core.Apply",
				Args: []BloksScriptNode{{
					BloksLiteralOf(callback),
				}, *action},
			},
		})
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.string.JsonEncode", "bk.action.string.JsonEncodeV3":
		arg, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(arg.Flatten(true))
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(string(encoded)), nil
	case "bk.action.string.Concat":
		parts := make([]string, 0, len(call.Args))
		if len(call.Args) == 1 {
			value, err := i.Evaluate(ctx, &call.Args[0])
			if err != nil {
				return nil, err
			}
			if array, ok := value.Value().([]*BloksScriptLiteral); ok {
				for itemIdx, item := range array {
					part, err := literalString(item, fmt.Sprintf("string.concat item %d", itemIdx))
					if err != nil {
						return nil, err
					}
					parts = append(parts, part)
				}
				return BloksLiteralOf(strings.Join(parts, "")), nil
			}
			part, err := literalString(value, "string.concat arg 0")
			if err != nil {
				return nil, err
			}
			return BloksLiteralOf(part), nil
		}
		for idx := range call.Args {
			part, err := evalAs[string](ctx, i, &call.Args[idx], fmt.Sprintf("string.concat arg %d", idx))
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
		return BloksLiteralOf(strings.Join(parts, "")), nil
	case "bk.action.string.Join":
		first, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		second, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		var items []*BloksScriptLiteral
		var separator string
		if array, ok := first.Value().([]*BloksScriptLiteral); ok {
			items = array
			separator, err = literalString(second, "string.join separator")
		} else {
			separator, err = literalString(first, "string.join separator")
			if err == nil {
				items, _ = second.Value().([]*BloksScriptLiteral)
				if items == nil {
					err = fmt.Errorf("string.join values have type %T", second.Value())
				}
			}
		}
		if err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(items))
		for idx, item := range items {
			part, err := literalString(item, fmt.Sprintf("string.join item %d", idx))
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
		return BloksLiteralOf(strings.Join(parts, separator)), nil
	case "bk.action.string.ValueOfNumber":
		value, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		switch number := value.Value().(type) {
		case int64:
			return BloksLiteralOf(strconv.FormatInt(number, 10)), nil
		case float64:
			return BloksLiteralOf(strconv.FormatFloat(number, 'f', -1, 64)), nil
		default:
			return nil, fmt.Errorf("string.valueofnumber got %T", value.Value())
		}
	case "bk.action.map.Merge":
		first, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[0], "merge")
		if err != nil {
			return nil, err
		}
		second, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[1], "merge")
		if err != nil {
			return nil, err
		}
		merged := map[string]*BloksScriptLiteral{}
		for key, val := range first {
			merged[key] = val
		}
		for key, val := range second {
			merged[key] = val
		}
		return BloksLiteralOf(merged), nil
	case "bk.action.string.MatchesRegex":
		str, err := evalAs[string](ctx, i, &call.Args[0], "regex")
		if err != nil {
			return nil, err
		}
		regex, err := evalAs[string](ctx, i, &call.Args[1], "regex")
		if err != nil {
			return nil, err
		}
		r, err := regexp.Compile(regex)
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(r.MatchString(str)), nil
	case "bk.action.function.BindWithArrayV2":
		fn, err := evalAs[*BloksLambda](ctx, i, &call.Args[0], "bind")
		if err != nil {
			return nil, err
		}
		newArgs, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[1], "bind")
		if err != nil {
			return nil, err
		}
		fnCopy := *fn
		fnCopy.BoundArgs = newArgs
		return BloksLiteralOf(&fnCopy), nil
	case "h9a":
		// ignore second argument for now, use first & third
		return i.Evaluate(ctx, &BloksScriptNode{
			Content: &BloksScriptFuncall{
				Function: "bk.action.core.Apply",
				Args: []BloksScriptNode{
					call.Args[2],
					{
						Content: &BloksScriptFuncall{
							Function: "bk.action.string.EncryptPassword",
							Args:     []BloksScriptNode{call.Args[0]},
						},
					},
				},
			},
		})
	case "bk.action.caa.HandleLoginResponseForContextChange", "bk.action.caa.HandleLoginResponse":
		data, err := evalTreeProp35(ctx, i, &call.Args[0], "handleloginresponse")
		if err != nil {
			return nil, err
		}
		err = i.Bridge.HandleLoginResponse(ctx, data)
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.i64.Const":
		return i.Evaluate(ctx, &call.Args[0])
	case "bk.action.i64.Convert", "bk.action.i32.Convert":
		// Both collapse to int64 here: this interpreter has no separate 32-bit integer
		// representation (BloksScriptLiteralValue only ever holds int64 for whole numbers,
		// see BloksScriptLiteral.Parse), so i32 vs i64 only mattered in Meta's original typed
		// VM, not in this reimplementation.
		val, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		switch v := val.Value().(type) {
		case int64:
			return BloksLiteralOf(v), nil
		case float64:
			return BloksLiteralOf(int64(v)), nil
		case string:
			n, perr := strconv.ParseInt(v, 10, 64)
			if perr != nil {
				return nil, fmt.Errorf("%s: cannot convert %q to int: %w", call.Function, v, perr)
			}
			return BloksLiteralOf(n), nil
		case bool:
			if v {
				return BloksLiteralOf(int64(1)), nil
			}
			return BloksLiteralOf(int64(0)), nil
		default:
			return nil, fmt.Errorf("%s: cannot convert %T to int", call.Function, val.Value())
		}
	case "bk.action.f32.Convert":
		n, err := evalFloat(ctx, i, &call.Args[0], "f32.Convert")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(n), nil
	case "bk.action.map.Get":
		obj, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[0], "merge")
		if err != nil {
			return nil, err
		}
		key, err := evalAs[string](ctx, i, &call.Args[1], "merge")
		if err != nil {
			return nil, err
		}
		val, ok := obj[key]
		if !ok {
			return BloksNull, nil
		}
		return val, nil
	case "bk.action.core.AsNonnull":
		result, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		if result.Value() == nil {
			return nil, fmt.Errorf("asnonnull got null")
		}
		return result, nil
	case "bk.action.mins.AssertType":
		val, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		expected, err := evalAs[int64](ctx, i, &call.Args[1], "asserttype")
		if err != nil {
			return nil, err
		}
		actual, err := getBloksType(val)
		if err != nil {
			return nil, err
		}
		// Special case in the native code. 100 means either
		// int or float. It's never returned by TypeOf.
		if expected == 100 {
			switch actual {
			case 3, 4:
				actual = expected
			}
		}
		if expected != actual {
			return nil, fmt.Errorf("bloks type assertion failure (%d != %d)", actual, expected)
		}
		return val, nil
	case "bk.action.mins.TypeOf":
		val, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		btype, err := getBloksType(val)
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(btype), nil
	case "bk.action.mins.GetByValOr":
		lookup, err := i.Evaluate(ctx, &BloksScriptNode{
			Content: &BloksScriptFuncall{
				Function: "bk.action.map.Get",
				Args: []BloksScriptNode{
					call.Args[0],
					call.Args[1],
				},
			},
		})
		if err != nil {
			return nil, err
		}
		if lookup.Value() == nil {
			return i.Evaluate(ctx, &call.Args[2])
		}
		return lookup, nil
	case "bk.action.fx.OpenSyncScreen", "bk.action.fx.PushSyncScreen":
		name, err := evalTreeProp35(ctx, i, &call.Args[0], "pushscreen")
		if err != nil {
			return nil, err
		}
		bundle, err := evalAs[*BloksBundleRef](ctx, i, &call.Args[1], "opensyncscreen")
		if err != nil {
			return nil, err
		}
		err = i.Bridge.DisplayNewScreen(ctx, name, bundle.Bundle)
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.bloks.GetPayload":
		name, err := evalAs[string](ctx, i, &call.Args[0], "getpayload")
		if err != nil {
			return nil, err
		}
		bundle := i.Payloads[BloksPayloadID(name)]
		if bundle == nil {
			return nil, fmt.Errorf("no such payload %q", name)
		}
		return BloksLiteralOf(bundle), nil
	case "bk.action.cds.PushScreen":
		name, err := evalTreeProp35(ctx, i, &call.Args[0], "pushscreen")
		if err != nil {
			return nil, err
		}
		params, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[2], "pushscreen")
		if err != nil {
			return nil, err
		}
		flatParams := map[string]string{}
		for key, val := range params {
			str, ok := val.Value().(string)
			if !ok {
				return nil, fmt.Errorf("non-string param %T for asyncaction", val.Value())
			}
			flatParams[key] = str
		}
		page, err := i.Bridge.DoPageRPC(ctx, name, flatParams)
		if err != nil {
			return nil, err
		}
		err = i.Bridge.DisplayNewScreen(ctx, name, page)
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.string.Length":
		str, err := evalAs[string](ctx, i, &call.Args[0], "string.length")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(int64(len(str))), nil
	case "bk.action.mins.Ge":
		lhs, err := evalAs[int64](ctx, i, &call.Args[0], "mins.ge")
		if err != nil {
			return nil, err
		}
		rhs, err := evalAs[int64](ctx, i, &call.Args[1], "mins.ge")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(lhs >= rhs), nil
	case "bk.action.mins.Le":
		lhs, err := evalAs[int64](ctx, i, &call.Args[0], "mins.ge")
		if err != nil {
			return nil, err
		}
		rhs, err := evalAs[int64](ctx, i, &call.Args[1], "mins.ge")
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(lhs <= rhs), nil
	case "bk.action.timer.Start":
		interval, err := evalAs[int64](ctx, i, &call.Args[1], "timer.start")
		if err != nil {
			return nil, err
		}
		cb, err := evalAs[*BloksLambda](ctx, i, &call.Args[3], "timer.start")
		if err != nil {
			return nil, err
		}
		name, err := evalAs[string](ctx, i, &call.Args[4], "timer.start")
		if err != nil {
			return nil, err
		}
		err = i.Bridge.StartTimer(name, time.Duration(interval)*time.Millisecond, func() error {
			_, err := i.Evaluate(ctx, &BloksScriptNode{
				Content: &BloksScriptFuncall{
					Function: "bk.action.core.Apply",
					Args: []BloksScriptNode{{
						BloksLiteralOf(cb),
					}},
				},
			})
			return err
		})
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.timer.Cancel":
		// Args are (timer context, name), the first of which we have no use for.
		name, err := evalAs[string](ctx, i, &call.Args[1], "timer.cancel")
		if err != nil {
			return nil, err
		}
		err = i.Bridge.CancelTimer(name)
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.caa.PresentCheckpointsFlow":
		flowB, err := evalAs[string](ctx, i, &call.Args[0], "presentcheckpointsflow")
		if err != nil {
			return nil, err
		}
		var flow checkpointsFlow
		err = json.Unmarshal([]byte(flowB), &flow)
		if err != nil {
			return nil, err
		}
		return nil, CheckpointError{fmt.Errorf("%s: %s", flow.Error.ErrorUserTitle, flow.Error.ErrorUserMessage)}
	case "ig.action.cdsdialog.OpenDialog":
		if len(call.Args) < 1 {
			return nil, fmt.Errorf("instagram dialog has no model argument")
		}
		dialog, err := evalInstagramDialog(ctx, i, &call.Args[0])
		if err != nil {
			return nil, err
		}
		if err = i.Bridge.OpenDialog(ctx, dialog); err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.cds.PopScreen":
		if len(call.Args) < 1 {
			return nil, fmt.Errorf("pop screen has no model argument")
		}
		style, err := evalOptionalTreeStringProp(ctx, i, &call.Args[0], 35, "pop screen")
		if err != nil {
			return nil, err
		}
		if style == "" {
			style = "default"
		}
		if err = i.Bridge.PopScreen(ctx, style); err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.dialog.OpenDialog":
		msg, err := evalTreeProp35(ctx, i, &call.Args[0], "opendialog")
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s", msg)
	case "bk.action.callback.MakeWithScopeOnly":
		return i.Evaluate(ctx, &call.Args[0])
	case "bk.action.session_store.Get":
		return BloksLiteralOf(i.SessionStore), nil
	case "bk.action.map.Update":
		target, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[0], "map.update target")
		if err != nil {
			return nil, err
		}
		updates, err := evalAs[map[string]*BloksScriptLiteral](ctx, i, &call.Args[1], "map.update source")
		if err != nil {
			return nil, err
		}
		maps.Copy(target, updates)
		return BloksLiteralOf(target), nil
	case "bk.action.io.CurrentTimeMillis":
		return BloksLiteralOf(time.Now().UnixMilli()), nil
	case "bk.action.io.Toast":
		msg, err := evalAs[string](ctx, i, &call.Args[0], "toast")
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s", msg)
	case "bk.action.navigation.OpenUrl", "bk.action.navigation.OpenUrlV2", "bk.action.OpenUniversalLink":
		// V2 has a second argument which is always null in the flows we see,
		// presumably some kind of navigation options.
		url, err := evalAs[string](ctx, i, &call.Args[0], "openurl")
		if err != nil {
			return nil, err
		}
		return BloksNothing, i.Bridge.OpenURL(url)
	case "bk.action.caa.GenerateUUID":
		// This may be wrong, just guessed the implementation based on the function name, it seems to work
		return BloksLiteralOf(uuid.New().String()), nil
	case "bk.action.gms.flashcall.IncomingCallRetrieverEligibilityChecker":
		// The bridge cannot receive Android GMS flash-call verifications, so report
		// the same boolean capability result as an ineligible Android device.
		return BloksLiteralOf(false), nil
	case "bk.action.qpl.IsMarkerOn":
		// QPL markers are performance instrumentation. The bridge intentionally
		// treats marker start/end/annotation actions as no-ops, so no marker can be
		// active in this interpreter.
		return BloksLiteralOf(false), nil
	case "bk.action.animated.IsInitialized":
		// Animations only control presentation in the Android client. The bridge
		// does not build an Android animation registry, so no named animation is
		// initialized here.
		return BloksLiteralOf(false), nil
	case "bk.action.animated.GetCurrentValue":
		// This can occur in the unused animation branch paired with
		// IsInitialized. Return the stable pre-animation value.
		return BloksLiteralOf(float64(0)), nil
	case "bk.action.animated.Create",
		"bk.action.animated.Parallel",
		"bk.action.animated.easing.CreateCubicBezier",
		"bk.action.template.Make",
		"bk.action.bloks.Find",
		"bk.action.ig.identitysafety.livechat.GetStartChatParams",
		"bk.action.context.Get",
		"bk.fx.action.FetchAllAvailableNativeAuthDataForCaller",
		"bk.action.cds.internal.GetContainerMode",
		"bk.action.caa.GetSPIEligibility":
		return BloksNull, nil
	case "bk.action.core.Delay":
		// First argument is time delay in milliseconds. It seems to be for
		// triggering asynchronous execution. I really hope we can get away
		// without actually doing that.
		return i.Evaluate(ctx, &call.Args[1])
	case "bk.action.i64.Convert":
		arg, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		switch val := arg.Value().(type) {
		case int64:
			return BloksLiteralOf(val), nil
		case float64:
			return BloksLiteralOf(int64(val)), nil
		}
		return nil, fmt.Errorf("can't convert %T to i64", arg.Value())
	case
		"bk.action.animated.Start",
		"bk.action.animated.Build",
		"bk.action.animated.StartToken",
		"bk.action.logging.LogEvent",
		"bk.action.LogFlytrapData",
		"bk.action.qpl.MarkerStartV2",
		"bk.action.qpl.MarkerAnnotate",
		"bk.action.bloks.ClearFocus",
		"bk.action.bloks.RequestFocus",
		"bk.action.bloks.ShowKeyboard",
		"bk.action.bloks.ReplaceEmbeddedChildren",
		"bk.action.bloks.FetchAsyncComponents",
		"bk.action.qpl.MarkerPoint",
		"bk.action.qpl.MarkerEndV2",
		"bk.action.qpl.MarkerDrop",
		"bk.action.bloks.DismissKeyboard",
		"bk.action.accessibility.Announcement",
		"bk.action.toast.ShowToastV2",
		"bk.action.accessibility.SetFocus",
		"bk.action.qpl.userflow.MarkPointV2",
		"bk.action.qpl.userflow.EndFlowSuccessV2",
		"bk.action.qpl.userflow.AnnotateV2",
		"bk.action.qpl.userflow.StartFlowV2",
		"bk.action.qpl.userflow.StartFlowV2IfNotOngoing",
		"bk.action.qpl.userflow.EndFlowCancelV2",
		"bk.action.qpl.userflow.EndFlowFailureV2",
		"bk.action.qpl.userflow.MarkErrorV2",
		"bk.action.logging.LogEventImmediately",
		"bk.action.text_input.ClearText",
		"bk.action.caa.reg.SaveCachedInfo",
		"bk.action.textinput.SetTextV2",
		"bk.action.caa.reg.SaveMachineID",
		"bk.action.caa.ShowLoggedInResetPassword":
		return BloksNothing, nil
	}
	return nil, fmt.Errorf("unimplemented function %s (%d args)", call.Function, len(call.Args))
}
