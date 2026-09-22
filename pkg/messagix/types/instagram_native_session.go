package types

type InstagramNativeSession struct {
	Authorization    string               `json:"authorization"`
	UserID           string               `json:"user_id"`
	Username         string               `json:"username,omitempty"`
	RUR              string               `json:"rur,omitempty"`
	SHBID            string               `json:"shbid,omitempty"`
	SHBTS            string               `json:"shbts,omitempty"`
	DirectRegionHint string               `json:"direct_region_hint,omitempty"`
	WWWClaim         string               `json:"www_claim,omitempty"`
	Device           InstagramLoginDevice `json:"device"`
}
