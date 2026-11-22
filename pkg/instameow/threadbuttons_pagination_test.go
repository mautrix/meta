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
				fmt.Fprintf(w, `{"status":"ok","thread":{"has_older":true,"oldest_cursor":%q,"items":[{"item_id":"available","item_type":"generic_xma"}]}}`, cursor)
			}))
			defer srv.Close()
			items, err := newPostbackTestClient().fetchThreadButtonItems(context.Background(), srv.URL, "thread", true)
			if err == nil || len(items) != 1 {
				t.Fatal("broken cursor must report error and preserve partial history")
			}
		})
	}
}
