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
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// DirectInboxResponse is a minimal subset of the response of the mobile
// direct_v2/inbox endpoint. Only the fields needed to bridge view-once media
// are parsed.
type DirectInboxResponse struct {
	Status string `json:"status"`
	Inbox  struct {
		Threads []*DirectThread `json:"threads"`
	} `json:"inbox"`
}

type DirectThread struct {
	ThreadID   string        `json:"thread_id"`
	ThreadV2ID string        `json:"thread_v2_id"`
	Users      []*DirectUser `json:"users"`
	Items      []*DirectItem `json:"items"`
}

type DirectUser struct {
	PK                       json.Number `json:"pk"`
	InteropMessagingUserFBID int64       `json:"interop_messaging_user_fbid"`
}

type DirectItem struct {
	ItemID         string             `json:"item_id"`
	MessageID      string             `json:"message_id"`
	ItemType       string             `json:"item_type"`
	UserID         json.Number        `json:"user_id"`
	TimestampUS    json.Number        `json:"timestamp"`
	IsSentByViewer bool               `json:"is_sent_by_viewer"`
	VisualMedia    *DirectVisualMedia `json:"visual_media"`
}

type DirectVisualMedia struct {
	ViewMode  string            `json:"view_mode"`
	SeenCount int               `json:"seen_count"`
	Media     *DirectVisualItem `json:"media"`
}

type DirectVisualItem struct {
	ID             string `json:"id"`
	MediaType      int    `json:"media_type"`
	OriginalWidth  int    `json:"original_width"`
	OriginalHeight int    `json:"original_height"`
	ImageVersions2 struct {
		Candidates []*DirectMediaVersion `json:"candidates"`
	} `json:"image_versions2"`
	VideoVersions []*DirectMediaVersion `json:"video_versions"`
	User          *DirectUser           `json:"user"`
}

type DirectMediaVersion struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

const (
	DirectItemTypeRavenMedia = "raven_media"

	DirectMediaTypeImage = 1
	DirectMediaTypeVideo = 2
)

// BestVersion returns the largest version from the given list.
func BestVersion(versions []*DirectMediaVersion) *DirectMediaVersion {
	var best *DirectMediaVersion
	for _, v := range versions {
		if v == nil || v.URL == "" {
			continue
		}
		if best == nil || v.Width*v.Height > best.Width*best.Height {
			best = v
		}
	}
	return best
}

// FetchDirectInbox fetches recent threads with the mobile REST API. It only
// works with a native Android session. The request is read-only: it does not
// mark anything as seen.
func (c *Client) FetchDirectInbox(ctx context.Context, threadLimit, messageLimit int) (*DirectInboxResponse, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	session := c.GetInstagramNativeSession()
	if session == nil || session.Authorization == "" || session.UserID == "" {
		return nil, errors.New("missing native Instagram session")
	}
	query := url.Values{
		"visual_message_return_type": {"unseen"},
		"persistentBadging":          {"true"},
		"limit":                      {strconv.Itoa(threadLimit)},
		"thread_message_limit":       {strconv.Itoa(messageLimit)},
		"fetch_reason":               {"manual_refresh"},
	}
	headers := c.instagramAccountManagerHeaders()
	headers.Set("x-ig-capabilities", "3brTv10=")
	headers.Set("x-ig-app-locale", "en_US")
	headers.Set("x-ig-device-locale", "en_US")
	headers.Set("accept-language", "en-US")
	_, body, err := c.http.MakeRequest(ctx, instagramMobileAPIBase+"direct_v2/inbox/?"+query.Encode(), http.MethodGet, headers, nil, types.NONE)
	if err != nil {
		c.checkResponseError(err)
		return nil, fmt.Errorf("failed to fetch direct inbox: %w", err)
	}
	var resp DirectInboxResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode direct inbox: %w", err)
	} else if resp.Status != "ok" {
		return nil, fmt.Errorf("direct inbox request returned status %q", resp.Status)
	}
	return &resp, nil
}
