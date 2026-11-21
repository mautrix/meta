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

package slidetypes

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
)

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

// ThreadDetailsResponse is the response of GET /api/v1/direct_v2/threads/{thread_id}/.
// Only the parts needed for recovering generic_xma postback buttons are modelled.
type ThreadDetailsResponse struct {
	Thread *ThreadDetails `json:"thread"`
	Status string         `json:"status"`

	Unrecognized map[string]any `json:",unknown"`
}

type ThreadDetails struct {
	Items []*ThreadItem `json:"items"`

	Unrecognized map[string]any `json:",unknown"`
}

type ThreadItem struct {
	ItemID   string `json:"item_id"`
	ItemType string `json:"item_type"`
	// Timestamp is in microseconds on the REST API (assumed, not documented in #338).
	// It may be encoded as either a number or a string.
	Timestamp  json.RawMessage `json:"timestamp"`
	CTAButtons []*CTAButton    `json:"cta_buttons"`
	// GenericXMA is kept raw because the issue doesn't say whether cta_buttons live directly
	// on the item or inside a nested generic_xma object (or a list of them).
	GenericXMA json.RawMessage `json:"generic_xma"`

	Unrecognized map[string]any `json:",unknown"`
}

// TimestampMS returns the item timestamp converted to milliseconds, or 0 if unknown.
func (item *ThreadItem) TimestampMS() int64 {
	raw := string(item.Timestamp)
	if raw == "" || raw == "null" {
		return 0
	}
	if unquoted, err := strconv.Unquote(raw); err == nil {
		raw = unquoted
	}
	ts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	if ts > 1e14 {
		return ts / 1000
	}
	return ts
}

// AllCTAButtons returns the cta_buttons of the item, looking at the item itself first and
// then at the nested generic_xma object(s).
func (item *ThreadItem) AllCTAButtons() []*CTAButton {
	if len(item.CTAButtons) > 0 {
		return item.CTAButtons
	}
	if len(item.GenericXMA) == 0 {
		return nil
	}
	var nested struct {
		CTAButtons []*CTAButton `json:"cta_buttons"`
	}
	if json.Unmarshal(item.GenericXMA, &nested) == nil && len(nested.CTAButtons) > 0 {
		return nested.CTAButtons
	}
	var nestedList []struct {
		CTAButtons []*CTAButton `json:"cta_buttons"`
	}
	if json.Unmarshal(item.GenericXMA, &nestedList) == nil {
		for _, n := range nestedList {
			if len(n.CTAButtons) > 0 {
				return n.CTAButtons
			}
		}
	}
	return nil
}

// GenericXMAItems returns the generic_xma items of the thread.
func (r *ThreadDetailsResponse) GenericXMAItems() []*ThreadItem {
	if r == nil || r.Thread == nil {
		return nil
	}
	var items []*ThreadItem
	for _, item := range r.Thread.Items {
		if item != nil && item.ItemType == "generic_xma" {
			items = append(items, item)
		}
	}
	return items
}

// DebugSummary is a structural summary of the response (key names and counts only, no
// content) that's safe to log.
type DebugSummary struct {
	Status         string
	TopKeys        []string
	ThreadKeys     []string
	TotalItems     int
	GenericXMA     int
	GenericXMAInfo []ItemDebugSummary
}

type ItemDebugSummary struct {
	ItemID        string
	CTACount      int
	HasGenericXMA bool
	ItemKeys      []string
}

const maxDebugSummaryItems = 5

func (r *ThreadDetailsResponse) DebugSummary() DebugSummary {
	s := DebugSummary{Status: r.Status, TopKeys: sortedKeys(r.Unrecognized)}
	if r.Thread == nil {
		return s
	}
	s.ThreadKeys = sortedKeys(r.Thread.Unrecognized)
	s.TotalItems = len(r.Thread.Items)
	for _, item := range r.GenericXMAItems() {
		s.GenericXMA++
		if len(s.GenericXMAInfo) < maxDebugSummaryItems {
			s.GenericXMAInfo = append(s.GenericXMAInfo, ItemDebugSummary{
				ItemID:        item.ItemID,
				CTACount:      len(item.AllCTAButtons()),
				HasGenericXMA: len(item.GenericXMA) > 0,
				ItemKeys:      sortedKeys(item.Unrecognized),
			})
		}
	}
	return s
}
