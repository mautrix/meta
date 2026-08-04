package connector

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestBusinessSuiteAlwaysScopesPortalsToTheSelectedPage(t *testing.T) {
	if !shouldScopeFBPortal(types.BusinessSuite, false, table.GROUP_THREAD) {
		t.Fatal("Business Suite group-like threads must remain scoped to the selected Page")
	}
	if shouldScopeFBPortal(types.Facebook, false, table.GROUP_THREAD) {
		t.Fatal("personal shared portals should retain the existing behavior")
	}
}
