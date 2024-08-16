package responses

type ProfileInfoResponse struct {
	Data struct {
		User struct {
			AiAgentType           any    `json:"ai_agent_type"`
			BioLinks              []any  `json:"bio_links"`
			Biography             string `json:"biography"`
			BiographyWithEntities struct {
				Entities []any  `json:"entities"`
				RawText  string `json:"raw_text"`
			} `json:"biography_with_entities"`
			BlockedByViewer        bool   `json:"blocked_by_viewer"`
			BusinessAddressJSON    any    `json:"business_address_json"`
			BusinessCategoryName   any    `json:"business_category_name"`
			BusinessContactMethod  string `json:"business_contact_method"`
			BusinessEmail          any    `json:"business_email"`
			BusinessPhoneNumber    any    `json:"business_phone_number"`
			CategoryEnum           any    `json:"category_enum"`
			CategoryName           any    `json:"category_name"`
			ConnectedFbPage        any    `json:"connected_fb_page"`
			CountryBlock           bool   `json:"country_block"`
			EdgeFelixVideoTimeline struct {
				Count    int   `json:"count"`
				Edges    []any `json:"edges"`
				PageInfo struct {
					EndCursor   any  `json:"end_cursor"`
					HasNextPage bool `json:"has_next_page"`
				} `json:"page_info"`
			} `json:"edge_felix_video_timeline"`
			EdgeFollow struct {
				Count int `json:"count"`
			} `json:"edge_follow"`
			EdgeFollowedBy struct {
				Count int `json:"count"`
			} `json:"edge_followed_by"`
			EdgeMediaCollections struct {
				Count    int   `json:"count"`
				Edges    []any `json:"edges"`
				PageInfo struct {
					EndCursor   any  `json:"end_cursor"`
					HasNextPage bool `json:"has_next_page"`
				} `json:"page_info"`
			} `json:"edge_media_collections"`
			EdgeMutualFollowedBy struct {
				Count int   `json:"count"`
				Edges []any `json:"edges"`
			} `json:"edge_mutual_followed_by"`
			EdgeOwnerToTimelineMedia EdgeOwnerToTimelineMedia `json:"edge_owner_to_timeline_media"`
			EdgeSavedMedia           struct {
				Count    int   `json:"count"`
				Edges    []any `json:"edges"`
				PageInfo struct {
					EndCursor   any  `json:"end_cursor"`
					HasNextPage bool `json:"has_next_page"`
				} `json:"page_info"`
			} `json:"edge_saved_media"`
			EimuID                         string   `json:"eimu_id"`
			ExternalURL                    any      `json:"external_url"`
			ExternalURLLinkshimmed         any      `json:"external_url_linkshimmed"`
			FbProfileBiolink               any      `json:"fb_profile_biolink"`
			Fbid                           string   `json:"fbid"`
			FollowedByViewer               bool     `json:"followed_by_viewer"`
			FollowsViewer                  bool     `json:"follows_viewer"`
			FullName                       string   `json:"full_name"`
			GroupMetadata                  any      `json:"group_metadata"`
			GuardianID                     any      `json:"guardian_id"`
			HasArEffects                   bool     `json:"has_ar_effects"`
			HasBlockedViewer               bool     `json:"has_blocked_viewer"`
			HasChannel                     bool     `json:"has_channel"`
			HasClips                       bool     `json:"has_clips"`
			HasGuides                      bool     `json:"has_guides"`
			HasRequestedViewer             bool     `json:"has_requested_viewer"`
			HideLikeAndViewCounts          bool     `json:"hide_like_and_view_counts"`
			HighlightReelCount             int      `json:"highlight_reel_count"`
			ID                             string   `json:"id"`
			IsBusinessAccount              bool     `json:"is_business_account"`
			IsEmbedsDisabled               bool     `json:"is_embeds_disabled"`
			IsGuardianOfViewer             bool     `json:"is_guardian_of_viewer"`
			IsJoinedRecently               bool     `json:"is_joined_recently"`
			IsPrivate                      bool     `json:"is_private"`
			IsProfessionalAccount          bool     `json:"is_professional_account"`
			IsRegulatedC18                 bool     `json:"is_regulated_c18"`
			IsSupervisedByViewer           bool     `json:"is_supervised_by_viewer"`
			IsSupervisedUser               bool     `json:"is_supervised_user"`
			IsSupervisionEnabled           bool     `json:"is_supervision_enabled"`
			IsVerified                     bool     `json:"is_verified"`
			IsVerifiedByMv4B               bool     `json:"is_verified_by_mv4b"`
			OverallCategoryName            any      `json:"overall_category_name"`
			PinnedChannelsListCount        int      `json:"pinned_channels_list_count"`
			ProfilePicURL                  string   `json:"profile_pic_url"`
			ProfilePicURLHd                string   `json:"profile_pic_url_hd"`
			Pronouns                       []string `json:"pronouns"`
			RequestedByViewer              bool     `json:"requested_by_viewer"`
			RestrictedByViewer             bool     `json:"restricted_by_viewer"`
			ShouldShowCategory             bool     `json:"should_show_category"`
			ShouldShowPublicContacts       bool     `json:"should_show_public_contacts"`
			ShowAccountTransparencyDetails bool     `json:"show_account_transparency_details"`
			TransparencyLabel              any      `json:"transparency_label"`
			TransparencyProduct            string   `json:"transparency_product"`
			Username                       string   `json:"username"`
		} `json:"user"`
	} `json:"data"`
	Status string `json:"status"`
}

type EdgeOwnerToTimelineMedia struct {
	Count int `json:"count"`
	Edges []struct {
		Node struct {
			Typename             string `json:"__typename"`
			AccessibilityCaption string `json:"accessibility_caption"`
			CoauthorProducers    []any  `json:"coauthor_producers"`
			CommentsDisabled     bool   `json:"comments_disabled"`
			Dimensions           struct {
				Height int `json:"height"`
				Width  int `json:"width"`
			} `json:"dimensions"`
			DisplayURL  string `json:"display_url"`
			EdgeLikedBy struct {
				Count int `json:"count"`
			} `json:"edge_liked_by"`
			EdgeMediaPreviewLike struct {
				Count int `json:"count"`
			} `json:"edge_media_preview_like"`
			EdgeMediaToCaption struct {
				Edges []struct {
					Node struct {
						Text string `json:"text"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"edge_media_to_caption"`
			EdgeMediaToComment struct {
				Count int `json:"count"`
			} `json:"edge_media_to_comment"`
			EdgeMediaToTaggedUser struct {
				Edges []any `json:"edges"`
			} `json:"edge_media_to_tagged_user"`
			EdgeSidecarToChildren struct {
				Edges []struct {
					Node struct {
						Typename             string `json:"__typename"`
						AccessibilityCaption string `json:"accessibility_caption"`
						Dimensions           struct {
							Height int `json:"height"`
							Width  int `json:"width"`
						} `json:"dimensions"`
						DisplayURL            string `json:"display_url"`
						EdgeMediaToTaggedUser struct {
							Edges []any `json:"edges"`
						} `json:"edge_media_to_tagged_user"`
						FactCheckInformation   any    `json:"fact_check_information"`
						FactCheckOverallRating any    `json:"fact_check_overall_rating"`
						GatingInfo             any    `json:"gating_info"`
						HasUpcomingEvent       bool   `json:"has_upcoming_event"`
						ID                     string `json:"id"`
						IsVideo                bool   `json:"is_video"`
						MediaOverlayInfo       any    `json:"media_overlay_info"`
						MediaPreview           string `json:"media_preview"`
						Owner                  struct {
							ID       string `json:"id"`
							Username string `json:"username"`
						} `json:"owner"`
						SharingFrictionInfo struct {
							BloksAppURL               any  `json:"bloks_app_url"`
							ShouldHaveSharingFriction bool `json:"should_have_sharing_friction"`
						} `json:"sharing_friction_info"`
						Shortcode string `json:"shortcode"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"edge_sidecar_to_children"`
			FactCheckInformation   any    `json:"fact_check_information"`
			FactCheckOverallRating any    `json:"fact_check_overall_rating"`
			GatingInfo             any    `json:"gating_info"`
			HasUpcomingEvent       bool   `json:"has_upcoming_event"`
			ID                     string `json:"id"`
			IsVideo                bool   `json:"is_video"`
			Location               any    `json:"location"`
			MediaOverlayInfo       any    `json:"media_overlay_info"`
			MediaPreview           any    `json:"media_preview"`
			NftAssetInfo           any    `json:"nft_asset_info"`
			Owner                  struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"owner"`
			PinnedForUsers      []any `json:"pinned_for_users"`
			SharingFrictionInfo struct {
				BloksAppURL               any  `json:"bloks_app_url"`
				ShouldHaveSharingFriction bool `json:"should_have_sharing_friction"`
			} `json:"sharing_friction_info"`
			Shortcode          string `json:"shortcode"`
			TakenAtTimestamp   int    `json:"taken_at_timestamp"`
			ThumbnailResources []struct {
				ConfigHeight int    `json:"config_height"`
				ConfigWidth  int    `json:"config_width"`
				Src          string `json:"src"`
			} `json:"thumbnail_resources"`
			ThumbnailSrc     string `json:"thumbnail_src"`
			ViewerCanReshare bool   `json:"viewer_can_reshare"`
		} `json:"node,omitempty"`
	} `json:"edges"`
	PageInfo struct {
		EndCursor   string `json:"end_cursor"`
		HasNextPage bool   `json:"has_next_page"`
	} `json:"page_info"`
}
