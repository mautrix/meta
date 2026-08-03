package httpclient

import (
	"encoding/json"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func TestBusinessInboxLightspeedResponseShape(t *testing.T) {
	raw := json.RawMessage(`{"data":{"viewer":{"unified_inbox_lightspeed_web_request":{"payload":"{\"name\":null,\"step\":[1]}","dependencies":[]}}}}`)
	var response graphql.LSPlatformGraphQLLightspeedRequestQuery
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Viewer.UnifiedInboxLightspeedWebRequest == nil {
		t.Fatal("Business Inbox Lightspeed payload was not decoded")
	}
}

func TestBusinessInboxPreloaderIsRoutedToLightspeedDecoder(t *testing.T) {
	parser := &ModuleParser{LS: &table.LSTable{}}
	raw := json.RawMessage(`{"data":{"viewer":{"unified_inbox_lightspeed_web_request":{"payload":"not-json","dependencies":[]}}}}`)
	if err := parser.handleLightSpeedQLRequest(raw, "LSBizInboxGraphQLLightspeedRequestQuery"); err == nil {
		t.Fatal("Business Inbox preloader was ignored instead of being routed to the Lightspeed decoder")
	}
}

func TestBusinessInboxPreloaderNameParsing(t *testing.T) {
	parser := &ModuleParser{}
	got := parser.parseGraphMethodName("adp_LSBizInboxGraphQLLightspeedRequestQueryRelayPreloader_fixture")
	if got != "LSBizInboxGraphQLLightspeedRequestQuery" {
		t.Fatalf("parsed name = %q", got)
	}
}

func TestBusinessInboxHashedRelayModuleIsHandled(t *testing.T) {
	parser := &ModuleParser{LS: &table.LSTable{}}
	entry := &ModuleEntry{
		Name: "RelayPrefetchedStreamCache@fixturehash",
		Data: []json.RawMessage{
			json.RawMessage(`null`),
			json.RawMessage(`null`),
			json.RawMessage(`["adp_LSBizInboxGraphQLLightspeedRequestQueryRelayPreloader_fixture",{"__bbox":{"complete":true,"result":{"data":{"viewer":{"unified_inbox_lightspeed_web_request":{"payload":"{\"name\":null,\"step\":[1]}","dependencies":[]}}}}}}]`),
		},
	}
	if err := parser.handleRequire(entry); err != nil {
		t.Fatal(err)
	}
	if entry.Name != "RelayPrefetchedStreamCache" {
		t.Fatalf("module name = %q", entry.Name)
	}
}
