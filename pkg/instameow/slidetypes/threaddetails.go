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
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

// unrecognizedKeys returns the top-level keys of data that aren't in known. encoding/json
// ignores the `unknown` struct tag option, so the types below fill Unrecognized themselves.
func unrecognizedKeys(data []byte, known ...string) map[string]any {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	for _, key := range known {
		delete(raw, key)
	}
	if len(raw) == 0 {
		return nil
	}
	return raw
}

// valueShape describes the shape of a decoded JSON value without exposing its content.
func valueShape(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("string(len=%d)", len(val))
	case bool:
		return "bool"
	case float64, json.Number:
		return "number"
	case []any:
		return fmt.Sprintf("array(len=%d)", len(val))
	case map[string]any:
		return "object(" + strings.Join(childShapes(val), ",") + ")"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// childShapes returns "key=shape" for each key of m, one level deep: nested objects are
// reduced to their key names and nested arrays to their length.
func childShapes(m map[string]any) []string {
	parts := make([]string, 0, len(m))
	for _, key := range sortedKeys(m) {
		shape := ""
		if nested, ok := m[key].(map[string]any); ok {
			shape = "object(keys=" + strings.Join(sortedKeys(nested), ",") + ")"
		} else {
			shape = valueShape(m[key])
		}
		parts = append(parts, key+"="+shape)
	}
	return parts
}

// ThreadDetailsResponse is the response of GET /api/v1/direct_v2/threads/{thread_id}/.
// Only the parts needed for recovering generic_xma postback buttons are modelled.
type ThreadDetailsResponse struct {
	Thread *ThreadDetails `json:"thread"`
	Status string         `json:"status"`

	Unrecognized map[string]any `json:",unknown"`
}

// UnmarshalJSON fills Unrecognized with the unknown keys.
func (r *ThreadDetailsResponse) UnmarshalJSON(data []byte) error {
	type plain ThreadDetailsResponse
	*r = ThreadDetailsResponse{}
	if err := json.Unmarshal(data, (*plain)(r)); err != nil {
		return err
	}
	if len(r.Unrecognized) == 0 {
		r.Unrecognized = unrecognizedKeys(data, "thread", "status")
	}
	return nil
}

type ThreadDetails struct {
	Items        []*ThreadItem `json:"items"`
	HasOlder     bool          `json:"has_older"`
	OldestCursor string        `json:"oldest_cursor"`

	Unrecognized map[string]any `json:",unknown"`
}

// UnmarshalJSON fills Unrecognized with the unknown keys.
func (d *ThreadDetails) UnmarshalJSON(data []byte) error {
	type plain ThreadDetails
	*d = ThreadDetails{}
	if err := json.Unmarshal(data, (*plain)(d)); err != nil {
		return err
	}
	if len(d.Unrecognized) == 0 {
		d.Unrecognized = unrecognizedKeys(data, "items", "has_older", "oldest_cursor")
	}
	return nil
}

type ThreadItem struct {
	ItemID string `json:"item_id"`
	// MessageID is the `mid.$...` ID shared with the GraphQL/MQTT message ID, unlike the
	// 35-digit decimal ItemID.
	MessageID string `json:"message_id"`
	ItemType  string `json:"item_type"`
	Text      string `json:"text"`
	// Timestamp is in microseconds on the REST API (assumed, not documented in #338).
	// It may be encoded as either a number or a string.
	Timestamp  json.RawMessage `json:"timestamp"`
	CTAButtons []*CTAButton    `json:"cta_buttons"`
	// GenericXMA is kept raw because the issue doesn't say whether cta_buttons live directly
	// on the item or inside a nested generic_xma object (or a list of them).
	GenericXMA json.RawMessage `json:"generic_xma"`
	// QuickReplies are the quick reply chips of text (or XMA) items. Tapping one posts its
	// title as a text message.
	QuickReplies []*QuickReply `json:"quick_replies"`

	Unrecognized map[string]any `json:",unknown"`
}

// QuickReply is one entry of the REST `quick_replies` array. The payload key name isn't
// verified: the official webhook calls it quick_reply.payload / quick_reply_payload.
type QuickReply struct {
	Title             string `json:"title"`
	Payload           string `json:"payload,omitempty"`
	QuickReplyPayload string `json:"quick_reply_payload,omitempty"`
	ContentType       string `json:"content_type,omitempty"`
}

// PayloadValue returns the payload of the quick reply, or an empty string if it has none.
func (q *QuickReply) PayloadValue() string {
	if q == nil {
		return ""
	}
	if q.QuickReplyPayload != "" {
		return q.QuickReplyPayload
	}
	return q.Payload
}

// UnmarshalJSON fills Unrecognized with the unknown keys.
func (item *ThreadItem) UnmarshalJSON(data []byte) error {
	type plain ThreadItem
	*item = ThreadItem{}
	if err := json.Unmarshal(data, (*plain)(item)); err != nil {
		return err
	}
	if len(item.Unrecognized) == 0 {
		item.Unrecognized = unrecognizedKeys(data, "item_id", "message_id", "item_type", "timestamp", "cta_buttons", "generic_xma", "quick_replies", "text")
	}
	return nil
}

// MatchesID reports whether id is the item's item_id or message_id.
func (item *ThreadItem) MatchesID(id string) bool {
	return id != "" && (item.ItemID == id || item.MessageID == id)
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

// AllQuickReplies returns the titled quick replies of the item, looking at the item itself
// first and then at the nested generic_xma object(s).
func (item *ThreadItem) AllQuickReplies() []*QuickReply {
	titled := func(in []*QuickReply) []*QuickReply {
		var out []*QuickReply
		for _, qr := range in {
			if qr != nil && qr.Title != "" {
				out = append(out, qr)
			}
		}
		return out
	}
	if out := titled(item.QuickReplies); len(out) > 0 {
		return out
	}
	if len(item.GenericXMA) == 0 {
		return nil
	}
	var nested struct {
		QuickReplies []*QuickReply `json:"quick_replies"`
	}
	if json.Unmarshal(item.GenericXMA, &nested) == nil {
		if out := titled(nested.QuickReplies); len(out) > 0 {
			return out
		}
	}
	var nestedList []struct {
		QuickReplies []*QuickReply `json:"quick_replies"`
	}
	if json.Unmarshal(item.GenericXMA, &nestedList) == nil {
		for _, n := range nestedList {
			if out := titled(n.QuickReplies); len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

// QuickRepliesFromAny converts an unrecognized `quick_replies` JSON value into quick replies.
// Entries that aren't objects or have no title are dropped.
func QuickRepliesFromAny(v any) []*QuickReply {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var qrs []*QuickReply
	if json.Unmarshal(raw, &qrs) != nil {
		return nil
	}
	return qrs
}

// HasButtons reports whether the item may carry tappable buttons: it's a generic_xma, or has
// quick replies, or has a CTA that isn't a link.
func (item *ThreadItem) HasButtons() bool {
	if item.ItemType == "generic_xma" || len(item.AllQuickReplies()) > 0 {
		return true
	}
	for _, cta := range item.AllCTAButtons() {
		if cta != nil && !cta.IsLink() {
			return true
		}
	}
	return false
}

// ButtonItems returns the thread items that may carry tappable buttons (see HasButtons),
// including text items that only have quick replies.
func (r *ThreadDetailsResponse) ButtonItems() []*ThreadItem {
	if r == nil || r.Thread == nil {
		return nil
	}
	var items []*ThreadItem
	for _, item := range r.Thread.Items {
		if item != nil && item.HasButtons() {
			items = append(items, item)
		}
	}
	return items
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

// DebugSummary is a content-free structural summary of the response that's safe to log.
// It holds only key names, counts, IDs, timestamps, CTA type, URL scheme, booleans and
// value shapes. It never holds message text, titles, payload values, full URLs or cookies.
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
	HasMessageID  bool
	CTACount      int
	HasGenericXMA bool
	ItemKeys      []string
	TimestampMS   int64
	// GenericXMAShape is the JSON type of generic_xma ("array(len=N)" for arrays).
	GenericXMAShape string
	// GenericXMAKeys are the sorted keys of the generic_xma object (or of its first array element).
	GenericXMAKeys []string
	CTAs           []CTADebugSummary
}

// CTADebugSummary is a content-free description of a CTA button: only booleans, the cta_type,
// the URL scheme and the shapes of unrecognized fields, never titles, payloads or URLs.
type CTADebugSummary struct {
	HasTitle           bool
	CTAType            string
	ActionURLScheme    string
	HasPostbackPayload bool
	HasPayload         bool
	HasPostback        bool
	UnrecognizedShapes map[string]string
}

func newCTADebugSummary(cta *CTAButton) CTADebugSummary {
	s := CTADebugSummary{
		HasTitle:           cta.Title != "",
		CTAType:            cta.CTAType,
		HasPostbackPayload: cta.PostbackPayload != "",
		HasPayload:         cta.Payload != "",
		HasPostback:        cta.Postback() != "",
	}
	if u, err := url.Parse(cta.ActionURL); err == nil {
		s.ActionURLScheme = u.Scheme
	}
	if len(cta.Unrecognized) > 0 {
		s.UnrecognizedShapes = make(map[string]string, len(cta.Unrecognized))
		for key, val := range cta.Unrecognized {
			s.UnrecognizedShapes[key] = valueShape(val)
		}
	}
	return s
}

// genericXMAShape describes the raw generic_xma value: the sorted keys of an object, or
// "array(len=N)" plus the keys of the first element for an array.
func genericXMAShape(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 {
		return "", nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "invalid", nil
	}
	switch val := v.(type) {
	case map[string]any:
		return "object", sortedKeys(val)
	case []any:
		shape := fmt.Sprintf("array(len=%d)", len(val))
		if len(val) > 0 {
			if first, ok := val[0].(map[string]any); ok {
				return shape, sortedKeys(first)
			}
		}
		return shape, nil
	default:
		return valueShape(v), nil
	}
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
			info := ItemDebugSummary{
				ItemID:        item.ItemID,
				HasMessageID:  item.MessageID != "",
				CTACount:      len(item.AllCTAButtons()),
				HasGenericXMA: len(item.GenericXMA) > 0,
				ItemKeys:      sortedKeys(item.Unrecognized),
				TimestampMS:   item.TimestampMS(),
			}
			info.GenericXMAShape, info.GenericXMAKeys = genericXMAShape(item.GenericXMA)
			for _, cta := range item.AllCTAButtons() {
				if cta != nil {
					info.CTAs = append(info.CTAs, newCTADebugSummary(cta))
				}
			}
			s.GenericXMAInfo = append(s.GenericXMAInfo, info)
		}
	}
	return s
}
