package table


type LSVerifyContactRowExists struct {
	ContactId int64 `index:"0"`
	Unknown1 int64 `index:"1"`
	ProfilePictureUrl string `index:"2"`
	Name string `index:"3"`
	ContactType int64 `index:"4"`
	ProfilePictureFallbackUrl string `index:"5"`
	Unknown6 int64 `index:"6"`
	Unknown7 int64 `index:"7"`
	IsMemorialized bool `index:"9"`
	BlockedByViewerStatus int64 `index:"11"`
	CanViewerMessage bool `index:"12"`
	AuthorityLevel int64 `index:"14"`
	Capabilities int64 `index:"15"`
	Capabilities2 int64 `index:"16"`
	Gender Gender `index:"18"`
	ContactViewerRelationship ContactViewerRelationship `index:"19"`
	SecondaryName string `index:"20"`
}

type LSDeleteThenInsertContact struct {
    Id int64 `index:"0"`
    ProfileRingState int64 `index:"0"`
    ProfilePictureUrl string `index:"2"`
    ProfilePictureFallbackUrl string `index:"3"`
    ProfilePictureUrlExpirationTimestampMs int64 `index:"4"`
    ProfilePictureLargeUrl string `index:"5"`
    ProfilePictureLargeFallbackUrl string `index:"6"`
    ProfilePictureLargeUrlExpirationTimestampMs int64 `index:"7"`
    Name string `index:"9"`
    NormalizedNameForSearch string `index:"10"`
    IsMessengerUser bool `index:"11"`
    IsMemorialized bool `index:"12"`
    BlockedByViewerStatus int64 `index:"14"`
    Rank float64 `index:"17"`
    FirstName string `index:"18"`
    ContactType int64 `index:"19"`
    ContactTypeExact int64 `index:"20"`
    AuthorityLevel int64 `index:"21"`
    MessengerCallLogThirdPartyId string `index:"22"`
    ProfileRingColor int64 `index:"23"`
    RequiresMultiway bool `index:"24"`
    BlockedSinceTimestampMs int64 `index:"25"`
    CanViewerMessage bool `index:"26"`
    ProfileRingColorExpirationTimestampMs int64 `index:"27"`
    PhoneNumber int64 `index:"28"`
    EmailAddress string `index:"29"`
    WorkCompanyId int64 `index:"30"`
    WorkCompanyName string `index:"31"`
    WorkJobTitle string `index:"32"`
    NormalizedSearchTerms string `index:"33"`
    DeviceContactId int64 `index:"34"`
    IsManagedByViewer bool `index:"35"`
    WorkForeignEntityType int64 `index:"36"`
    Capabilities int64 `index:"37"`
    Capabilities2 int64 `index:"38"`
    ContactViewerRelationship ContactViewerRelationship `index:"39"`
    Gender Gender `index:"40"`
    SecondaryName string `index:"41"`
    ContactReachabilityStatusType int64 `index:"43"`
    RestrictionType int64 `index:"44"`
    WaConnectStatus int64 `index:"45"`
    FbUnblockedSinceTimestampMs int64 `index:"46"`
}

type LSDeleteThenInsertContactPresence struct {
    ContactId int64 `index:"0"`
    Status int64 `index:"1"` // make enum ?
    LastActiveTimestampMs int64 `index:"2"`
    ExpirationTimestampMs int64 `index:"3"`
    Capabilities int64 `index:"4"`
    PublishId string `index:"5"`
}

type LSDeleteThenInsertIGContactInfo struct {
    ContactId int64 `index:"0"`
    IgId string `index:"1"`
    LinkedFbid int64 `index:"2"`
    IgFollowStatus int64 `index:"4"`
    VerificationStatus int64 `index:"5"`
    E2eeEligibility int64 `index:"6"`
    SupportsE2eeSpamdStorage bool `index:"7"`
}