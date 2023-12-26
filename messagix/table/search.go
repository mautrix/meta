package table

type LSUpdateSearchQueryStatus struct {
    Query string `index:"0"`
    Unknown int64 `index:"1"`
    StatusSecondary int64 `index:"2"`
    EndTimeMs int64 `index:"3"`
    ResultCount int64 `index:"4"`
}

type LSInsertSearchResult struct {
    Query string `index:"0"`
    ResultId string `index:"1"`
    GlobalIndex int64 `index:"2"`
    Type_ int64 `index:"3"`
    ThreadType int64 `index:"4"`
    DisplayName string `index:"5"`
    ProfilePicUrl string `index:"6"`
    SecondaryProfilePicUrl string `index:"7"`
    ContextLine string `index:"8"`
    MessageId string `index:"9"`
    MessageTimestampMs int64 `index:"10"`
    BlockedByViewerStatus int64 `index:"11"`
    IsVerified bool `index:"12"`
    IsInteropEligible bool `index:"13"`
    RestrictionType int64 `index:"14"`
    IsGroupsXacEligible bool `index:"15"`
    IsInvitedToCmChannel bool `index:"16"`
    IsEligibleForCmInvite bool `index:"17"`
    CanViewerMessage bool `index:"18"`
    IsWaAddressable bool `index:"19"`
    WaEligibility int64 `index:"20"`
    IsArmadilloTlcEligible bool `index:"21"`
    CommunityId int64 `index:"22"`
    OtherUserId int64 `index:"23"`
    ThreadJoinLinkHash int64 `index:"24"`
    SupportsE2eeSpamdStorage bool `index:"25"`
    DefaultE2eeThreads bool `index:"26"`
    IsRestrictedByViewer bool `index:"27"`
    DefaultE2eeThreadOneToOne bool `index:"28"`
    HasCutoverThread bool `index:"29"`
    IsViewerUnconnected bool `index:"30"`
    ResultIgid string `index:"31"`
}

type LSInsertSearchSection struct {
	Query string `index:"0"`
    GlobalIndex int64 `index:"1"`
	DisplayName string `index:"2"`
	AnalyticsName string `index:"3"`
	StartIndex int64 `index:"4"`
	EndIndex int64 `index:"5"`
}