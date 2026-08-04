package messagix

import (
	"context"
	"reflect"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestBusinessSuiteInitialSyncNeverUsesPersonalMailboxDatabases(t *testing.T) {
	got := initialSyncDatabases(types.BusinessSuite)
	want := []int64{2, 26}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("initial databases = %v, want %v", got, want)
	}
	for _, database := range got {
		if database == 1 || database == 95 {
			t.Fatalf("personal mailbox database %d must not be used by Business Suite", database)
		}
	}
}

func TestRequestedBusinessSuiteThreadResponseIsDispatched(t *testing.T) {
	client := &Client{}
	var received any
	client.SetEventHandler(func(_ context.Context, event any) {
		received = event
	})
	response := &PublishResponseData{Payload: `{"name":null,"step":[1]}`}

	if err := client.dispatchRequestedTable(context.Background(), response); err != nil {
		t.Fatalf("dispatch response: %v", err)
	}
	if _, ok := received.(*table.LSTable); !ok {
		t.Fatalf("received event type %T, want *table.LSTable", received)
	}
}

func TestBusinessSuiteDoesNotUseGenericMessengerGraphQLBootstrap(t *testing.T) {
	if shouldBootstrapViaGraphQL(types.BusinessSuite) {
		t.Fatal("Business Suite must not bootstrap personal Messenger databases through GraphQL")
	}
	if !shouldBootstrapViaGraphQL(types.Facebook) {
		t.Fatal("Facebook should retain its existing GraphQL bootstrap")
	}
}

func TestBusinessSuiteInitialThreadTasksMatchCapturedPageInboxProtocol(t *testing.T) {
	tasks := businessSuiteInitialThreadTasks("messenger-cursor")
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1 Messenger-only task", len(tasks))
	}
	want := []struct {
		group     int
		secondary int
		cursor    string
	}{{205, 24, "messenger-cursor"}}
	for i, task := range tasks {
		if task.GetLabel() != socket.BusinessInboxFetchThreadsLabel {
			t.Errorf("task %d label = %q", i, task.GetLabel())
		}
		payload, queue := task.Create()
		if queue != "trq" {
			t.Errorf("task %d queue = %q", i, queue)
		}
		request := payload.(*socket.FetchBusinessInboxThreadsTask)
		if request.SyncGroup != want[i].group || request.SecondaryFilter != want[i].secondary {
			t.Errorf("task %d route = (%d, %d), want (%d, %d)", i, request.SyncGroup, request.SecondaryFilter, want[i].group, want[i].secondary)
		}
		if request.Cursor != want[i].cursor {
			t.Errorf("task %d cursor = %q, want %q", i, request.Cursor, want[i].cursor)
		}
		if request.ReferenceThreadKey != 0 || request.ReferenceActivityTimestamp != 9999999999999 {
			t.Errorf("task %d does not start at the beginning of the Page inbox", i)
		}
	}
}
