package bloks

import (
	"encoding/json"
	"reflect"
)

// Messenger iOS 544.0.0.20.406 of 1768247148
const BloksVersion = "7f577336851f32ef4842b8eb2394aaf9d036c1dda7c1064b3f3090b6212b63e5"

type ExtraStringification[T any] struct {
	Body T
}

func (extra *ExtraStringification[T]) UnmarshalJSON(data []byte) error {
	var first string
	err := json.Unmarshal(data, &first)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(first), extra.Body)
}

func (extra *ExtraStringification[T]) MarshalJSON() ([]byte, error) {
	first, err := json.Marshal(&extra.Body)
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(first))
}

type BloksParamsInner map[string]any

type BloksBkContext struct {
	PixelRatio   float64 `json:"pixel_ratio"`
	BloksVersion string  `json:"bloks_version"`
}

func NewBkContext() *BloksBkContext {
	return &BloksBkContext{
		PixelRatio:   3,
		BloksVersion: BloksVersion,
	}
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
	Params    *BloksParamsOuter `json:"params,omitempty"`
}

func NewBloksRequest(appID string, inner BloksParamsInner) *BloksRequestOuter {
	return &BloksRequestOuter{
		BkContext: NewBkContext(),
		Params: &BloksParamsOuter{
			BloksVersioningId: BloksVersion,
			AppID:             appID,
			Params: ExtraStringification[BloksParamsMiddle]{BloksParamsMiddle{
				Params: ExtraStringification[BloksParamsInner]{inner},
			}},
		},
	}
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
	BloksAction *BloksActionData `json:"1$bloks_action(bk_context:$bk_context,params:$params)"`
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
	Tree string `json:"bloks_bundle_tree"` // BloksLayout
}

type BloksActionData struct {
	Action BloksAction `json:"action"`
}

type BloksAction struct {
	Bundle BloksActionBundle `json:"action_bundle"`
}

type BloksActionBundle struct {
	BundleAction string `json:"bloks_bundle_action"` // BloksLayout
}
