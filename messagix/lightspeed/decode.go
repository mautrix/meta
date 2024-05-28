package lightspeed

import (
	"encoding/base64"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	badGlobalLog "github.com/rs/zerolog/log"
)

type LightSpeedDecoder struct {
	Table               any // struct that contains pointers to all the dependencies/stores
	Dependencies        DependencyMap
	StatementReferences map[int]any
}

func NewLightSpeedDecoder(dependencies DependencyMap, table any) *LightSpeedDecoder {
	if reflect.ValueOf(table).Kind() != reflect.Ptr {
		return nil
	}
	return &LightSpeedDecoder{
		Table:               table,
		Dependencies:        dependencies,
		StatementReferences: make(map[int]any),
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
			stepDataArr, ok := blockData.([]interface{})
			if !ok {
				badGlobalLog.Warn().Any("block_data", blockData).Msg("Failed to decode block data")
				continue
			}
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
		referenceName, ok := stepData[0].(string)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in CALL_STORED_PROCEDURE (expected string)")
			return nil
		}
		ls.handleStoredProcedure(referenceName, stepData[1:])
	case UNDEFINED:
		return nil
	case I64_FROM_STRING:
		strVal, ok := stepData[0].(string)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in I64_FROM_STRING (expected string)")
			return nil
		}
		i64, err := strconv.ParseInt(strVal, 10, 64)
		if err != nil {
			badGlobalLog.Err(err).Any("input_data", stepData[0]).Msg("[I64_FROM_STRING] failed to convert string to int64")
			return 0
		}
		return i64
	case IF:
		statement := stepData[0]
		result, ok := ls.Decode(statement).(int64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Failed to decode statement in IF")
			return nil
		}
		if result > 0 {
			ls.Decode(stepData[1])
		} else if len(stepData) >= 3 {
			if stepData[2] != nil {
				ls.Decode(stepData[2])
			}
		}
	case NOT:
		val, ok := ls.Decode(stepData[0]).(int64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in NOT (expected int64)")
			return nil
		}
		// TODO why is this just returning the value?
		return val
	case NATIVE_OP_CURRENT_TIME:
		return time.Now().UnixMilli()
	case CALL_NATIVE_OPERATION:
		badGlobalLog.Warn().Any("step_data", stepData).Msg("Call native operation")
		return nil
	case NATIVE_OP_MAP_CREATE:
		return make(map[string]interface{}, 0)
	case NATIVE_OP_MAP_SET:
		mapToUpdate, ok := ls.Decode(stepData[0]).(map[string]interface{})
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in NATIVE_OP_MAP_SET (expected map)")
			return nil
		}
		mapKey := ls.Decode(stepData[1])
		mapVal := ls.Decode(stepData[2])
		mapKeyStr, ok := mapKey.(string)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Map set contains non-string key")
			mapKeyStr = fmt.Sprintf("%v", mapKey)
		}
		mapToUpdate[mapKeyStr] = mapVal
	case NATIVE_OP_ARRAY_CREATE:
		return make([]any, 0)
	case NATIVE_OP_ARRAY_APPEND:
		decodedArr, ok := ls.Decode(stepData[0]).([]any)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in NATIVE_OP_ARRAY_APPEND (expected array)")
			return nil
		}
		return append(decodedArr, ls.Decode(stepData[1]))
	case NATIVE_OP_ARRAY_GET_SIZE:
		decodedArr, ok := ls.Decode(stepData[0]).([]any)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in NATIVE_OP_ARRAY_GET_SIZE (expected array)")
			return nil
		}
		return len(decodedArr)
	case LOGGER_LOG:
		badGlobalLog.Debug().Msgf("Facebook server log: %v", stepData[0]) // zerolog-allow-msgf
		return nil
	case I64_ADD:
		first, ok := ls.Decode(stepData[0]).(int64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in I64_ADD (expected int64)")
			return nil
		}
		second, ok := ls.Decode(stepData[1]).(int64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in I64_ADD (expected int64)")
			return nil
		}
		return first + second
	case I64_EQUAL:
		first, ok := ls.Decode(stepData[0]).(int64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in I64_EQUAL (expected int64)")
			return nil
		}
		second, ok := ls.Decode(stepData[1]).(int64)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in I64_EQUAL (expected int64)")
			return nil
		}
		return first == second
	case TO_BLOB:
		blobBase64, ok := stepData[0].(string)
		if !ok {
			badGlobalLog.Warn().Any("step_data", stepData).Msg("Unexpected step data in TO_BLOB (expected string)")
			return nil
		}
		blob, err := base64.StdEncoding.DecodeString(blobBase64)
		if err != nil {
			badGlobalLog.Err(err).Str("blob_base64", blobBase64).Msg("Failed to decode blob")
			return nil
		}
		return blob
	default:
		badGlobalLog.Error().Int("step_type", int(stepType)).Any("step_data", stepData).Msg("Got unknown step type")
	}

	return nil
}

func (ls *LightSpeedDecoder) handleStoredProcedure(referenceName string, data []interface{}) {
	depReference, ok := ls.Dependencies[referenceName]
	if !ok {
		logEvt := badGlobalLog.Warn().
			Str("reference_name", referenceName)
		if badGlobalLog.Logger.GetLevel() == zerolog.TraceLevel {
			logEvt.Any("data", data)
		}
		logEvt.Msg("Skipping dependency with no reference")
		return
	}

	reflectedMs := reflect.ValueOf(ls.Table).Elem()
	//badGlobalLog.Println(depReference)
	depField := reflectedMs.FieldByName(depReference)

	if !depField.IsValid() {
		logEvt := badGlobalLog.Warn().
			Str("reference_name", referenceName).
			Str("ls_type", depReference)
		if badGlobalLog.Logger.GetLevel() == zerolog.TraceLevel {
			logEvt.Any("data", data)
		}
		logEvt.Msg("Skipping dependency with unrecognized type")
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

		switch kind {
		case reflect.Int64:
			i64, ok := val.(int64)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", val).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set int64")
				continue
			}
			newDepInstance.Field(i).SetInt(i64)
		case reflect.String:
			str, ok := val.(string)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", val).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set string")
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
				badGlobalLog.Warn().Any("val", val).Type("val_type", val).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set bool")
				continue
			}
			newDepInstance.Field(i).SetBool(boolean)
		case reflect.Int:
			integer, ok := val.(int)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", val).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set int")
				continue
			}
			newDepInstance.Field(i).SetInt(int64(integer))
		case reflect.Float64:
			floatVal, ok := val.(float64)
			if !ok {
				badGlobalLog.Warn().Any("val", val).Type("val_type", val).Int("field_index", index).Str("field_name", fieldInfo.Name).Str("struct_name", depFieldsType.Name()).Msg("Failed to set float64")
				continue
			}
			newDepInstance.Field(i).SetFloat(floatVal)
		case reflect.Slice:
			// TODO
			fallthrough
		default:
			badGlobalLog.Warn().Stringer("kind", kind).Any("val", val).Type("val_type", val).Msg("Unknown kind")
			//os.Exit(1)
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
