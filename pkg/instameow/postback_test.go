// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package instameow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

func TestSendItemPostbackJSON(t *testing.T) {
	body, err := json.Marshal(newSendItemPostback("thread-1", "Yes please", "1:GT-PB:ACT::abcdef", "ctx-1"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	// The first six keys are the schema documented in issue #338, the last two are the
	// thread_id/client_context envelope used by other /ig_send_message payloads.
	want := map[string]any{
		"action":           "send_item",
		"item_type":        "text",
		"text":             "Yes please",
		"send_attribution": "business_generic_template",
		"cta_type":         "postback",
		"postback_payload": "1:GT-PB:ACT::abcdef",
		"thread_id":        "thread-1",
		"client_context":   "ctx-1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected postback body:\n got: %v\nwant: %v", got, want)
	}
}

func TestSendPostbackValidatesInput(t *testing.T) {
	c := newPostbackTestClient()
	for _, args := range [][3]string{{"", "t", "p"}, {"id", "", "p"}, {"id", "t", ""}} {
		if _, err := c.SendPostback(context.Background(), args[0], args[1], args[2]); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

func TestThreadDetailsURL(t *testing.T) {
	got := ThreadDetailsURL("https://www.instagram.com", "123456")
	if want := "https://www.instagram.com/api/v1/direct_v2/threads/123456/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := ThreadDetailsURL("https://www.instagram.com", "a/b"); got != "https://www.instagram.com/api/v1/direct_v2/threads/a%2Fb/" {
		t.Fatalf("thread ID wasn't escaped: %q", got)
	}
}

func newPostbackTestClient() *Client {
	c := &cookies.Cookies{Platform: types.Instagram}
	c.UpdateValues(map[cookies.MetaCookieName]string{cookies.IGCookieSessionID: "test-session"})
	return NewClient(ClientParams{Cookies: c, Log: zerolog.Nop()})
}

func TestFetchThreadButtonItems(t *testing.T) {
	var lock sync.Mutex
	var requests []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		requests = append(requests, r)
		lock.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","thread":{"items":[
			{"item_id":"1","item_type":"text"},
			{"item_id":"2","item_type":"generic_xma","cta_buttons":[{"cta_type":"postback","title":"Yes","payload":"1:GT-PB:ACT::ff"}]}]}}`))
	}))
	defer srv.Close()

	c := newPostbackTestClient()
	items, err := c.fetchThreadButtonItems(context.Background(), srv.URL, "98765", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AllCTAButtons()[0].Postback() != "1:GT-PB:ACT::ff" {
		t.Fatalf("unexpected items: %+v", items)
	}
	if len(requests) != 1 {
		t.Fatalf("expected exactly one request, got %d", len(requests))
	}
	req := requests[0]
	if req.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", req.Method)
	}
	if req.URL.Path != "/api/v1/direct_v2/threads/98765/" {
		t.Errorf("unexpected path %q", req.URL.Path)
	}
	for _, r := range requests {
		if r.URL.Path == "/api/v1/direct_v2/threads/broadcast/text/" || r.Method == http.MethodPost {
			t.Errorf("must never call the broadcast POST: %s %s", r.Method, r.URL.Path)
		}
	}
	if got := req.Header.Get("User-Agent"); got != useragent.IGAndroidUserAgent {
		t.Errorf("unexpected user agent %q", got)
	}
	if got := req.Header.Get("X-IG-App-ID"); got != "567067343352427" {
		t.Errorf("unexpected app ID %q", got)
	}
	if useragent.IGAndroidAppID != "567067343352427" {
		t.Errorf("unexpected IGAndroidAppID constant")
	}
	if req.Header.Get("Cookie") == "" {
		t.Errorf("session cookies weren't sent")
	}
}

func TestFetchThreadTriesMobileHostFirst(t *testing.T) {
	c := newPostbackTestClient()
	var seen string
	c.http.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = r.Method + " " + r.URL.String()
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       http.NoBody,
			Request:    r,
		}, nil
	})
	_, _ = c.FetchThreadButtonItems(context.Background(), "42")
	if want := "GET https://i.instagram.com/api/v1/direct_v2/threads/42/"; seen != want {
		t.Fatalf("got %q, want %q", seen, want)
	}
}

func TestThreadDetailsHeadersAreCleanAndroid(t *testing.T) {
	h := newPostbackTestClient().threadDetailsHeaders()
	for _, name := range []string{"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "x-fb-lsd", "dpr", "viewport-width"} {
		if h.Get(name) != "" {
			t.Errorf("header %s must not be sent", name)
		}
	}
	if got := h.Get("User-Agent"); got != useragent.IGAndroidUserAgent {
		t.Errorf("unexpected user agent %q", got)
	}
	if got := h.Get("X-IG-App-ID"); got != useragent.IGAndroidAppID {
		t.Errorf("unexpected app ID %q", got)
	}
	if len(h.Values("Authorization")) == 0 {
		t.Errorf("Authorization header missing")
	}
	if h.Get("Cookie") == "" {
		t.Errorf("Cookie header missing")
	}
}

func TestFetchThreadFallsBackToWebHost(t *testing.T) {
	c := newPostbackTestClient()
	var seen []string
	c.http.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.Method+" "+r.URL.String())
		status, body := 404, `{"message":"nope"}`
		if r.URL.Host == "www.instagram.com" {
			status, body = 200, `{"status":"ok","thread":{"items":[]}}`
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})
	if _, err := c.FetchThreadButtonItems(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET https://i.instagram.com/api/v1/direct_v2/threads/42/",
		"GET https://www.instagram.com/api/v1/direct_v2/threads/42/",
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("got %v, want %v", seen, want)
	}
}

func TestSendQuickReplyBody(t *testing.T) {
	var got map[string]any
	marshal := func(payload string) map[string]any {
		raw, err := json.Marshal(newSendItemQuickReply("thread-1", "Maybe later", payload, "ctx-1"))
		if err != nil {
			t.Fatal(err)
		}
		got = nil
		if err = json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	titleOnly := marshal("")
	want := map[string]any{"action": "send_item", "item_type": "text", "text": "Maybe later", "thread_id": "thread-1", "client_context": "ctx-1"}
	if !reflect.DeepEqual(titleOnly, want) {
		t.Errorf("title-only body = %v, want %v", titleOnly, want)
	}
	withPayload := marshal("payload-qr-1")
	if withPayload["cta_type"] != "quick_reply" || withPayload["quick_reply_payload"] != "payload-qr-1" || withPayload["text"] != "Maybe later" {
		t.Errorf("payload body = %v", withPayload)
	}
	for _, body := range []map[string]any{titleOnly, withPayload} {
		if _, ok := body["postback_payload"]; ok || body["cta_type"] == "postback" {
			t.Errorf("quick reply must never look like a postback: %v", body)
		}
	}
}

func TestSendQuickReplyValidatesInput(t *testing.T) {
	c := newPostbackTestClient()
	for _, args := range [][2]string{{"", "t"}, {"id", ""}} {
		if _, err := c.SendQuickReply(context.Background(), args[0], args[1], "p", ""); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
	if _, err := (*Client)(nil).SendQuickReply(context.Background(), "id", "t", "", ""); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("nil client: %v", err)
	}
}

func TestFetchThreadKeepsTextItemWithOnlyQuickReplies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","thread":{"items":[
			{"item_id":"1","item_type":"text"},
			{"item_id":"2","item_type":"text","quick_replies":[{"title":"Yes","payload":"payload-qr-1"}]},
			{"item_id":"3","item_type":"text","cta_buttons":[{"cta_type":"xma_web_url","action_url":"https://example.com","title":"Site"}]},
			{"item_id":"4","item_type":"text","cta_buttons":[{"cta_type":"text","title":"Tap"}]}]}}`))
	}))
	defer srv.Close()
	items, err := newPostbackTestClient().fetchThreadButtonItems(context.Background(), srv.URL, "98765", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ItemID != "2" || items[0].AllQuickReplies()[0].PayloadValue() != "payload-qr-1" || items[1].ItemID != "4" {
		t.Fatalf("unexpected items: %+v", items)
	}
}
