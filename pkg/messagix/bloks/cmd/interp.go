package main

import (
	"context"
	"fmt"
)

type Interpreter struct {
	DeviceID        string
	FamilyDeviceID  string
	MachineID       string
	EncryptPassword func(string) string

	Scripts map[BloksScriptID]*BloksLambda
	Vars    map[BloksVariableID]*BloksScriptLiteral
}

func NewInterpreter(b *BloksBundle) *Interpreter {
	scripts := map[BloksScriptID]*BloksLambda{}
	for id, script := range b.Layout.Payload.Scripts {
		scripts[id] = &BloksLambda{
			Body: &script.AST,
		}
	}
	return &Interpreter{
		// some temporary values for testing
		DeviceID:       "571CEE83-37A1-46D3-860D-B83398943DF7",
		FamilyDeviceID: "f121bbd7-7b3f-412f-b664-cf33451f4471",
		MachineID:      "",
		EncryptPassword: func(pw string) string {
			return "encrypted:" + pw
		},

		Scripts: scripts,
		Vars:    map[BloksVariableID]*BloksScriptLiteral{},
	}
}

type BloksLambda struct {
	Body *BloksScriptNode
}

type BloksElemRef struct {
	Component *BloksTreeComponent
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
		return zero, fmt.Errorf("expected %T in %s, got %T %q", zero, where, val.Value(), val.Value())
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
	switch call.Function {
	case "bk.action.core.If":
		{
			cond, err := i.Evaluate(ctx, &call.Args[0])
			if err != nil {
				return nil, err
			}
			if cond.IsTruthy() {
				return i.Evaluate(ctx, &call.Args[1])
			}
			return i.Evaluate(ctx, &call.Args[2])
		}
	case "bk.action.bool.Or":
		{
			first, err := i.Evaluate(ctx, &call.Args[0])
			if err != nil {
				return nil, err
			}
			if first.IsTruthy() {
				return first, nil
			}
			return i.Evaluate(ctx, &call.Args[1])
		}
	case "bk.action.bloks.GetVariable2":
		{
			varname, err := evalAs[string](ctx, i, &call.Args[0], "getvar")
			if err != nil {
				return nil, err
			}
			return i.Vars[BloksVariableID(varname)], nil
		}
	case "bk.action.core.TakeLast":
		{
			var result *BloksScriptLiteral
			var err error
			for _, arg := range call.Args {
				result, err = i.Evaluate(ctx, &arg)
				if err != nil {
					return nil, err
				}
			}
			return result, nil
		}
	case "bk.action.core.Apply":
		{
			fn, err := evalAs[*BloksLambda](ctx, i, &call.Args[0], "apply")
			if err != nil {
				return nil, err
			}
			newArgs := make([]*BloksScriptLiteral, maxInterpArgs)
			for idx := 0; idx < len(call.Args)-1; idx++ {
				result, err := i.Evaluate(ctx, &call.Args[idx+1])
				if err != nil {
					return nil, err
				}
				newArgs[idx] = result
			}
			ctx := context.WithValue(ctx, interpCtxArgs, newArgs)
			return i.Evaluate(ctx, fn.Body)
		}
	case "bk.action.core.FuncConst":
		{
			return BloksLiteralOf(&BloksLambda{&call.Args[0]}), nil
		}
	case "bk.action.core.GetArg":
		{
			idx, err := evalAs[int64](ctx, i, &call.Args[0], "getarg")
			if err != nil {
				return nil, err
			}
			return ambientArgs[idx], nil
		}
	case "bk.action.core.SetArg":
		{
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
		}
	case "bk.action.f32.Eq":
		{
			first, err := i.Evaluate(ctx, &call.Args[0])
			if err != nil {
				return nil, err
			}
			second, err := i.Evaluate(ctx, &call.Args[1])
			if err != nil {
				return nil, err
			}
			return BloksLiteralOf(first == second), nil
		}
	case "bk.action.bloks.GetScript":
		{
			name, err := evalAs[string](ctx, i, &call.Args[0], "getscript")
			if err != nil {
				return nil, err
			}
			script := i.Scripts[BloksScriptID(name)]
			if script == nil {
				return nil, fmt.Errorf("no such script %q", name)
			}
			return BloksLiteralOf(script), nil
		}
	case "bk.action.bloks.WriteLocalState":
		{
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
		}
	case "bk.action.array.Make":
		{
			results := []*BloksScriptLiteral{}
			for _, arg := range call.Args {
				result, err := i.Evaluate(ctx, &arg)
				if err != nil {
					return nil, err
				}
				results = append(results, result)
			}
			return BloksLiteralOf(results), nil
		}
	case "bk.action.map.Make":
		{
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
					return nil, fmt.Errorf("non-string key %T %q", first[0].Value(), first[0].Value())
				}
				result[key] = second[idx]
			}
			return BloksLiteralOf(result), nil
		}
	case "bk.action.caa.login.GetUniqueDeviceId":
		{
			return BloksLiteralOf(i.DeviceID), nil
		}
	case "bk.fx.action.GetFamilyDeviceId":
		{
			return BloksLiteralOf(i.FamilyDeviceID), nil
		}
	case "bk.action.caa.FetchMachineID":
		{
			return BloksLiteralOf(i.MachineID), nil
		}
	case "bk.action.string.EncryptPassword":
		{
			pass, err := evalAs[string](ctx, i, &call.Args[0], "encryptpassword")
			if err != nil {
				return nil, err
			}
			return BloksLiteralOf(pass), nil
		}
	case "bk.action.textinput.GetText", "bk.action.caa.GetPasswordText":
		{
			ref, err := evalAs[*BloksElemRef](ctx, i, &call.Args[0], "gettext")
			if err != nil {
				return nil, err
			}
			text := ref.Component.textContent
			if text == nil {
				return nil, fmt.Errorf("no text content in referenced element")
			}
			return BloksLiteralOf(*text), nil
		}
	case "bk.action.bool.Not":
		{
			arg, err := i.Evaluate(ctx, &call.Args[0])
			if err != nil {
				return nil, err
			}
			return BloksLiteralOf(!arg.IsTruthy()), nil
		}
	case "null":
		{
			return i.Evaluate(ctx, &call.Args[0])
		}
	case "bk.action.mins.CallRuntime":
		{
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
		}
	case "bk.action.array.Put", "bk.action.mins.PutByVal":
		{
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
		}
	case "ig.action.IsDarkModeEnabled":
		{
			return BloksLiteralOf(false), nil
		}
	case "bk.action.mins.InByVal":
		{
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
		}
	case
		"bk.action.animated.Start",
		"bk.action.logging.LogEvent",
		"bk.action.LogFlytrapData",
		"bk.action.qpl.MarkerStartV2",
		"bk.action.qpl.MarkerAnnotate",
		"bk.action.bloks.WriteGlobalConsistencyStore",
		"bk.action.bloks.ClearFocus":
		return BloksNothing, nil
	}
	return nil, fmt.Errorf("unimplemented function %s (%d args)", call.Function, len(call.Args))
}
