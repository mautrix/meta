package bloks

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.mau.fi/util/random"
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
	DoRPC               func(name string, params map[string]string) error
	HandleLoginResponse func(data string) error
	DisplayNewScreen    func(*BloksBundle) error
}

type Interpreter struct {
	Bridge InterpBridge

	Scripts  map[BloksScriptID]*BloksLambda
	Payloads map[BloksPayloadID]*BloksBundleRef
	Vars     map[BloksVariableID]*BloksScriptLiteral
}

func NewInterpreter(b *BloksBundle, br *InterpBridge) *Interpreter {
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
	vars := map[BloksVariableID]*BloksScriptLiteral{}
	for _, item := range p.Variables {
		vars[BloksVariableID(item.ID)] = BloksLiteralOf(item.Info.Initial)
	}
	interp := Interpreter{
		Bridge: *br,

		Scripts:  scripts,
		Payloads: payloads,
		Vars:     vars,
	}
	br = &interp.Bridge
	if br.DeviceID == "" {
		br.DeviceID = strings.ToUpper(uuid.New().String())
	}
	if br.FamilyDeviceID == "" {
		br.FamilyDeviceID = strings.ToUpper(uuid.New().String())
	}
	if br.MachineID == "" {
		br.MachineID = string(random.StringBytes(25))
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
		br.DoRPC = func(name string, params map[string]string) error {
			return fmt.Errorf("unhandled rpc %s", name)
		}
	}
	if br.HandleLoginResponse == nil {
		br.HandleLoginResponse = func(data string) error {
			return fmt.Errorf("unhandled login response")
		}
	}
	if br.DisplayNewScreen == nil {
		br.DisplayNewScreen = func(bb *BloksBundle) error {
			return fmt.Errorf("unhandled new screen")
		}
	}
	return &interp
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

const maxInterpArgs = 100

func InterpBindThis(ctx context.Context, this *BloksTreeComponent) context.Context {
	ambientArgs, ok := ctx.Value(interpCtxArgs).([]*BloksScriptLiteral)
	if !ok {
		ambientArgs = make([]*BloksScriptLiteral, maxInterpArgs)
	}
	ambientArgs[0] = BloksLiteralOf(&BloksElemRef{this})
	return context.WithValue(ctx, interpCtxArgs, ambientArgs)
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
		varname, err := evalAs[string](ctx, i, &call.Args[0], "getvar")
		if err != nil {
			return nil, err
		}
		value, ok := i.Vars[BloksVariableID(varname)]
		if !ok {
			return BloksNull, nil
		}
		return value, nil
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
		varname, err := evalAs[string](ctx, i, &call.Args[0], "getvar")
		if err != nil {
			return nil, err
		}
		value, err := i.Evaluate(ctx, &call.Args[1])
		if err != nil {
			return nil, err
		}
		i.Vars[BloksVariableID(varname)] = value
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
		value, ok := i.Vars[BloksVariableID(varname)]
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
		i.Vars[BloksVariableID(varname)] = value
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
		err = i.Bridge.DoRPC(name, flatParams)
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
		newArgs, err := evalAs[[]*BloksScriptLiteral](ctx, i, &call.Args[0], "bind")
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
		make, ok := call.Args[0].BloksScriptNodeContent.(*BloksScriptFuncall)
		if !ok {
			return nil, fmt.Errorf("handleloginresponse non-funcall %T", call.Args[0].BloksScriptNodeContent)
		}
		if make.Function != "bk.action.tree.Make" {
			return nil, fmt.Errorf("handleloginresponse non-tree funcall %s", make.Function)
		}
		if len(make.Args)%2 != 1 {
			return nil, fmt.Errorf("tree.make even number of args %d", len(make.Args))
		}
		for idx := 1; idx < len(make.Args); idx += 2 {
			attr, err := evalAs[int64](ctx, i, &make.Args[idx], "tree.make")
			if err != nil {
				return nil, err
			}
			if attr != 35 {
				continue
			}
			data, err := evalAs[string](ctx, i, &make.Args[idx+1], "tree.make")
			if err != nil {
				return nil, err
			}
			err = i.Bridge.HandleLoginResponse(data)
			if err != nil {
				return nil, err
			}
			return BloksNothing, nil
		}
		return nil, fmt.Errorf("no prop 35 in handleloginresponse tree")
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
	case "bk.action.fx.OpenSyncScreen":
		bundle, err := evalAs[*BloksBundleRef](ctx, i, &call.Args[1], "opensyncscreen")
		if err != nil {
			return nil, err
		}
		err = i.Bridge.DisplayNewScreen(bundle.Bundle)
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
	case
		"bk.action.animated.Start",
		"bk.action.logging.LogEvent",
		"bk.action.LogFlytrapData",
		"bk.action.qpl.MarkerStartV2",
		"bk.action.qpl.MarkerAnnotate",
		"bk.action.bloks.WriteGlobalConsistencyStore",
		"bk.action.bloks.ClearFocus",
		"bk.action.qpl.MarkerPoint":
		return BloksNothing, nil
	}
	return nil, fmt.Errorf("unimplemented function %s (%d args)", call.Function, len(call.Args))
}
