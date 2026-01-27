package bloks

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type InterpBridge struct {
	DeviceID            string
	FamilyDeviceID      string
	MachineID           string
	EncryptPassword     func(string) (string, error)
	SIMPhones           any
	DeviceEmails        any
	IsAppInstalled      func(url string, pkgnames ...string) bool
	HasAppPermissions   func(permissions ...string) bool
	GetSecureNonces     func() []string
	DoRPC               func(name string, params map[string]string, isPage bool, callback func(result *BloksScriptLiteral) error) error
	DisplayNewScreen    func(string, *BloksBundle) error
	HandleLoginResponse func(data string) error
	StartTimer          func(name string, interval time.Duration, callback func() error) error
}

type Interpreter struct {
	Bridge InterpBridge

	Scripts    map[BloksScriptID]*BloksLambda
	Payloads   map[BloksPayloadID]*BloksBundleRef
	LocalVars  map[BloksVariableID]*BloksScriptLiteral
	GlobalVars map[BloksVariableID]*BloksScriptLiteral
}

func NewInterpreter(ctx context.Context, b *BloksBundle, br *InterpBridge, old *Interpreter) (*Interpreter, error) {
	p := b.Layout.Payload
	scripts := map[BloksScriptID]*BloksLambda{}
	for id, script := range p.Scripts {
		scripts[id] = &BloksLambda{
			Body: &script.AST,
		}
	}
	payloads := map[BloksPayloadID]*BloksBundleRef{}
	for _, payload := range p.Embedded {
		payloads[payload.ID] = &BloksBundleRef{
			Bundle: &payload.Contents,
		}
	}
	globals := map[BloksVariableID]*BloksScriptLiteral{}
	locals := map[BloksVariableID]*BloksScriptLiteral{}
	for _, item := range p.Variables {
		// Deal with the dynamic variables later
		if item.Info.InitialScript != nil {
			continue
		}
		id := BloksVariableID(item.ID)
		switch item.Type {
		case "gs":
			if old != nil {
				if oldval, ok := old.GlobalVars[id]; ok {
					globals[id] = oldval
					break
				}
			}
			globals[id] = BloksLiteralOf(item.Info.Initial)
		case "ls":
			locals[id] = BloksLiteralOf(item.Info.Initial)
		default:
			return nil, fmt.Errorf("unexpected var type %s", item.Type)
		}
	}
	interp := Interpreter{
		Bridge: *br,

		Scripts:    scripts,
		Payloads:   payloads,
		GlobalVars: globals,
		LocalVars:  locals,
	}
	br = &interp.Bridge
	if br.DeviceID == "" {
		br.DeviceID = strings.ToUpper(uuid.New().String())
	}
	if br.FamilyDeviceID == "" {
		br.FamilyDeviceID = strings.ToUpper(uuid.New().String())
	}
	if br.EncryptPassword == nil {
		br.EncryptPassword = func(pw string) (string, error) {
			return fmt.Sprintf(
				"#PWD_LIGHTSPEED_FAKE:%s",
				base64.StdEncoding.EncodeToString(sha256.New().Sum([]byte(pw))),
			), nil
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
	if br.DoRPC == nil {
		br.DoRPC = func(name string, params map[string]string, isPage bool, callback func(result *BloksScriptLiteral) error) error {
			return fmt.Errorf("unhandled rpc %s (isPage %v)", name, isPage)
		}
	}
	if br.DisplayNewScreen == nil {
		br.DisplayNewScreen = func(name string, bb *BloksBundle) error {
			return fmt.Errorf("unhandled new screen %s", name)
		}
	}
	if br.HandleLoginResponse == nil {
		br.HandleLoginResponse = func(data string) error {
			return fmt.Errorf("unhandled login response")
		}
	}
	if br.StartTimer == nil {
		br.StartTimer = func(name string, interval time.Duration, callback func() error) error {
			return fmt.Errorf("unhandled timer %s", name)
		}
	}
	for _, item := range p.Variables {
		// We already handled the static variables
		if item.Info.InitialScript == nil {
			continue
		}
		value, err := interp.Evaluate(ctx, &item.Info.InitialScript.AST)
		if err != nil {
			return nil, fmt.Errorf("var %s: %w", item.ID, err)
		}
		id := BloksVariableID(item.ID)
		switch item.Type {
		case "gs":
			if old != nil {
				if oldval, ok := old.GlobalVars[id]; ok {
					globals[id] = oldval
					break
				}
			}
			globals[id] = value
		case "ls":
			locals[id] = value
		default:
			return nil, fmt.Errorf("unexpected var type %s", item.Type)
		}
	}
	return &interp, nil
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

func evalTreeProp35(ctx context.Context, i *Interpreter, form *BloksScriptNode, where string) (string, error) {
	make, ok := form.BloksScriptNodeContent.(*BloksScriptFuncall)
	if !ok {
		return "", fmt.Errorf("%s non-funcall %T", where, form.BloksScriptNodeContent)
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

func evalTreeCallback(ctx context.Context, i *Interpreter, form *BloksScriptNode, where string) (*BloksLambda, error) {
	make, ok := form.BloksScriptNodeContent.(*BloksScriptFuncall)
	if !ok {
		return nil, fmt.Errorf("%s non-funcall %T", where, form.BloksScriptNodeContent)
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
	ambientArgs, ok := ctx.Value(interpCtxArgs).([]*BloksScriptLiteral)
	if !ok {
		ambientArgs = make([]*BloksScriptLiteral, maxInterpArgs)
	}
	ambientArgs[0] = BloksLiteralOf(&BloksElemRef{this})
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

func (i *Interpreter) Evaluate(ctx context.Context, form *BloksScriptNode) (*BloksScriptLiteral, error) {
	if lit, ok := form.BloksScriptNodeContent.(*BloksScriptLiteral); ok {
		return lit, nil
	}
	ambientArgs, ok := ctx.Value(interpCtxArgs).([]*BloksScriptLiteral)
	if !ok {
		ambientArgs = make([]*BloksScriptLiteral, maxInterpArgs)
	}
	call, ok := form.BloksScriptNodeContent.(*BloksScriptFuncall)
	if !ok {
		return nil, fmt.Errorf("unexpected script node %T", form.BloksScriptNodeContent)
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
		pass, err = i.Bridge.EncryptPassword(pass)
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(pass), nil
	case "bk.action.textinput.GetText", "bk.action.caa.GetPasswordText":
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
	case "null":
		return i.Evaluate(ctx, &call.Args[0])
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
	case "ig.action.IsDarkModeEnabled":
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
		ref, ok := call.Args[0].BloksScriptNodeContent.(*BloksScriptFuncall)
		if !ok {
			return nil, fmt.Errorf("reading from non-ref %T", call.Args[0].BloksScriptNodeContent)
		}
		if ref.Function != "bk.action.bloks.GetVariable2" {
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
		ref, ok := call.Args[0].BloksScriptNodeContent.(*BloksScriptFuncall)
		if !ok {
			return nil, fmt.Errorf("reading from non-ref %T", call.Args[0].BloksScriptNodeContent)
		}
		if ref.Function != "bk.action.bloks.GetVariable2" {
			return nil, fmt.Errorf("reading from non-ref funcall %s", ref.Function)
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
		err = i.Bridge.DoRPC(name, flatParams, false, func(result *BloksScriptLiteral) error {
			_, err := i.Evaluate(ctx, &BloksScriptNode{
				BloksScriptNodeContent: &BloksScriptFuncall{
					Function: "bk.action.core.Apply",
					Args: []BloksScriptNode{{
						BloksLiteralOf(callback),
					}, {
						result,
					}},
				},
			})
			return err
		})
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.string.JsonEncode":
		arg, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(arg.Flatten(true))
		if err != nil {
			return nil, err
		}
		return BloksLiteralOf(string(encoded)), nil
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
			BloksScriptNodeContent: &BloksScriptFuncall{
				Function: "bk.action.core.Apply",
				Args: []BloksScriptNode{
					call.Args[2],
					{
						BloksScriptNodeContent: &BloksScriptFuncall{
							Function: "bk.action.string.EncryptPassword",
							Args:     []BloksScriptNode{call.Args[0]},
						},
					},
				},
			},
		})
	case "bk.action.caa.HandleLoginResponseForContextChange":
		data, err := evalTreeProp35(ctx, i, &call.Args[0], "handleloginresponse")
		if err != nil {
			return nil, err
		}
		err = i.Bridge.HandleLoginResponse(data)
		if err != nil {
			return nil, err
		}
		return BloksNothing, nil
	case "bk.action.i64.Const":
		return i.Evaluate(ctx, &call.Args[0])
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
		// Ignore the second argument which is a numeric type, for now
		val, err := i.Evaluate(ctx, &call.Args[0])
		if err != nil {
			return nil, err
		}
		return val, nil
	case "bk.action.mins.GetByValOr":
		return i.Evaluate(ctx, &BloksScriptNode{
			BloksScriptNodeContent: &BloksScriptFuncall{
				Function: "bk.action.bool.Or",
				Args: []BloksScriptNode{{
					BloksScriptNodeContent: &BloksScriptFuncall{
						Function: "bk.action.map.Get",
						Args: []BloksScriptNode{
							call.Args[0],
							call.Args[1],
						},
					},
				}, call.Args[2]},
			},
		})
	case "bk.action.fx.OpenSyncScreen", "bk.action.fx.PushSyncScreen":
		name, err := evalTreeProp35(ctx, i, &call.Args[0], "pushscreen")
		if err != nil {
			return nil, err
		}
		bundle, err := evalAs[*BloksBundleRef](ctx, i, &call.Args[1], "opensyncscreen")
		if err != nil {
			return nil, err
		}
		err = i.Bridge.DisplayNewScreen(name, bundle.Bundle)
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
		err = i.Bridge.DoRPC(name, flatParams, true, nil)
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
				BloksScriptNodeContent: &BloksScriptFuncall{
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
		return nil, fmt.Errorf("%s: %s", flow.Error.ErrorUserTitle, flow.Error.ErrorUserMessage)
	case "bk.action.dialog.OpenDialog":
		msg, err := evalTreeProp35(ctx, i, &call.Args[0], "opendialog")
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s", msg)
	case
		"bk.action.animated.Start",
		"bk.action.logging.LogEvent",
		"bk.action.LogFlytrapData",
		"bk.action.qpl.MarkerStartV2",
		"bk.action.qpl.MarkerAnnotate",
		"bk.action.bloks.ClearFocus",
		"bk.action.qpl.MarkerPoint",
		"bk.action.qpl.MarkerEndV2",
		"bk.action.bloks.DismissKeyboard",
		"bk.action.qpl.userflow.MarkPointV2",
		"bk.action.qpl.userflow.EndFlowSuccessV2":
		return BloksNothing, nil
	}
	return nil, fmt.Errorf("unimplemented function %s (%d args)", call.Function, len(call.Args))
}
