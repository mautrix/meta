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
	"strconv"

	"go.mau.fi/util/exslices"
	"go.mau.fi/util/jsontime"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
)

type Message struct {
	IsReported                  bool                      `json:"is_reported"`
	MessageID                   string                    `json:"message_id"`
	SenderFBID                  string                    `json:"sender_fbid"`
	ThreadFBID                  string                    `json:"thread_fbid"`
	Content                     MessageContent            `json:"content"`
	ContentType                 string                    `json:"content_type"`
	OfflineThreadingID          string                    `json:"offline_threading_id"`
	TimestampMS                 jsontime.UnixMilliString  `json:"timestamp_ms"`
	Reactions                   []*Reaction               `json:"reactions"`
	ID                          string                    `json:"id"`
	BotResponseID               any                       `json:"bot_response_id"`
	IsAIGenerated               bool                      `json:"is_ai_generated"`
	TextBody                    string                    `json:"text_body"`
	Mentions                    MentionList               `json:"mentions"`
	IGDIsForwarded              bool                      `json:"igd_is_forwarded"`
	RepliedToMessageId          any                       `json:"replied_to_message_id"`
	RepliedToMessage            any                       `json:"replied_to_message"`
	Sender                      *MessageSender            `json:"sender"`
	SlideEditHistory            []MessageEditHistoryEntry `json:"slide_edit_history"`
	IsPinned                    bool                      `json:"is_pinned"`
	IGDWearablesAttributionText any                       `json:"igd_wearables_attribution_text"`
	IGDWearablesAttributionType any                       `json:"igd_wearables_attribution_type"`
	ExpirationTimestampMS       jsontime.UnixMilliString  `json:"expiration_timestamp_ms"`
	ViewExpirationTimestampMS   jsontime.UnixMilliString  `json:"view_expiration_timestamp_ms"`
	Typename                    string                    `json:"__typename"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageSender struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	IGID string `json:"igid"`

	UserDict User `json:"user_dict"`

	Unrecognized map[string]any `json:",unknown"`
}

type Reaction struct {
	LogMessageID        string                   `json:"log_message_id"`
	Reaction            string                   `json:"reaction"`
	ReactionTimestampMS jsontime.UnixMilliString `json:"reaction_timestamp_ms"`
	SenderFBID          string                   `json:"sender_fbid"`
}

type MentionList []*Mention

func (ml MentionList) ToSocket() []socket.Mention {
	return exslices.CastFunc(ml, (*Mention).ToSocket)
}

type ProfileRangeType string

func (t ProfileRangeType) ToSocket() socket.MentionType {
	switch t {
	case ProfileRangeTypeProfile:
		return socket.MentionTypePerson
	case ProfileRangeTypeThread:
		return socket.MentionTypeThread
	case ProfileRangeTypeSilent:
		return socket.MentionTypeSilent
	case ProfileRangeTypeThreadActive:
		return socket.MentionTypeThreadActive
	case ProfileRangeTypeCommunityChannel:
		return socket.MentionTypeCommunityChannel
	case ProfileRangeTypeCustom:
		return socket.MentionTypeCustomCommand
	case ProfileRangeTypeAI:
		return socket.MentionTypeAI
	default:
		return socket.MentionTypeUnknown
	}
}

const (
	ProfileRangeTypeProfile          ProfileRangeType = "PROFILE"
	ProfileRangeTypeThread           ProfileRangeType = "THREAD" // @everyone
	ProfileRangeTypeThreadActive     ProfileRangeType = "THREAD_ACTIVE"
	ProfileRangeTypeCommunityChannel ProfileRangeType = "COMMUNITY_CHANNEL"
	ProfileRangeTypeSilent           ProfileRangeType = "SILENT"
	ProfileRangeTypeCustom           ProfileRangeType = "CUSTOM"
	ProfileRangeTypeAI               ProfileRangeType = "AI"
)

type Mention struct {
	Offset           int              `json:"offset"`
	Length           int              `json:"length"`
	ProfileRangeType ProfileRangeType `json:"profile_range_type"`
	UserFBID         int64            `json:"user_fbid,string"` // when @everyone is mentioned, this is the chat id
}

type InputMention struct {
	Offset int   `json:"offset"`
	Length int   `json:"length"`
	FBID   int64 `json:"fbid,string"`
}

type InputCommand struct {
	Offset           int    `json:"offset"`
	Length           int    `json:"length"`
	Type             string `json:"type"` // EVERYONE for @everyone
	UserOrThreadFBID int64  `json:"user_or_thread_fbid,string"`
}

func (m *Mention) ToSocket() socket.Mention {
	return socket.Mention{
		ID:     m.UserFBID,
		Offset: m.Offset,
		Length: m.Length,
		Type:   m.ProfileRangeType.ToSocket(),
	}
}

func SocketMentionsToInput(s socket.Mentions) (mentions []InputMention, mentionedUserIDs []string, commands []InputCommand) {
	for _, m := range s {
		switch m.Type {
		case socket.MentionTypePerson:
			mentions = append(mentions, InputMention{
				Offset: m.Offset,
				Length: m.Length,
				FBID:   m.ID,
			})
			mentionedUserIDs = append(mentionedUserIDs, strconv.FormatInt(m.ID, 10))
		case socket.MentionTypeThread:
			commands = append(commands, InputCommand{
				Offset:           m.Offset,
				Length:           m.Length,
				Type:             "EVERYONE",
				UserOrThreadFBID: m.ID,
			})
			// TODO other types?
		}
	}
	return
}

type MessageEditHistoryEntry struct {
	Typename    string                   `json:"__typename"`
	Body        string                   `json:"body"`
	TimestampMS jsontime.UnixMilliString `json:"timestamp_ms"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContent struct {
	Typename              string        `json:"__typename"`
	IsSlideMessageContent string        `json:"__isSlideMessageContent"`
	TextBody              string        `json:"text_body,omitempty"`
	Videos                []*Attachment `json:"videos,omitempty"`
	Attachments           []*Attachment `json:"attachments,omitempty"`
	XMA                   *XMAContent   `json:"xma,omitempty"`
	XMATextBody           string        `json:"xma_text_body,omitempty"`

	Unrecognized map[string]any `json:",unknown"`
}

type Attachment struct {
	Typename                        string `json:"__typename"`
	IsSlideMessagingMediaAttachment string `json:"__isSlideMessagingMediaAttachment"`

	PreviewCDNURL            string `json:"preview_cdn_url"`
	AttachmentFBID           string `json:"attachment_fbid"`
	AttachmentType           int    `json:"attachment_type"`
	PreviewCDNFallbackURL    string `json:"preview_cdn_fallback_url"`
	PreviewHeight            int    `json:"preview_height"`
	PreviewWidth             int    `json:"preview_width"`
	AttachmentCDNURL         string `json:"attachment_cdn_url"`
	AttachmentCDNFallbackURL string `json:"attachment_cdn_fallback_url"`

	// Only for videos
	DashManifest any `json:"dash_manifest"`

	Unrecognized map[string]any `json:",unknown"`
}

type XMAPreviewImage struct {
	URL                        string `json:"url"`
	FallbackURL                string `json:"fallback_url"`
	Width                      int    `json:"width"`
	Height                     int    `json:"height"`
	PreviewImageDecorationType any    `json:"preview_image_decoration_type"`

	Unrecognized map[string]any `json:",unknown"`
}

type XMAContent struct {
	Typename               string           `json:"__typename"`
	XMAHeaderTitle         any              `json:"xmaHeaderTitle"`
	XMATitle               string           `json:"xmaTitle"`
	XMAPreviewImage        *XMAPreviewImage `json:"xmaPreviewImage"`
	TitleText              string           `json:"title_text"`
	HeaderTitleText        any              `json:"header_title_text"`
	PreviewImage           *XMAPreviewImage `json:"preview_image"`
	TargetID               any              `json:"target_id"`
	TargetURL              string           `json:"target_url"`
	HeaderIcon             any              `json:"header_icon"`
	HeaderSubtitleText     any              `json:"header_subtitle_text"`
	VerifiedType           any              `json:"verified_type"`
	CaptionBodyText        any              `json:"caption_body_text"`
	SubtitleText           any              `json:"subtitle_text"`
	SubtitleDecorationType any              `json:"subtitle_decoration_type"`
	CtaButtons             []any            `json:"cta_buttons"`
	PreviewLayoutType      string           `json:"preview_layout_type"`
	PreviewExtraURLsInfo   []any            `json:"preview_extra_urls_info"`
	OverlayTitle           any              `json:"overlay_title"`
	OverlayDescription     any              `json:"overlay_description"`
	OverlayIconGlyph       any              `json:"overlay_icon_glyph"`
	Favicon                any              `json:"favicon"`
	EyebrowText            any              `json:"eyebrow_text"`
	CollapsibleID          any              `json:"collapsible_id"`

	Unrecognized map[string]any `json:",unknown"`
}

type SentMessage struct {
	MessageID   string                   `json:"message_id"`
	TimestampMS jsontime.UnixMilliString `json:"timestamp_ms"`
	ID          string                   `json:"id"`
}
