package types

import (
	"encoding/json"
	"strconv"
)

type UserInfo interface {
	GetUsername() string
	GetName() string
	GetAvatarURL() string
	GetFBID() int64
}

type CurrentBusinessAccount struct {
	BusinessAccountName                              any    `json:"businessAccountName,omitempty"`
	BusinessID                                       any    `json:"business_id,omitempty"`
	BusinessPersonaID                                any    `json:"business_persona_id,omitempty"`
	BusinessProfilePicURL                            any    `json:"business_profile_pic_url,omitempty"`
	BusinessRole                                     any    `json:"business_role,omitempty"`
	BusinessUserID                                   any    `json:"business_user_id,omitempty"`
	Email                                            any    `json:"email,omitempty"`
	EnterpriseProfilePicURL                          any    `json:"enterprise_profile_pic_url,omitempty"`
	ExpiryTime                                       any    `json:"expiry_time,omitempty"`
	FirstName                                        any    `json:"first_name,omitempty"`
	HasVerifiedEmail                                 any    `json:"has_verified_email,omitempty"`
	IPPermission                                     any    `json:"ip_permission,omitempty"`
	IsBusinessPerson                                 bool   `json:"isBusinessPerson,omitempty"`
	IsEnterpriseBusiness                             bool   `json:"isEnterpriseBusiness,omitempty"`
	IsFacebookWorkAccount                            bool   `json:"isFacebookWorkAccount,omitempty"`
	IsInstagramBusinessPerson                        bool   `json:"isInstagramBusinessPerson,omitempty"`
	IsTwoFacNewFlow                                  bool   `json:"isTwoFacNewFlow,omitempty"`
	IsUserOptInAccountSwitchInfraUpgrade             bool   `json:"isUserOptInAccountSwitchInfraUpgrade,omitempty"`
	IsAdsFeatureLimited                              any    `json:"is_ads_feature_limited,omitempty"`
	IsBusinessBanhammered                            any    `json:"is_business_banhammered,omitempty"`
	LastName                                         any    `json:"last_name,omitempty"`
	PermittedBusinessAccountTaskIds                  []any  `json:"permitted_business_account_task_ids,omitempty"`
	PersonalUserID                                   string `json:"personal_user_id,omitempty"`
	ShouldHideComponentsByUnsupportedFirstPartyTools bool   `json:"shouldHideComponentsByUnsupportedFirstPartyTools,omitempty"`
	ShouldShowAccountSwitchComponents                bool   `json:"shouldShowAccountSwitchComponents,omitempty"`
}

type MessengerWebInitData struct {
	AccountKey string `json:"accountKey,omitempty"`
	//ActiveThreadKeys
	//AllActiveThreadKeys
	AppID           int64           `json:"appId,omitempty"`
	CryptoAuthToken CryptoAuthToken `json:"cryptoAuthToken,omitempty"`
	LogoutToken     string          `json:"logoutToken,omitempty"`
	SessionID       string          `json:"sessionId,omitempty"`
	UserID          json.Number     `json:"userId,omitempty"` // number on fb, string on ig?
	AccountKeyV2    string          `json:"accountKeyV2,omitempty"`
}

type CryptoAuthToken struct {
	EncryptedSerializedCat  string `json:"encrypted_serialized_cat,omitempty"`
	ExpirationTimeInSeconds int64  `json:"expiration_time_in_seconds,omitempty"`
}

type LSD struct {
	Token string `json:"token,omitempty"`
}

type IntlViewerContext struct {
	Gender         int `json:"GENDER,omitempty"`
	RegionalLocale any `json:"regionalLocale,omitempty"`
}

type IntlCurrentLocale struct {
	Code string `json:"code,omitempty"`
}

type DTSGInitData struct {
	AsyncGetToken string `json:"async_get_token,omitempty"`
	Token         string `json:"token,omitempty"`
}

type DTSGInitialData struct {
	Token string `json:"token,omitempty"`
}

type CurrentUserInitialData struct {
	AccountID                       string `json:"ACCOUNT_ID,omitempty"`
	AppID                           string `json:"APP_ID,omitempty"`
	HasSecondaryBusinessPerson      bool   `json:"HAS_SECONDARY_BUSINESS_PERSON,omitempty"`
	IsBusinessDomain                bool   `json:"IS_BUSINESS_DOMAIN,omitempty"`
	IsBusinessPersonAccount         bool   `json:"IS_BUSINESS_PERSON_ACCOUNT,omitempty"`
	IsDeactivatedAllowedOnMessenger bool   `json:"IS_DEACTIVATED_ALLOWED_ON_MESSENGER,omitempty"`
	IsFacebookWorkAccount           bool   `json:"IS_FACEBOOK_WORK_ACCOUNT,omitempty"`
	IsMessengerCallGuestUser        bool   `json:"IS_MESSENGER_CALL_GUEST_USER,omitempty"`
	IsMessengerOnlyUser             bool   `json:"IS_MESSENGER_ONLY_USER,omitempty"`
	IsWorkroomsUser                 bool   `json:"IS_WORKROOMS_USER,omitempty"`
	IsWorkMessengerCallGuestUser    bool   `json:"IS_WORK_MESSENGER_CALL_GUEST_USER,omitempty"`
	Name                            string `json:"NAME,omitempty"`
	ShortName                       string `json:"SHORT_NAME,omitempty"`
	UserID                          string `json:"USER_ID,omitempty"`

	NonFacebookUserID         string `json:"NON_FACEBOOK_USER_ID,omitempty"`
	IGUserEIMU                string `json:"IG_USER_EIMU,omitempty"`
	IsInstagramUser           int    `json:"IS_INSTAGRAM_USER,omitempty"`
	IsInstagramBusinessPerson bool   `json:"IS_INSTAGRAM_BUSINESS_PERSON,omitempty"`
	// may be int?
	//IsEmployee                bool   `json:"IS_EMPLOYEE,omitempty"`
	//IsTestUser                bool   `json:"IS_TEST_USER,omitempty"`
}

func (c *CurrentUserInitialData) GetBusinessEmail() string {
	return "" // TO-DO
}

func (c *CurrentUserInitialData) GetUserId() string {
	return c.UserID
}

func (c *CurrentUserInitialData) GetName() string {
	return c.Name
}

func (c *CurrentUserInitialData) GetUsername() string {
	return c.ShortName
}

func (c *CurrentUserInitialData) GetFBID() int64 {
	id, _ := strconv.ParseInt(c.AccountID, 10, 64)
	return id
}

func (c *CurrentUserInitialData) IsPrivate() bool {
	return false // TO-DO
}

func (c *CurrentUserInitialData) GetAvatarURLHD() string {
	return "" // TO-DO
}

func (c *CurrentUserInitialData) GetAvatarURL() string {
	return "" // TO-DO
}

func (c *CurrentUserInitialData) GetBiography() string {
	return "" // TO-DO
}

func (c *CurrentUserInitialData) HasPhoneNumber() bool {
	return false // TO-DO
}

func (c *CurrentUserInitialData) GetExternalUrl() string {
	return "" // TO-DO
}

type ViewerData struct {
	Biography                string `json:"biography,omitempty"`
	BusinessAddressJSON      any    `json:"business_address_json,omitempty"`
	BusinessContactMethod    string `json:"business_contact_method,omitempty"`
	BusinessEmail            string `json:"business_email,omitempty"`
	BusinessPhoneNumber      any    `json:"business_phone_number,omitempty"`
	CanSeeOrganicInsights    bool   `json:"can_see_organic_insights,omitempty"`
	CategoryName             any    `json:"category_name,omitempty"`
	ExternalURL              string `json:"external_url,omitempty"`
	Fbid                     string `json:"fbid,omitempty"`
	FullName                 string `json:"full_name,omitempty"`
	HasPhoneNumber           bool   `json:"has_phone_number,omitempty"`
	HasProfilePic            bool   `json:"has_profile_pic,omitempty"`
	HasTabbedInbox           bool   `json:"has_tabbed_inbox,omitempty"`
	HideLikeAndViewCounts    bool   `json:"hide_like_and_view_counts,omitempty"`
	ID                       string `json:"id,omitempty"`
	IsBusinessAccount        bool   `json:"is_business_account,omitempty"`
	IsJoinedRecently         bool   `json:"is_joined_recently,omitempty"`
	IsSupervisedUser         bool   `json:"is_supervised_user,omitempty"`
	GuardianID               any    `json:"guardian_id,omitempty"`
	IsPrivate                bool   `json:"is_private,omitempty"`
	IsProfessionalAccount    bool   `json:"is_professional_account,omitempty"`
	IsSupervisionEnabled     bool   `json:"is_supervision_enabled,omitempty"`
	IsUserInCanada           bool   `json:"is_user_in_canada,omitempty"`
	ProfilePicURL            string `json:"profile_pic_url,omitempty"`
	ProfilePicURLHd          string `json:"profile_pic_url_hd,omitempty"`
	ShouldShowCategory       bool   `json:"should_show_category,omitempty"`
	ShouldShowPublicContacts bool   `json:"should_show_public_contacts,omitempty"`
	Username                 string `json:"username,omitempty"`
	BadgeCount               string `json:"badge_count,omitempty"`
	IsBasicAdsOptedIn        bool   `json:"is_basic_ads_opted_in,omitempty"`
	BasicAdsTier             int    `json:"basic_ads_tier,omitempty"`
	ProbablyHasApp           bool   `json:"probably_has_app,omitempty"`
}

type PolarisViewer struct {
	Data ViewerData `json:"data,omitempty"`
	ID   string     `json:"id,omitempty"`
}

func (p *PolarisViewer) GetUserId() string {
	return p.ID
}

func (p *PolarisViewer) GetUsername() string {
	return p.Data.Username
}

func (p *PolarisViewer) GetName() string {
	return p.Data.FullName
}

func (p *PolarisViewer) GetFBID() int64 {
	id, _ := strconv.ParseInt(p.Data.Fbid, 10, 64)
	return id
}

func (p *PolarisViewer) GetAvatarURLHD() string {
	return p.Data.ProfilePicURLHd
}

func (p *PolarisViewer) GetAvatarURL() string {
	if p.Data.ProfilePicURLHd != "" {
		return p.Data.ProfilePicURLHd
	}
	return p.Data.ProfilePicURL
}

func (p *PolarisViewer) GetBiography() string {
	return p.Data.Biography
}

func (p *PolarisViewer) GetExternalUrl() string {
	return p.Data.ExternalURL
}

func (p *PolarisViewer) IsPrivate() bool {
	return p.Data.IsPrivate
}

func (p *PolarisViewer) HasPhoneNumber() bool {
	return p.Data.HasPhoneNumber
}

func (p *PolarisViewer) GetBusinessEmail() string {
	return p.Data.BusinessEmail
}
