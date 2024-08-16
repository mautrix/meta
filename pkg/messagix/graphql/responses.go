package graphql

import (
	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

/*
	Most likely missing a bunch of data, so if you find more fields in the payload add them
*/

type LSPlatformGraphQLLightspeedRequestQuery struct {
	types.ErrorResponse
	Data *struct {
		Viewer struct {
			LightspeedWebRequest *LightspeedWebRequest `json:"lightspeed_web_request,omitempty"`
		} `json:"viewer,omitempty"`
		LightspeedWebRequestForIG *LightspeedWebRequest `json:"lightspeed_web_request_for_igd,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type LightspeedWebRequest struct {
	Dependencies lightspeed.DependencyList `json:"dependencies,omitempty"`
	Experiments  any                       `json:"experiments,omitempty"`
	Payload      string                    `json:"payload,omitempty"`
	Target       string                    `json:"target,omitempty"`
}

type CometActorGatewayHandlerQuery struct {
	Data struct {
		XfbActorGatewayOpenExperience any `json:"xfb_actor_gateway_open_experience,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometAppNavigationProfileSwitcherConfigQuery struct {
	Data struct {
		Viewer struct {
			Actor struct {
				Typename       string `json:"__typename,omitempty"`
				ID             string `json:"id,omitempty"`
				ProfilePicture struct {
					URI string `json:"uri,omitempty"`
				} `json:"profile_picture,omitempty"`
				ProfileSwitcherEligibleProfiles struct {
					Count int `json:"count,omitempty"`
				} `json:"profile_switcher_eligible_profiles,omitempty"`
			} `json:"actor,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal        bool `json:"is_final,omitempty"`
		PrefetchUrisV2 []struct {
			Label any    `json:"label,omitempty"`
			URI   string `json:"uri,omitempty"`
		} `json:"prefetch_uris_v2,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometAppNavigationTargetedTabBarContentInnerImplQuery struct {
	Data struct {
		Viewer struct {
			Actor struct {
				Typename string `json:"__typename,omitempty"`
				ID       string `json:"id,omitempty"`
				Name     string `json:"name,omitempty"`
			} `json:"actor,omitempty"`
			FeedsTabTooltip struct {
				Nodes []any `json:"nodes,omitempty"`
			} `json:"feeds_tab_tooltip,omitempty"`
			TabBookmarks struct {
				Edges []struct {
					Node struct {
						BookmarkedNode struct {
							Typename string `json:"__typename,omitempty"`
							ID       string `json:"id,omitempty"`
						} `json:"bookmarked_node,omitempty"`
						ID                string `json:"id,omitempty"`
						LastUsedTimestamp int    `json:"last_used_timestamp,omitempty"`
						UnreadCount       int    `json:"unread_count,omitempty"`
					} `json:"node,omitempty"`
				} `json:"edges,omitempty"`
			} `json:"tab_bookmarks,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometLogoutHandlerQuery struct {
	Data struct {
		LogoutWhitelist []string `json:"logout_whitelist,omitempty"`
		Viewer          struct {
			AccountUser struct {
				HasConfirmedContactpoints bool   `json:"has_confirmed_contactpoints,omitempty"`
				ID                        string `json:"id,omitempty"`
				PendingContactpoints      []any  `json:"pending_contactpoints,omitempty"`
			} `json:"account_user,omitempty"`
			LogoutHash string `json:"logout_hash,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometNotificationsBadgeCountQuery struct {
	Data struct {
		Viewer struct {
			NotificationsUnseenCount int `json:"notifications_unseen_count,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometQuickPromotionInterstitialQuery struct {
	Data struct {
		Viewer struct {
			EligiblePromotions struct {
				Nodes []any `json:"nodes,omitempty"`
			} `json:"eligible_promotions,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometSearchBootstrapKeywordsDataSourceQuery struct {
	Data struct {
		Viewer struct {
			BootstrapKeywords struct {
				Edges []struct {
					Node struct {
						ItemLoggingID   string `json:"item_logging_id,omitempty"`
						ItemLoggingInfo string `json:"item_logging_info,omitempty"`
						KeywordText     string `json:"keyword_text,omitempty"`
						StsInfo         struct {
							DirectNavResult struct {
								EntID      string `json:"ent_id,omitempty"`
								EntityType string `json:"entity_type,omitempty"`
								ImgURL     string `json:"img_url,omitempty"`
								LinkURL    string `json:"link_url,omitempty"`
								Snippet    string `json:"snippet,omitempty"`
								Title      string `json:"title,omitempty"`
								Type       string `json:"type,omitempty"`
							} `json:"direct_nav_result,omitempty"`
							DisambiguationResult any `json:"disambiguation_result,omitempty"`
							HighConfidenceResult any `json:"high_confidence_result,omitempty"`
						} `json:"sts_info,omitempty"`
						SuggestionKeys struct {
							DefaultKey string   `json:"default_key,omitempty"`
							Keys       []string `json:"keys,omitempty"`
							Tier       int      `json:"tier,omitempty"`
						} `json:"suggestion_keys,omitempty"`
					} `json:"node,omitempty"`
				} `json:"edges,omitempty"`
			} `json:"bootstrap_keywords,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometSearchRecentDataSourceQuery struct {
	Data struct {
		SearchNullState struct {
			Results struct {
				Edges []struct {
					Node struct {
						Typename          string `json:"__typename,omitempty"`
						RenderingStrategy struct {
							Typename string `json:"__typename,omitempty"`
							Key      string `json:"key,omitempty"`
							Result   struct {
								HeaderText string `json:"header_text,omitempty"`
							} `json:"result,omitempty"`
						} `json:"rendering_strategy,omitempty"`
					} `json:"node,omitempty"`
				} `json:"edges,omitempty"`
			} `json:"results,omitempty"`
		} `json:"search_null_state,omitempty"`
		Viewer struct {
			RecentSearches struct {
				Edges []struct {
					Node struct {
						IsNode         string `json:"__isNode,omitempty"`
						IsProfile      string `json:"__isProfile,omitempty"`
						Typename       string `json:"__typename,omitempty"`
						ID             string `json:"id,omitempty"`
						Name           string `json:"name,omitempty"`
						ProfilePicture struct {
							URI string `json:"uri,omitempty"`
						} `json:"profile_picture,omitempty"`
						URL string `json:"url,omitempty"`
					} `json:"node,omitempty"`
					SearchBadge struct {
						Enabled bool `json:"enabled,omitempty"`
						Snippet any  `json:"snippet,omitempty"`
					} `json:"search_badge,omitempty"`
				} `json:"edges,omitempty"`
			} `json:"recent_searches,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometSettingsBadgeQuery struct {
	Data struct {
		Viewer struct {
			DeviceSwitchableAccountHasNotification bool `json:"device_switchable_account_has_notification,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometSettingsDropdownListQuery struct {
	Data struct {
		Viewer struct {
			Actor struct {
				Typename                        string `json:"__typename,omitempty"`
				ProfileSwitcherEligibleProfiles struct {
					Count int `json:"count,omitempty"`
				} `json:"profileSwitcherEligibleProfiles,omitempty"`
				ID                             string `json:"id,omitempty"`
				Name                           string `json:"name,omitempty"`
				SettingsDropdownProfilePicture struct {
					URI string `json:"uri,omitempty"`
				} `json:"settings_dropdown_profile_picture,omitempty"`
				ProfileSwitcherEligibleProfiles0 struct {
					Nodes []any `json:"nodes,omitempty"`
				} `json:"profile_switcher_eligible_profiles,omitempty"`
				FirstProfiles struct {
					Nodes []any `json:"nodes,omitempty"`
				} `json:"first_profiles,omitempty"`
				ShouldShowAccountLevelSettings bool   `json:"should_show_account_level_settings,omitempty"`
				Username                       string `json:"username,omitempty"`
				ProfileCount                   struct {
					Count int `json:"count,omitempty"`
				} `json:"profile_count,omitempty"`
				CanUserTypeSeeCompanyIdentitySwitcher bool `json:"can_user_type_see_company_identity_switcher,omitempty"`
				FirstProfile                          struct {
					Count int   `json:"count,omitempty"`
					Nodes []any `json:"nodes,omitempty"`
				} `json:"first_profile,omitempty"`
				PagePublishingAuthorizationHubActionURL string `json:"page_publishing_authorization_hub_action_url,omitempty"`
				PagePublishingAuthorizationAdminNotice  any    `json:"page_publishing_authorization_admin_notice,omitempty"`
				Profiles                                struct {
					Edges    []any `json:"edges,omitempty"`
					PageInfo struct {
						EndCursor   any  `json:"end_cursor,omitempty"`
						HasNextPage bool `json:"has_next_page,omitempty"`
					} `json:"page_info,omitempty"`
				} `json:"profiles,omitempty"`
				IsFailingPagePublishingAuthorization bool `json:"is_failing_page_publishing_authorization,omitempty"`
				ProfilePicture                       struct {
					URI    string  `json:"uri,omitempty"`
					Width  int     `json:"width,omitempty"`
					Height int     `json:"height,omitempty"`
					Scale  float64 `json:"scale,omitempty"`
				} `json:"profile_picture,omitempty"`
				UnseenUpdateCount                  int  `json:"unseen_update_count,omitempty"`
				IsAdditionalProfilePlus            bool `json:"is_additional_profile_plus,omitempty"`
				ProfileSwitcherUnreadNotifications int  `json:"profile_switcher_unread_notifications,omitempty"`
			} `json:"actor,omitempty"`
			SettingsMenuBusinessSuite struct {
				ShouldShowEntrypoint bool   `json:"should_show_entrypoint,omitempty"`
				BizwebHomeLink       string `json:"bizweb_home_link,omitempty"`
			} `json:"settings_menu_business_suite,omitempty"`
			IsEligibleForAccountLevelSettings bool `json:"is_eligible_for_account_level_settings,omitempty"`
			CanAccessPrivacyCheckup           bool `json:"can_access_privacy_checkup,omitempty"`
			PrivacyCenter                     struct {
				ShouldShowPrivacyCenter bool `json:"should_show_privacy_center,omitempty"`
			} `json:"privacy_center,omitempty"`
			DeviceSwitchableAccounts             []any `json:"device_switchable_accounts,omitempty"`
			AdditionalProfileCreationEligibility struct {
				SingleOwner struct {
					CanCreate   bool `json:"can_create,omitempty"`
					Explanation any  `json:"explanation,omitempty"`
				} `json:"single_owner,omitempty"`
			} `json:"additional_profile_creation_eligibility,omitempty"`
			FirstAccount []any  `json:"first_account,omitempty"`
			LogoutHash   string `json:"logout_hash,omitempty"`
		} `json:"viewer,omitempty"`
		IntlLocaleSelectorQuery struct {
			CurrentLocale struct {
				LocalizedName string `json:"localized_name,omitempty"`
			} `json:"current_locale,omitempty"`
		} `json:"intl_locale_selector_query,omitempty"`
		EligibleProfiles struct {
			Actor struct {
				Typename                        string `json:"__typename,omitempty"`
				ProfileSwitcherEligibleProfiles struct {
					Nodes []any `json:"nodes,omitempty"`
				} `json:"profile_switcher_eligible_profiles,omitempty"`
				ID string `json:"id,omitempty"`
			} `json:"actor,omitempty"`
		} `json:"eligibleProfiles,omitempty"`
		LogoutWhitelist []string `json:"logout_whitelist,omitempty"`
		FxcalSettings   struct {
			AcPhase string `json:"ac_phase,omitempty"`
		} `json:"fxcal_settings,omitempty"`
		FxIdentityManagement struct {
			IdentitiesAndCentralIdentities struct {
				LinkedIdentitiesToPci []struct {
					Typename                  string `json:"__typename,omitempty"`
					IdentityType              string `json:"identity_type,omitempty"`
					SwitcherNotificationCount int    `json:"switcher_notification_count,omitempty"`
				} `json:"linked_identities_to_pci,omitempty"`
				ID string `json:"id,omitempty"`
			} `json:"identities_and_central_identities,omitempty"`
		} `json:"fx_identity_management,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type CometSettingsDropdownTriggerQuery struct {
	Data struct {
		CoreAppAdminProfileSwitcherNux   any      `json:"core_app_admin_profile_switcher_nux,omitempty"`
		LogoutWhitelist                  []string `json:"logout_whitelist,omitempty"`
		PageManagementNux                any      `json:"page_management_nux,omitempty"`
		ProfileSwitcherAdminEducationNux any      `json:"profile_switcher_admin_education_nux,omitempty"`
		ProfileSwitcherNux               any      `json:"profile_switcher_nux,omitempty"`
		Viewer                           struct {
			Actor struct {
				Typename                string `json:"__typename,omitempty"`
				ID                      string `json:"id,omitempty"`
				IsAdditionalProfilePlus bool   `json:"is_additional_profile_plus,omitempty"`
				Name                    string `json:"name,omitempty"`
				ProfilePicture          struct {
					URI string `json:"uri,omitempty"`
				} `json:"profile_picture,omitempty"`
				ProfileSwitcherEligibleProfiles struct {
					Count int   `json:"count,omitempty"`
					Nodes []any `json:"nodes,omitempty"`
				} `json:"profile_switcher_eligible_profiles,omitempty"`
				ProfileTypeNameForContent                  string `json:"profile_type_name_for_content,omitempty"`
				ShouldShowSoapOnboardingDialog             bool   `json:"should_show_soap_onboarding_dialog,omitempty"`
				UserCategoryWithAdminsOrLimitedAccessRoles string `json:"user_category_with_admins_or_limited_access_roles,omitempty"`
				Username                                   string `json:"username,omitempty"`
			} `json:"actor,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type MWChatBadgeCountQuery struct {
	Data struct {
		Viewer struct {
			MessageThreads struct {
				UnseenCount int `json:"unseen_count,omitempty"`
			} `json:"message_threads,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type MWChatVideoAutoplaySettingContextQuery struct {
	Data struct {
		Viewer struct {
			VideoSettings struct {
				AutoplaySettingWww string `json:"autoplay_setting_www,omitempty"`
			} `json:"video_settings,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type MWLSInboxQuery struct {
	Data struct {
		Viewer struct {
			GroupsTab struct {
				SuggestedChats []any `json:"suggested_chats,omitempty"`
			} `json:"groups_tab,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}

type PresenceStatusProviderSubscriptionComponentQuery struct {
	Data struct {
		Viewer struct {
			ChatSidebarContactRankings []any `json:"chat_sidebar_contact_rankings,omitempty"`
		} `json:"viewer,omitempty"`
	} `json:"data,omitempty"`
	Extensions struct {
		IsFinal bool `json:"is_final,omitempty"`
	} `json:"extensions,omitempty"`
}
