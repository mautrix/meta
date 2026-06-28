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

	"go.mau.fi/util/jsontime"
)

type DeltaEvent interface {
	isDeltaEvent()
}

func (*NewMessageEvent) isDeltaEvent()       {}
func (*AdminMessageEvent) isDeltaEvent()     {}
func (*CreateReactionEvent) isDeltaEvent()   {}
func (*MarkReadEvent) isDeltaEvent()         {}
func (*MarkUnreadEvent) isDeltaEvent()       {}
func (*ReadReceiptEvent) isDeltaEvent()      {}
func (*EditMessageEvent) isDeltaEvent()      {}
func (*DeleteMessageEvent) isDeltaEvent()    {}
func (*DeleteThreadEvent) isDeltaEvent()     {}
func (*ParticipantLeaveEvent) isDeltaEvent() {}
func (*ParticipantJoinEvent) isDeltaEvent()  {}
func (*AdminChangeEvent) isDeltaEvent()      {}
func (*MuteThreadEvent) isDeltaEvent()       {}
func (UnknownEvent) isDeltaEvent()           {}

type NewMessageEvent struct {
	ThreadReadStateEffect string   `json:"thread_read_state_effect"`
	Message               *Message `json:"message"`

	Unrecognized map[string]any `json:",unknown"`
}

type AdminMessageEvent struct {
	SkipBumpThread bool     `json:"skip_bump_thread"`
	Message        *Message `json:"message"`

	Unrecognized map[string]any `json:",unknown"`
}

type CreateReactionEvent struct {
	MessageID string   `json:"message_id"`
	Reaction  Reaction `json:"reaction"`

	ReactedToMessageSenderFBID string `json:"reacted_to_message_sender_fbid"`

	Unrecognized map[string]any `json:",unknown"`
}

type MarkReadEvent struct {
	ReadTimestampMS jsontime.UnixMilliString `json:"read_timestamp_ms"`

	Unrecognized map[string]any `json:",unknown"`
}

type MarkUnreadEvent struct {
	MarkedAsUnread bool `json:"marked_as_unread"`

	Unrecognized map[string]any `json:",unknown"`
}

type ReadReceiptEvent struct {
	ReadReceipt ReadReceipt `json:"read_receipt"`

	Unrecognized map[string]any `json:",unknown"`
}

type EditMessageEvent struct {
	MessageID             string `json:"message_id"`
	TextBody              string `json:"text_body"`
	IGDSnippet            string `json:"igd_snippet"`
	SlideEditHistoryEntry struct {
		TimestampMS jsontime.UnixMilliString `json:"timestamp_ms"`
	} `json:"slide_edit_history_entry"`

	Unrecognized map[string]any `json:",unknown"`
}

type DeleteMessageEvent struct {
	MessageID    string         `json:"message_id"`
	Unrecognized map[string]any `json:",unknown"`
}

type DeleteThreadEvent struct {
	Unrecognized map[string]any `json:",unknown"`
}

type ParticipantLeaveEvent struct {
	ActorFBID           int64                `json:"actor_fbid,string"`
	LeftParticipantFBID int64                `json:"left_participant_fbid,string"`
	ViewerFBID          int64                `json:"viewer_fbid,string"`
	Thread              AsThread[TinyThread] `json:"thread"`
	Message             *Message             `json:"message"`

	Unrecognized map[string]any `json:",unknown"`
}

type ParticipantJoinEvent struct {
	Message *Message                         `json:"message"`
	Thread  AsThread[ParticipantsOnlyThread] `json:"thread"`
}

type AdminChangeEvent struct {
	AdminIGIDs []string `json:"admin_igids"`

	Unrecognized map[string]any `json:",unknown"`
}

type MuteThreadEvent struct {
	IsMutedNow bool `json:"is_muted_now"`

	Unrecognized map[string]any `json:",unknown"`
}

type CreateThreadEvent struct{}

type UnknownEvent map[string]any

type DeltaWrapper struct {
	Data struct {
		SlideDeltaProcessor []*Delta `json:"slide_delta_processor"`
	} `json:"data"`
}

type Delta struct {
	TypeName   string     `json:"__typename"`
	UQSeqID    int64      `json:"uq_seq_id,string"`
	ThreadIGID string     `json:"thread_fbid"`
	Data       DeltaEvent `json:"-"`
}

type marshalableDelta Delta

func (d *Delta) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, (*marshalableDelta)(d))
	if err != nil {
		return fmt.Errorf("failed to parse delta: %w", err)
	}
	switch d.TypeName {
	case "SlideUQPPCreateReaction":
		d.Data = &CreateReactionEvent{}
	case "SlideUQPPMarkRead":
		d.Data = &MarkReadEvent{}
	case "SlideUQPPMarkUnread":
		d.Data = &MarkUnreadEvent{}
	case "SlideUQPPReadReceipt":
		d.Data = &ReadReceiptEvent{}
	case "SlideUQPPNewMessage":
		d.Data = &NewMessageEvent{}
	case "SlideUQPPAdminTextMessage":
		d.Data = &AdminMessageEvent{}
	case "SlideUQPPEditMessage":
		d.Data = &EditMessageEvent{}
	case "SlideUQPPDeleteMessage":
		d.Data = &DeleteMessageEvent{}
	case "SlideUQPPDeleteThread":
		d.Data = &DeleteThreadEvent{}
	case "SlideUQPPParticipantLeftGroupThread":
		d.Data = &ParticipantLeaveEvent{}
	case "SlideUQPPParticipantsAddedToGroupThread":
		d.Data = &ParticipantJoinEvent{}
	case "SlideUQPPGenericMapAdminsKeyMutation":
		d.Data = &AdminChangeEvent{}
	case "SlideUQPPChangeMuteSettings":
		d.Data = &MuteThreadEvent{}
	default:
		var raw map[string]any
		err = json.Unmarshal(data, &raw)
		d.Data = UnknownEvent(raw)
		return err
	}
	err = json.Unmarshal(data, d.Data)
	if err != nil {
		return fmt.Errorf("failed to parse delta data into %T: %w", d.Data, err)
	}
	return nil
}
