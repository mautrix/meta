package table

import (
	"strconv"
)

type LSUpdateSearchQueryStatus struct {
	Query           string `index:"0" json:",omitempty"`
	Unknown         int64  `index:"1" json:",omitempty"`
	StatusSecondary int64  `index:"2" json:",omitempty"`
	EndTimeMs       int64  `index:"3" json:",omitempty"`
	ResultCount     int64  `index:"4" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

type LSInsertSearchResult struct {
	Query                       string     `index:"0" json:",omitempty"`
	ResultId                    string     `index:"1" json:",omitempty"`
	GlobalIndex                 int64      `index:"2" json:",omitempty"`
	Type_                       SearchType `index:"3" json:",omitempty"`
	ThreadType                  ThreadType `index:"4" json:",omitempty"`
	DisplayName                 string     `index:"5" json:",omitempty"`
	ProfilePicUrl               string     `index:"6" json:",omitempty"`
	SecondaryProfilePicUrl      string     `index:"7" json:",omitempty"`
	ContextLine                 string     `index:"8" json:",omitempty"`
	MessageId                   string     `index:"9" json:",omitempty"`
	MessageTimestampMs          int64      `index:"10" json:",omitempty"`
	BlockedByViewerStatus       int64      `index:"11" json:",omitempty"`
	IsVerified                  bool       `index:"12" json:",omitempty"`
	IsInteropEligible           bool       `index:"13" json:",omitempty"`
	RestrictionType             int64      `index:"14" json:",omitempty"`
	IsGroupsXacEligible         bool       `index:"15" json:",omitempty"`
	IsInvitedToCmChannel        bool       `index:"16" json:",omitempty"`
	IsEligibleForCmInvite       bool       `index:"17" json:",omitempty"`
	CanViewerMessage            bool       `index:"18" json:",omitempty"`
	IsWaAddressable             bool       `index:"19" json:",omitempty"`
	WaEligibility               int64      `index:"20" json:",omitempty"`
	IsArmadilloTlcEligible      bool       `index:"21" json:",omitempty"`
	CommunityId                 int64      `index:"22" json:",omitempty"`
	OtherUserId                 int64      `index:"23" json:",omitempty"`
	ThreadJoinLinkHash          int64      `index:"24" json:",omitempty"`
	SupportsE2eeSpamdStorage    bool       `index:"25" json:",omitempty"`
	DefaultE2eeThreads          bool       `index:"26" json:",omitempty"`
	IsRestrictedByViewer        bool       `index:"27" json:",omitempty"`
	DefaultE2eeThreadOneToOne   bool       `index:"28" json:",omitempty"`
	HasCutoverThread            bool       `index:"29" json:",omitempty"`
	IsViewerUnconnected         bool       `index:"30" json:",omitempty"`
	ResultIgid                  string     `index:"31" json:",omitempty"`
	SecondaryContextLine        string     `index:"32" json:",omitempty"`
	IsInstamadilloCutover       bool       `index:"33" json:",omitempty"`
	IsDisappearingModeSettingOn bool       `index:"34" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}

func (lsisr *LSInsertSearchResult) GetUsername() string {
	return ""
}

func (lsisr *LSInsertSearchResult) GetName() string {
	return lsisr.DisplayName
}

func (lsisr *LSInsertSearchResult) GetAvatarURL() string {
	return lsisr.ProfilePicUrl
}

func (lsisr *LSInsertSearchResult) GetFBID() int64 {
	if lsisr.ThreadType == ONE_TO_ONE {
		fbid, _ := strconv.ParseInt(lsisr.ResultId, 10, 64)
		return fbid
	}
	return 0
}

type LSInsertSearchSection struct {
	Query         string `index:"0" json:",omitempty"`
	GlobalIndex   int64  `index:"1" json:",omitempty"`
	DisplayName   string `index:"2" json:",omitempty"`
	AnalyticsName string `index:"3" json:",omitempty"`
	StartIndex    int64  `index:"4" json:",omitempty"`
	EndIndex      int64  `index:"5" json:",omitempty"`

	Unrecognized map[int]any `json:",omitempty"`
}
