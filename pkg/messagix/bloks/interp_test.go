package bloks

import (
	"context"
	"testing"
)

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
