package bloks

import (
	"context"
	"testing"
)

// litNode wraps a literal value in a script node so it can be used as a call argument.
func litNode(v any) BloksScriptNode {
	return BloksScriptNode{Content: BloksLiteralOf(v)}
}

// callNode builds a single-argument function call node.
func callNode(fn BloksFunctionID, arg any) *BloksScriptNode {
	return &BloksScriptNode{
		Content: &BloksScriptFuncall{
			Function: fn,
			Args:     []BloksScriptNode{litNode(arg)},
		},
	}
}

// TestI64Convert covers bk.action.i64.Convert, which Meta's two-step verification
// screen calls during the Messenger Lite login flow. Before this was implemented the
// interpreter aborted with "unimplemented function bk.action.i64.Convert (1 args)",
// which broke login immediately after the password was accepted.
func TestI64Convert(t *testing.T) {
	i := &Interpreter{}
	ctx := context.Background()

	tests := []struct {
		name string
		in   any
		want int64
	}{
		{"int64 passthrough", int64(42), 42},
		{"float64 truncates", float64(3.9), 3},
		{"negative float64 truncates toward zero", float64(-3.9), -3},
		{"numeric string", "1234", 1234},
		{"negative numeric string", "-7", -7},
		{"float-formatted string", "3.0", 3},
		{"bool true", true, 1},
		{"bool false", false, 0},
		{"nil is zero", nil, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := i.Evaluate(ctx, callNode("bk.action.i64.Convert", tc.in))
			if err != nil {
				t.Fatalf("Evaluate(%#v) returned error: %v", tc.in, err)
			}
			gotVal, ok := got.Value().(int64)
			if !ok {
				t.Fatalf("Evaluate(%#v) returned %T, want int64", tc.in, got.Value())
			}
			if gotVal != tc.want {
				t.Errorf("Evaluate(%#v) = %d, want %d", tc.in, gotVal, tc.want)
			}
		})
	}
}

func TestI64ConvertRejectsNonNumericString(t *testing.T) {
	i := &Interpreter{}
	if _, err := i.Evaluate(context.Background(), callNode("bk.action.i64.Convert", "abc")); err == nil {
		t.Fatal("expected an error converting a non-numeric string, got nil")
	}
}
