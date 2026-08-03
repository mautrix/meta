package connector

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func TestBusinessSuiteMessengerFilterPrefersInstagramMarkers(t *testing.T) {
	client := &MetaClient{LoginMeta: &metaid.UserLoginMetadata{Platform: types.BusinessSuite}}
	tbl := &table.LSTable{
		LSDeleteThenInsertThread: []*table.LSDeleteThenInsertThread{
			{ThreadKey: 101, SyncGroup: 205},
			{ThreadKey: 202, SyncGroup: 205},
			{ThreadKey: 303, SyncGroup: 127},
		},
		LSDeleteThenInsertIgThreadInfo: []*table.LSDeleteThenInsertIgThreadInfo{
			{ThreadKey: 202, IgThreadId: "instagram-thread"},
		},
	}

	client.rememberBusinessSuiteThreadChannels(tbl)

	if client.shouldIgnoreBusinessSuiteThread(101) {
		t.Fatal("Page Messenger thread in group 205 was incorrectly filtered")
	}
	if !client.shouldIgnoreBusinessSuiteThread(202) {
		t.Fatal("Instagram marker did not override the shared Business Suite sync group")
	}
	if !client.shouldIgnoreBusinessSuiteThread(303) {
		t.Fatal("Instagram group-127 thread was not filtered")
	}
}

func TestBusinessSuiteMessengerFilterDoesNotAffectFacebook(t *testing.T) {
	client := &MetaClient{LoginMeta: &metaid.UserLoginMetadata{Platform: types.Facebook}}
	tbl := &table.LSTable{
		LSDeleteThenInsertThread: []*table.LSDeleteThenInsertThread{
			{ThreadKey: 303, SyncGroup: 205},
		},
		LSDeleteThenInsertIgThreadInfo: []*table.LSDeleteThenInsertIgThreadInfo{
			{ThreadKey: 303, IgThreadId: "instagram-thread"},
		},
	}

	client.rememberBusinessSuiteThreadChannels(tbl)

	if client.shouldIgnoreBusinessSuiteThread(303) {
		t.Fatal("non-Business Suite connection was incorrectly filtered")
	}
}
