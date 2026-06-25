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

func (*NewMessageEvent) isDeltaEvent()     {}
func (*CreateReactionEvent) isDeltaEvent() {}
func (*MarkReadEvent) isDeltaEvent()       {}
func (UnknownEvent) isDeltaEvent()         {}

type NewMessageEvent struct {
	ThreadFBID            string  `json:"thread_fbid"`
	ThreadReadStateEffect string  `json:"thread_read_state_effect"`
	Message               Message `json:"message"`
}

type CreateReactionEvent struct {
	ThreadFBID string   `json:"thread_fbid"`
	MessageID  string   `json:"message_id"`
	Reaction   Reaction `json:"reaction"`

	ReactedToMessageSenderFBID string `json:"reacted_to_message_sender_fbid"`
}

type MarkReadEvent struct {
	ThreadFBID      string                   `json:"thread_fbid"`
	ReadTimestampMS jsontime.UnixMilliString `json:"read_timestamp_ms"`
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
	ThreadFBID string     `json:"thread_fbid"`
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
	case "SlideUQPPNewMessage":
		d.Data = &NewMessageEvent{}
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
