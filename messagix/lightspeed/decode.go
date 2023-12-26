package lightspeed

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type LightSpeedDecoder struct {
	Table interface{} // struct that contains pointers to all the dependencies/stores
	Dependencies map[string]string
	StatementReferences map[int]interface{}
}

func NewLightSpeedDecoder(dependencies map[string]string, table interface{}) *LightSpeedDecoder {
	if reflect.ValueOf(table).Kind() != reflect.Ptr {
		return nil
	}
	return &LightSpeedDecoder{
		Table:              table,
		Dependencies:       dependencies,
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
			log.Println("[LOAD] failed to store key to float64")
			return 0
		}
		
		shouldLoad, ok := ls.StatementReferences[int(key)]
		if !ok {
			log.Println("[LOAD] failed to fetch statement reference for key:", key)
			return 0
		}
		return shouldLoad
	case STORE:
		retVal := ls.Decode(stepData[1])
		ls.StatementReferences[int(stepData[0].(float64))] = retVal
	case STORE_ARRAY:
		key, ok := stepData[0].(float64)
		if !ok {
			log.Println(stepData...)
			os.Exit(1)
		}

		shouldStore, ok := stepData[1].(float64)
		if !ok {
			log.Println(stepData...)
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
			log.Println("[I64_FROM_STRING] failed to convert string to int64:", err.Error())
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
		log.Println("call native operation...", stepData)
		return nil
	case NATIVE_OP_MAP_CREATE:
		return make(map[interface{}]interface{}, 0)
	case NATIVE_OP_MAP_SET:
		mapToUpdate, ok := ls.Decode(stepData[0]).(map[interface{}]interface{})
		if !ok {
			log.Println("failed to type assert map from statement references...")
			return nil
		}
		mapKey := ls.Decode(stepData[1])
		mapVal := ls.Decode(stepData[2])
		mapToUpdate[mapKey] = mapVal
	case LOGGER_LOG:
		log.Println("[FB-LOGGER] Server log:", stepData[0])
		return nil
	case I64_ADD:
		first := ls.Decode(stepData[0]).(int64)
		second := ls.Decode(stepData[1]).(int64)
		return first + second
	default:
		log.Println("got unknown step type:", stepType)
		log.Println(stepData...)
		log.Println(s...)
		os.Exit(1)
	}

	return nil
}

func (ls *LightSpeedDecoder) handleStoredProcedure(referenceName string, data []interface{}) {
	depReference, ok := ls.Dependencies[referenceName]
	if !ok {
		log.Println("Skipping dependency with reference name:",referenceName, data)
		return
	}

	reflectedMs := reflect.ValueOf(ls.Table).Elem()
	//log.Println(depReference)
	depField := reflectedMs.FieldByName(depReference)
	
	if !depField.IsValid() {
		log.Println("Skipping dependency with reference name:",referenceName, data)
		return
	}

	var err error

	depFieldsType := depField.Type().Elem()
	newDepInstance := reflect.New(depFieldsType).Elem()
	for i := 0; i < depFieldsType.NumField(); i++ {
		fieldInfo := depFieldsType.Field(i)
		var index int
		conditionField := fieldInfo.Tag.Get("conditionField")
		if conditionField != "" {
			indexChoices := fieldInfo.Tag.Get("indexes")
			conditionVal := newDepInstance.FieldByName(conditionField)
			index, err = ls.parseConditionIndex(conditionVal.Bool(), indexChoices)
			if err != nil {
				log.Println(fmt.Sprintf("failed to parse condition index in dependency %v for field %v", depFieldsType.Name(), fieldInfo.Name))
				continue
			}
		} else {
			index, _ = strconv.Atoi(fieldInfo.Tag.Get("index"))
		}
		
		if index > len(data)-1 {
			log.Println(fmt.Sprintf("breaking handleStoredProcedure loop because the defined struct exceeds the length of the lightspeed data slice: (index=%d, sliceLen=%d, field=%s, dependency=%s)", index, len(data)-1, fieldInfo.Name, depFieldsType.Name()))
			break
		}

		kind := fieldInfo.Type.Kind()
		val := ls.Decode(data[index])
		if val == nil { // skip setting field, because the index in the array was [9] which is undefined.
			continue
		}

		valType := reflect.TypeOf(val)
		switch kind {
		case reflect.Int64:
			i64, ok := val.(int64)
			if !ok {
				log.Println(fmt.Sprintf("failed to set int64 to %v in dependency %v for field %v (index=%d, actualType=%v)", val, depFieldsType.Name(), fieldInfo.Name, index, valType))
				continue
			}
			newDepInstance.Field(i).SetInt(i64)
		case reflect.String:
			str, ok := val.(string)
			if !ok {
				log.Println(fmt.Sprintf("failed to set string to %v in dependency %v for field %v (index=%d, actualType=%v)", val, depFieldsType.Name(), fieldInfo.Name, index, valType))
				continue
			}
			newDepInstance.Field(i).SetString(str)
		case reflect.Interface:
			if val == nil {
				continue
			}
			log.Println(val)
			newDepInstance.Field(i).Set(reflect.ValueOf(val))
		case reflect.Bool:
			boolean, ok := val.(bool)
			if !ok {
				log.Println(fmt.Sprintf("failed to set bool to %v in dependency %v for field %v (index=%d, actualType=%v)", val, depFieldsType.Name(), fieldInfo.Name, index, valType))
				continue
			}
			newDepInstance.Field(i).SetBool(boolean)
		case reflect.Int:
			integer, ok := val.(int)
			if !ok {
				log.Println(fmt.Sprintf("failed to set int to %v in dependency %v for field %v (index=%d, actualType=%v)", val, depFieldsType.Name(), fieldInfo.Name, index, valType))
				continue
			}
			newDepInstance.Field(i).SetInt(int64(integer))
		case reflect.Float64:
			floatVal, ok := val.(float64)
			if !ok {
				log.Println(fmt.Sprintf("failed to set float64 to %v in dependency %v for field %v (index=%d, actualType=%v)", val, depFieldsType.Name(), fieldInfo.Name, index, valType))
				continue
			}
			newDepInstance.Field(i).SetFloat(floatVal)
		default:
			log.Println("invalid kind:", kind, val, valType)
			os.Exit(1)
		}
	}
	newSlice := reflect.Append(depField, newDepInstance)
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