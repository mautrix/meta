package messagix

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"

	"github.com/0xzer/messagix/graphql"
	"github.com/0xzer/messagix/lightspeed"
	"github.com/0xzer/messagix/methods"
	"github.com/0xzer/messagix/types"
)


var GraphQLData = &graphql.GraphQLTable{}
var VersionId int64

func (m *ModuleParser) HandleRawJSON(data []byte, id string) error {
	var err error
	switch id {
	case "envjson":
		var d types.EnvJSON
		err = json.Unmarshal(data, &d)
		if err != nil {
			return fmt.Errorf("failed to parse data from types.EnvJSON: %e", err)
		}
	case "__eqmc":
		var d types.Eqmc
		err = json.Unmarshal(data, &d)
		if err != nil {
			return fmt.Errorf("failed to parse data from types.Eqmc: %e", err)
		}
		ajaxData, err := d.ParseAjaxURLData()
		if err != nil {
			return fmt.Errorf("failed to parse ajax url data from types.Eqmc: %e", err)
		}
		m.client.configs.Jazoest = ajaxData.Jazoest
		m.client.configs.CometReq = ajaxData.CometReq
	}
	return nil
}


func (m *ModuleParser) handleConfigData(configData []interface{}, reflectedMs reflect.Value) error {
	if len(configData) < 4 {
		return nil
	}
	
	configName := configData[0].(string)
	config := configData[2]
	configId := int(configData[3].(float64))

	if configId <= 0 {
		return nil
	}

	m.client.configs.Bitmap.BMap = append(m.client.configs.Bitmap.BMap, configId)
	field := reflectedMs.FieldByName(configName)
	if !field.IsValid() {
		//fmt.Printf("Invalid field name: %s\n", configName)
		return nil
	}
	if !field.CanSet() {
		//fmt.Printf("Unsettable field: %s\n", configName)
		return nil
	}

	err := methods.InterfaceToStructJSON(config, field.Addr().Interface())
	if err != nil {
		return err
	}

	return nil
}

func (m *ModuleParser) handleRequire(modName string, data []interface{}) error {
	switch modName {
		case "ssjs":
			//reflectedMs := reflect.ValueOf(&SchedulerJSRequired).Elem()
			for _, requireData := range data {
				d := requireData.([]interface{})
				requireType := d[0].(string)
				switch requireType {
					case "CometPlatformRootClient":
						moduleData := d[3].([]interface{})
						for _, v := range moduleData {
							requestsMap, ok := v.([]interface{})
							if !ok {
								continue
							}
							for _, req := range requestsMap {
								var reqData *graphql.GraphQLPreloader
								err := methods.InterfaceToStructJSON(req, &reqData)
								if err != nil {
									continue
								}
								if len(reqData.Variables.RequestPayload) > 0 {
									var syncData *graphql.LSPlatformGraphQLLightspeedVariables
									err = json.Unmarshal([]byte(reqData.Variables.RequestPayload), &syncData)
									if err != nil {
										continue
									}
									VersionId = syncData.Version
								}
							}

						}
					case "RelayPrefetchedStreamCache":
						moduleData := d[3].([]interface{})
						//method := d[1].(string)
						//dependencies := d[2].(string)
						parserFunc := m.parseGraphMethodName(moduleData[0].(string))
						graphQLData := moduleData[1].(map[string]interface{})
						boxData, ok := graphQLData["__bbox"].(map[string]interface{})
						if !ok {
							return fmt.Errorf("could not find __bbox in graphQLData map for parser func: %s", parserFunc)
						}

						result, ok := boxData["result"]
						if !ok {
							return fmt.Errorf("could not find result in __bbox for parser func: %s", parserFunc)
						}

						if parserFunc == "LSPlatformGraphQLLightspeedRequestQuery" || parserFunc == "LSPlatformGraphQLLightspeedRequestForIGDQuery" {
							m.handleLightSpeedQLRequest(result, parserFunc)
						} else {
							m.handleGraphQLData(parserFunc, result)
						}
				}
			}
		}
	return nil
}

func (m *ModuleParser) handleLightSpeedQLRequest(data interface{}, parserFunc string) error {
	var lsPayloadStr string
	var deps interface{}
	switch parserFunc {
	case "LSPlatformGraphQLLightspeedRequestForIGDQuery":
		var lsData *graphql.LSPlatformGraphQLLightspeedRequestForIGDQuery
		err := methods.InterfaceToStructJSON(&data, &lsData)
		if err != nil {
			return fmt.Errorf("messagix-moduleparser: failed to parse LightSpeedQLRequest data from html (INSTAGRAM): %e", err)
		}
		lsPayloadStr = lsData.Data.LightspeedWebRequestForIgd.Payload
		deps = lsData.Data.LightspeedWebRequestForIgd.Dependencies
	case "LSPlatformGraphQLLightspeedRequestQuery":
		var lsData *graphql.LSPlatformGraphQLLightspeedRequestQuery
		err := methods.InterfaceToStructJSON(&data, &lsData)
		if err != nil {
			return fmt.Errorf("messagix-moduleparser: failed to parse LightSpeedQLRequest data from html (FACEBOOK): %e", err)
		}
		lsPayloadStr = lsData.Data.Viewer.LightspeedWebRequest.Payload
		deps = lsData.Data.Viewer.LightspeedWebRequest.Dependencies
	}
	
	if lsPayloadStr == "" { // skip cuz payload null
		return nil
	}

	dependencies := lightspeed.DependenciesToMap(deps)
	decoder := lightspeed.NewLightSpeedDecoder(dependencies, m.client.configs.accountConfigTable)
	
	var payload lightspeed.LightSpeedData
	err := json.Unmarshal([]byte(lsPayloadStr), &payload)
	if err != nil {
		return fmt.Errorf("messagix-moduleparser: failed to marshal lsPayloadStr into LightSpeedData: %e", err)
	}
	
	decoder.Decode(payload.Steps)
	return nil
}

func (m *ModuleParser) handleGraphQLData(name string, data interface{}) {
	reflectedMs := reflect.ValueOf(GraphQLData).Elem()
	dataField := reflectedMs.FieldByName(name)
	if !dataField.IsValid() {
		log.Println("Not handling GraphQLData for operation:", name)
		return
	}
	
	definition := dataField.Type().Elem()
	newDefinition := reflect.New(definition).Interface()

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		log.Println(fmt.Sprintf("failed to marshal GraphQL operation data %s", name))
		return
	}

	err = json.Unmarshal(jsonBytes, newDefinition)
	if err != nil {
		log.Println(fmt.Sprintf("failed to unmarshal GraphQL operation data %s", name))
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

func (m *ModuleParser) SSJSHandle(data interface{}) error {
	var err error
	box, ok := data.(map[string]interface{})
	if !ok {
		interfaceData, ok := data.([]interface{})
		if ok {
			err = m.handleDefine("default_define", interfaceData)
			return err
		}
		return fmt.Errorf("messagix-moduleparser: failed to convert ssjs data to map[string]interface{}")
	}

	for k, v := range box {
		if v == nil {
			continue
		}
		switch k {
			case "__bbox":
				boxMap := v.(map[string]interface{})
				for boxKey, boxData := range boxMap {
					boxDataArr := boxData.([]interface{})
					switch boxKey {
					case "require":
						err = m.handleRequire("ssjs", boxDataArr)
						continue
					case "define":
						err = m.handleDefine("ssjs", boxDataArr)
					}
				}
		}
	}
	return err
}

func (m *ModuleParser) handleDefine(modName string, data []interface{}) error {
	reflectedMs := reflect.ValueOf(m.client.configs.browserConfigTable).Elem()
	switch modName {
		case "ssjs":
			for _, child := range data {
				configData := child.([]interface{})
				err := m.handleConfigData(configData, reflectedMs)
				if err != nil {
					return err
				}
			}
		case "default_define":
			err := m.handleConfigData(data, reflectedMs)
			if err != nil {
				return err
			}
		}
		return nil
}

func (m *ModuleParser) Bootloader_HandlePayload(payload interface{}, bootloaderConfig *types.BootLoaderConfig) error {
	var data *types.Bootloader_HandlePayload
	err := methods.InterfaceToStructJSON(&payload, &data)
	if err != nil {
		return err
	}

	if data.CsrUpgrade != "" {
		newBits, err := m.parseCSRBit(data.CsrUpgrade)
		if err != nil {
			return err
		}
		m.client.configs.CsrBitmap.BMap = append(m.client.configs.CsrBitmap.BMap, newBits...)
	}

	if len(data.RsrcMap) > 0 {
		for _, v := range data.RsrcMap {
			shouldAdd := (bootloaderConfig.PhdOn && v.C == 2) || (!bootloaderConfig.PhdOn && v.C != 0)
			if shouldAdd {
				newBits, err := m.parseCSRBit(v.P)
				if err != nil {
					return err
				}
				m.client.configs.CsrBitmap.BMap = append(m.client.configs.CsrBitmap.BMap, newBits...)
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
			return nil, fmt.Errorf("messagix-moduleparser: failed to parse csrbit: %e", err)
		}
		if conv == 0 {
			continue
		}
		bits = append(bits, int(conv))
	}
	return bits, nil
}