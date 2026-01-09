package main

import (
	"context"
	"fmt"
)

type Interpreter struct {
	Vars map[BloksVariableID]BloksJavascriptValue
}

func NewInterpreter(b *BloksBundle) *Interpreter {
	return &Interpreter{}
}

type BloksLambda struct {
	Body *BloksScriptNode
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
			return BloksLiteralOf(i.Vars[BloksVariableID(varname)]), nil
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
			return nil, nil
		}
	case "bk.action.animated.Start":
		return nil, nil
	}
	return nil, fmt.Errorf("unimplemented function %s (%d args)", call.Function, len(call.Args))
}
