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

type MailboxResponse struct {
	Mailbox *Mailbox `json:"get_slide_mailbox_for_iris_subscription"`
}

func (r *MailboxResponse) GetMailbox() *Mailbox {
	if r == nil {
		return nil
	}
	return r.Mailbox
}

type ThreadInfoResponse struct {
	ThreadInfo *WrappedThreadInfo `json:"get_slide_thread_nullable"`
}

type FetchMessagesResponse struct {
	// Note: this will only contain slide_messages and id
	ThreadInfo *WrappedThreadInfo `json:"fetch__SlideThread"`
}

type SendTextResponse struct {
	Message SentMessage `json:"xig_direct_text_send_with_slide_messaging_response"`
}

func (str *SendTextResponse) GetMessage() SentMessage {
	return str.Message
}

type SendMediaResponse struct {
	Message SentMessage `json:"xig_direct_media_send_with_slide_messaging_response"`
}

func (smr *SendMediaResponse) GetMessage() SentMessage {
	return smr.Message
}

type EditMessageResponse struct {
	Message SentMessage `json:"direct_edit_message_with_slide_messaging_response"`
}

type UnsendMessageResponse struct {
	DirectUnsendMessage bool `json:"direct_unsend_message"`
}

type ReactionUpdateMessage struct {
	ID        string     `json:"id"`
	Reactions []Reaction `json:"reactions"`
}

type SendReactionResponse struct {
	Message ReactionUpdateMessage `json:"xig_direct_reaction_send_with_slide_messaging_response"`
}

type MarkReadResponse struct {
	ReadReceipt ReadReceipt `json:"xig_direct_item_seen_mutation_with_slide_messaging_response"`
}

type MarkReadValidationResponse struct {
	Validated bool `json:"xig_direct_mark_read_id_validation"`
}

type ProfilePageResponse struct {
	User   *User `json:"user"`
	Viewer *User `json:"viewer"`
}

type DeleteThreadResponse struct {
	HideThread struct {
		ID string `json:"id"`
	} `json:"ig_direct_hide_thread"`
}

type AcceptMessageRequestResponse struct {
	Data struct {
		Folder       string `json:"folder"`
		ThreadID     string `json:"id"`
		SystemFolder string `json:"system_folder"`
	} `json:"ig_direct_accept_message_request"`
}

type MuteThreadResponse struct {
	Data struct {
		IsMuted   bool   `json:"ig_direct_mute_thread"`
		MuteUntil *int64 `json:"mute_until"` // null if unmuted, -1 if permanent
		ThreadID  string `json:"id"`
	} `json:"xig_direct_mute_thread_with_fbid"`
}
type PinThreadResponse struct {
	Data struct {
		IsPin    bool   `json:"is_pin"`
		ThreadID string `json:"id"`
	} `json:"xig_direct_pin_thread"`
}
