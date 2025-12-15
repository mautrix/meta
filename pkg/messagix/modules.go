package messagix

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	badGlobalLog "github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"

	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func (m *ModuleParser) HandleRawJSON(data []byte, id string) error {
	var err error
	switch id {
	case "envjson":
		var d types.EnvJSON
		err = json.Unmarshal(data, &d)
		if err != nil {
			return fmt.Errorf("failed to parse data from types.EnvJSON: %w", err)
		}
		m.client.configs.RoutingNamespace = d.RoutingNamespace
	case "__eqmc":
		var d types.Eqmc
		err = json.Unmarshal(data, &d)
		if err != nil {
			return fmt.Errorf("failed to parse data from types.Eqmc: %w", err)
		}
		ajaxData, err := d.ParseAjaxURLData()
		if err != nil {
			return fmt.Errorf("failed to parse ajax url data from types.Eqmc: %w", err)
		}
		m.client.configs.Jazoest = ajaxData.Jazoest
		m.client.configs.CometReq = ajaxData.CometReq
	}
	return nil
}

func (m *ModuleParser) handleConfigData(configData *ModuleEntry, reflectedMs reflect.Value) error {
	if len(configData.Data) < 3 {
		return nil
	}

	var configID int
	if err := json.Unmarshal(configData.Data[2], &configID); err != nil {
		return fmt.Errorf("failed to parse config id from config data %s: %w", configData.Name, err)
	}
	if configID <= 0 {
		return nil
	}

	m.client.configs.Bitmap.BMap = append(m.client.configs.Bitmap.BMap, configID)
	field := reflectedMs.FieldByName(configData.Name)
	if !field.IsValid() || !field.CanSet() {
		return nil
	}
	err := json.Unmarshal(configData.Data[1], field.Addr().Interface())
	if err != nil {
		return fmt.Errorf("failed to parse config data %s: %w", configData.Name, err)
	}
	return nil
}

type RPSCBBox struct {
	Complete bool            `json:"complete"`
	Result   json.RawMessage `json:"result"`
}

type RPSCBBoxContainer struct {
	BBox *RPSCBBox `json:"__bbox"`
}

type RelayPrefetchedStreamCache struct {
	PreloadID string
	BBox      *RPSCBBoxContainer
}

func (rpsc *RelayPrefetchedStreamCache) UnmarshalJSON(data []byte) error {
	var d []json.RawMessage
	err := json.Unmarshal(data, &d)
	if err != nil {
		return fmt.Errorf("failed to unmarshal RelayPrefetchedStreamCache array: %w", err)
	} else if len(d) < 2 {
		return fmt.Errorf("RelayPrefetchedStreamCache array has less than 2 elements")
	}
	err = json.Unmarshal(d[0], &rpsc.PreloadID)
	if err != nil {
		return fmt.Errorf("failed to unmarshal PreloadID for RelayPrefetchedStreamCache: %w", err)
	}
	err = json.Unmarshal(d[1], &rpsc.BBox)
	if err != nil {
		return fmt.Errorf("failed to unmarshal BBox for RelayPrefetchedStreamCache: %w", err)
	}
	return nil
}

func (m *ModuleParser) handleRequire(data *ModuleEntry) error {
	if strings.HasPrefix(data.Name, "CometPlatformRootClient@") {
		data.Name = "CometPlatformRootClient"
	}
	switch data.Name {
	case "CometPlatformRootClient":
		var cometType string
		if err := json.Unmarshal(data.Data[0], &cometType); err != nil {
			return fmt.Errorf("failed to parse comet type from CometPlatformRootClient: %w", err)
		}
		var expectedPreloaders json.RawMessage
		if cometType == "init" {
			var innerData []json.RawMessage
			if err := json.Unmarshal(data.Data[2], &innerData); err != nil {
				return fmt.Errorf("failed to parse inner array from CometPlatformRootClient: %w", err)
			} else if len(innerData) < 5 {
				return fmt.Errorf("inner array from CometPlatformRootClient init has less than 5 elements")
			}
			expectedPreloaders = innerData[4]
		} else if cometType == "initialize" {
			expectedPreloadersRes := gjson.GetBytes(data.Data[2], "0.expectedPreloaders")
			if !expectedPreloadersRes.IsArray() {
				m.client.Logger.Trace().
					Str("comet_type", cometType).
					Bytes("comet_data", data.Data[2]).
					Msg("Unsupported comet data: expectedPreloaders not found in CometPlatformRootClient initialize")
				return nil
			}
			if expectedPreloadersRes.Index > 0 {
				expectedPreloaders = data.Data[2][expectedPreloadersRes.Index : expectedPreloadersRes.Index+len(expectedPreloadersRes.Raw)]
			} else {
				expectedPreloaders = json.RawMessage(expectedPreloadersRes.Raw)
			}
		} else {
			m.client.Logger.Trace().
				Str("comet_type", cometType).
				Bytes("comet_data", data.Data[2]).
				Msg("Unsupported comet data")
			return nil
		}
		var requests []*graphql.GraphQLPreloader
		if err := json.Unmarshal(expectedPreloaders, &requests); err != nil {
			return fmt.Errorf("failed to parse graphql preload requests from CometPlatformRootClient: %w", err)
		}
		for _, req := range requests {
			if !strings.HasPrefix(req.PreloaderID, "adp_LSPlatformGraphQLLightspeedRequest") {
				continue
			}
			vars, err := req.ParseVariables()
			if err != nil {
				return fmt.Errorf("failed to parse graphql lightspeed preload request variables: %w", err)
			}
			var syncData *graphql.LSPlatformGraphQLLightspeedVariables
			err = json.Unmarshal([]byte(vars.RequestPayload), &syncData)
			if err != nil {
				return fmt.Errorf("failed to parse graphql lightspeed preload request payload: %w", err)
			}
			m.client.configs.VersionID = syncData.Version
			m.client.Logger.Debug().Int64("ls_version", m.client.configs.VersionID).Msg("Found LSVersion in SSJS")
			break
		}
	case "RelayPrefetchedStreamCache":
		if len(data.Data) < 3 {
			return nil
		}
		var rpsc RelayPrefetchedStreamCache
		if err := json.Unmarshal(data.Data[2], &rpsc); err != nil {
			return err
		}
		return m.handleLightSpeedQLRequest(rpsc.BBox.BBox.Result, m.parseGraphMethodName(rpsc.PreloadID))
	}
	return nil
}

func (m *ModuleParser) handleLightSpeedQLRequest(data json.RawMessage, parserFunc string) error {
	var lsPayloadStr string
	var deps lightspeed.DependencyList
	switch parserFunc {
	case "LSPlatformGraphQLLightspeedRequestForIGDQuery":
		var lsData *graphql.LSPlatformGraphQLLightspeedRequestQuery
		err := json.Unmarshal(data, &lsData)
		if err != nil {
			return fmt.Errorf("messagix-moduleparser: failed to parse LightSpeedQLRequest data from html (INSTAGRAM): %w", err)
		}
		if lsData.Data.LightspeedWebRequestForIG == nil {
			return nil
		}
		lsPayloadStr = lsData.Data.LightspeedWebRequestForIG.Payload
		deps = lsData.Data.LightspeedWebRequestForIG.Dependencies
	case "LSPlatformGraphQLLightspeedRequestQuery":
		var lsData *graphql.LSPlatformGraphQLLightspeedRequestQuery
		err := json.Unmarshal(data, &lsData)
		if err != nil {
			return fmt.Errorf("messagix-moduleparser: failed to parse LightSpeedQLRequest data from html (FACEBOOK): %w", err)
		}
		if lsData.Data.Viewer.LightspeedWebRequest == nil {
			return nil
		}
		lsPayloadStr = lsData.Data.Viewer.LightspeedWebRequest.Payload
		deps = lsData.Data.Viewer.LightspeedWebRequest.Dependencies
	default:
		//m.handleGraphQLData(parserFunc, data)
		return nil
	}

	if lsPayloadStr == "" { // skip cuz payload null
		return nil
	}

	var payload lightspeed.LightSpeedData
	err := json.Unmarshal([]byte(lsPayloadStr), &payload)
	if err != nil {
		return fmt.Errorf("messagix-moduleparser: failed to marshal lsPayloadStr into LightSpeedData: %w", err)
	}

	decoder := lightspeed.NewLightSpeedDecoder(deps.ToMap(), m.LS)
	decoder.Decode(payload.Steps)
	return nil
}

//lint:ignore U1000 lots of unused code
func (m *ModuleParser) handleGraphQLData(name string, data json.RawMessage) {
	reflectedMs := reflect.ValueOf(m.client.configs.graphqlConfigTable).Elem()
	dataField := reflectedMs.FieldByName(name)
	if !dataField.IsValid() {
		badGlobalLog.Error().Str("field_name", name).Msg("Not handling GraphQLData for operation")
		return
	}

	definition := dataField.Type().Elem()
	newDefinition := reflect.New(definition).Interface()

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		badGlobalLog.Err(err).Msg("failed to marshal GraphQL operation data")
		return
	}

	err = json.Unmarshal(jsonBytes, newDefinition)
	if err != nil {
		badGlobalLog.Err(err).Msg("failed to unmarshal GraphQL operation data")
		return
	}

	newSlice := reflect.Append(dataField, reflect.Indirect(reflect.ValueOf(newDefinition)))
	dataField.Set(newSlice)
}

func (m *ModuleParser) parseGraphMethodName(name string) string {
	var s string
	s = strings.Replace(name, "adp_", "", -1)
	s = strings.Split(s, "RelayPreloader_")[0]
	return s
}

func (m *ModuleParser) SSJSHandle(data json.RawMessage) error {
	if data[0] == '[' {
		var entries []*ModuleEntry
		err := json.Unmarshal(data, &entries)
		if err != nil {
			if data[1] == '"' {
				return nil
			}
			return fmt.Errorf("failed to unmarshal ssjs data into default_define module entries: %w", err)
		}
		reflectedConfigTable := reflect.ValueOf(m.client.configs.BrowserConfigTable).Elem()
		for _, entry := range entries {
			err = m.handleConfigData(entry, reflectedConfigTable)
			if err != nil {
				return fmt.Errorf("error handling ssjs default define: %w", err)
			}
		}
		return nil
	}
	var bboxContainer BBoxContainer
	err := json.Unmarshal(data, &bboxContainer)
	if err != nil {
		m.client.Logger.Trace().
			Str("ssjs_content", base64.StdEncoding.EncodeToString(data)).
			Msg("Errored ssjs data")
		return fmt.Errorf("failed to unmarshal ssjs data into bbox container: %w", err)
	} else if bboxContainer.BBox == nil {
		return nil
	}
	reflectedConfigTable := reflect.ValueOf(m.client.configs.BrowserConfigTable).Elem()
	for _, req := range bboxContainer.BBox.Require {
		err = m.handleRequire(req)
		if err != nil {
			return fmt.Errorf("error handling ssjs require: %w", err)
		}
	}
	for _, def := range bboxContainer.BBox.Define {
		err = m.handleConfigData(def, reflectedConfigTable)
		if err != nil {
			return fmt.Errorf("error handling ssjs define: %w", err)
		}
	}
	return nil
}

func (m *ModuleParser) HandleBootloaderPayload(payload json.RawMessage, bootloaderConfig *types.BootLoaderConfig) error {
	var datas []*types.Bootloader_HandlePayload
	err := json.Unmarshal(payload, &datas)
	if err != nil {
		return err
	}

	for _, data := range datas {
		if data.CsrUpgrade != "" {
			newBits, err := m.parseCSRBit(data.CsrUpgrade)
			if err != nil {
				return err
			}
			m.client.configs.CSRBitmap.BMap = append(m.client.configs.CSRBitmap.BMap, newBits...)
		}

		if len(data.RsrcMap) > 0 {
			for _, v := range data.RsrcMap {
				shouldAdd := (bootloaderConfig.PhdOn && v.C == 2) || (!bootloaderConfig.PhdOn && v.C != 0)
				if shouldAdd {
					newBits, err := m.parseCSRBit(v.P)
					if err != nil {
						return err
					}
					m.client.configs.CSRBitmap.BMap = append(m.client.configs.CSRBitmap.BMap, newBits...)
				}
			}
		}
	}

	return nil
}

// s always start with :
func (m *ModuleParser) parseCSRBit(s string) ([]int, error) {
	bits := make([]int, 0)
	splitUp := strings.Split(s[1:], ",")
	for _, b := range splitUp {
		conv, err := strconv.ParseInt(b, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("messagix-moduleparser: failed to parse csrbit: %w", err)
		}
		if conv == 0 {
			continue
		}
		bits = append(bits, int(conv))
	}
	return bits, nil
}
