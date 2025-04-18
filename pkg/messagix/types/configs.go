package types

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

type MqttWebDeviceID struct {
	ClientID string `json:"clientID,omitempty"`
}

type MQTTWebConfig struct {
	Fbid               string `json:"fbid,omitempty"`
	AppID              int64  `json:"appID,omitempty"`
	Endpoint           string `json:"endpoint,omitempty"`
	PollingEndpoint    string `json:"pollingEndpoint,omitempty"`
	SubscribedTopics   []any  `json:"subscribedTopics,omitempty"`
	Capabilities       int    `json:"capabilities,omitempty"`
	ClientCapabilities int    `json:"clientCapabilities,omitempty"`
	ChatVisibility     bool   `json:"chatVisibility,omitempty"`
	HostNameOverride   string `json:"hostNameOverride,omitempty"`
}

type MQTTConfig struct {
	ProtocolName       string
	ProtocolLevel      uint8
	ClientId           string
	Broker             string
	KeepAliveTimeout   uint16
	SessionId          int64
	AppId              int64
	ClientCapabilities int
	Capabilities       int
	ChatOn             bool
	SubscribedTopics   []any
	ConnectionType     string
	HostNameOverride   string
	Cid                string // device id
}

func (m *MQTTConfig) BuildBrokerUrl() string {
	query := &url.Values{}
	query.Add("cid", m.Cid)
	query.Add("sid", strconv.FormatInt(m.SessionId, 10))

	encodedQuery := query.Encode()
	if !strings.HasSuffix(m.Broker, "=") {
		return m.Broker + encodedQuery
	} else {
		return m.Broker + "&" + encodedQuery
	}
}

type SprinkleConfig struct {
	ParamName       string `json:"param_name,omitempty"`
	ShouldRandomize bool   `json:"should_randomize,omitempty"`
	Version         int    `json:"version,omitempty"`
}

type WebConnectionClassServerGuess struct {
	ConnectionClass string `json:"connectionClass,omitempty"`
}

type WebDevicePerfClassData struct {
	DeviceLevel string `json:"deviceLevel,omitempty"`
	YearClass   any    `json:"yearClass,omitempty"`
}

type USIDMetadata struct {
	BrowserID    string `json:"browser_id,omitempty"`
	PageID       string `json:"page_id,omitempty"`
	TabID        string `json:"tab_id,omitempty"`
	TransitionID int    `json:"transition_id,omitempty"`
	Version      int    `json:"version,omitempty"`
}

type MessengerWebRegion struct {
	Region string `json:"regionNullable,omitempty"`
}

type LSPlatformMessengerSyncParams struct {
	Mailbox string `json:"mailbox,omitempty"`
	Contact string `json:"contact,omitempty"`
	E2Ee    string `json:"e2ee,omitempty"`
}

type InitialCookieConsent struct {
	DeferCookies           bool  `json:"deferCookies,omitempty"`
	InitialConsent         []int `json:"initialConsent,omitempty"`
	NoCookies              bool  `json:"noCookies,omitempty"`
	ShouldShowCookieBanner bool  `json:"shouldShowCookieBanner,omitempty"`
}

type InstagramPasswordEncryption struct {
	KeyID     string `json:"key_id,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	Version   string `json:"version,omitempty"`
}

type XIGSharedData struct {
	ConfigData *XIGConfigData
	Raw        string `json:"raw,omitempty"`
	Native     struct {
		Config struct {
			CsrfToken string `json:"csrf_token,omitempty"`
			ViewerID  string `json:"viewerId,omitempty"`
			Viewer    struct {
				IsBasicAdsOptedIn bool `json:"is_basic_ads_opted_in,omitempty"`
				BasicAdsTier      int  `json:"basic_ads_tier,omitempty"`
			} `json:"viewer,omitempty"`
		} `json:"config,omitempty"`
		SendDeviceIDHeader bool `json:"send_device_id_header,omitempty"`
		ServerChecks       struct {
			Hfe bool `json:"hfe,omitempty"`
		} `json:"server_checks,omitempty"`
		WwwRoutingConfig struct {
			FrontendOnlyRoutes []struct {
				Path        string `json:"path,omitempty"`
				Destination string `json:"destination,omitempty"`
			} `json:"frontend_only_routes,omitempty"`
		} `json:"www_routing_config,omitempty"`
		DeviceID               string `json:"device_id,omitempty"`
		SignalCollectionConfig struct {
			Sid int `json:"sid,omitempty"`
		} `json:"signal_collection_config,omitempty"`
		PrivacyFlowTrigger        any `json:"privacy_flow_trigger,omitempty"`
		PlatformInstallBadgeLinks struct {
			Ios         string `json:"ios,omitempty"`
			Android     string `json:"android,omitempty"`
			WindowsNt10 string `json:"windows_nt_10,omitempty"`
		} `json:"platform_install_badge_links,omitempty"`
		CountryCode    string `json:"country_code,omitempty"`
		ProbablyHasApp bool   `json:"probably_has_app,omitempty"`
	} `json:"native,omitempty"`
}

func (xig *XIGSharedData) ParseRaw() error {
	return json.Unmarshal([]byte(xig.Raw), &xig.ConfigData)
}

type RelayAPIConfigDefaults struct {
	AccessToken   string `json:"accessToken,omitempty"`
	ActorID       string `json:"actorID,omitempty"`
	CustomHeaders struct {
		XIGAppID string `json:"X-IG-App-ID,omitempty"`
		XIGD     string `json:"X-IG-D,omitempty"`
	} `json:"customHeaders,omitempty"`
	EnableNetworkLogger       bool        `json:"enableNetworkLogger,omitempty"`
	FetchTimeout              int         `json:"fetchTimeout,omitempty"`
	GraphBatchURI             string      `json:"graphBatchURI,omitempty"`
	GraphURI                  string      `json:"graphURI,omitempty"`
	RetryDelays               []int       `json:"retryDelays,omitempty"`
	UseXController            bool        `json:"useXController,omitempty"`
	XhrEncoding               interface{} `json:"xhrEncoding,omitempty"`
	SubscriptionTopicURI      interface{} `json:"subscriptionTopicURI,omitempty"`
	WithCredentials           bool        `json:"withCredentials,omitempty"`
	IsProductionEndpoint      bool        `json:"isProductionEndpoint,omitempty"`
	WorkRequestTaggingProduct interface{} `json:"workRequestTaggingProduct,omitempty"`
	EncryptionKeyParams       interface{} `json:"encryptionKeyParams,omitempty"`
}

type EnvJSON struct {
	UseTrustedTypes          bool   `json:"useTrustedTypes,omitempty"`
	IsTrustedTypesReportOnly bool   `json:"isTrustedTypesReportOnly,omitempty"`
	RoutingNamespace         string `json:"routing_namespace,omitempty"`
	Ghlss                    string `json:"ghlss,omitempty"`
	ScheduledCSSJSScheduler  bool   `json:"scheduledCSSJSScheduler,omitempty"`
	UseFbtVirtualModules     bool   `json:"use_fbt_virtual_modules,omitempty"`
	CompatIframeToken        string `json:"compat_iframe_token,omitempty"`
}

type Eqmc struct {
	AjaxURL        string `json:"u,omitempty"`
	HasteSessionId string `json:"e,omitempty"`
	S              string `json:"s,omitempty"`
	W              int    `json:"w,omitempty"`
	FbDtsg         string `json:"f,omitempty"`
	L              any    `json:"l,omitempty"`
}

type AjaxQueryParams struct {
	A        string `json:"__a"`
	User     string `json:"__user"`
	CometReq string `json:"__comet_req"`
	Jazoest  string `json:"jazoest"`
}

func (e *Eqmc) ParseAjaxURLData() (*AjaxQueryParams, error) {
	u, err := url.Parse(e.AjaxURL)
	if err != nil {
		return nil, err
	}

	params, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, err
	}

	var result AjaxQueryParams

	result.A = params.Get("__a")
	result.User = params.Get("__user")
	result.CometReq = params.Get("__comet_req")
	result.Jazoest = params.Get("jazoest")
	return &result, nil
}

type RawJSONConfigs struct {
	Eqmc    Eqmc
	EnvJSON EnvJSON
}

type SchedulerJSDefineConfig struct {
	// NOTE: ModuleParser.handleConfigData accesses these fields by name with reflection
	// TODO: change it to use struct tags instead of field names

	MqttWebConfig                 MqttWebConfig
	MqttWebDeviceID               MqttWebDeviceID
	WebDevicePerfClassData        WebDevicePerfClassData
	BootloaderConfig              BootLoaderConfig
	CurrentBusinessUser           CurrentBusinessAccount
	SiteData                      SiteData
	SprinkleConfig                SprinkleConfig
	USIDMetadata                  USIDMetadata
	WebConnectionClassServerGuess WebConnectionClassServerGuess
	MessengerWebRegion            MessengerWebRegion
	MessengerWebInitData          MessengerWebInitData
	LSD                           LSD
	IntlViewerContext             IntlViewerContext
	IntlCurrentLocale             IntlCurrentLocale
	DTSGInitData                  DTSGInitData
	DTSGInitialData               DTSGInitialData
	CurrentUserInitialData        CurrentUserInitialData
	LSPlatformMessengerSyncParams LSPlatformMessengerSyncParams
	ServerNonce                   ServerNonce
	InitialCookieConsent          InitialCookieConsent
	InstagramPasswordEncryption   InstagramPasswordEncryption
	XIGSharedData                 XIGSharedData
	RelayAPIConfigDefaults        RelayAPIConfigDefaults
	PolarisViewer                 PolarisViewer
}

type MqttWebConfig struct {
	AppID              int64    `json:"appID,omitempty"`
	Capabilities       int      `json:"capabilities,omitempty"`
	ChatVisibility     bool     `json:"chatVisibility,omitempty"`
	ClientCapabilities int      `json:"clientCapabilities,omitempty"`
	Endpoint           string   `json:"endpoint,omitempty"`
	Fbid               string   `json:"fbid,omitempty"`
	HostNameOverride   string   `json:"hostNameOverride,omitempty"`
	PollingEndpoint    string   `json:"pollingEndpoint,omitempty"`
	SubscribedTopics   []string `json:"subscribedTopics,omitempty"`
}

type SiteData struct {
	SpinB                 string  `json:"__spin_b,omitempty"`
	SpinR                 int     `json:"__spin_r,omitempty"`
	SpinT                 int     `json:"__spin_t,omitempty"`
	BeOneAhead            bool    `json:"be_one_ahead,omitempty"`
	BlHashVersion         int64   `json:"bl_hash_version,omitempty"`
	ClientRevision        int64   `json:"client_revision,omitempty"`
	CometEnv              int64   `json:"comet_env,omitempty"`
	HasteSession          string  `json:"haste_session,omitempty"`
	HasteSite             string  `json:"haste_site,omitempty"`
	Hsi                   string  `json:"hsi,omitempty"`
	IsComet               bool    `json:"is_comet,omitempty"`
	IsExperimentalTier    bool    `json:"is_experimental_tier,omitempty"`
	IsJitWarmedUp         bool    `json:"is_jit_warmed_up,omitempty"`
	IsRtl                 bool    `json:"is_rtl,omitempty"`
	ManifestBaseURI       string  `json:"manifest_base_uri,omitempty"`
	ManifestOrigin        string  `json:"manifest_origin,omitempty"`
	ManifestVersionPrefix string  `json:"manifest_version_prefix,omitempty"`
	PkgCohort             string  `json:"pkg_cohort,omitempty"`
	Pr                    float64 `json:"pr,omitempty"`
	PushPhase             string  `json:"push_phase,omitempty"`
	SemrHostBucket        string  `json:"semr_host_bucket,omitempty"`
	ServerRevision        int64   `json:"server_revision,omitempty"`
	SkipRdBl              bool    `json:"skip_rd_bl,omitempty"`
	Spin                  int64   `json:"spin,omitempty"`
	Tier                  string  `json:"tier,omitempty"`
	Vip                   string  `json:"vip,omitempty"`
	WbloksEnv             bool    `json:"wbloks_env,omitempty"`
}

type ServerNonce struct {
	ServerNonce string `json:"ServerNonce,omitempty"`
}

type SchedulerJSRequire struct {
	LSTable *table.LSTable
}
