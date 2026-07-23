package bloks

import (
	"encoding/json"
	"fmt"
	"reflect"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// Messenger iOS 571.0.0.18.106 of 2026-07-22
const BloksVersionIOS = "7f577336851f32ef4842b8eb2394aaf9d036c1dda7c1064b3f3090b6212b63e5"

// Messenger Android 569.0.0.44.91 of 2026-07-22
const BloksVersionAndroid = "194a25b5ca64b7e2cc9a1b57a306ae5b1536d14d53e54f273f19a639c21cb197"

func GetBloksVersion(p types.Platform) (string, error) {
	switch p {
	case types.MessengerLiteIOS:
		return BloksVersionIOS, nil
	case types.MessengerLiteAndroid:
		return BloksVersionAndroid, nil
	default:
		return "", fmt.Errorf("platform %s does not have a bloks version", p.String())
	}
}

type ExtraStringification[T any] struct {
	Body T

	// Set this to skip the special behavior on marshal and
	// unmarshal, and instead treat the wrapper struct like it is not
	// present.
	SkipExtraStringification bool
}

func (extra *ExtraStringification[T]) UnmarshalJSON(data []byte) error {
	if !extra.SkipExtraStringification {
		var first string
		err := json.Unmarshal(data, &first)
		if err != nil {
			return err
		}
		data = []byte(first)
	}
	return json.Unmarshal(data, extra.Body)
}

func (extra *ExtraStringification[T]) MarshalJSON() ([]byte, error) {
	var obj any = &extra.Body
	if !extra.SkipExtraStringification {
		first, err := json.Marshal(&extra.Body)
		if err != nil {
			return nil, err
		}
		obj = string(first)
	}
	return json.Marshal(obj)
}

type BloksParamsInner map[string]any

type BloksBkContext struct {
	PixelRatio   float64 `json:"pixel_ratio"`
	BloksVersion string  `json:"bloks_version"`
}

type BloksNtContext struct {
	DebugToolingMetadataToken any                  `json:"debug_tooling_metadata_token"`
	IsFlipperEnabled          bool                 `json:"is_flipper_enabled"`
	ThemeParams               []BloksNtThemeParams `json:"theme_params"`
}

type BloksNtThemeParams struct {
	DesignSystemName string   `json:"design_system_name"`
	Value            []string `json:"value"`
}

type BloksParamsMiddle struct {
	Params ExtraStringification[BloksParamsInner] `json:"params"`
}

type BloksParamsOuter struct {
	BloksVersioningId string `json:"bloks_versioning_id"`
	AppID             string `json:"app_id"`

	Params ExtraStringification[BloksParamsMiddle] `json:"params"`
}

type BloksRequestOuter struct {
	BkContext *BloksBkContext   `json:"bk_context,omitempty"`
	NtContext *BloksNtContext   `json:"nt_context,omitempty"`
	Params    *BloksParamsOuter `json:"params,omitempty"`
	Scale     string            `json:"scale,omitempty"`
}

func NewBloksRequest(doc *BloksDoc, appID string, inner BloksParamsInner) *BloksRequestOuter {
	outer := &BloksRequestOuter{
		Params: &BloksParamsOuter{
			BloksVersioningId: doc.Version,
			AppID:             appID,
			Params: ExtraStringification[BloksParamsMiddle]{BloksParamsMiddle{
				Params: ExtraStringification[BloksParamsInner]{inner, false},
			}, false},
		},
	}
	if doc.UseNT {
		outer.NtContext = &BloksNtContext{
			DebugToolingMetadataToken: nil,
			IsFlipperEnabled:          false,
			ThemeParams: []BloksNtThemeParams{
				{
					DesignSystemName: "XMDS",
					Value:            []string{"three_neutral_gray"},
				},
				{
					DesignSystemName: "FDS",
					Value:            []string{},
				},
			},
		}
		outer.Scale = "3"
		outer.Params.Params.Body.Params.SkipExtraStringification = true
	} else {
		outer.BkContext = &BloksBkContext{
			PixelRatio:   3,
			BloksVersion: doc.Version,
		}
	}
	return outer
}

type BloksResponse struct {
	Data   BloksResponseData    `json:"data"`
	Errors []BloksResponseError `json:"errors"`
}

type BloksResponseError struct {
	Message        string   `json:"message"`
	Severity       string   `json:"severity"`
	MIDs           []string `json:"mids"`
	DebugLink      string   `json:"debug_link"`
	Code           int      `json:"code"`
	APIErrorCode   int      `json:"api_error_code"`
	Summary        string   `json:"summary"`
	Description    string   `json:"description"`
	DescriptionRaw string   `json:"description_raw"`
	IsSilent       bool     `json:"is_silent"`
	IsTransient    bool     `json:"is_transient"`
	IsNotCritical  bool     `json:"is_not_critical"`
	RequiresReauth bool     `json:"requires_reauth"`
	AllowUserRetry bool     `json:"allow_user_retry"`
	DebugInfo      any      `json:"debug_info"`
	QueryPath      any      `json:"query_path"`
	FBTraceID      string   `json:"fbtrace_id"`
	WWWRequestID   string   `json:"www_request_id"`
	Path           []string `json:"path"`
}

type BloksResponseData struct {
	//lint:ignore SA5008 handled with custom unmarshaler
	BloksApp *BloksAppData `json:"1$bloks_app(bk_context:$bk_context,params:$params)"`
	//lint:ignore SA5008 handled with custom unmarshaler
	BloksAppFB *BloksAppDataFB `json:"1$fb_bloks_app(nt_context:$nt_context,params:$params)"`
	//lint:ignore SA5008 handled with custom unmarshaler
	BloksAction *BloksActionData `json:"1$bloks_action(bk_context:$bk_context,params:$params)"`

	// normal shit
	BloksActionFB *BloksActionDataFB `json:"fb_bloks_action"`
}

// Workaround https://github.com/golang/go/issues/15000
// Could also upgrade to json/v2 and use single quotes in the struct
// tag, once that is released and stabilized
func (b *BloksResponseData) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(b)
	t := reflect.TypeOf(BloksResponseData{})
	for i := 0; i < t.NumField(); i++ {
		key := t.Field(i).Tag.Get("json")
		if raw[key] == nil {
			continue
		}
		err := json.Unmarshal(raw[key], v.Elem().Field(i).Addr().Interface())
		if err != nil {
			return err
		}
	}
	return nil
}

type BloksAppDataFB struct {
	RootComponent BloksComponent `json:"root_component"`
}

type BloksAppData struct {
	Screen BloksScreenContent `json:"screen_content"`
}

type BloksScreenContent struct {
	Component BloksComponent `json:"component"`
}

type BloksComponent struct {
	Bundle BloksAppBundle `json:"bundle"`
}

type BloksAppBundle struct {
	Tree string `json:"bloks_bundle_tree"` // BloksBundle struct
}

type BloksActionDataFB struct {
	RootAction BloksActionData `json:"root_action"`
}

type BloksActionData struct {
	Action BloksAction `json:"action"`
}

type BloksAction struct {
	Bundle BloksActionBundle `json:"action_bundle"`
}

type BloksActionBundle struct {
	BundleAction string `json:"bloks_bundle_action"` // BloksBundle struct
}
