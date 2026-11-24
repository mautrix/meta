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

// Support for tapping ManyChat-style generic_xma postback buttons.
//
// The GraphQL message data hides the button payloads, so they're recovered with the REST
// thread endpoint and sent back over the mqttbypass socket. The protocol is taken from
// https://github.com/mautrix/meta/issues/338 and has not been verified by this project.
//
// Posting to /api/v1/direct_v2/threads/broadcast/text/ must NOT be used for this: the issue
// reports that web cookies were redirected to login and the sessionid was cleared by it.
// Sending postbacks through the GraphQL text send mutation doesn't trigger the webhook either.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const (
	threadDetailsBaseURL       = "https://www.instagram.com"
	mobileThreadDetailsBaseURL = "https://i.instagram.com"
)

var errThreadNotFound = errors.New("thread not found")

// ThreadDetailsURL returns the URL of the REST thread endpoint (the trailing slash is required).
func ThreadDetailsURL(base, threadID string) string {
	return base + "/api/v1/direct_v2/threads/" + url.PathEscape(threadID) + "/"
}

// threadDetailsHeaders builds clean Android app headers for the thread GET. Desktop browser
// headers (sec-ch-ua*, x-fb-lsd, ...) are deliberately not sent next to the Android user agent.
// Cookie-only logins also send their session cookies.
func (c *Client) threadDetailsHeaders() http.Header {
	h := c.buildAndroidHeaders()
	if c.mobileSession.Load() == nil && c.cookies != nil {
		if cookieStr := c.cookies.String(); cookieStr != "" {
			h.Set("cookie", cookieStr)
		}
	}
	h.Set("user-agent", useragent.IGAndroidUserAgent)
	h.Set("x-ig-app-id", useragent.IGAndroidAppID)
	return h
}

// FetchThreadButtonItems fetches the thread over the REST API and returns the items that may
// carry buttons: generic_xma items, items with quick replies and items with non-link CTAs.
// It tries i.instagram.com first (www.instagram.com answers 404 HTML for this endpoint) and
// retries once on www.instagram.com if the thread isn't found.
func (c *Client) FetchThreadButtonItems(ctx context.Context, threadID string) ([]*slidetypes.ThreadItem, error) {
	return c.fetchThreadButtons(ctx, threadID, false)
}

// FetchAllThreadButtonItems follows older history pages for existing-card refresh and tap recovery.
// Arrival recovery uses FetchThreadButtonItems to avoid scanning history for each new card.
func (c *Client) FetchAllThreadButtonItems(ctx context.Context, threadID string) ([]*slidetypes.ThreadItem, error) {
	return c.fetchThreadButtons(ctx, threadID, true)
}

func (c *Client) fetchThreadButtons(ctx context.Context, threadID string, allHistory bool) ([]*slidetypes.ThreadItem, error) {
	items, err := c.fetchThreadButtonItems(ctx, mobileThreadDetailsBaseURL, threadID, allHistory)
	if errors.Is(err, errThreadNotFound) && len(items) == 0 {
		return c.fetchThreadButtonItems(ctx, threadDetailsBaseURL, threadID, allHistory)
	}
	return items, err
}

func (c *Client) fetchThreadButtonItems(ctx context.Context, baseURL, threadID string, allHistory bool) ([]*slidetypes.ThreadItem, error) {
	var items []*slidetypes.ThreadItem
	cursor := ""
	seen := make(map[string]bool)
	seenItems := make(map[string]bool)
	const maxHistoryPages = 50
	pagesFetched := 0
	for {
		if pagesFetched >= maxHistoryPages {
			return items, nil
		}
		page, err := c.fetchThreadButtonPage(ctx, baseURL, threadID, cursor)
		if err != nil {
			return items, err
		}
		pagesFetched++
		addedItem := false
		for _, item := range page.ButtonItems() {
			itemKey := item.ItemID
			if itemKey == "" {
				itemKey = item.MessageID
			}
			if itemKey != "" && seenItems[itemKey] {
				continue
			}
			if itemKey != "" {
				seenItems[itemKey] = true
				addedItem = true
			}
			items = append(items, item)
		}
		if !allHistory || !addedItem || page.Thread == nil || !page.Thread.HasOlder {
			return items, nil
		}
		cursor = page.Thread.OldestCursor
		if cursor == "" || seen[cursor] {
			return items, errors.New("thread history has a missing or repeated older cursor")
		}
		seen[cursor] = true
	}
}

func (c *Client) fetchThreadButtonPage(ctx context.Context, baseURL, threadID, cursor string) (*slidetypes.ThreadDetailsResponse, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	if threadID == "" {
		return nil, errors.New("thread ID is empty")
	}
	reqURL := ThreadDetailsURL(baseURL, threadID)
	if cursor != "" {
		reqURL += "?" + url.Values{"cursor": {cursor}, "direction": {"older"}}.Encode()
	}
	resp, body, err := c.http.MakeRequest(ctx, reqURL, http.MethodGet, c.threadDetailsHeaders(), nil, types.NONE)
	if err != nil {
		if resp != nil {
			evt := zerolog.Ctx(ctx).Debug().
				Str("url", ThreadDetailsURL(baseURL, "<id>")).
				Int("status_code", resp.StatusCode).
				Str("content_type", resp.Header.Get("Content-Type"))
			httpclient.AddRedactedResponse(evt, body).Msg("Thread details request failed")
		}
		c.checkResponseError(err)
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			err = fmt.Errorf("%w: %w", errThreadNotFound, err)
		}
		return nil, fmt.Errorf("failed to fetch thread details: %w", err)
	}
	c.updateNativeSession(ctx, resp)
	var parsed slidetypes.ThreadDetailsResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse thread details: %w", err)
	}
	if parsed.Thread == nil {
		return nil, errors.New("thread details response has no thread")
	}
	logThreadDetailsSummary(ctx, threadID, &parsed)
	return &parsed, nil
}

// logThreadDetailsSummary logs only structure (key names, counts, shapes, booleans) and IDs,
// never cookies, message text, button titles, payload values or URLs.
func logThreadDetailsSummary(ctx context.Context, threadID string, parsed *slidetypes.ThreadDetailsResponse) {
	log := zerolog.Ctx(ctx)
	if !log.Debug().Enabled() {
		return
	}
	sum := parsed.DebugSummary()
	arr := zerolog.Arr()
	for _, item := range sum.GenericXMAInfo {
		ctas := zerolog.Arr()
		for _, cta := range item.CTAs {
			shapes := zerolog.Dict()
			for key, shape := range cta.UnrecognizedShapes {
				shapes.Str(key, shape)
			}
			ctas.Dict(zerolog.Dict().
				Bool("has_title", cta.HasTitle).
				Str("cta_type", cta.CTAType).
				Str("action_url_scheme", cta.ActionURLScheme).
				Bool("has_postback_payload", cta.HasPostbackPayload).
				Bool("has_payload", cta.HasPayload).
				Bool("has_postback", cta.HasPostback).
				Dict("unrecognized_shapes", shapes))
		}
		arr.Dict(zerolog.Dict().
			Str("item_id", item.ItemID).
			Bool("has_message_id", item.HasMessageID).
			Int("cta_count", item.CTACount).
			Bool("has_generic_xma", item.HasGenericXMA).
			Strs("item_keys", item.ItemKeys).
			Int64("timestamp_ms", item.TimestampMS).
			Str("generic_xma_shape", item.GenericXMAShape).
			Strs("generic_xma_keys", item.GenericXMAKeys).
			Array("ctas", ctas))
	}
	log.Debug().
		Str("thread_id", threadID).
		Str("status", sum.Status).
		Strs("top_keys", sum.TopKeys).
		Strs("thread_keys", sum.ThreadKeys).
		Int("total_items", sum.TotalItems).
		Int("generic_xma_items", sum.GenericXMA).
		Array("generic_xma", arr).
		Msg("Fetched thread details for XMA postbacks")
}

// sendItemPostback is the /ig_send_message body for tapping a postback button, as documented
// in #338 (action, item_type, text, send_attribution, cta_type, postback_payload), plus the
// thread_id and client_context fields that other /ig_send_message payloads in this package use.
type sendItemPostback struct {
	Action          string `json:"action"`
	ItemType        string `json:"item_type"`
	Text            string `json:"text"`
	SendAttribution string `json:"send_attribution"`
	CTAType         string `json:"cta_type"`
	PostbackPayload string `json:"postback_payload"`
	ThreadID        string `json:"thread_id"`
	ClientContext   string `json:"client_context"`
}

func newSendItemPostback(threadID, title, payload, clientContext string) *sendItemPostback {
	return &sendItemPostback{
		Action:          "send_item",
		ItemType:        "text",
		Text:            title,
		SendAttribution: "business_generic_template",
		CTAType:         "postback",
		PostbackPayload: payload,
		ThreadID:        threadID,
		ClientContext:   clientContext,
	}
}

// SendPostback taps a postback button by publishing a send_item request over the mqttbypass
// socket. It doesn't wait for a response from the server.
func (c *Client) SendPostback(ctx context.Context, threadID, title, payload string) (string, error) {
	return c.SendPostbackWithContext(ctx, threadID, title, payload, "")
}

// SendPostbackWithContext is like SendPostback, but uses the given client context (the
// offline threading ID) instead of generating one. An empty clientContext generates one.
func (c *Client) SendPostbackWithContext(ctx context.Context, threadID, title, payload, clientContext string) (string, error) {
	if c == nil {
		return "", ErrClientIsNil
	} else if threadID == "" || title == "" || payload == "" {
		return "", errors.New("thread ID, button title and postback payload are required")
	}
	if clientContext == "" {
		clientContext = strconv.FormatInt(methods.GenerateEpochID(), 10)
	}
	body, err := json.Marshal(newSendItemPostback(threadID, title, payload, clientContext))
	if err != nil {
		return "", fmt.Errorf("failed to marshal postback request: %w", err)
	}
	if err = c.publishMQTTBypass(ctx, body); err != nil {
		return "", err
	}
	return clientContext, nil
}

// sendItemQuickReply is the /ig_send_message body for tapping a quick reply. Per the official
// Instagram Messaging docs a tap posts the button title as a text message, so the required part
// is action/item_type/text. cta_type and quick_reply_payload (the name of the inbound webhook
// field) are best-effort: the private MQTT key for the payload is unverified. It deliberately
// has no postback_payload field.
type sendItemQuickReply struct {
	Action            string `json:"action"`
	ItemType          string `json:"item_type"`
	Text              string `json:"text"`
	SendAttribution   string `json:"send_attribution,omitempty"`
	CTAType           string `json:"cta_type,omitempty"`
	QuickReplyPayload string `json:"quick_reply_payload,omitempty"`
	ThreadID          string `json:"thread_id"`
	ClientContext     string `json:"client_context"`
}

func newSendItemQuickReply(threadID, title, payload, clientContext string) *sendItemQuickReply {
	body := &sendItemQuickReply{
		Action:        "send_item",
		ItemType:      "text",
		Text:          title,
		ThreadID:      threadID,
		ClientContext: clientContext,
	}
	if payload != "" {
		body.SendAttribution = "quick_reply"
		body.CTAType = "quick_reply"
		body.QuickReplyPayload = payload
	}
	return body
}

// SendQuickReply taps a quick reply by publishing a text send_item request over the mqttbypass
// socket. It doesn't wait for a response from the server.
func (c *Client) SendQuickReply(ctx context.Context, threadID, title, payload, clientContext string) (string, error) {
	return c.SendQuickReplyWithContext(ctx, threadID, title, payload, clientContext)
}

// SendQuickReplyWithContext sends the quick reply title as text, with the payload fields only if
// the payload is non-empty. An empty clientContext generates one. It never uses broadcast/text.
func (c *Client) SendQuickReplyWithContext(ctx context.Context, threadID, title, payload, clientContext string) (string, error) {
	if c == nil {
		return "", ErrClientIsNil
	} else if threadID == "" || title == "" {
		return "", errors.New("thread ID and quick reply title are required")
	}
	if clientContext == "" {
		clientContext = strconv.FormatInt(methods.GenerateEpochID(), 10)
	}
	body, err := json.Marshal(newSendItemQuickReply(threadID, title, payload, clientContext))
	if err != nil {
		return "", fmt.Errorf("failed to marshal quick reply request: %w", err)
	}
	if err = c.publishMQTTBypass(ctx, body); err != nil {
		return "", err
	}
	return clientContext, nil
}
