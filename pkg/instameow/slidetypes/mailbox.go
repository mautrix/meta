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

type Mailbox struct {
	UQSeqID         int64                          `json:"iris_inactive_subscription_uq_seq_id,string"`
	ThreadsByFolder Edged[Node[WrappedThreadInfo]] `json:"threads_by_folder"`
	PinnedThreadsV2 []WrappedThreadInfo            `json:"pinned_threads_v2"`
	ID              string                         `json:"id"`
	Token           string                         `json:"__token"`

	Unrecognized map[string]any `json:",unknown"`
}

type Edged[T any] struct {
	Edges    []T      `json:"edges"`
	PageInfo PageInfo `json:"page_info,omitzero"`
}

type Node[T any] struct {
	Node   T      `json:"node"`
	Cursor string `json:"cursor,omitempty"`
}

type PageInfo struct {
	EndCursor       string `json:"end_cursor,omitempty"`
	HasNextPage     bool   `json:"has_next_page,omitempty"`
	HasPreviousPage bool   `json:"has_previous_page,omitempty"`
	StartCursor     string `json:"start_cursor,omitempty"`
}
