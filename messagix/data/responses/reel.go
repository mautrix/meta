package responses

/*
Reel info is mapped by the reel id
*/
type ReelInfoResponse struct {
	Reels      map[string]ReelInfo `json:"reels,omitempty"`
	ReelsMedia []ReelInfo          `json:"reels_media,omitempty"`
	Status     string              `json:"status,omitempty"`
}

type ReelInfo struct {
	ID                          string      `json:"id,omitempty"`
	StrongID                    string      `json:"strong_id__,omitempty"`
	LatestReelMedia             int         `json:"latest_reel_media,omitempty"`
	ExpiringAt                  int         `json:"expiring_at,omitempty"`
	Seen                        int         `json:"seen,omitempty"`
	CanReply                    bool        `json:"can_reply,omitempty"`
	CanGifQuickReply            bool        `json:"can_gif_quick_reply,omitempty"`
	CanReshare                  bool        `json:"can_reshare,omitempty"`
	CanReactWithAvatar          bool        `json:"can_react_with_avatar,omitempty"`
	ReelType                    string      `json:"reel_type,omitempty"`
	AdExpiryTimestampInMillis   any         `json:"ad_expiry_timestamp_in_millis,omitempty"`
	IsCtaStickerAvailable       any         `json:"is_cta_sticker_available,omitempty"`
	AppStickerInfo              any         `json:"app_sticker_info,omitempty"`
	ShouldTreatLinkStickerAsCta any         `json:"should_treat_link_sticker_as_cta,omitempty"`
	CoverMedia                  CoverMedia  `json:"cover_media,omitempty"`
	User                        *User       `json:"user,omitempty"`
	Items                       []*ReelItem `json:"items,omitempty"`
	Title                       string      `json:"title,omitempty"`
	CreatedAt                   int         `json:"created_at,omitempty"`
	IsPinnedHighlight           bool        `json:"is_pinned_highlight,omitempty"`
	PrefetchCount               int         `json:"prefetch_count,omitempty"`
	MediaCount                  int         `json:"media_count,omitempty"`
	MediaIds                    []string    `json:"media_ids,omitempty"`
	IsCacheable                 bool        `json:"is_cacheable,omitempty"`
	IsConvertedToClips          bool        `json:"is_converted_to_clips,omitempty"`
	DisabledReplyTypes          []string    `json:"disabled_reply_types,omitempty"`
	HighlightReelType           string      `json:"highlight_reel_type,omitempty"`
}
type CoverMedia struct {
	CropRect            []float64 `json:"crop_rect,omitempty"`
	CroppedImageVersion struct {
		Height       int    `json:"height,omitempty"`
		ScansProfile string `json:"scans_profile,omitempty"`
		URL          string `json:"url,omitempty"`
		Width        int    `json:"width,omitempty"`
	} `json:"cropped_image_version,omitempty"`
	FullImageVersion any    `json:"full_image_version,omitempty"`
	MediaID          string `json:"media_id,omitempty"`
	UploadID         string `json:"upload_id,omitempty"`
}
type HighlightsInfo struct {
	AddedTo []struct {
		ReelID string `json:"reel_id,omitempty"`
		Title  string `json:"title,omitempty"`
	} `json:"added_to,omitempty"`
}
type StoryFeedMedia struct {
	X                float64 `json:"x,omitempty"`
	Y                float64 `json:"y,omitempty"`
	Z                float64 `json:"z,omitempty"`
	Width            float64 `json:"width,omitempty"`
	Height           float64 `json:"height,omitempty"`
	Rotation         float64 `json:"rotation,omitempty"`
	IsPinned         int     `json:"is_pinned,omitempty"`
	IsHidden         int     `json:"is_hidden,omitempty"`
	IsSticker        int     `json:"is_sticker,omitempty"`
	IsFbSticker      int     `json:"is_fb_sticker,omitempty"`
	StartTimeMs      float64 `json:"start_time_ms,omitempty"`
	EndTimeMs        float64 `json:"end_time_ms,omitempty"`
	MediaID          string  `json:"media_id,omitempty"`
	ProductType      string  `json:"product_type,omitempty"`
	MediaCode        string  `json:"media_code,omitempty"`
	MediaCompoundStr string  `json:"media_compound_str,omitempty"`
}
type ReelItem struct {
	Items

	CaptionPosition             float64          `json:"caption_position,omitempty"`
	IsReelMedia                 bool             `json:"is_reel_media,omitempty"`
	IsTerminalVideoSegment      bool             `json:"is_terminal_video_segment,omitempty"`
	ExpiringAt                  int              `json:"expiring_at,omitempty"`
	Likers                      []any            `json:"likers,omitempty"`
	HasTranslation              bool             `json:"has_translation,omitempty"`
	AccessibilityCaption        string           `json:"accessibility_caption,omitempty"`
	HighlightsInfo              HighlightsInfo   `json:"highlights_info,omitempty"`
	AttributionContentURL       string           `json:"attribution_content_url,omitempty"`
	Crosspost                   []string         `json:"crosspost,omitempty"`
	CanReshare                  bool             `json:"can_reshare,omitempty"`
	CanReply                    bool             `json:"can_reply,omitempty"`
	CanSendPrompt               bool             `json:"can_send_prompt,omitempty"`
	IsFirstTake                 bool             `json:"is_first_take,omitempty"`
	IsRollcallV2                bool             `json:"is_rollcall_v2,omitempty"`
	IsSuperlative               bool             `json:"is_superlative,omitempty"`
	IsFbPostFromFbStory         bool             `json:"is_fb_post_from_fb_story,omitempty"`
	CanPlaySpotifyAudio         bool             `json:"can_play_spotify_audio,omitempty"`
	ArchiveStoryDeletionTs      int              `json:"archive_story_deletion_ts,omitempty"`
	CreatedFromAddYoursBrowsing bool             `json:"created_from_add_yours_browsing,omitempty"`
	StoryFeedMedia              []StoryFeedMedia `json:"story_feed_media,omitempty"`
	SupportsReelReactions       bool             `json:"supports_reel_reactions,omitempty"`
	CanSendCustomEmojis         bool             `json:"can_send_custom_emojis,omitempty"`
	ShowOneTapFbShareTooltip    bool             `json:"show_one_tap_fb_share_tooltip,omitempty"`
}
