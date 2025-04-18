package table

type LSVerifyContactRowExists struct {
	ContactId                              int64                     `index:"0" json:",omitempty"`
	ContactIdType                          ContactIDType             `index:"1" json:",omitempty"`
	ProfilePictureUrl                      string                    `index:"2" json:",omitempty"`
	Name                                   string                    `index:"3" json:",omitempty"`
	ContactType                            int64                     `index:"4" json:",omitempty"`
	ProfilePictureFallbackUrl              string                    `index:"5" json:",omitempty"`
	ProfilePictureUrlExpirationTimestampMs int64                     `index:"6" json:",omitempty"`
	UrlExpirationTimestampMs               int64                     `index:"7" json:",omitempty"`
	NormalizedNameForSearch                string                    `index:"8" json:",omitempty"`
	IsMemorialized                         bool                      `index:"9" json:",omitempty"`
	IsBlocked                              bool                      `index:"10" json:",omitempty"`
	BlockedByViewerStatus                  int64                     `index:"11" json:",omitempty"`
	CanViewerMessage                       bool                      `index:"12" json:",omitempty"`
	IsSelf                                 bool                      `index:"13" json:",omitempty"`
	AuthorityLevel                         int64                     `index:"14" json:",omitempty"`
	Capabilities                           int64                     `index:"15" json:",omitempty"`
	Capabilities2                          int64                     `index:"16" json:",omitempty"`
	WorkForeignEntityType                  int64                     `index:"17" json:",omitempty"` // TODO enum
	Gender                                 Gender                    `index:"18" json:",omitempty"`
	ContactViewerRelationship              ContactViewerRelationship `index:"19" json:",omitempty"`
	SecondaryName                          string                    `index:"20" json:",omitempty"`
	FirstName                              string                    `index:"21" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (vcre *LSVerifyContactRowExists) GetUsername() string {
	return vcre.SecondaryName
}

func (vcre *LSVerifyContactRowExists) GetName() string {
	return vcre.Name
}

func (vcre *LSVerifyContactRowExists) GetAvatarURL() string {
	return vcre.ProfilePictureUrl
}

func (vcre *LSVerifyContactRowExists) GetFBID() int64 { return vcre.ContactId }

type LSDeleteThenInsertContact struct {
	Id                                          int64                     `index:"0" json:",omitempty"`
	ProfilePictureUrl                           string                    `index:"2" json:",omitempty"`
	ProfilePictureFallbackUrl                   string                    `index:"3" json:",omitempty"`
	ProfilePictureUrlExpirationTimestampMs      int64                     `index:"4" json:",omitempty"`
	ProfilePictureLargeUrl                      string                    `index:"5" json:",omitempty"`
	ProfilePictureLargeFallbackUrl              string                    `index:"6" json:",omitempty"`
	ProfilePictureLargeUrlExpirationTimestampMs int64                     `index:"7" json:",omitempty"`
	Name                                        string                    `index:"9" json:",omitempty"`
	NormalizedNameForSearch                     string                    `index:"10" json:",omitempty"`
	IsMessengerUser                             bool                      `index:"11" json:",omitempty"`
	IsMemorialized                              bool                      `index:"12" json:",omitempty"`
	BlockedByViewerStatus                       int64                     `index:"14" json:",omitempty"`
	Rank                                        float64                   `index:"17" json:",omitempty"`
	FirstName                                   string                    `index:"18" json:",omitempty"`
	ContactType                                 int64                     `index:"19" json:",omitempty"` // TODO enum
	ContactTypeExact                            int64                     `index:"20" json:",omitempty"` // TODO enum
	AuthorityLevel                              int64                     `index:"21" json:",omitempty"`
	MessengerCallLogThirdPartyId                string                    `index:"22" json:",omitempty"`
	ProfileRingColor                            int64                     `index:"23" json:",omitempty"`
	RequiresMultiway                            bool                      `index:"24" json:",omitempty"`
	BlockedSinceTimestampMs                     int64                     `index:"25" json:",omitempty"`
	CanViewerMessage                            bool                      `index:"26" json:",omitempty"`
	ProfileRingColorExpirationTimestampMs       int64                     `index:"27" json:",omitempty"`
	PhoneNumber                                 int64                     `index:"28" json:",omitempty"`
	EmailAddress                                string                    `index:"29" json:",omitempty"`
	WorkCompanyId                               int64                     `index:"30" json:",omitempty"`
	WorkCompanyName                             string                    `index:"31" json:",omitempty"`
	WorkJobTitle                                string                    `index:"32" json:",omitempty"`
	NormalizedSearchTerms                       string                    `index:"33" json:",omitempty"`
	DeviceContactId                             int64                     `index:"34" json:",omitempty"`
	IsManagedByViewer                           bool                      `index:"35" json:",omitempty"`
	WorkForeignEntityType                       int64                     `index:"36" json:",omitempty"`
	Capabilities                                int64                     `index:"37" json:",omitempty"`
	Capabilities2                               int64                     `index:"38" json:",omitempty"`
	ContactViewerRelationship                   ContactViewerRelationship `index:"39" json:",omitempty"`
	Gender                                      Gender                    `index:"40" json:",omitempty"`
	SecondaryName                               string                    `index:"41" json:",omitempty"`
	Username                                    string                    `index:"43" json:",omitempty"`
	ContactReachabilityStatusType               int64                     `index:"44" json:",omitempty"` // TODO enum
	RestrictionType                             int64                     `index:"45" json:",omitempty"` // TODO enum
	WaConnectStatus                             int64                     `index:"46" json:",omitempty"`
	FbUnblockedSinceTimestampMs                 int64                     `index:"47" json:",omitempty"`
	PageType                                    int64                     `index:"48" json:",omitempty"`

	// TODO figure out where this is
	//ProfileRingState                            int64                     `index:"0" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (ls *LSDeleteThenInsertContact) GetUsername() string {
	return ls.SecondaryName
}

func (ls *LSDeleteThenInsertContact) GetName() string {
	return ls.Name
}

func (ls *LSDeleteThenInsertContact) GetAvatarURL() string {
	if ls.ProfilePictureLargeUrl != "" {
		return ls.ProfilePictureLargeUrl
	}
	return ls.ProfilePictureUrl
}

func (ls *LSDeleteThenInsertContact) GetFBID() int64 {
	return ls.Id
}

type LSDeleteThenInsertContactPresence struct {
	ContactId             int64  `index:"0" json:",omitempty"`
	Status                int64  `index:"1" json:",omitempty"` // make enum ?
	LastActiveTimestampMs int64  `index:"2" json:",omitempty"`
	ExpirationTimestampMs int64  `index:"3" json:",omitempty"`
	Capabilities          int64  `index:"4" json:",omitempty"`
	PublishId             string `index:"5" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSDeleteThenInsertIGContactInfo struct {
	ContactId                int64  `index:"0" json:",omitempty"`
	IgId                     string `index:"1" json:",omitempty"`
	LinkedFbid               int64  `index:"2" json:",omitempty"`
	IgFollowStatus           int64  `index:"4" json:",omitempty"` // TODO enum?
	VerificationStatus       int64  `index:"5" json:",omitempty"` // TODO enum?
	E2eeEligibility          int64  `index:"6" json:",omitempty"`
	SupportsE2eeSpamdStorage bool   `index:"7" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSVerifyContactParticipantExist struct {
	ContactID int64  `index:"0" json:",omitempty"`
	ThreadKey int64  `index:"1" json:",omitempty"`
	Name      string `index:"2" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSVerifyCommunityMemberContextualProfileExists struct {
	ContactID   int64 `index:"0" json:",omitempty"`
	CommunityID int64 `index:"1" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSInsertCommunityMember struct {
	CommunityID                            int64  `index:"0" json:",omitempty"`
	ContactID                              int64  `index:"1" json:",omitempty"`
	Name                                   string `index:"2" json:",omitempty"`
	ProfilePictureURL                      string `index:"3" json:",omitempty"`
	IsAdmin                                bool   `index:"4" json:",omitempty"`
	IsBlocked                              bool   `index:"5" json:",omitempty"`
	ProfilePictureURLFallback              string `index:"6" json:",omitempty"`
	ProfilePictureURLExpirationTimestampMS int64  `index:"7" json:",omitempty"`
	FirstName                              string `index:"8" json:",omitempty"`
	IsModerator                            bool   `index:"9" json:",omitempty"`
	IsMuted                                bool   `index:"10" json:",omitempty"`
	IsBlockedFromCommunity                 bool   `index:"11" json:",omitempty"`
	Source                                 int64  `index:"12" json:",omitempty"`
	Subtitle                               string `index:"13" json:",omitempty"`
	IsEligibleForCMInvite                  bool   `index:"14" json:",omitempty"`
	Nickname                               string `index:"15" json:",omitempty"`
	ThreadRole                             int64  `index:"16" json:",omitempty"`
	ContactCapabilities                    int64  `index:"17" json:",omitempty"`
	ThreadRoles                            int64  `index:"18" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpdateOrInsertCommunityMember struct {
	CommunityID                            int64  `index:"0" json:",omitempty"`
	ContactID                              int64  `index:"1" json:",omitempty"`
	Name                                   string `index:"2" json:",omitempty"`
	FirstName                              string `index:"3" json:",omitempty"`
	ProfilePictureURL                      string `index:"4" json:",omitempty"`
	ProfilePictureURLExpirationTimestampMS int64  `index:"5" json:",omitempty"`
	ProfilePictureURLFallback              string `index:"6" json:",omitempty"`
	IsBlocked                              bool   `index:"7" json:",omitempty"`
	IsAdmin                                bool   `index:"8" json:",omitempty"`
	IsBlockedFromCommunity                 bool   `index:"9" json:",omitempty"`
	IsMuted                                bool   `index:"10" json:",omitempty"`
	IsCommunityMember                      bool   `index:"11" json:",omitempty"`
	IsModerator                            bool   `index:"13" json:",omitempty"`
	FetchingInfoTaskID                     int64  `index:"14" json:",omitempty"`
	AdminActions                           int64  `index:"15" json:",omitempty"`
	Source                                 int64  `index:"16" json:",omitempty"`
	Nickname                               string `index:"17" json:",omitempty"`
	ContactCapabilities                    int64  `index:"18" json:",omitempty"`
	ThreadRoles                            int64  `index:"19" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSUpsertCommunityMemberRanges struct {
	CommunityID int64 `index:"0" json:",omitempty"`
	//IsAdmin    int64  `index:"1" json:",omitempty"`
	HasMoreAfter   bool   `index:"3" json:",omitempty"`
	IsLoadingAfter bool   `index:"4" json:",omitempty"`
	NextPageCursor string `index:"5" json:",omitempty"`
	MaxName        string `index:"6" json:",omitempty"`
	Source         int64  `index:"7" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}
