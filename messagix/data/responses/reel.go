package responses

/*
	Reel info is mapped by the reel id
*/
type ReelInfoResponse struct {
	Reels map[string]ReelInfo `json:"reels,omitempty"`
	ReelsMedia []ReelInfo `json:"reels_media,omitempty"`
	Status string `json:"status,omitempty"`
}

type ReelInfo struct {
	ID                          string `json:"id,omitempty"`
	StrongID                    string `json:"strong_id__,omitempty"`
	LatestReelMedia             int    `json:"latest_reel_media,omitempty"`
	ExpiringAt                  int    `json:"expiring_at,omitempty"`
	Seen                        int    `json:"seen,omitempty"`
	CanReply                    bool   `json:"can_reply,omitempty"`
	CanGifQuickReply            bool   `json:"can_gif_quick_reply,omitempty"`
	CanReshare                  bool   `json:"can_reshare,omitempty"`
	CanReactWithAvatar          bool   `json:"can_react_with_avatar,omitempty"`
	ReelType                    string `json:"reel_type,omitempty"`
	AdExpiryTimestampInMillis   any    `json:"ad_expiry_timestamp_in_millis,omitempty"`
	IsCtaStickerAvailable       any    `json:"is_cta_sticker_available,omitempty"`
	AppStickerInfo              any    `json:"app_sticker_info,omitempty"`
	ShouldTreatLinkStickerAsCta any    `json:"should_treat_link_sticker_as_cta,omitempty"`
	CoverMedia struct {
		CropRect            []float64 `json:"crop_rect,omitempty"`
		CroppedImageVersion struct {
			Height       int    `json:"height,omitempty"`
			ScansProfile string `json:"scans_profile,omitempty"`
			URL          string `json:"url,omitempty"`
			Width        int    `json:"width,omitempty"`
		} `json:"cropped_image_version,omitempty"`
		FullImageVersion any    `json:"full_image_version,omitempty"`
		MediaID          string `json:"media_id,omitempty"`
		UploadID         string    `json:"upload_id,omitempty"`
	} `json:"cover_media,omitempty"`
	User                        struct {
		Pk                       string `json:"pk,omitempty"`
		PkID                     string `json:"pk_id,omitempty"`
		FullName                 string `json:"full_name,omitempty"`
		IsPrivate                bool   `json:"is_private,omitempty"`
		StrongID                 string `json:"strong_id__,omitempty"`
		Username                 string `json:"username,omitempty"`
		InteropMessagingUserFbid int64  `json:"interop_messaging_user_fbid,omitempty"`
		IsVerified               bool   `json:"is_verified,omitempty"`
		FriendshipStatus         struct {
			Following       bool `json:"following,omitempty"`
			IsPrivate       bool `json:"is_private,omitempty"`
			IncomingRequest bool `json:"incoming_request,omitempty"`
			OutgoingRequest bool `json:"outgoing_request,omitempty"`
			IsBestie        bool `json:"is_bestie,omitempty"`
			IsRestricted    bool `json:"is_restricted,omitempty"`
			IsFeedFavorite  bool `json:"is_feed_favorite,omitempty"`
		} `json:"friendship_status,omitempty"`
		ProfilePicID  string `json:"profile_pic_id,omitempty"`
		ProfilePicURL string `json:"profile_pic_url,omitempty"`
	} `json:"user,omitempty"`
	Items []struct {
		TakenAt                             int    `json:"taken_at,omitempty"`
		Pk                                  string `json:"pk,omitempty"`
		ID                                  string `json:"id,omitempty"`
		CaptionPosition                     float64    `json:"caption_position,omitempty"`
		IsReelMedia                         bool   `json:"is_reel_media,omitempty"`
		IsTerminalVideoSegment              bool   `json:"is_terminal_video_segment,omitempty"`
		DeviceTimestamp                     int64  `json:"device_timestamp,omitempty"`
		ClientCacheKey                      string `json:"client_cache_key,omitempty"`
		FilterType                          int    `json:"filter_type,omitempty"`
		CaptionIsEdited                     bool   `json:"caption_is_edited,omitempty"`
		LikeAndViewCountsDisabled           bool   `json:"like_and_view_counts_disabled,omitempty"`
		StrongID                            string `json:"strong_id__,omitempty"`
		IsReshareOfTextPostAppMediaInIg     bool   `json:"is_reshare_of_text_post_app_media_in_ig,omitempty"`
		IsPostLiveClipsMedia                bool   `json:"is_post_live_clips_media,omitempty"`
		DeletedReason                       int    `json:"deleted_reason,omitempty"`
		IntegrityReviewDecision             string `json:"integrity_review_decision,omitempty"`
		HasSharedToFb                       int    `json:"has_shared_to_fb,omitempty"`
		ExpiringAt                          int    `json:"expiring_at,omitempty"`
		IsUnifiedVideo                      bool   `json:"is_unified_video,omitempty"`
		ShouldRequestAds                    bool   `json:"should_request_ads,omitempty"`
		IsVisualReplyCommenterNoticeEnabled bool   `json:"is_visual_reply_commenter_notice_enabled,omitempty"`
		CommercialityStatus                 string `json:"commerciality_status,omitempty"`
		ExploreHideComments                 bool   `json:"explore_hide_comments,omitempty"`
		ShopRoutingUserID                   any    `json:"shop_routing_user_id,omitempty"`
		CanSeeInsightsAsBrand               bool   `json:"can_see_insights_as_brand,omitempty"`
		IsOrganicProductTaggingEligible     bool   `json:"is_organic_product_tagging_eligible,omitempty"`
		Likers                              []any  `json:"likers,omitempty"`
		MediaType                           int    `json:"media_type,omitempty"`
		Code                                string `json:"code,omitempty"`
		Caption                             any    `json:"caption,omitempty"`
		ClipsTabPinnedUserIds               []any  `json:"clips_tab_pinned_user_ids,omitempty"`
		CommentInformTreatment              struct {
			ShouldHaveInformTreatment bool   `json:"should_have_inform_treatment,omitempty"`
			Text                      string `json:"text,omitempty"`
			URL                       any    `json:"url,omitempty"`
			ActionType                any    `json:"action_type,omitempty"`
		} `json:"comment_inform_treatment,omitempty"`
		SharingFrictionInfo struct {
			ShouldHaveSharingFriction bool `json:"should_have_sharing_friction,omitempty"`
			BloksAppURL               any  `json:"bloks_app_url,omitempty"`
			SharingFrictionPayload    any  `json:"sharing_friction_payload,omitempty"`
		} `json:"sharing_friction_info,omitempty"`
		HasTranslation                   bool   `json:"has_translation,omitempty"`
		AccessibilityCaption             string `json:"accessibility_caption,omitempty"`
		OriginalMediaHasVisualReplyMedia bool   `json:"original_media_has_visual_reply_media,omitempty"`
		CanViewerSave                    bool   `json:"can_viewer_save,omitempty"`
		IsInProfileGrid                  bool   `json:"is_in_profile_grid,omitempty"`
		ProfileGridControlEnabled        bool   `json:"profile_grid_control_enabled,omitempty"`
		IsCommentsGifComposerEnabled     bool   `json:"is_comments_gif_composer_enabled,omitempty"`
		HighlightsInfo                   struct {
			AddedTo []struct {
				ReelID string `json:"reel_id,omitempty"`
				Title  string `json:"title,omitempty"`
			} `json:"added_to,omitempty"`
		} `json:"highlights_info,omitempty"`
		ProductSuggestions               []any  `json:"product_suggestions,omitempty"`
		AttributionContentURL            string `json:"attribution_content_url,omitempty"`
		ImageVersions2                   struct {
			Candidates []struct {
				Width  int    `json:"width,omitempty"`
				Height int    `json:"height,omitempty"`
				URL    string `json:"url,omitempty"`
			} `json:"candidates,omitempty"`
		} `json:"image_versions2,omitempty"`
		OriginalWidth            int      `json:"original_width,omitempty"`
		OriginalHeight           int      `json:"original_height,omitempty"`
		ProductType              string   `json:"product_type,omitempty"`
		IsPaidPartnership        bool     `json:"is_paid_partnership,omitempty"`
		MusicMetadata            any      `json:"music_metadata,omitempty"`
		OrganicTrackingToken     string   `json:"organic_tracking_token,omitempty"`
		IgMediaSharingDisabled   bool     `json:"ig_media_sharing_disabled,omitempty"`
		Crosspost                []string `json:"crosspost,omitempty"`
		IsOpenToPublicSubmission bool     `json:"is_open_to_public_submission,omitempty"`
		HasDelayedMetadata       bool     `json:"has_delayed_metadata,omitempty"`
		IsAutoCreated            bool     `json:"is_auto_created,omitempty"`
		IsCutoutStickerAllowed   bool     `json:"is_cutout_sticker_allowed,omitempty"`
		User                     struct {
			Pk        string `json:"pk,omitempty"`
			IsPrivate bool   `json:"is_private,omitempty"`
		} `json:"user,omitempty"`
		CanReshare                  bool `json:"can_reshare,omitempty"`
		CanReply                    bool `json:"can_reply,omitempty"`
		CanSendPrompt               bool `json:"can_send_prompt,omitempty"`
		IsFirstTake                 bool `json:"is_first_take,omitempty"`
		IsRollcallV2                bool `json:"is_rollcall_v2,omitempty"`
		IsSuperlative               bool `json:"is_superlative,omitempty"`
		IsFbPostFromFbStory         bool `json:"is_fb_post_from_fb_story,omitempty"`
		CanPlaySpotifyAudio         bool `json:"can_play_spotify_audio,omitempty"`
		ArchiveStoryDeletionTs      int  `json:"archive_story_deletion_ts,omitempty"`
		CreatedFromAddYoursBrowsing bool `json:"created_from_add_yours_browsing,omitempty"`
		StoryFeedMedia              []struct {
			X                float64 `json:"x,omitempty"`
			Y                float64 `json:"y,omitempty"`
			Z                float64     `json:"z,omitempty"`
			Width            float64 `json:"width,omitempty"`
			Height           float64 `json:"height,omitempty"`
			Rotation         float64     `json:"rotation,omitempty"`
			IsPinned         int     `json:"is_pinned,omitempty"`
			IsHidden         int     `json:"is_hidden,omitempty"`
			IsSticker        int     `json:"is_sticker,omitempty"`
			IsFbSticker      int     `json:"is_fb_sticker,omitempty"`
			StartTimeMs      float64     `json:"start_time_ms,omitempty"`
			EndTimeMs        float64     `json:"end_time_ms,omitempty"`
			MediaID          string  `json:"media_id,omitempty"`
			ProductType      string  `json:"product_type,omitempty"`
			MediaCode        string  `json:"media_code,omitempty"`
			MediaCompoundStr string  `json:"media_compound_str,omitempty"`
		} `json:"story_feed_media,omitempty"`
		HasLiked                 bool `json:"has_liked,omitempty"`
		SupportsReelReactions    bool `json:"supports_reel_reactions,omitempty"`
		CanSendCustomEmojis      bool `json:"can_send_custom_emojis,omitempty"`
		ShowOneTapFbShareTooltip bool `json:"show_one_tap_fb_share_tooltip,omitempty"`
	} `json:"items,omitempty"`
	Title              string   `json:"title,omitempty"`
	CreatedAt          int      `json:"created_at,omitempty"`
	IsPinnedHighlight  bool     `json:"is_pinned_highlight,omitempty"`
	PrefetchCount      int      `json:"prefetch_count,omitempty"`
	MediaCount         int      `json:"media_count,omitempty"`
	MediaIds           []string `json:"media_ids,omitempty"`
	IsCacheable        bool     `json:"is_cacheable,omitempty"`
	IsConvertedToClips bool     `json:"is_converted_to_clips,omitempty"`
	DisabledReplyTypes []string `json:"disabled_reply_types,omitempty"`
	HighlightReelType  string   `json:"highlight_reel_type,omitempty"`
}