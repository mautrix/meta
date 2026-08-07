// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
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
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type instagramMobilePendingInboxResponse struct {
	Inbox struct {
		Threads []struct {
			ThreadV2ID json.RawMessage `json:"thread_v2_id"`
		} `json:"threads"`
	} `json:"inbox"`
}

func (c *Client) GetMobilePendingThreadIDs(ctx context.Context) ([]string, error) {
	if c == nil {
		return nil, ErrClientIsNil
	} else if c.mobileSession == nil || c.mobileSession.Authorization == "" {
		return nil, fmt.Errorf("instagram mobile pending inbox requires an authorized session")
	}
	query := url.Values{
		"visual_message_return_type": {"unseen"},
		"persistentBadging":          {"true"},
		"is_prefetching":             {"false"},
	}
	response, body, requestErr := c.http.MakeRequest(
		ctx,
		instagramMobileAPIBase+"direct_v2/pending_inbox/?"+query.Encode(),
		http.MethodGet,
		c.buildAndroidHeaders(),
		nil,
		types.NONE,
	)
	if requestErr != nil {
		if response != nil {
			return nil, fmt.Errorf(
				"instagram mobile pending inbox returned HTTP %d",
				response.StatusCode,
			)
		}
		return nil, fmt.Errorf("failed to fetch Instagram mobile pending inbox: %w", requestErr)
	}
	var pending instagramMobilePendingInboxResponse
	if err := json.Unmarshal(body, &pending); err != nil {
		return nil, fmt.Errorf("failed to parse Instagram mobile pending inbox: %w", err)
	}
	threadIDs := make([]string, 0, len(pending.Inbox.Threads))
	for _, thread := range pending.Inbox.Threads {
		threadID := rawJSONScalar(thread.ThreadV2ID)
		if !isDecimalString(threadID) {
			continue
		}
		threadIDs = append(threadIDs, threadID)
	}
	slices.Sort(threadIDs)
	return slices.Compact(threadIDs), nil
}
