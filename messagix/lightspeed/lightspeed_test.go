package lightspeed_test

import (
	"encoding/json"
	"log"
	"os"
	"reflect"
	"testing"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/graphql"
	"go.mau.fi/mautrix-meta/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/messagix/table"
)

func TestDecode(t *testing.T) {
	data, err := os.ReadFile("test_data.json")
	if err != nil {
		log.Fatal(err)
	}

	var res *messagix.PublishResponseData
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatal(err)
	}

	deps := table.SPToDepMap(res.Sp)
	var lsData *lightspeed.LightSpeedData
	err = json.Unmarshal([]byte(res.Payload), &lsData)
	if err != nil {
		log.Fatal(err)
	}

	lsTable := &table.LSTable{}
	lsDecoder := lightspeed.NewLightSpeedDecoder(deps, lsTable)
	lsDecoder.Decode(lsData.Steps)

	tableReflectionTest(lsTable)
}

func TestDecodeIG(t *testing.T) {
	data, err := os.ReadFile("test_data_ig.json")
	if err != nil {
		log.Fatal(err)
	}

	var queryData *graphql.LSPlatformGraphQLLightspeedRequestQuery
	err = json.Unmarshal(data, &queryData)
	if err != nil {
		log.Fatalf("failed to parse LightSpeedQLRequest data from html (INSTAGRAM): %v", err)
	}
	lsPayloadStr := queryData.Data.LightspeedWebRequestForIG.Payload
	deps := queryData.Data.LightspeedWebRequestForIG.Dependencies
	var lsData *lightspeed.LightSpeedData
	err = json.Unmarshal([]byte(lsPayloadStr), &lsData)
	if err != nil {
		log.Fatal(err)
	}

	depsMap, _ := lightspeed.DependenciesToMap(deps)
	lsTable := &table.LSTable{}
	lsDecoder := lightspeed.NewLightSpeedDecoder(depsMap, lsTable)
	lsDecoder.Decode(lsData.Steps)

	tableReflectionTest(lsTable)
}

func tableReflectionTest(loadedTable *table.LSTable) {
	values := reflect.ValueOf(loadedTable).Elem()
	for i := 0; i < values.NumField(); i++ {
		fieldValue := values.Field(i)
		fieldKind := fieldValue.Kind()
		if fieldKind == reflect.Slice && fieldValue.Len() > 0 {
			switch data := fieldValue.Interface().(type) {
			case []table.LSDeleteThenInsertThread:
				for _, d := range data {
					log.Println(data)
					log.Println(d.ThreadKey, d.Snippet, d.ThreadType, d.LastReadWatermarkTimestampMs)
				}
			case []table.LSDeleteThenInsertIGContactInfo:
				log.Println(data[0])
			case []table.LSBumpThread:
				log.Println(data[0].BumpStatus, data[0].LastReadWatermarkTimestampMs, data[0])
			case []table.LSVerifyThreadExists:
				log.Println(data[0].ThreadType, data[0], data[0].ThreadKey, data[0])
			case []table.LSVerifyContactRowExists:
				for _, d := range data {
					log.Println(d.ContactId)
				}
			default:
				log.Println(fieldValue.Type().Elem().String())
			}

			slice := reflect.ValueOf(fieldValue.Interface())
			interfaceSlice := make([]interface{}, slice.Len())
			for i := 0; i < slice.Len(); i++ {
				interfaceSlice[i] = slice.Index(i).Interface()
			}
		}
	}
}
