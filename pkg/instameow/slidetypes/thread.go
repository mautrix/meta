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
	"go.mau.fi/util/jsontime"
)

type WrappedThreadInfo = AsThread[ThreadInfo]

type AsThread[T any] struct {
	AsIGDirectThread *T `json:"as_ig_direct_thread"`
}

type ThreadSubtype string

const (
	ThreadSubtypeDirect         ThreadSubtype = "IG_ONLY_ONE_TO_ONE"
	ThreadSubtypeDirectBusiness ThreadSubtype = "IG_BUSINESS_ACCOUNT_ONE_TO_ONE"
	ThreadSubtypeAIBot          ThreadSubtype = "IG_AI_BOT_ONE_TO_ONE"
	ThreadSubtypeGroup          ThreadSubtype = "IGD_GROUP"
)

type ThreadInfo struct {
	ID                      string                   `json:"id"`
	ThreadKey               int64                    `json:"thread_key,string"`
	ThreadFBID              string                   `json:"thread_fbid"`
	ThreadSubtype           ThreadSubtype            `json:"thread_subtype"`
	ThreadID                string                   `json:"thread_id"`
	IsGroup                 bool                     `json:"is_group"`
	ViewerID                string                   `json:"viewer_id"`
	Viewer                  *User                    `json:"viewer"`
	EventChatInfo           any                      `json:"event_chat_info"`
	ThreadTitle             string                   `json:"thread_title"`
	ThreadImageURL          string                   `json:"thread_image_url"`
	Users                   []*User                  `json:"users"`
	Nicknames               []Nickname               `json:"nicknames"`
	LastActivityTimestampMS jsontime.UnixMilliString `json:"last_activity_timestamp_ms"`
	SlideMessages           *Edged[Node[*Message]]   `json:"slide_messages"`
	SlideReadReceipts       []ReadReceipt            `json:"slide_read_receipts"`
	MarkedAsUnread          bool                     `json:"marked_as_unread"`
	SystemFolder            string                   `json:"system_folder"`
	InputMode               int                      `json:"input_mode"`

	// Only present when listing threads
	IsMuted            *bool      `json:"is_muted"`
	IsPin              *bool      `json:"is_pin"`
	Folder             string     `json:"folder"`
	MessagingFolderTag string     `json:"messaging_folder_tag"`
	UsersWithoutViewer []TinyUser `json:"usersWithoutViewer"`
	ThreadLabel        int        `json:"thread_label"`
	TakedownData       any        `json:"takedown_data"`

	// Only present when getting a single thread
	PinnedMessages             []any              `json:"pinned_messages"`
	IGThreadCapabilities       ThreadCapabilities `json:"ig_thread_capabilities"`
	ReachabilityStatus         string             `json:"reachability_status"`
	AdminUserIDs               []string           `json:"admin_user_ids"`
	GroupCreator               *GroupCreator      `json:"group_creator"`
	CreatorBroadcastThreadData any                `json:"creator_broadcast_thread_data"`
	HiddenChatInfo             any                `json:"hidden_chat_info"`

	Unrecognized map[string]any `json:",unknown"`
}

type TinyThread struct {
	ID          string `json:"id"`
	ThreadTitle string `json:"thread_title"`
	InputMode   int    `json:"input_mode"`

	Unrecognized map[string]any `json:",unknown"`
}

type ParticipantsOnlyThread struct {
	ID          string `json:"id"`
	ThreadTitle string `json:"thread_title"`
	Users       []User `json:"users"`

	Unrecognized map[string]any `json:",unknown"`
}

type SendData struct {
	// This is the very long thread ID
	ThreadID string `json:"thread_id"`
}

type MarkReadMetadata struct {
	// This is also the very long thread ID
	IGThreadIGID string `json:"ig_thread_igid"`
}

type TinyUser struct {
	IsInternalUser bool   `json:"is_internal_user"`
	ID             string `json:"id"`

	Unrecognized map[string]any `json:",unknown"`
}

type GroupCreator struct {
	FullName                 string `json:"full_name"`
	InteropMessagingUserFBID int64  `json:"interop_messaging_user_fbid,string"`
	ID                       string `json:"id"`
}

type Nickname struct {
	EIMUID   int64  `json:"eimu_id,string"`
	Nickname string `json:"nickname"`
	IGID     string `json:"igid"`
}

type ThreadCapabilities struct {
	Capabilities0 string `json:"capabilities_0"`
	Capabilities1 string `json:"capabilities_1"`
}

type ReadReceipt struct {
	ParticipantFBID      int64                    `json:"participant_fbid,string"`
	WatermarkTimestampMS jsontime.UnixMilliString `json:"watermark_timestamp_ms"`
}
