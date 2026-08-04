package httpclient

import "testing"

func TestFindProfileSwitcherDocID(t *testing.T) {
	js := `__d("CometSettingsDropdownListQuery.graphql",[],function(){return{params:{id:"5589011011152787",metadata:{},name:"CometSettingsDropdownListQuery",operationKind:"query",text:null}}})`

	if got := findProfileSwitcherDocID([]byte(js)); got != "5589011011152787" {
		t.Fatalf("expected profile switcher query ID, got %q", got)
	}
}
