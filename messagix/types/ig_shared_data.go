package types

type XIGConfigData struct {
	Config struct {
		CsrfToken string     `json:"csrf_token,omitempty"`
		Viewer    ViewerData `json:"viewer,omitempty"`
		ViewerID  string     `json:"viewerId,omitempty"`
	} `json:"config,omitempty"`
	CountryCode             string  `json:"country_code,omitempty"`
	LanguageCode            string  `json:"language_code,omitempty"`
	Locale                  string  `json:"locale,omitempty"`
	Hostname                string  `json:"hostname,omitempty"`
	IsWhitelistedCrawlBot   bool    `json:"is_whitelisted_crawl_bot,omitempty"`
	ConnectionQualityRating string  `json:"connection_quality_rating,omitempty"`
	DeploymentStage         string  `json:"deployment_stage,omitempty"`
	Platform                string  `json:"platform,omitempty"`
	Nonce                   string  `json:"nonce,omitempty"`
	MidPct                  float64 `json:"mid_pct,omitempty"`
	CacheSchemaVersion      int     `json:"cache_schema_version,omitempty"`
	DeviceID                string  `json:"device_id,omitempty"`
	BrowserPushPubKey       string  `json:"browser_push_pub_key,omitempty"`
	Encryption              struct {
		KeyID     string `json:"key_id,omitempty"`
		PublicKey string `json:"public_key,omitempty"`
		Version   string `json:"version,omitempty"`
	} `json:"encryption,omitempty"`
	IsDev                  bool `json:"is_dev,omitempty"`
	IsOnVpn                bool `json:"is_on_vpn,omitempty"`
	SignalCollectionConfig struct {
		Bbs int `json:"bbs,omitempty"`
		Ctw any `json:"ctw,omitempty"`
		Dbs int `json:"dbs,omitempty"`
		Fd  int `json:"fd,omitempty"`
		Hbc struct {
			Hbbi  int    `json:"hbbi,omitempty"`
			Hbcbc int    `json:"hbcbc,omitempty"`
			Hbi   int    `json:"hbi,omitempty"`
			Hbv   string `json:"hbv,omitempty"`
			Hbvbc int    `json:"hbvbc,omitempty"`
		} `json:"hbc,omitempty"`
		I   int `json:"i,omitempty"`
		Rt  any `json:"rt,omitempty"`
		Sbs int `json:"sbs,omitempty"`
		Sc  struct {
			C [][]int `json:"c,omitempty"`
			T int     `json:"t,omitempty"`
		} `json:"sc,omitempty"`
		Sid int `json:"sid,omitempty"`
	} `json:"signal_collection_config,omitempty"`
	ConsentDialogConfig struct {
		IsUserLinkedToFb        bool `json:"is_user_linked_to_fb,omitempty"`
		ShouldShowConsentDialog bool `json:"should_show_consent_dialog,omitempty"`
	} `json:"consent_dialog_config,omitempty"`
	PrivacyFlowTrigger any `json:"privacy_flow_trigger,omitempty"`
	WwwRoutingConfig   struct {
		FrontendAndProxygenRoutes []struct {
			Path        string `json:"path,omitempty"`
			Destination string `json:"destination,omitempty"`
		} `json:"frontend_and_proxygen_routes,omitempty"`
		FrontendOnlyRoutes []struct {
			Path        string `json:"path,omitempty"`
			Destination string `json:"destination,omitempty"`
		} `json:"frontend_only_routes,omitempty"`
		ProxygenRequestHandlerOnlyRoutes []struct {
			Paths           []string `json:"paths,omitempty"`
			Destination     string   `json:"destination,omitempty"`
			InVpnDogfooding bool     `json:"in_vpn_dogfooding,omitempty"`
			InQe            bool     `json:"in_qe,omitempty"`
		} `json:"proxygen_request_handler_only_routes,omitempty"`
	} `json:"www_routing_config,omitempty"`
	ShouldShowDigitalCollectiblesPrivacyNotice bool   `json:"should_show_digital_collectibles_privacy_notice,omitempty"`
	RolloutHash                                string `json:"rollout_hash,omitempty"`
	BundleVariant                              string `json:"bundle_variant,omitempty"`
	FrontendEnv                                string `json:"frontend_env,omitempty"`
}
