package lightspeed

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	badGlobalLog "github.com/rs/zerolog/log"
)

type LightSpeedDecoder struct {
	Table               interface{} // struct that contains pointers to all the dependencies/stores
	Dependencies        map[string]string
	StatementReferences map[int]interface{}
}

func NewLightSpeedDecoder(dependencies map[string]string, table interface{}) *LightSpeedDecoder {
	if reflect.ValueOf(table).Kind() != reflect.Ptr {
		return nil
	}
	return &LightSpeedDecoder{
		Table:               table,
		Dependencies:        dependencies,
		StatementReferences: make(map[int]interface{}),
	}
}

// pass the LightSpeedData.Steps to this function
func (ls *LightSpeedDecoder) Decode(data interface{}) interface{} {
	s, ok := data.([]interface{})
	if !ok {
		return data
	}

	stepType := StepType(int(s[0].(float64)))
	stepData := s[1:]
	switch stepType {
	case BLOCK:
		for _, blockData := range stepData {
			stepDataArr := blockData.([]interface{})
			ls.Decode(stepDataArr)
		}
	case LOAD:
		key, ok := stepData[0].(float64)
		if !ok {
			badGlobalLog.Warn().Msg("[LOAD] failed to store key to float64")
			return 0
		}

		shouldLoad, ok := ls.StatementReferences[int(key)]
		if !ok {
			badGlobalLog.Warn().Float64("key", key).Msg("[LOAD] failed to fetch statement reference for key")
			return 0
		}
		return shouldLoad
	case STORE:
		retVal := ls.Decode(stepData[1])
		ls.StatementReferences[int(stepData[0].(float64))] = retVal
	case STORE_ARRAY:
		key, ok := stepData[0].(float64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Bad step data in STORE_ARRAY")
			os.Exit(1)
		}

		shouldStore, ok := stepData[1].(float64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Bad step data in STORE_ARRAY")
			os.Exit(1)
		}

		ls.StatementReferences[int(key)] = int64(shouldStore)
		ls.Decode(s[2:])
	case CALL_STORED_PROCEDURE:
		referenceName := stepData[0].(string)
		ls.handleStoredProcedure(referenceName, stepData[1:])
	case UNDEFINED:
		return nil
	case I64_FROM_STRING:
		i64, err := strconv.ParseInt(stepData[0].(string), 10, 64)
		if err != nil {
			badGlobalLog.Err(err).Any("input_data", stepData[0]).Msg("[I64_FROM_STRING] failed to convert string to int64")
			return 0
		}
		return i64
	case IF:
		statement := stepData[0]
		result := ls.Decode(statement).(int64)
		if result > 0 {
			ls.Decode(stepData[1])
		} else if len(stepData) >= 3 {
			if stepData[2] != nil {
				ls.Decode(stepData[2])
			}
		}
	case NOT:
		return ls.Decode(stepData[0]).(int64)
	case NATIVE_OP_CURRENT_TIME:
		return time.Now().UnixMilli()
	case CALL_NATIVE_OPERATION:
		badGlobalLog.Warn().Any("step_data", stepData).Msg("Call native operation")
		return nil
	case NATIVE_OP_MAP_CREATE:
		return make(map[interface{}]interface{}, 0)
	case NATIVE_OP_MAP_SET:
		mapToUpdate, ok := ls.Decode(stepData[0]).(map[interface{}]interface{})
		if !ok {
			badGlobalLog.Warn().Msg("failed to type assert map from statement references...")
			return nil
		}
		mapKey := ls.Decode(stepData[1])
		mapVal := ls.Decode(stepData[2])
		mapToUpdate[mapKey] = mapVal
	case LOGGER_LOG:
		badGlobalLog.Debug().Msgf("Facebook server log: %v", stepData[0]) // zerolog-allow-msgf
		return nil
	case I64_ADD:
		first := ls.Decode(stepData[0]).(int64)
		second := ls.Decode(stepData[1]).(int64)
		return first + second
	default:
		badGlobalLog.Warn().Int("step_type", int(stepType)).Any("step_data", stepData).Msg("Got unknown step type")
		os.Exit(1)
	}

	return nil
}

func (ls *LightSpeedDecoder) handleStoredProcedure(referenceName string, data []interface{}) {
	depReference, ok := ls.Dependencies[referenceName]
	if !ok {
		badGlobalLog.Warn().
			Str("reference_name", referenceName).
			Any("data", data).
			Msg("Skipping dependency with reference name (unknown dependency)")
		return
	}

	reflectedMs := reflect.ValueOf(ls.Table).Elem()
	//badGlobalLog.Println(depReference)
	depField := reflectedMs.FieldByName(depReference)

	if !depField.IsValid() {
		badGlobalLog.Warn().
			Str("reference_name", referenceName).
			Any("data", data).
			Msg("Skipping dependency with reference name (invalid field)")
		return
	}

	var err error

	depFieldsType := depField.Type().Elem().Elem()
	newDepInstancePtr := reflect.New(depFieldsType)
	newDepInstance := newDepInstancePtr.Elem()
	decodedData := make([]any, len(data))
	for i, d := range data {
		decodedData[i] = ls.Decode(d)
	}
	for i := 0; i < depFieldsType.NumField(); i++ {
		fieldInfo := depFieldsType.Field(i)
		if fieldInfo.Name == "Unrecognized" {
			continue
		}
		var index int
		conditionField := fieldInfo.Tag.Get("conditionField")
		if conditionField != "" {
			indexChoices := fieldInfo.Tag.Get("indexes")
			conditionVal := newDepInstance.FieldByName(conditionField)
			index, err = ls.parseConditionIndex(conditionVal.Bool(), indexChoices)
			if err != nil {
				badGlobalLog.Warn().Str("struct_name", depFieldsType.Name()).Str("field_name", fieldInfo.Name).Msg("Failed to parse condition index")
				continue
			}
		} else {
			index, _ = strconv.Atoi(fieldInfo.Tag.Get("index"))
		}

		if index >= len(data) {
			badGlobalLog.Warn().
				Int("data_length", len(data)).
				Str("struct_name", depFieldsType.Name()).
				Msg("Struct has more fields than the data slice")
			break
		}

		kind := fieldInfo.Type.Kind()
		val := decodedData[index]
		if val == nil { // skip setting field, because the index in the array was [9] which is undefined.
			continue
		}

		valType := reflect.TypeOf(val)
		switch kind {
		case reflect.Int64:
			i64, ok := val.(int64)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", valType).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set int64")
				continue
			}
			newDepInstance.Field(i).SetInt(i64)
		case reflect.String:
			str, ok := val.(string)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", valType).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set string")
				continue
			}
			newDepInstance.Field(i).SetString(str)
		case reflect.Interface:
			if val == nil {
				continue
			}
			newDepInstance.Field(i).Set(reflect.ValueOf(val))
		case reflect.Bool:
			boolean, ok := val.(bool)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", valType).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set bool")
				continue
			}
			newDepInstance.Field(i).SetBool(boolean)
		case reflect.Int:
			integer, ok := val.(int)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", valType).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set int")
				continue
			}
			newDepInstance.Field(i).SetInt(int64(integer))
		case reflect.Float64:
			floatVal, ok := val.(float64)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", valType).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set float64")
				continue
			}
			newDepInstance.Field(i).SetFloat(floatVal)
		default:
			badGlobalLog.Warn().Stringer("kind", kind).Any("val", val).Type("val_type", val).Msg("Unknown kind")
			os.Exit(1)
		}
		decodedData[index] = nil
	}
	for i, item := range decodedData {
		if item != nil {
			unrec := newDepInstance.FieldByName("Unrecognized")
			if unrec.IsValid() {
				if unrec.IsNil() {
					unrec.Set(reflect.MakeMap(reflect.TypeOf(make(map[int]any))))
				}
				unrecMap := unrec.Interface().(map[int]any)
				unrecMap[i] = item
			} else {
				badGlobalLog.Warn().Str("struct_name", depFieldsType.Name()).Int("index", i).Any("item", item).Type("item_type", item).Msg("Found unknown non-nil field")
			}
		}
	}
	newSlice := reflect.Append(depField, newDepInstancePtr)
	depField.Set(newSlice)
}

// conditionVal ? trueIndex : falseIndex
func (ls *LightSpeedDecoder) parseConditionIndex(val bool, choices string) (int, error) {
	indexes := strings.Split(choices, ",")
	var index int
	var err error
	if val {
		index, err = strconv.Atoi(indexes[0])
	} else {
		index, err = strconv.Atoi(indexes[1])
	}
	return index, err
}
