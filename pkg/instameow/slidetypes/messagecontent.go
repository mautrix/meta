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
)

type MessageContent interface {
	isMessageContent()
}

func (*MessageContentText) isMessageContent()             {}
func (*MessageContentAdminText) isMessageContent()        {}
func (*MessageContentImage) isMessageContent()            {}
func (*MessageContentVideo) isMessageContent()            {}
func (*MessageContentAudio) isMessageContent()            {}
func (*MessageContentRavenMedia) isMessageContent()       {}
func (*MessageContentAnimatedMedia) isMessageContent()    {}
func (*MessageContentMultiMedia) isMessageContent()       {}
func (*MessageContentSticker) isMessageContent()          {}
func (*MessageContentXMA) isMessageContent()              {}
func (*MessageContentMusicSticker) isMessageContent()     {}
func (*MessageContentAIRichResponse) isMessageContent()   {}
func (*MessageContentAISearchResponse) isMessageContent() {}
func (MessageContentUnknown) isMessageContent()           {}

type MessageContentText struct {
	TextBody string `json:"text_body"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentImage struct {
	Attachments []*Attachment `json:"attachments"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentVideo struct {
	Videos []*Attachment `json:"videos"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentMultiMedia struct {
	Attachments []*Attachment `json:"ordered_photo_video_attachments"`

	Unrecognized map[string]any `json:",unknown"`
}

type RavenViewMode int

const (
	RavenViewModeViewOnce   RavenViewMode = 0
	RavenViewModeViewTwice  RavenViewMode = 1 // "allow replay"
	RavenViewModeKeepInChat RavenViewMode = 2
)

type MessageContentRavenMedia struct {
	ViewMode   RavenViewMode `json:"view_mode"`
	Attachment *Attachment   `json:"attachment"`

	Unrecognized map[string]any `json:",unknown"`
}

type Attachment struct {
	Typename                        string `json:"__typename"`
	IsSlideMessagingMediaAttachment string `json:"__isSlideMessagingMediaAttachment"`

	PreviewCDNURL            string `json:"preview_cdn_url"`
	AttachmentFBID           string `json:"attachment_fbid"`
	AttachmentType           int    `json:"attachment_type"` // Not present for raven attachments
	PreviewCDNFallbackURL    string `json:"preview_cdn_fallback_url"`
	PreviewHeight            int    `json:"preview_height"`
	PreviewWidth             int    `json:"preview_width"`
	AttachmentCDNURL         string `json:"attachment_cdn_url"`
	AttachmentCDNFallbackURL string `json:"attachment_cdn_fallback_url"`

	// Only for videos
	DashManifest any `json:"dash_manifest"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentAudio struct {
	AudioAttachments []*AudioAttachment `json:"audio_attachments"`

	Unrecognized map[string]any `json:",unknown"`
}

type AudioAttachment struct {
	AttachmentFBID     string `json:"attachment_fbid"`
	WaveformData       []any  `json:"waveform_data"`
	PlayableDurationMS int    `json:"playable_duration_ms"`
	AttachmentCDNURL   string `json:"attachment_cdn_url"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentAnimatedMedia struct {
	AnimatedMedia []*AnimatedAttachment `json:"animated_media"`

	Unrecognized map[string]any `json:",unknown"`
}

type AnimatedAttachment struct {
	PreviewCDNURL     string `json:"preview_cdn_url"`
	AltText           string `json:"alt_text"`
	AttachmentWebpURL string `json:"attachment_webp_url"`
	PreviewHeight     int    `json:"preview_height"`
	PreviewWidth      int    `json:"preview_width"`
	IsSticker         bool   `json:"is_sticker"`
	AttachmentMP4URL  string `json:"attachment_mp4_url"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentAdminText struct {
	IsReactionActionLog bool           `json:"is_reaction_action_log"`
	TextFragments       []TextFragment `json:"text_fragments"`

	Unrecognized map[string]any `json:",unknown"`
}

type TextFragment struct {
	Plaintext    string        `json:"plaintext"`
	LinkFragment *LinkFragment `json:"link_fragment"`

	Unrecognized map[string]any `json:",unknown"`
}

type LinkFragment struct {
	URI string `json:"uri"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentSticker struct {
	AltText       string `json:"alt_text"`
	PreviewURL    string `json:"preview_url"`
	PreviewHeight int    `json:"preview_height"`
	PreviewWidth  int    `json:"preview_width"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentXMA struct {
	XMA         *XMAContent `json:"xma"`
	XMATextBody string      `json:"xma_text_body"`

	Unrecognized map[string]any `json:",unknown"`
}

type XMAContent struct {
	Typename               string           `json:"__typename"`
	XMAHeaderTitle         any              `json:"xmaHeaderTitle"`
	XMATitle               string           `json:"xmaTitle"`
	XMAPreviewImage        *XMAPreviewImage `json:"xmaPreviewImage"`
	TitleText              string           `json:"title_text"`
	HeaderTitleText        string           `json:"header_title_text"`
	PreviewImage           *XMAPreviewImage `json:"preview_image"`
	TargetID               string           `json:"target_id"`
	TargetURL              string           `json:"target_url"`
	HeaderIcon             *XMAIcon         `json:"header_icon"`
	HeaderSubtitleText     string           `json:"header_subtitle_text"`
	VerifiedType           string           `json:"verified_type"`
	CaptionBodyText        string           `json:"caption_body_text"`
	SubtitleText           string           `json:"subtitle_text"`
	SubtitleDecorationType any              `json:"subtitle_decoration_type"`
	CTAButtons             []any            `json:"cta_buttons"`
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

type XMAIcon struct {
	URL string `json:"url"`

	Unrecognized map[string]any `json:",unknown"`
}

type XMAPreviewImage struct {
	URL                        string `json:"url"`
	FallbackURL                string `json:"fallback_url"`
	Width                      int    `json:"width"`
	Height                     int    `json:"height"`
	PreviewImageDecorationType string `json:"preview_image_decoration_type"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentMusicSticker struct {
	MediaContentFBID string `json:"media_content_fbid"`
	AudioTrack       struct {
		Web30SPreviewDownloadURL string `json:"web_30s_preview_download_url"`
		Title                    string `json:"title"`
		DisplayArtist            string `json:"display_artist"`
	} `json:"audio_track"`
	AudioAssetID    string `json:"audio_asset_id"`
	AttributionLink string `json:"attribution_link"`
	PreviewURL      string `json:"preview_url"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentAIRichResponse struct { // TODO confirm types, these haven't been observed in the wild yet
	UnifiedResponse        string `json:"unified_response"`
	UnifiedResponsePayload any    `json:"unified_response_payload"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentAISearchResponse struct { // TODO confirm types, these haven't been observed in the wild yet
	MessageTextBody string           `json:"messasge_text_body"`
	SearchSources   []AISearchSource `json:"search_sources"`

	Unrecognized map[string]any `json:",unknown"`
}

type AISearchSource struct {
	Typename string `json:"__typename"`

	SourceTitle    string `json:"source_title"`
	SourceSubtitle string `json:"source_subtitle"`
	SourceURL      string `json:"source_url"`

	Unrecognized map[string]any `json:",unknown"`
}

type MessageContentUnknown map[string]any

type MessageContentWrapper struct {
	TypeName              string         `json:"__typename"`
	IsSlideMessageContent string         `json:"__isSlideMessageContent"`
	Content               MessageContent `json:"-"`
}

type marshalableContent MessageContentWrapper

func (mc *MessageContentWrapper) UnmarshalJSON(data []byte) (err error) {
	err = json.Unmarshal(data, (*marshalableContent)(mc))
	if err != nil {
		return err
	}
	switch mc.TypeName {
	case "SlideMessageText":
		mc.Content = &MessageContentText{}
	case "SlideMessageAdminText":
		mc.Content = &MessageContentAdminText{}
	case "SlideMessageImageContent":
		mc.Content = &MessageContentImage{}
	case "SlideMessageVideosContent":
		mc.Content = &MessageContentVideo{}
	case "SlideMessageAudiosContent":
		mc.Content = &MessageContentAudio{}
	case "SlideMessageAnimatedMediaContent":
		mc.Content = &MessageContentAnimatedMedia{}
	case "SlideMessageRavenImageContent":
		mc.Content = &MessageContentRavenMedia{}
	case "SlideMessageRavenVideoContent":
		mc.Content = &MessageContentRavenMedia{}
	case "SlideMessageMultiMediaContent":
		mc.Content = &MessageContentMultiMedia{}
	case "SlideMessageStoreStickerContent",
		"SlideMessageAvatarStickerXMAContent",
		"SlideMessageCutoutStickerXMAContent",
		"SlideMessageAIStickerXMAContent":
		mc.Content = &MessageContentSticker{}
	case "SlideMessageXMAContent":
		mc.Content = &MessageContentXMA{}
	case "SlideMessageMusicStickerXMAContent":
		mc.Content = &MessageContentMusicSticker{}
	case "SlideMessageGenAIRichResponseXMAContent":
		mc.Content = &MessageContentAIRichResponse{}
	case "SlideMessageGenAISearchPluginResponseXMAContent":
		mc.Content = &MessageContentAISearchResponse{}
	default:
		var unknown map[string]any
		err = json.Unmarshal(data, &unknown)
		mc.Content = MessageContentUnknown(unknown)
		return err
	}
	err = json.Unmarshal(data, mc.Content)
	return err
}
