package instameow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestThreadButtonHistoryPagination(t *testing.T) {
	for _, allHistory := range []bool{false, true} {
		t.Run(fmt.Sprint(allHistory), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet {
					t.Error("history must only use GET")
				}
				w.Header().Set("Content-Type", "application/json")
				switch calls {
				case 1:
					fmt.Fprint(w, `{"status":"ok","thread":{"has_older":true,"oldest_cursor":"page-2","items":[{"item_id":"new","item_type":"generic_xma"}]}}`)
				case 2:
					if r.URL.Query().Get("cursor") != "page-2" || r.URL.Query().Get("direction") != "older" {
						t.Error("missing older page query")
					}
					fmt.Fprint(w, `{"status":"ok","thread":{"has_older":true,"oldest_cursor":"page-3","items":[{"item_id":"new","item_type":"generic_xma"},{"item_id":"middle","item_type":"generic_xma"}]}}`)
				case 3:
					if r.URL.Query().Get("cursor") != "page-3" {
						t.Error("wrong cursor")
					}
					fmt.Fprint(w, `{"status":"ok","thread":{"has_older":false,"items":[{"item_id":"old","item_type":"generic_xma"}]}}`)
				default:
					t.Error("unexpected history request")
					http.Error(w, "unexpected", 500)
				}
			}))
			defer srv.Close()
			items, err := newPostbackTestClient().fetchThreadButtonItems(context.Background(), srv.URL, "thread", allHistory)
			if err != nil {
				t.Fatal("history fetch failed")
			}
			want := 1
			if allHistory {
				want = 3
			}
			if len(items) != want || calls != want {
				t.Fatalf("got %d items in %d calls, want %d", len(items), calls, want)
			}
			if allHistory && items[2].ItemID != "old" {
				t.Fatal("old card missing")
			}
		})
	}
}

func TestThreadButtonHistoryStopsWhenPageAddsNoItems(t *testing.T) {
	for _, initialItems := range [][]string{{"first"}, {"first", "second", "third"}} {
		t.Run(fmt.Sprint(len(initialItems)), func(t *testing.T) {
			calls := 0
			itemsByPage := make([][]string, 0, len(initialItems)+1)
			for _, id := range initialItems {
				itemsByPage = append(itemsByPage, []string{id})
			}
			itemsByPage = append(itemsByPage, []string{initialItems[len(initialItems)-1]})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet {
					t.Errorf("history request method = %s, want GET", r.Method)
				}
				page := calls - 1
				if page >= len(itemsByPage) {
					page = len(itemsByPage) - 1
				}
				itemJSON := ""
				for i, id := range itemsByPage[page] {
					if i > 0 {
						itemJSON += ","
					}
					itemJSON += fmt.Sprintf(`{"item_id":%q,"item_type":"generic_xma"}`, id)
				}
				w.Header().Set("Content-Type", "application/json")
				hasOlder := calls <= 50
				fmt.Fprintf(w, `{"status":"ok","thread":{"has_older":%t,"oldest_cursor":"cursor-%d","items":[%s]}}`, hasOlder, calls, itemJSON)
			}))
			defer srv.Close()

			items, err := newPostbackTestClient().fetchThreadButtonItems(context.Background(), srv.URL, "thread", true)
			if err != nil {
				t.Fatalf("history fetch failed: %v", err)
			}
			if calls != len(initialItems)+1 {
				t.Fatalf("got %d page requests, want %d", calls, len(initialItems)+1)
			}
			if len(items) != len(initialItems) {
				t.Fatalf("got %d items, want %d", len(items), len(initialItems))
			}
			for i, item := range items {
				if item.ItemID != initialItems[i] {
					t.Errorf("item %d ID = %q, want %q", i, item.ItemID, initialItems[i])
				}
			}
		})
	}
}

func TestThreadButtonHistoryStopsAtPageLimit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet {
			t.Errorf("history request method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","thread":{"has_older":true,"oldest_cursor":"cursor-%d","items":[{"item_id":"item-%d","item_type":"generic_xma"}]}}`, calls, calls)
	}))
	defer srv.Close()

	items, err := newPostbackTestClient().fetchThreadButtonItems(context.Background(), srv.URL, "thread", true)
	if err != nil {
		t.Fatalf("history fetch failed: %v", err)
	}
	if calls != 50 || len(items) != 50 {
		t.Fatalf("got %d page requests and %d items, want 50 each", calls, len(items))
	}
}

func TestThreadHistoryRejectsBrokenCursor(t *testing.T) {
	for _, cursor := range []string{"", "repeated"} {
		t.Run(cursor, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > 2 {
					t.Error("unbounded pagination")
					http.Error(w, "stop", 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				itemID := "available"
				if cursor == "repeated" && calls == 2 {
					itemID = "another"
				}
				fmt.Fprintf(w, `{"status":"ok","thread":{"has_older":true,"oldest_cursor":%q,"items":[{"item_id":%q,"item_type":"generic_xma"}]}}`, cursor, itemID)
			}))
			defer srv.Close()
			items, err := newPostbackTestClient().fetchThreadButtonItems(context.Background(), srv.URL, "thread", true)
			wantItems := 1
			if cursor == "repeated" {
				wantItems = 2
			}
			if err == nil || len(items) != wantItems {
				t.Fatalf("broken cursor must report error and preserve %d items, got %d", wantItems, len(items))
			}
		})
	}
}
