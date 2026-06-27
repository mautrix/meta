package responses

type FetchMediaResponse struct {
	Items               []*Items `json:"items,omitempty"`
	NumResults          int      `json:"num_results,omitempty"`
	MoreAvailable       bool     `json:"more_available,omitempty"`
	AutoLoadMoreEnabled bool     `json:"auto_load_more_enabled,omitempty"`
	Status              string   `json:"status,omitempty"`
}
type FriendshipStatus struct {
	Following       bool `json:"following,omitempty"`
	FollowedBy      bool `json:"followed_by,omitempty"`
	Blocking        bool `json:"blocking,omitempty"`
	Muting          bool `json:"muting,omitempty"`
	IsPrivate       bool `json:"is_private,omitempty"`
	IncomingRequest bool `json:"incoming_request,omitempty"`
	OutgoingRequest bool `json:"outgoing_request,omitempty"`
	IsBestie        bool `json:"is_bestie,omitempty"`
	IsRestricted    bool `json:"is_restricted,omitempty"`
	IsFeedFavorite  bool `json:"is_feed_favorite,omitempty"`
}
type In struct {
	User                  User      `json:"user,omitempty"`
	Position              []float64 `json:"position,omitempty"`
	StartTimeInVideoInSec any       `json:"start_time_in_video_in_sec,omitempty"`
	DurationInVideoInSec  any       `json:"duration_in_video_in_sec,omitempty"`
}
type Usertags struct {
	In []In `json:"in,omitempty"`
}
type FanConsiderationPageRevampEligiblity struct {
	ShouldShowSocialContext  bool `json:"should_show_social_context,omitempty"`
	ShouldShowContentPreview bool `json:"should_show_content_preview,omitempty"`
}
type FanClubInfo struct {
	FanClubID                            string                               `json:"fan_club_id,omitempty"`
	FanClubName                          string                               `json:"fan_club_name,omitempty"`
	IsFanClubReferralEligible            bool                                 `json:"is_fan_club_referral_eligible,omitempty"`
	FanConsiderationPageRevampEligiblity FanConsiderationPageRevampEligiblity `json:"fan_consideration_page_revamp_eligiblity,omitempty"`
	IsFanClubGiftingEligible             bool                                 `json:"is_fan_club_gifting_eligible,omitempty"`
	SubscriberCount                      int                                  `json:"subscriber_count,omitempty"`
	ConnectedMemberCount                 int                                  `json:"connected_member_count,omitempty"`
	AutosaveToExclusiveHighlight         bool                                 `json:"autosave_to_exclusive_highlight,omitempty"`
	HasEnoughSubscribersForSsc           bool                                 `json:"has_enough_subscribers_for_ssc,omitempty"`
}

type HdProfilePicURLInfo struct {
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}
type HdProfilePicVersions struct {
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	URL    string `json:"url,omitempty"`
}
type User struct {
	FbidV2                         string                 `json:"fbid_v2,omitempty"`
	FeedPostReshareDisabled        bool                   `json:"feed_post_reshare_disabled,omitempty"`
	FullName                       string                 `json:"full_name,omitempty"`
	ID                             string                 `json:"id,omitempty"`
	IsPrivate                      bool                   `json:"is_private,omitempty"`
	IsUnpublished                  bool                   `json:"is_unpublished,omitempty"`
	Pk                             string                 `json:"pk,omitempty"`
	PkID                           string                 `json:"pk_id,omitempty"`
	ShowAccountTransparencyDetails bool                   `json:"show_account_transparency_details,omitempty"`
	StrongID                       string                 `json:"strong_id__,omitempty"`
	ThirdPartyDownloadsEnabled     int                    `json:"third_party_downloads_enabled,omitempty"`
	AccountBadges                  []any                  `json:"account_badges,omitempty"`
	FanClubInfo                    FanClubInfo            `json:"fan_club_info,omitempty"`
	FriendshipStatus               FriendshipStatus       `json:"friendship_status,omitempty"`
	HasAnonymousProfilePicture     bool                   `json:"has_anonymous_profile_picture,omitempty"`
	HdProfilePicURLInfo            HdProfilePicURLInfo    `json:"hd_profile_pic_url_info,omitempty"`
	HdProfilePicVersions           []HdProfilePicVersions `json:"hd_profile_pic_versions,omitempty"`
	IsFavorite                     bool                   `json:"is_favorite,omitempty"`
	IsVerified                     bool                   `json:"is_verified,omitempty"`
	LatestReelMedia                int                    `json:"latest_reel_media,omitempty"`
	ProfilePicID                   string                 `json:"profile_pic_id,omitempty"`
	ProfilePicURL                  string                 `json:"profile_pic_url,omitempty"`
	TransparencyProductEnabled     bool                   `json:"transparency_product_enabled,omitempty"`
	Username                       string                 `json:"username,omitempty"`
	InteropMessagingUserFbid       any                    `json:"interop_messaging_user_fbid,omitempty"` // int or string
}
type Caption struct {
	Pk                 string `json:"pk,omitempty"`
	UserID             string `json:"user_id,omitempty"`
	User               User   `json:"user,omitempty"`
	Type               int    `json:"type,omitempty"`
	Text               string `json:"text,omitempty"`
	DidReportAsSpam    bool   `json:"did_report_as_spam,omitempty"`
	CreatedAt          int    `json:"created_at,omitempty"`
	CreatedAtUtc       int    `json:"created_at_utc,omitempty"`
	ContentType        string `json:"content_type,omitempty"`
	Status             string `json:"status,omitempty"`
	BitFlags           int    `json:"bit_flags,omitempty"`
	ShareEnabled       bool   `json:"share_enabled,omitempty"`
	IsRankedComment    bool   `json:"is_ranked_comment,omitempty"`
	IsCovered          bool   `json:"is_covered,omitempty"`
	PrivateReplyStatus int    `json:"private_reply_status,omitempty"`
	MediaID            string `json:"media_id,omitempty"`
}
type CommentInformTreatment struct {
	ShouldHaveInformTreatment bool   `json:"should_have_inform_treatment,omitempty"`
	Text                      string `json:"text,omitempty"`
	URL                       any    `json:"url,omitempty"`
	ActionType                any    `json:"action_type,omitempty"`
}
type SharingFrictionInfo struct {
	ShouldHaveSharingFriction bool `json:"should_have_sharing_friction,omitempty"`
	BloksAppURL               any  `json:"bloks_app_url,omitempty"`
	SharingFrictionPayload    any  `json:"sharing_friction_payload,omitempty"`
}
type MediaAppreciationSettings struct {
	MediaGiftingState   string `json:"media_gifting_state,omitempty"`
	GiftCountVisibility string `json:"gift_count_visibility,omitempty"`
}
type SquareCrop struct {
	CropLeft   float64 `json:"crop_left,omitempty"`
	CropRight  float64 `json:"crop_right,omitempty"`
	CropTop    float64 `json:"crop_top,omitempty"`
	CropBottom float64 `json:"crop_bottom,omitempty"`
}
type MediaCroppingInfo struct {
	SquareCrop SquareCrop `json:"square_crop,omitempty"`
}
type Candidates struct {
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	URL    string `json:"url,omitempty"`
}
type IgtvFirstFrame struct {
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	URL    string `json:"url,omitempty"`
}
type FirstFrame struct {
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	URL    string `json:"url,omitempty"`
}
type AdditionalCandidates struct {
	IgtvFirstFrame IgtvFirstFrame `json:"igtv_first_frame,omitempty"`
	FirstFrame     FirstFrame     `json:"first_frame,omitempty"`
	SmartFrame     any            `json:"smart_frame,omitempty"`
}
type Default struct {
	VideoLength                float64  `json:"video_length,omitempty"`
	ThumbnailWidth             int      `json:"thumbnail_width,omitempty"`
	ThumbnailHeight            int      `json:"thumbnail_height,omitempty"`
	ThumbnailDuration          float64  `json:"thumbnail_duration,omitempty"`
	SpriteUrls                 []string `json:"sprite_urls,omitempty"`
	ThumbnailsPerRow           int      `json:"thumbnails_per_row,omitempty"`
	TotalThumbnailNumPerSprite int      `json:"total_thumbnail_num_per_sprite,omitempty"`
	MaxThumbnailsPerSprite     int      `json:"max_thumbnails_per_sprite,omitempty"`
	SpriteWidth                int      `json:"sprite_width,omitempty"`
	SpriteHeight               int      `json:"sprite_height,omitempty"`
	RenderedWidth              int      `json:"rendered_width,omitempty"`
	FileSizeKb                 int      `json:"file_size_kb,omitempty"`
}
type ScrubberSpritesheetInfoCandidates struct {
	Default Default `json:"default,omitempty"`
}
type ImageVersions2 struct {
	Candidates                        []Candidates                      `json:"candidates,omitempty"`
	AdditionalCandidates              AdditionalCandidates              `json:"additional_candidates,omitempty"`
	SmartThumbnailEnabled             bool                              `json:"smart_thumbnail_enabled,omitempty"`
	ScrubberSpritesheetInfoCandidates ScrubberSpritesheetInfoCandidates `json:"scrubber_spritesheet_info_candidates,omitempty"`
}
type IgArtist struct {
	Pk            string `json:"pk,omitempty"`
	PkID          string `json:"pk_id,omitempty"`
	FullName      string `json:"full_name,omitempty"`
	IsPrivate     bool   `json:"is_private,omitempty"`
	StrongID      string `json:"strong_id__,omitempty"`
	Username      string `json:"username,omitempty"`
	IsVerified    bool   `json:"is_verified,omitempty"`
	ProfilePicID  string `json:"profile_pic_id,omitempty"`
	ProfilePicURL string `json:"profile_pic_url,omitempty"`
}
type ConsumptionInfo struct {
	IsBookmarked              bool   `json:"is_bookmarked,omitempty"`
	ShouldMuteAudioReason     string `json:"should_mute_audio_reason,omitempty"`
	IsTrendingInClips         bool   `json:"is_trending_in_clips,omitempty"`
	ShouldMuteAudioReasonType any    `json:"should_mute_audio_reason_type,omitempty"`
	DisplayMediaID            any    `json:"display_media_id,omitempty"`
}
type OriginalSoundInfo struct {
	AudioAssetID                    string          `json:"audio_asset_id,omitempty"`
	MusicCanonicalID                any             `json:"music_canonical_id,omitempty"`
	ProgressiveDownloadURL          string          `json:"progressive_download_url,omitempty"`
	DurationInMs                    int             `json:"duration_in_ms,omitempty"`
	DashManifest                    string          `json:"dash_manifest,omitempty"`
	IgArtist                        IgArtist        `json:"ig_artist,omitempty"`
	ShouldMuteAudio                 bool            `json:"should_mute_audio,omitempty"`
	HideRemixing                    bool            `json:"hide_remixing,omitempty"`
	OriginalMediaID                 string          `json:"original_media_id,omitempty"`
	TimeCreated                     int             `json:"time_created,omitempty"`
	OriginalAudioTitle              string          `json:"original_audio_title,omitempty"`
	ConsumptionInfo                 ConsumptionInfo `json:"consumption_info,omitempty"`
	CanRemixBeSharedToFb            bool            `json:"can_remix_be_shared_to_fb,omitempty"`
	FormattedClipsMediaCount        any             `json:"formatted_clips_media_count,omitempty"`
	AllowCreatorToRename            bool            `json:"allow_creator_to_rename,omitempty"`
	AudioParts                      []any           `json:"audio_parts,omitempty"`
	IsExplicit                      bool            `json:"is_explicit,omitempty"`
	OriginalAudioSubtype            string          `json:"original_audio_subtype,omitempty"`
	IsAudioAutomaticallyAttributed  bool            `json:"is_audio_automatically_attributed,omitempty"`
	IsReuseDisabled                 bool            `json:"is_reuse_disabled,omitempty"`
	IsXpostFromFb                   bool            `json:"is_xpost_from_fb,omitempty"`
	XpostFbCreatorInfo              any             `json:"xpost_fb_creator_info,omitempty"`
	IsOriginalAudioDownloadEligible bool            `json:"is_original_audio_download_eligible,omitempty"`
	TrendRank                       any             `json:"trend_rank,omitempty"`
	AudioFilterInfos                []any           `json:"audio_filter_infos,omitempty"`
	OaOwnerIsMusicArtist            bool            `json:"oa_owner_is_music_artist,omitempty"`
}
type MashupInfo struct {
	MashupsAllowed                      bool `json:"mashups_allowed,omitempty"`
	CanToggleMashupsAllowed             bool `json:"can_toggle_mashups_allowed,omitempty"`
	HasBeenMashedUp                     bool `json:"has_been_mashed_up,omitempty"`
	IsLightWeightCheck                  bool `json:"is_light_weight_check,omitempty"`
	FormattedMashupsCount               any  `json:"formatted_mashups_count,omitempty"`
	OriginalMedia                       any  `json:"original_media,omitempty"`
	PrivacyFilteredMashupsMediaCount    any  `json:"privacy_filtered_mashups_media_count,omitempty"`
	NonPrivacyFilteredMashupsMediaCount any  `json:"non_privacy_filtered_mashups_media_count,omitempty"`
	MashupType                          any  `json:"mashup_type,omitempty"`
	IsCreatorRequestingMashup           bool `json:"is_creator_requesting_mashup,omitempty"`
	HasNonmimicableAdditionalAudio      bool `json:"has_nonmimicable_additional_audio,omitempty"`
	IsPivotPageAvailable                bool `json:"is_pivot_page_available,omitempty"`
}
type BrandedContentTagInfo struct {
	CanAddTag bool `json:"can_add_tag,omitempty"`
}
type AudioReattributionInfo struct {
	ShouldAllowRestore bool `json:"should_allow_restore,omitempty"`
}
type AdditionalAudioInfo struct {
	AdditionalAudioUsername any                    `json:"additional_audio_username,omitempty"`
	AudioReattributionInfo  AudioReattributionInfo `json:"audio_reattribution_info,omitempty"`
}
type AudioRankingInfo struct {
	BestAudioClusterID string `json:"best_audio_cluster_id,omitempty"`
}
type ContentAppreciationInfo struct {
	Enabled             bool `json:"enabled,omitempty"`
	EntryPointContainer any  `json:"entry_point_container,omitempty"`
}
type AchievementsInfo struct {
	ShowAchievements      bool `json:"show_achievements,omitempty"`
	NumEarnedAchievements any  `json:"num_earned_achievements,omitempty"`
}
type ClipsMetadata struct {
	MusicInfo                    any                     `json:"music_info,omitempty"`
	OriginalSoundInfo            OriginalSoundInfo       `json:"original_sound_info,omitempty"`
	AudioType                    string                  `json:"audio_type,omitempty"`
	MusicCanonicalID             string                  `json:"music_canonical_id,omitempty"`
	FeaturedLabel                any                     `json:"featured_label,omitempty"`
	MashupInfo                   MashupInfo              `json:"mashup_info,omitempty"`
	ReusableTextInfo             any                     `json:"reusable_text_info,omitempty"`
	ReusableTextAttributeString  any                     `json:"reusable_text_attribute_string,omitempty"`
	NuxInfo                      any                     `json:"nux_info,omitempty"`
	ViewerInteractionSettings    any                     `json:"viewer_interaction_settings,omitempty"`
	BrandedContentTagInfo        BrandedContentTagInfo   `json:"branded_content_tag_info,omitempty"`
	ShoppingInfo                 any                     `json:"shopping_info,omitempty"`
	AdditionalAudioInfo          AdditionalAudioInfo     `json:"additional_audio_info,omitempty"`
	IsSharedToFb                 bool                    `json:"is_shared_to_fb,omitempty"`
	BreakingContentInfo          any                     `json:"breaking_content_info,omitempty"`
	ChallengeInfo                any                     `json:"challenge_info,omitempty"`
	ReelsOnTheRiseInfo           any                     `json:"reels_on_the_rise_info,omitempty"`
	BreakingCreatorInfo          any                     `json:"breaking_creator_info,omitempty"`
	AssetRecommendationInfo      any                     `json:"asset_recommendation_info,omitempty"`
	ContextualHighlightInfo      any                     `json:"contextual_highlight_info,omitempty"`
	ClipsCreationEntryPoint      string                  `json:"clips_creation_entry_point,omitempty"`
	AudioRankingInfo             AudioRankingInfo        `json:"audio_ranking_info,omitempty"`
	TemplateInfo                 any                     `json:"template_info,omitempty"`
	IsFanClubPromoVideo          bool                    `json:"is_fan_club_promo_video,omitempty"`
	DisableUseInClipsClientCache bool                    `json:"disable_use_in_clips_client_cache,omitempty"`
	ContentAppreciationInfo      ContentAppreciationInfo `json:"content_appreciation_info,omitempty"`
	AchievementsInfo             AchievementsInfo        `json:"achievements_info,omitempty"`
	ShowAchievements             bool                    `json:"show_achievements,omitempty"`
	ShowTips                     any                     `json:"show_tips,omitempty"`
	MerchandisingPillInfo        any                     `json:"merchandising_pill_info,omitempty"`
	IsPublicChatWelcomeVideo     bool                    `json:"is_public_chat_welcome_video,omitempty"`
	ProfessionalClipsUpsellType  int                     `json:"professional_clips_upsell_type,omitempty"`
	ExternalMediaInfo            any                     `json:"external_media_info,omitempty"`
}
type VideoVersions struct {
	Type   int    `json:"type,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	URL    string `json:"url,omitempty"`
	ID     string `json:"id,omitempty"`
}
type Items struct {
	TakenAt                             int                       `json:"taken_at,omitempty"`
	Pk                                  string                    `json:"pk,omitempty"`
	ID                                  string                    `json:"id,omitempty"`
	DeviceTimestamp                     int64                     `json:"device_timestamp,omitempty"`
	ClientCacheKey                      string                    `json:"client_cache_key,omitempty"`
	FilterType                          int                       `json:"filter_type,omitempty"`
	CaptionIsEdited                     bool                      `json:"caption_is_edited,omitempty"`
	LikeAndViewCountsDisabled           bool                      `json:"like_and_view_counts_disabled,omitempty"`
	StrongID                            string                    `json:"strong_id__,omitempty"`
	IsReshareOfTextPostAppMediaInIg     bool                      `json:"is_reshare_of_text_post_app_media_in_ig,omitempty"`
	IsPostLiveClipsMedia                bool                      `json:"is_post_live_clips_media,omitempty"`
	DeletedReason                       int                       `json:"deleted_reason,omitempty"`
	IntegrityReviewDecision             string                    `json:"integrity_review_decision,omitempty"`
	HasSharedToFb                       int                       `json:"has_shared_to_fb,omitempty"`
	IsUnifiedVideo                      bool                      `json:"is_unified_video,omitempty"`
	ShouldRequestAds                    bool                      `json:"should_request_ads,omitempty"`
	IsVisualReplyCommenterNoticeEnabled bool                      `json:"is_visual_reply_commenter_notice_enabled,omitempty"`
	CommercialityStatus                 string                    `json:"commerciality_status,omitempty"`
	ExploreHideComments                 bool                      `json:"explore_hide_comments,omitempty"`
	Usertags                            Usertags                  `json:"usertags,omitempty"`
	PhotoOfYou                          bool                      `json:"photo_of_you,omitempty"`
	ShopRoutingUserID                   any                       `json:"shop_routing_user_id,omitempty"`
	CanSeeInsightsAsBrand               bool                      `json:"can_see_insights_as_brand,omitempty"`
	IsOrganicProductTaggingEligible     bool                      `json:"is_organic_product_tagging_eligible,omitempty"`
	FbLikeCount                         int                       `json:"fb_like_count,omitempty"`
	HasLiked                            bool                      `json:"has_liked,omitempty"`
	LikeCount                           int                       `json:"like_count,omitempty"`
	FacepileTopLikers                   []any                     `json:"facepile_top_likers,omitempty"`
	TopLikers                           []any                     `json:"top_likers,omitempty"`
	MediaType                           int                       `json:"media_type,omitempty"`
	Code                                string                    `json:"code,omitempty"`
	CanViewerReshare                    bool                      `json:"can_viewer_reshare,omitempty"`
	Caption                             Caption                   `json:"caption,omitempty"`
	ClipsTabPinnedUserIds               []any                     `json:"clips_tab_pinned_user_ids,omitempty"`
	CommentInformTreatment              CommentInformTreatment    `json:"comment_inform_treatment,omitempty"`
	SharingFrictionInfo                 SharingFrictionInfo       `json:"sharing_friction_info,omitempty"`
	PlayCount                           int                       `json:"play_count,omitempty"`
	FbPlayCount                         int                       `json:"fb_play_count,omitempty"`
	MediaAppreciationSettings           MediaAppreciationSettings `json:"media_appreciation_settings,omitempty"`
	OriginalMediaHasVisualReplyMedia    bool                      `json:"original_media_has_visual_reply_media,omitempty"`
	CanViewerSave                       bool                      `json:"can_viewer_save,omitempty"`
	IsInProfileGrid                     bool                      `json:"is_in_profile_grid,omitempty"`
	ProfileGridControlEnabled           bool                      `json:"profile_grid_control_enabled,omitempty"`
	FeaturedProducts                    []any                     `json:"featured_products,omitempty"`
	IsCommentsGifComposerEnabled        bool                      `json:"is_comments_gif_composer_enabled,omitempty"`
	MediaCroppingInfo                   MediaCroppingInfo         `json:"media_cropping_info,omitempty"`
	ProductSuggestions                  []any                     `json:"product_suggestions,omitempty"`
	User                                User                      `json:"user,omitempty"`
	ImageVersions2                      ImageVersions2            `json:"image_versions2,omitempty"`
	OriginalWidth                       int                       `json:"original_width,omitempty"`
	OriginalHeight                      int                       `json:"original_height,omitempty"`
	IsArtistPick                        bool                      `json:"is_artist_pick,omitempty"`
	ProductType                         string                    `json:"product_type,omitempty"`
	IsPaidPartnership                   bool                      `json:"is_paid_partnership,omitempty"`
	MusicMetadata                       any                       `json:"music_metadata,omitempty"`
	OrganicTrackingToken                string                    `json:"organic_tracking_token,omitempty"`
	IsThirdPartyDownloadsEligible       bool                      `json:"is_third_party_downloads_eligible,omitempty"`
	CommerceIntegrityReviewDecision     string                    `json:"commerce_integrity_review_decision,omitempty"`
	IgMediaSharingDisabled              bool                      `json:"ig_media_sharing_disabled,omitempty"`
	IsOpenToPublicSubmission            bool                      `json:"is_open_to_public_submission,omitempty"`
	CarouselMediaIDs                    []string                  `json:"carousel_media_ids,omitempty"`
	CarouselMedia                       []*Items                  `json:"carousel_media,omitempty"`
	CommentingDisabledForViewer         bool                      `json:"commenting_disabled_for_viewer,omitempty"`
	CommentThreadingEnabled             bool                      `json:"comment_threading_enabled,omitempty"`
	MaxNumVisiblePreviewComments        int                       `json:"max_num_visible_preview_comments,omitempty"`
	HasMoreComments                     bool                      `json:"has_more_comments,omitempty"`
	PreviewComments                     []any                     `json:"preview_comments,omitempty"`
	Comments                            []any                     `json:"comments,omitempty"`
	CommentCount                        int                       `json:"comment_count,omitempty"`
	CanViewMorePreviewComments          bool                      `json:"can_view_more_preview_comments,omitempty"`
	HideViewAllCommentEntrypoint        bool                      `json:"hide_view_all_comment_entrypoint,omitempty"`
	InlineComposerDisplayCondition      string                    `json:"inline_composer_display_condition,omitempty"`
	SubscribeCtaVisible                 bool                      `json:"subscribe_cta_visible,omitempty"`
	HasDelayedMetadata                  bool                      `json:"has_delayed_metadata,omitempty"`
	IsAutoCreated                       bool                      `json:"is_auto_created,omitempty"`
	IsQuietPost                         bool                      `json:"is_quiet_post,omitempty"`
	IsCutoutStickerAllowed              bool                      `json:"is_cutout_sticker_allowed,omitempty"`
	ClipsMetadata                       ClipsMetadata             `json:"clips_metadata,omitempty"`
	IsDashEligible                      int                       `json:"is_dash_eligible,omitempty"`
	VideoDashManifest                   string                    `json:"video_dash_manifest,omitempty"`
	VideoCodec                          string                    `json:"video_codec,omitempty"`
	NumberOfQualities                   int                       `json:"number_of_qualities,omitempty"`
	VideoVersions                       []VideoVersions           `json:"video_versions,omitempty"`
	HasAudio                            bool                      `json:"has_audio,omitempty"`
	VideoDuration                       float64                   `json:"video_duration,omitempty"`
}
