package responses

type FetchMediaResponse struct {
	Items               []Items `json:"items"`
	NumResults          int     `json:"num_results"`
	MoreAvailable       bool    `json:"more_available"`
	AutoLoadMoreEnabled bool    `json:"auto_load_more_enabled"`
	Status              string  `json:"status"`
}
type FriendshipStatus struct {
	Following       bool `json:"following"`
	FollowedBy      bool `json:"followed_by"`
	Blocking        bool `json:"blocking"`
	Muting          bool `json:"muting"`
	IsPrivate       bool `json:"is_private"`
	IncomingRequest bool `json:"incoming_request"`
	OutgoingRequest bool `json:"outgoing_request"`
	IsBestie        bool `json:"is_bestie"`
	IsRestricted    bool `json:"is_restricted"`
	IsFeedFavorite  bool `json:"is_feed_favorite"`
}
type In struct {
	User                  User  `json:"user"`
	Position              []float64 `json:"position"`
	StartTimeInVideoInSec any   `json:"start_time_in_video_in_sec"`
	DurationInVideoInSec  any   `json:"duration_in_video_in_sec"`
}
type Usertags struct {
	In []In `json:"in"`
}
type FanConsiderationPageRevampEligiblity struct {
	ShouldShowSocialContext  bool `json:"should_show_social_context"`
	ShouldShowContentPreview bool `json:"should_show_content_preview"`
}
type FanClubInfo struct {
	FanClubID                            string                               `json:"fan_club_id"`
	FanClubName                          string                               `json:"fan_club_name"`
	IsFanClubReferralEligible            bool                                 `json:"is_fan_club_referral_eligible"`
	FanConsiderationPageRevampEligiblity FanConsiderationPageRevampEligiblity `json:"fan_consideration_page_revamp_eligiblity"`
	IsFanClubGiftingEligible             bool                                 `json:"is_fan_club_gifting_eligible"`
	SubscriberCount                      int                                  `json:"subscriber_count"`
	ConnectedMemberCount                 int                                  `json:"connected_member_count"`
	AutosaveToExclusiveHighlight         bool                                 `json:"autosave_to_exclusive_highlight"`
	HasEnoughSubscribersForSsc           bool                                 `json:"has_enough_subscribers_for_ssc"`
}

type HdProfilePicURLInfo struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type HdProfilePicVersions struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
}
type User struct {
	FbidV2                         string                 `json:"fbid_v2"`
	FeedPostReshareDisabled        bool                   `json:"feed_post_reshare_disabled"`
	FullName                       string                 `json:"full_name"`
	ID                             string                 `json:"id"`
	IsPrivate                      bool                   `json:"is_private"`
	IsUnpublished                  bool                   `json:"is_unpublished"`
	Pk                             string                 `json:"pk"`
	PkID                           string                 `json:"pk_id"`
	ShowAccountTransparencyDetails bool                   `json:"show_account_transparency_details"`
	StrongID                       string                 `json:"strong_id__"`
	ThirdPartyDownloadsEnabled     int                    `json:"third_party_downloads_enabled"`
	AccountBadges                  []any                  `json:"account_badges"`
	FanClubInfo                    FanClubInfo            `json:"fan_club_info"`
	FriendshipStatus               FriendshipStatus       `json:"friendship_status"`
	HasAnonymousProfilePicture     bool                   `json:"has_anonymous_profile_picture"`
	HdProfilePicURLInfo            HdProfilePicURLInfo    `json:"hd_profile_pic_url_info"`
	HdProfilePicVersions           []HdProfilePicVersions `json:"hd_profile_pic_versions"`
	IsFavorite                     bool                   `json:"is_favorite"`
	IsVerified                     bool                   `json:"is_verified"`
	LatestReelMedia                int                    `json:"latest_reel_media"`
	ProfilePicID                   string                 `json:"profile_pic_id"`
	ProfilePicURL                  string                 `json:"profile_pic_url"`
	TransparencyProductEnabled     bool                   `json:"transparency_product_enabled"`
	Username                       string                 `json:"username"`
}
type Caption struct {
	Pk                 string `json:"pk"`
	UserID             string `json:"user_id"`
	User               User   `json:"user"`
	Type               int    `json:"type"`
	Text               string `json:"text"`
	DidReportAsSpam    bool   `json:"did_report_as_spam"`
	CreatedAt          int    `json:"created_at"`
	CreatedAtUtc       int    `json:"created_at_utc"`
	ContentType        string `json:"content_type"`
	Status             string `json:"status"`
	BitFlags           int    `json:"bit_flags"`
	ShareEnabled       bool   `json:"share_enabled"`
	IsRankedComment    bool   `json:"is_ranked_comment"`
	IsCovered          bool   `json:"is_covered"`
	PrivateReplyStatus int    `json:"private_reply_status"`
	MediaID            string `json:"media_id"`
}
type CommentInformTreatment struct {
	ShouldHaveInformTreatment bool   `json:"should_have_inform_treatment"`
	Text                      string `json:"text"`
	URL                       any    `json:"url"`
	ActionType                any    `json:"action_type"`
}
type SharingFrictionInfo struct {
	ShouldHaveSharingFriction bool `json:"should_have_sharing_friction"`
	BloksAppURL               any  `json:"bloks_app_url"`
	SharingFrictionPayload    any  `json:"sharing_friction_payload"`
}
type MediaAppreciationSettings struct {
	MediaGiftingState   string `json:"media_gifting_state"`
	GiftCountVisibility string `json:"gift_count_visibility"`
}
type SquareCrop struct {
	CropLeft   float64 `json:"crop_left"`
	CropRight  float64 `json:"crop_right"`
	CropTop    float64 `json:"crop_top"`
	CropBottom float64 `json:"crop_bottom"`
}
type MediaCroppingInfo struct {
	SquareCrop SquareCrop `json:"square_crop"`
}
type Candidates struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
}
type IgtvFirstFrame struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
}
type FirstFrame struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
}
type AdditionalCandidates struct {
	IgtvFirstFrame IgtvFirstFrame `json:"igtv_first_frame"`
	FirstFrame     FirstFrame     `json:"first_frame"`
	SmartFrame     any            `json:"smart_frame"`
}
type Default struct {
	VideoLength                float64  `json:"video_length"`
	ThumbnailWidth             int      `json:"thumbnail_width"`
	ThumbnailHeight            int      `json:"thumbnail_height"`
	ThumbnailDuration          float64  `json:"thumbnail_duration"`
	SpriteUrls                 []string `json:"sprite_urls"`
	ThumbnailsPerRow           int      `json:"thumbnails_per_row"`
	TotalThumbnailNumPerSprite int      `json:"total_thumbnail_num_per_sprite"`
	MaxThumbnailsPerSprite     int      `json:"max_thumbnails_per_sprite"`
	SpriteWidth                int      `json:"sprite_width"`
	SpriteHeight               int      `json:"sprite_height"`
	RenderedWidth              int      `json:"rendered_width"`
	FileSizeKb                 int      `json:"file_size_kb"`
}
type ScrubberSpritesheetInfoCandidates struct {
	Default Default `json:"default"`
}
type ImageVersions2 struct {
	Candidates                        []Candidates                      `json:"candidates"`
	AdditionalCandidates              AdditionalCandidates              `json:"additional_candidates"`
	SmartThumbnailEnabled             bool                              `json:"smart_thumbnail_enabled"`
	ScrubberSpritesheetInfoCandidates ScrubberSpritesheetInfoCandidates `json:"scrubber_spritesheet_info_candidates"`
}
type IgArtist struct {
	Pk            string `json:"pk"`
	PkID          string `json:"pk_id"`
	FullName      string `json:"full_name"`
	IsPrivate     bool   `json:"is_private"`
	StrongID      string `json:"strong_id__"`
	Username      string `json:"username"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicID  string `json:"profile_pic_id"`
	ProfilePicURL string `json:"profile_pic_url"`
}
type ConsumptionInfo struct {
	IsBookmarked              bool   `json:"is_bookmarked"`
	ShouldMuteAudioReason     string `json:"should_mute_audio_reason"`
	IsTrendingInClips         bool   `json:"is_trending_in_clips"`
	ShouldMuteAudioReasonType any    `json:"should_mute_audio_reason_type"`
	DisplayMediaID            any    `json:"display_media_id"`
}
type OriginalSoundInfo struct {
	AudioAssetID                    string          `json:"audio_asset_id"`
	MusicCanonicalID                any             `json:"music_canonical_id"`
	ProgressiveDownloadURL          string          `json:"progressive_download_url"`
	DurationInMs                    int             `json:"duration_in_ms"`
	DashManifest                    string          `json:"dash_manifest"`
	IgArtist                        IgArtist        `json:"ig_artist"`
	ShouldMuteAudio                 bool            `json:"should_mute_audio"`
	HideRemixing                    bool            `json:"hide_remixing"`
	OriginalMediaID                 string          `json:"original_media_id"`
	TimeCreated                     int             `json:"time_created"`
	OriginalAudioTitle              string          `json:"original_audio_title"`
	ConsumptionInfo                 ConsumptionInfo `json:"consumption_info"`
	CanRemixBeSharedToFb            bool            `json:"can_remix_be_shared_to_fb"`
	FormattedClipsMediaCount        any             `json:"formatted_clips_media_count"`
	AllowCreatorToRename            bool            `json:"allow_creator_to_rename"`
	AudioParts                      []any           `json:"audio_parts"`
	IsExplicit                      bool            `json:"is_explicit"`
	OriginalAudioSubtype            string          `json:"original_audio_subtype"`
	IsAudioAutomaticallyAttributed  bool            `json:"is_audio_automatically_attributed"`
	IsReuseDisabled                 bool            `json:"is_reuse_disabled"`
	IsXpostFromFb                   bool            `json:"is_xpost_from_fb"`
	XpostFbCreatorInfo              any             `json:"xpost_fb_creator_info"`
	IsOriginalAudioDownloadEligible bool            `json:"is_original_audio_download_eligible"`
	TrendRank                       any             `json:"trend_rank"`
	AudioFilterInfos                []any           `json:"audio_filter_infos"`
	OaOwnerIsMusicArtist            bool            `json:"oa_owner_is_music_artist"`
}
type MashupInfo struct {
	MashupsAllowed                      bool `json:"mashups_allowed"`
	CanToggleMashupsAllowed             bool `json:"can_toggle_mashups_allowed"`
	HasBeenMashedUp                     bool `json:"has_been_mashed_up"`
	IsLightWeightCheck                  bool `json:"is_light_weight_check"`
	FormattedMashupsCount               any  `json:"formatted_mashups_count"`
	OriginalMedia                       any  `json:"original_media"`
	PrivacyFilteredMashupsMediaCount    any  `json:"privacy_filtered_mashups_media_count"`
	NonPrivacyFilteredMashupsMediaCount any  `json:"non_privacy_filtered_mashups_media_count"`
	MashupType                          any  `json:"mashup_type"`
	IsCreatorRequestingMashup           bool `json:"is_creator_requesting_mashup"`
	HasNonmimicableAdditionalAudio      bool `json:"has_nonmimicable_additional_audio"`
	IsPivotPageAvailable                bool `json:"is_pivot_page_available"`
}
type BrandedContentTagInfo struct {
	CanAddTag bool `json:"can_add_tag"`
}
type AudioReattributionInfo struct {
	ShouldAllowRestore bool `json:"should_allow_restore"`
}
type AdditionalAudioInfo struct {
	AdditionalAudioUsername any                    `json:"additional_audio_username"`
	AudioReattributionInfo  AudioReattributionInfo `json:"audio_reattribution_info"`
}
type AudioRankingInfo struct {
	BestAudioClusterID string `json:"best_audio_cluster_id"`
}
type ContentAppreciationInfo struct {
	Enabled             bool `json:"enabled"`
	EntryPointContainer any  `json:"entry_point_container"`
}
type AchievementsInfo struct {
	ShowAchievements      bool `json:"show_achievements"`
	NumEarnedAchievements any  `json:"num_earned_achievements"`
}
type ClipsMetadata struct {
	MusicInfo                    any                     `json:"music_info"`
	OriginalSoundInfo            OriginalSoundInfo       `json:"original_sound_info"`
	AudioType                    string                  `json:"audio_type"`
	MusicCanonicalID             string                  `json:"music_canonical_id"`
	FeaturedLabel                any                     `json:"featured_label"`
	MashupInfo                   MashupInfo              `json:"mashup_info"`
	ReusableTextInfo             any                     `json:"reusable_text_info"`
	ReusableTextAttributeString  any                     `json:"reusable_text_attribute_string"`
	NuxInfo                      any                     `json:"nux_info"`
	ViewerInteractionSettings    any                     `json:"viewer_interaction_settings"`
	BrandedContentTagInfo        BrandedContentTagInfo   `json:"branded_content_tag_info"`
	ShoppingInfo                 any                     `json:"shopping_info"`
	AdditionalAudioInfo          AdditionalAudioInfo     `json:"additional_audio_info"`
	IsSharedToFb                 bool                    `json:"is_shared_to_fb"`
	BreakingContentInfo          any                     `json:"breaking_content_info"`
	ChallengeInfo                any                     `json:"challenge_info"`
	ReelsOnTheRiseInfo           any                     `json:"reels_on_the_rise_info"`
	BreakingCreatorInfo          any                     `json:"breaking_creator_info"`
	AssetRecommendationInfo      any                     `json:"asset_recommendation_info"`
	ContextualHighlightInfo      any                     `json:"contextual_highlight_info"`
	ClipsCreationEntryPoint      string                  `json:"clips_creation_entry_point"`
	AudioRankingInfo             AudioRankingInfo        `json:"audio_ranking_info"`
	TemplateInfo                 any                     `json:"template_info"`
	IsFanClubPromoVideo          bool                    `json:"is_fan_club_promo_video"`
	DisableUseInClipsClientCache bool                    `json:"disable_use_in_clips_client_cache"`
	ContentAppreciationInfo      ContentAppreciationInfo `json:"content_appreciation_info"`
	AchievementsInfo             AchievementsInfo        `json:"achievements_info"`
	ShowAchievements             bool                    `json:"show_achievements"`
	ShowTips                     any                     `json:"show_tips"`
	MerchandisingPillInfo        any                     `json:"merchandising_pill_info"`
	IsPublicChatWelcomeVideo     bool                    `json:"is_public_chat_welcome_video"`
	ProfessionalClipsUpsellType  int                     `json:"professional_clips_upsell_type"`
	ExternalMediaInfo            any                     `json:"external_media_info"`
}
type VideoVersions struct {
	Type   int    `json:"type"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
	ID     string `json:"id"`
}
type Items struct {
	TakenAt                             int                       `json:"taken_at"`
	Pk                                  string                    `json:"pk"`
	ID                                  string                    `json:"id"`
	DeviceTimestamp                     int64                     `json:"device_timestamp"`
	ClientCacheKey                      string                    `json:"client_cache_key"`
	FilterType                          int                       `json:"filter_type"`
	CaptionIsEdited                     bool                      `json:"caption_is_edited"`
	LikeAndViewCountsDisabled           bool                      `json:"like_and_view_counts_disabled"`
	StrongID                            string                    `json:"strong_id__"`
	IsReshareOfTextPostAppMediaInIg     bool                      `json:"is_reshare_of_text_post_app_media_in_ig"`
	IsPostLiveClipsMedia                bool                      `json:"is_post_live_clips_media"`
	DeletedReason                       int                       `json:"deleted_reason"`
	IntegrityReviewDecision             string                    `json:"integrity_review_decision"`
	HasSharedToFb                       int                       `json:"has_shared_to_fb"`
	IsUnifiedVideo                      bool                      `json:"is_unified_video"`
	ShouldRequestAds                    bool                      `json:"should_request_ads"`
	IsVisualReplyCommenterNoticeEnabled bool                      `json:"is_visual_reply_commenter_notice_enabled"`
	CommercialityStatus                 string                    `json:"commerciality_status"`
	ExploreHideComments                 bool                      `json:"explore_hide_comments"`
	Usertags                            Usertags                  `json:"usertags"`
	PhotoOfYou                          bool                      `json:"photo_of_you"`
	ShopRoutingUserID                   any                       `json:"shop_routing_user_id"`
	CanSeeInsightsAsBrand               bool                      `json:"can_see_insights_as_brand"`
	IsOrganicProductTaggingEligible     bool                      `json:"is_organic_product_tagging_eligible"`
	FbLikeCount                         int                       `json:"fb_like_count"`
	HasLiked                            bool                      `json:"has_liked"`
	LikeCount                           int                       `json:"like_count"`
	FacepileTopLikers                   []any                     `json:"facepile_top_likers"`
	TopLikers                           []any                     `json:"top_likers"`
	MediaType                           int                       `json:"media_type"`
	Code                                string                    `json:"code"`
	CanViewerReshare                    bool                      `json:"can_viewer_reshare"`
	Caption                             Caption                   `json:"caption"`
	ClipsTabPinnedUserIds               []any                     `json:"clips_tab_pinned_user_ids"`
	CommentInformTreatment              CommentInformTreatment    `json:"comment_inform_treatment"`
	SharingFrictionInfo                 SharingFrictionInfo       `json:"sharing_friction_info"`
	PlayCount                           int                       `json:"play_count"`
	FbPlayCount                         int                       `json:"fb_play_count"`
	MediaAppreciationSettings           MediaAppreciationSettings `json:"media_appreciation_settings"`
	OriginalMediaHasVisualReplyMedia    bool                      `json:"original_media_has_visual_reply_media"`
	CanViewerSave                       bool                      `json:"can_viewer_save"`
	IsInProfileGrid                     bool                      `json:"is_in_profile_grid"`
	ProfileGridControlEnabled           bool                      `json:"profile_grid_control_enabled"`
	FeaturedProducts                    []any                     `json:"featured_products"`
	IsCommentsGifComposerEnabled        bool                      `json:"is_comments_gif_composer_enabled"`
	MediaCroppingInfo                   MediaCroppingInfo         `json:"media_cropping_info"`
	ProductSuggestions                  []any                     `json:"product_suggestions"`
	User                                User                      `json:"user"`
	ImageVersions2                      ImageVersions2            `json:"image_versions2"`
	OriginalWidth                       int                       `json:"original_width"`
	OriginalHeight                      int                       `json:"original_height"`
	IsArtistPick                        bool                      `json:"is_artist_pick"`
	ProductType                         string                    `json:"product_type"`
	IsPaidPartnership                   bool                      `json:"is_paid_partnership"`
	MusicMetadata                       any                       `json:"music_metadata"`
	OrganicTrackingToken                string                    `json:"organic_tracking_token"`
	IsThirdPartyDownloadsEligible       bool                      `json:"is_third_party_downloads_eligible"`
	CommerceIntegrityReviewDecision     string                    `json:"commerce_integrity_review_decision"`
	IgMediaSharingDisabled              bool                      `json:"ig_media_sharing_disabled"`
	IsOpenToPublicSubmission            bool                      `json:"is_open_to_public_submission"`
	CommentingDisabledForViewer         bool                      `json:"commenting_disabled_for_viewer"`
	CommentThreadingEnabled             bool                      `json:"comment_threading_enabled"`
	MaxNumVisiblePreviewComments        int                       `json:"max_num_visible_preview_comments"`
	HasMoreComments                     bool                      `json:"has_more_comments"`
	PreviewComments                     []any                     `json:"preview_comments"`
	Comments                            []any                     `json:"comments"`
	CommentCount                        int                       `json:"comment_count"`
	CanViewMorePreviewComments          bool                      `json:"can_view_more_preview_comments"`
	HideViewAllCommentEntrypoint        bool                      `json:"hide_view_all_comment_entrypoint"`
	InlineComposerDisplayCondition      string                    `json:"inline_composer_display_condition"`
	SubscribeCtaVisible                 bool                      `json:"subscribe_cta_visible"`
	HasDelayedMetadata                  bool                      `json:"has_delayed_metadata"`
	IsAutoCreated                       bool                      `json:"is_auto_created"`
	IsQuietPost                         bool                      `json:"is_quiet_post"`
	IsCutoutStickerAllowed              bool                      `json:"is_cutout_sticker_allowed"`
	ClipsMetadata                       ClipsMetadata             `json:"clips_metadata"`
	IsDashEligible                      int                       `json:"is_dash_eligible"`
	VideoDashManifest                   string                    `json:"video_dash_manifest"`
	VideoCodec                          string                    `json:"video_codec"`
	NumberOfQualities                   int                       `json:"number_of_qualities"`
	VideoVersions                       []VideoVersions           `json:"video_versions"`
	HasAudio                            bool                      `json:"has_audio"`
	VideoDuration                       float64                   `json:"video_duration"`
}