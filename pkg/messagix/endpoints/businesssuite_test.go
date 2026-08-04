package endpoints

import (
	"strings"
	"testing"
)

func TestBusinessSuiteMessengerEndpointsDoNotOpenUnifiedInbox(t *testing.T) {
	endpoints := MakeBusinessSuiteEndpoints("12345")
	for _, name := range []string{"messages", "thread"} {
		if !strings.Contains(endpoints[name], "/latest/inbox/messenger") {
			t.Errorf("%s endpoint = %q, want Messenger-only Business Suite route", name, endpoints[name])
		}
		if strings.Contains(endpoints[name], "/latest/inbox/all") {
			t.Errorf("%s endpoint must not open the unified inbox", name)
		}
	}
}
