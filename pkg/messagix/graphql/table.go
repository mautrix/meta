package graphql

import (
	"encoding/json"
)

type GraphQLTable struct {
	CometActorGatewayHandlerQuery                         []CometActorGatewayHandlerQuery
	CometAppNavigationProfileSwitcherConfigQuery          []CometAppNavigationProfileSwitcherConfigQuery
	CometAppNavigationTargetedTabBarContentInnerImplQuery []CometAppNavigationTargetedTabBarContentInnerImplQuery
	CometLogoutHandlerQuery                               []CometLogoutHandlerQuery
	CometNotificationsBadgeCountQuery                     []CometNotificationsBadgeCountQuery
	CometQuickPromotionInterstitialQuery                  []CometQuickPromotionInterstitialQuery
	CometSearchBootstrapKeywordsDataSourceQuery           []CometSearchBootstrapKeywordsDataSourceQuery
	CometSearchRecentDataSourceQuery                      []CometSearchRecentDataSourceQuery
	CometSettingsBadgeQuery                               []CometSettingsBadgeQuery
	CometSettingsDropdownListQuery                        []CometSettingsDropdownListQuery
	CometSettingsDropdownTriggerQuery                     []CometSettingsDropdownTriggerQuery
	MWChatBadgeCountQuery                                 []MWChatBadgeCountQuery
	MWChatVideoAutoplaySettingContextQuery                []MWChatVideoAutoplaySettingContextQuery
	MWLSInboxQuery                                        []MWLSInboxQuery
	PresenceStatusProviderSubscriptionComponentQuery      []PresenceStatusProviderSubscriptionComponentQuery
}

type GraphQLPreloader struct {
	ActorID     any             `json:"actorID,omitempty"`
	PreloaderID string          `json:"preloaderID,omitempty"`
	QueryID     string          `json:"queryID,omitempty"`
	Variables   json.RawMessage `json:"variables,omitempty"`
}

func (gqp *GraphQLPreloader) ParseVariables() (vars Variables, err error) {
	err = json.Unmarshal(gqp.Variables, &vars)
	return
}

type Variables struct {
	DeviceID              string `json:"deviceId,omitempty"`
	IncludeChatVisibility bool   `json:"includeChatVisibility,omitempty"`
	RequestID             int    `json:"requestId,omitempty"`
	RequestPayload        string `json:"requestPayload,omitempty"`
	RequestType           int    `json:"requestType,omitempty"`
}
