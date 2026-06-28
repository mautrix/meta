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

type SensitiveString struct {
	Value string `json:"sensitive_string_value"`
}

type SendTextRequest struct {
	IGThreadIGID              *string         `json:"ig_thread_igid"`
	OfflineThreadingID        string          `json:"offline_threading_id"`
	RecipientIGIDs            []string        `json:"recipient_igids"`
	RepliedToClientContext    *string         `json:"replied_to_client_context"`
	RepliedToItemID           *string         `json:"replied_to_item_id"`
	ReplyToMessageID          *string         `json:"reply_to_message_id"`
	Sampled                   any             `json:"sampled"`
	Text                      SensitiveString `json:"text"`
	Mentions                  []InputMention  `json:"mentions"`
	MentionedUserIDs          []string        `json:"mentioned_user_ids"`
	Commands                  []InputCommand  `json:"commands"`
	ForwardedFromThreadID     *string         `json:"forwarded_from_thread_id"` // Note: this is the long thread ID
	IsForwardedFromOwnMessage *bool           `json:"is_forwarded_from_own_message"`
	SendAttribution           *string         `json:"send_attribution"`
}

type SendMediaRequest struct {
	AttachmentFBID            string  `json:"attachment_fbid"`
	ThreadID                  string  `json:"thread_id"`
	OfflineThreadingID        string  `json:"offline_threading_id"`
	ReplyToMessageID          *string `json:"reply_to_message_id"`
	ForwardedFromThreadID     *string `json:"forwarded_from_thread_id"`
	IsForwardedFromOwnMessage *bool   `json:"is_forwarded_from_own_message"`
}

type EditMessageRequest struct {
	ThreadID           string          `json:"thread_id"`
	TargetItemID       string          `json:"target_item_id"`
	Body               SensitiveString `json:"body"`
	OfflineThreadingID string          `json:"offline_threading_id"`
	TargetMessageID    string          `json:"target_message_id"`
}

type UnsendMessageRequest struct {
	MessageID string   `json:"message_id"`
	SendData  SendData `json:"send_data"`
}

type MarkReadRequest struct {
	Metadata MarkReadMetadata `json:"metadata"`
	Data     MarkReadData     `json:"data"`
}

type MarkReadData struct {
	MessageID string `json:"message_id"`
	// Only present for the initial non-validation request (usually an empty string)
	ItemID *string `json:"item_id,omitempty"`
	// Only present for the validation request
	MessageTimestampMS jsontime.UnixMilliString `json:"message_timestamp_ms,omitzero"`
}

type DeleteThreadRequest struct {
	ThreadID   string `json:"thread_fbid,string"`
	MarkAsSpam bool   `json:"should_move_future_requests_to_spam"`
}

type MuteThreadRequest struct {
	ThreadID           string `json:"thread_fbid"`
	MuteSeconds        int    `json:"mute_seconds"` // -1 for infinite, 0 to unmute
	OfflineThreadingID string `json:"offline_threading_id"`
}

type CreateReactionRequest struct {
	Input ReactionInput `json:"input"`
}

type ReactionStatus string

const (
	ReactionStatusCreated ReactionStatus = "created"
	ReactionStatusDeleted ReactionStatus = "deleted"
)

type ReactionInput struct {
	Emoji          string         `json:"emoji"`
	ItemID         string         `json:"item_id"`
	MessageID      string         `json:"message_id"`
	ReactionStatus ReactionStatus `json:"reaction_status"`
	ThreadID       string         `json:"thread_id"`
}

type GetThreadInfoRequest struct {
	MinUQSeqID *int64 `json:"min_uq_seq_id"`
	ThreadFBID string `json:"thread_fbid"`

	EnableOffMsysChatThemesQE      bool `json:"__relay_internal__pv__IGDEnableOffMsysChatThemesQErelayprovider"`
	InitialMessagePageCount        int  `json:"__relay_internal__pv__IGDInitialMessagePageCountrelayprovider"`
	PolarisAIGMAccountLabelEnabled bool `json:"__relay_internal__pv__PolarisAIGMAccountLabelEnabledrelayprovider"`
}

func MakeGetThreadInfoRequest(threadID string) *GetThreadInfoRequest {
	return &GetThreadInfoRequest{
		MinUQSeqID:                     nil,
		ThreadFBID:                     threadID,
		EnableOffMsysChatThemesQE:      false,
		InitialMessagePageCount:        20,
		PolarisAIGMAccountLabelEnabled: false,
	}
}

type GetMailboxRequest struct {
	DeviceIDForIrisSubscription    string `json:"device_id_for_iris_subscription"`
	IsProfessionalAccountGK        bool   `json:"__relay_internal__pv__IGDIsProfessionalAccountGKrelayprovider"`
	PinnedThreadsRenderEnabledGK   bool   `json:"__relay_internal__pv__IGDPinnedThreadsRenderEnabledGKrelayprovider"`
	MaxUnreadMessagesCount         int    `json:"__relay_internal__pv__IGDMaxUnreadMessagesCountrelayprovider"`
	PolarisAIGMAccountLabelEnabled bool   `json:"__relay_internal__pv__PolarisAIGMAccountLabelEnabledrelayprovider"`
	ThreadListActionsEnabledGK     bool   `json:"__relay_internal__pv__IGDThreadListActionsEnabledGKrelayprovider"`
}

func MakeMailboxRequest(deviceID string) *GetMailboxRequest {
	return &GetMailboxRequest{
		DeviceIDForIrisSubscription:    deviceID,
		IsProfessionalAccountGK:        false,
		PinnedThreadsRenderEnabledGK:   true,
		MaxUnreadMessagesCount:         5,
		PolarisAIGMAccountLabelEnabled: false,
		ThreadListActionsEnabledGK:     true,
	}
}

type GetProfilePageRequest struct {
	EnableIntegrityFilters          bool   `json:"enable_integrity_filters"`
	ID                              string `json:"id"`
	CannesGuardianExperienceEnabled bool   `json:"__relay_internal__pv__PolarisCannesGuardianExperienceEnabledrelayprovider"`
	CASB976ProfileEnabled           bool   `json:"__relay_internal__pv__PolarisCASB976ProfileEnabledrelayprovider"`
	WebSchoolsEnabled               bool   `json:"__relay_internal__pv__PolarisWebSchoolsEnabledrelayprovider"`
	RepostsConsumptionEnabled       bool   `json:"__relay_internal__pv__PolarisRepostsConsumptionEnabledrelayprovider"`
}

func MakeProfilePageRequest(igid string) *GetProfilePageRequest {
	return &GetProfilePageRequest{
		EnableIntegrityFilters:          true,
		ID:                              igid,
		CannesGuardianExperienceEnabled: true,
		CASB976ProfileEnabled:           false,
		WebSchoolsEnabled:               false,
		RepostsConsumptionEnabled:       true,
	}
}
