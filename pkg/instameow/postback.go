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

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const threadDetailsBaseURL = "https://www.instagram.com"

// ThreadDetailsURL returns the URL of the REST thread endpoint (the trailing slash is required).
func ThreadDetailsURL(base, threadID string) string {
	return base + "/api/v1/direct_v2/threads/" + url.PathEscape(threadID) + "/"
}

// threadDetailsHeaders builds the headers for the thread GET. This is a workaround from #338:
// the web session cookies are used together with the Android app's user agent and app ID.
func (c *Client) threadDetailsHeaders() http.Header {
	var h http.Header
	if c.mobileSession.Load() != nil {
		h = c.buildAndroidHeaders()
	} else {
		h = c.http.BuildHeaders(true, false)
	}
	h.Set("user-agent", useragent.IGAndroidUserAgent)
	h.Set("x-ig-app-id", useragent.IGAndroidAppID)
	return h
}

// FetchThreadGenericXMAItems fetches the thread over the REST API and returns the generic_xma items.
func (c *Client) FetchThreadGenericXMAItems(ctx context.Context, threadID string) ([]*slidetypes.ThreadItem, error) {
	return c.fetchThreadGenericXMAItems(ctx, threadDetailsBaseURL, threadID)
}

func (c *Client) fetchThreadGenericXMAItems(ctx context.Context, baseURL, threadID string) ([]*slidetypes.ThreadItem, error) {
	if c == nil {
		return nil, ErrClientIsNil
	} else if threadID == "" {
		return nil, errors.New("thread ID is empty")
	}
	resp, body, err := c.http.MakeRequest(ctx, ThreadDetailsURL(baseURL, threadID), http.MethodGet, c.threadDetailsHeaders(), nil, types.NONE)
	if err != nil {
		c.checkResponseError(err)
		return nil, fmt.Errorf("failed to fetch thread details: %w", err)
	}
	c.updateNativeSession(ctx, resp)
	var parsed slidetypes.ThreadDetailsResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse thread details: %w", err)
	}
	return parsed.GenericXMAItems(), nil
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
	if c == nil {
		return "", ErrClientIsNil
	} else if threadID == "" || title == "" || payload == "" {
		return "", errors.New("thread ID, button title and postback payload are required")
	}
	clientContext := strconv.FormatInt(methods.GenerateEpochID(), 10)
	body, err := json.Marshal(newSendItemPostback(threadID, title, payload, clientContext))
	if err != nil {
		return "", fmt.Errorf("failed to marshal postback request: %w", err)
	}
	if err = c.publishMQTTBypass(ctx, body); err != nil {
		return "", err
	}
	return clientContext, nil
}
