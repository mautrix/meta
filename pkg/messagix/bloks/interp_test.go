package bloks

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestIncomingCallRetrieverEligibilityIsFalse(t *testing.T) {
	unminifier, err := GetUnminifier(&BloksBundle{})
	if err != nil {
		t.Fatalf("failed to load Bloks unminifier: %v", err)
	}
	const function = BloksFunctionID("bk.action.gms.flashcall.IncomingCallRetrieverEligibilityChecker")
	if got := unminifier.Functions["i5f"]; got != function {
		t.Fatalf("i5f unminified to %q instead of %q", got, function)
	}

	var action BloksScriptNode
	if _, err = action.ParseAny(`(i5f null)`, 0); err != nil {
		t.Fatalf("failed to parse i5f action: %v", err)
	}
	action.Content.Unminify(unminifier)
	result, err := (&Interpreter{}).Evaluate(context.Background(), &action)
	if err != nil {
		t.Fatalf("i5f evaluation returned error: %v", err)
	}
	eligible, ok := result.Value().(bool)
	if !ok {
		t.Fatalf("i5f returned %T instead of bool", result.Value())
	}
	if eligible {
		t.Fatal("bridge must not claim GMS incoming-call retriever eligibility")
	}
}

func TestQPLMarkerIsNotActive(t *testing.T) {
	unminifier, err := GetUnminifier(&BloksBundle{})
	if err != nil {
		t.Fatalf("failed to load Bloks unminifier: %v", err)
	}
	const function = BloksFunctionID("bk.action.qpl.IsMarkerOn")
	if got := unminifier.Functions["igo"]; got != function {
		t.Fatalf("igo unminified to %q instead of %q", got, function)
	}

	var action BloksScriptNode
	if _, err = action.ParseAny(`(igo 123 456)`, 0); err != nil {
		t.Fatalf("failed to parse IsMarkerOn action: %v", err)
	}
	action.Content.Unminify(unminifier)
	result, err := (&Interpreter{}).Evaluate(context.Background(), &action)
	if err != nil {
		t.Fatalf("IsMarkerOn evaluation returned error: %v", err)
	}
	active, ok := result.Value().(bool)
	if !ok {
		t.Fatalf("IsMarkerOn returned %T instead of bool", result.Value())
	}
	if active {
		t.Fatal("QPL marker cannot be active when marker actions are no-ops")
	}
}

func TestCurrentTimeMillisReturnsUnixMilliseconds(t *testing.T) {
	unminifier, err := GetUnminifier(&BloksBundle{})
	if err != nil {
		t.Fatalf("failed to load Bloks unminifier: %v", err)
	}
	const function = BloksFunctionID("bk.action.io.CurrentTimeMillis")
	if got := unminifier.Functions["f2g"]; got != function {
		t.Fatalf("f2g unminified to %q instead of %q", got, function)
	}

	var action BloksScriptNode
	if _, err = action.ParseAny(`(f2g)`, 0); err != nil {
		t.Fatalf("failed to parse CurrentTimeMillis action: %v", err)
	}
	action.Content.Unminify(unminifier)
	earliest := time.Now().UnixMilli()
	result, err := (&Interpreter{}).Evaluate(context.Background(), &action)
	latest := time.Now().UnixMilli()
	if err != nil {
		t.Fatalf("CurrentTimeMillis evaluation returned error: %v", err)
	}
	millis, ok := result.Value().(int64)
	if !ok {
		t.Fatalf("CurrentTimeMillis returned %T instead of int64", result.Value())
	}
	if millis < earliest || millis > latest {
		t.Fatalf("CurrentTimeMillis returned %d outside call interval [%d, %d]", millis, earliest, latest)
	}
}

func TestInstagramGetSecureNoncesMatchesCleanInstallation(t *testing.T) {
	var action BloksScriptNode
	if _, err := action.ParseAny(
		`(bk.action.ig.protection.GetSecureNonces "synthetic-user-key")`,
		0,
	); err != nil {
		t.Fatalf("failed to parse GetSecureNonces action: %v", err)
	}
	var requestedUserKey string
	interp := &Interpreter{Bridge: InterpBridge{
		GetSecureNoncesForUser: func(userKey string) any {
			requestedUserKey = userKey
			return nil
		},
	}}
	result, err := interp.Evaluate(context.Background(), &action)
	if err != nil {
		t.Fatalf("GetSecureNonces evaluation returned error: %v", err)
	}
	if requestedUserKey != "synthetic-user-key" {
		t.Fatalf("GetSecureNonces received unexpected user key %q", requestedUserKey)
	}
	if result.Value() != nil {
		t.Fatalf("clean-installation GetSecureNonces returned %#v instead of null", result.Value())
	}
}

func TestGenericNumberAdd(t *testing.T) {
	unminifier, err := GetUnminifier(&BloksBundle{})
	if err != nil {
		t.Fatalf("failed to load Bloks unminifier: %v", err)
	}
	const function = BloksFunctionID("bk.action.num.Add")
	if got := unminifier.Functions["jmu"]; got != function {
		t.Fatalf("jmu unminified to %q instead of %q", got, function)
	}

	tests := []struct {
		script   string
		expected any
	}{
		{script: `(jmu 2 3)`, expected: int64(5)},
		{script: `(jmu 1.25 2)`, expected: 3.25},
		{script: `(jmu true 2)`, expected: int64(3)},
	}
	for _, test := range tests {
		var action BloksScriptNode
		if _, err = action.ParseAny(test.script, 0); err != nil {
			t.Fatalf("failed to parse %s: %v", test.script, err)
		}
		action.Content.Unminify(unminifier)
		result, evalErr := (&Interpreter{}).Evaluate(context.Background(), &action)
		if evalErr != nil {
			t.Fatalf("%s evaluation returned error: %v", test.script, evalErr)
		}
		if !reflect.DeepEqual(result.Value(), test.expected) {
			t.Fatalf("%s returned %#v, expected %#v", test.script, result.Value(), test.expected)
		}
	}
}

func TestGenericNumberSub(t *testing.T) {
	unminifier, err := GetUnminifier(&BloksBundle{})
	if err != nil {
		t.Fatalf("failed to load Bloks unminifier: %v", err)
	}
	const function = BloksFunctionID("bk.action.num.Sub")
	if got := unminifier.Functions["jn3"]; got != function {
		t.Fatalf("jn3 unminified to %q instead of %q", got, function)
	}

	tests := []struct {
		script   string
		expected any
	}{
		{script: `(jn3 5 3)`, expected: int64(2)},
		{script: `(jn3 1.25 2)`, expected: -0.75},
		{script: `(jn3 true 2)`, expected: int64(-1)},
	}
	for _, test := range tests {
		var action BloksScriptNode
		if _, err = action.ParseAny(test.script, 0); err != nil {
			t.Fatalf("failed to parse %s: %v", test.script, err)
		}
		action.Content.Unminify(unminifier)
		result, evalErr := (&Interpreter{}).Evaluate(context.Background(), &action)
		if evalErr != nil {
			t.Fatalf("%s evaluation returned error: %v", test.script, evalErr)
		}
		if !reflect.DeepEqual(result.Value(), test.expected) {
			t.Fatalf("%s returned %#v, expected %#v", test.script, result.Value(), test.expected)
		}
	}
}

func TestInstagramNativeDialog(t *testing.T) {
	var action BloksScriptNode
	_, err := action.ParseAny(`(ig.action.cdsdialog.OpenDialog (bk.action.tree.Make 0 40 "Confirm login" 35 "Continue this Instagram login?" 36 (bk.action.tree.Make 0 36 "Continue" 35 (bk.action.core.FuncConst (bk.action.bloks.WriteGlobalConsistencyStore "dialog_selected" "yes"))) 38 (bk.action.tree.Make 0 36 "Cancel")) null)`, 0)
	if err != nil {
		t.Fatalf("failed to parse Instagram dialog action: %v", err)
	}

	var dialog *BloksDialog
	var changed bool
	interp := &Interpreter{
		Bridge: InterpBridge{
			OpenDialog: func(_ context.Context, opened *BloksDialog) error {
				dialog = opened
				return nil
			},
			HandleVariableChange: func(_ context.Context, name string, value *BloksScriptLiteral) error {
				changed = name == "dialog_selected" && value.Value() == "yes"
				return nil
			},
		},
		LocalVars:  map[BloksVariableID]*BloksScriptLiteral{},
		GlobalVars: map[BloksVariableID]*BloksScriptLiteral{},
	}
	result, err := interp.Evaluate(context.Background(), &action)
	if err != nil {
		t.Fatalf("Instagram dialog evaluation returned error: %v", err)
	}
	if result != BloksNothing {
		t.Fatalf("Instagram dialog returned %#v instead of BloksNothing", result)
	}
	if dialog == nil {
		t.Fatal("Instagram dialog was not passed to the bridge")
	}
	if dialog.Title != "Confirm login" || dialog.Message != "Continue this Instagram login?" {
		t.Fatalf("unexpected dialog copy: %#v", dialog)
	}
	if len(dialog.Buttons) != 2 {
		t.Fatalf("unexpected dialog button count %d", len(dialog.Buttons))
	}
	if dialog.Buttons[0].Label != "Continue" || dialog.Buttons[0].Role != "positive" {
		t.Fatalf("unexpected positive dialog button: %#v", dialog.Buttons[0])
	}
	if dialog.Buttons[0].Callback == nil {
		t.Fatal("positive dialog callback was not retained")
	}
	if err = dialog.Buttons[0].Callback(context.Background()); err != nil {
		t.Fatalf("positive dialog callback returned error: %v", err)
	}
	if !changed {
		t.Fatal("positive dialog callback did not execute its Bloks action")
	}
}

func TestInventoryPureFunctions(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		expected any
	}{
		{
			name:     "subtract",
			script:   `(bk.action.f32.Sub 7.5 2.0)`,
			expected: 5.5,
		},
		{
			name:     "concat",
			script:   `(bk.action.string.Concat "insta" "gram")`,
			expected: "instagram",
		},
		{
			name:     "join",
			script:   `(bk.action.string.Join (bk.action.array.Make "a" "b") "-")`,
			expected: "a-b",
		},
		{
			name:     "number",
			script:   `(bk.action.string.ValueOfNumber 42)`,
			expected: "42",
		},
		{
			name:     "filter",
			script:   `(bk.action.array.Filter (bk.action.array.Make 1 2 3) (bk.action.core.FuncConst (bk.action.f32.Gt (bk.action.core.GetArg 0) 1)))`,
			expected: []*BloksScriptLiteral{BloksLiteralOf(int64(2)), BloksLiteralOf(int64(3))},
		},
		{
			name:     "match",
			script:   `(bk.action.core.Match "b" (bk.action.array.Make (bk.action.core.Pattern "a" (bk.action.core.FuncConst "A")) (bk.action.core.Pattern "b" (bk.action.core.FuncConst "B"))) (bk.action.core.Default (bk.action.core.FuncConst "fallback")))`,
			expected: "B",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var action BloksScriptNode
			if _, err := action.ParseAny(test.script, 0); err != nil {
				t.Fatalf("failed to parse action: %v", err)
			}
			result, err := (&Interpreter{}).Evaluate(context.Background(), &action)
			if err != nil {
				t.Fatalf("evaluation returned error: %v", err)
			}
			if !reflect.DeepEqual(result.Value(), test.expected) {
				t.Fatalf("got %#v, expected %#v", result.Value(), test.expected)
			}
		})
	}
}
