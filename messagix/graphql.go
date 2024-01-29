package messagix

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/go-querystring/query"

	"go.mau.fi/mautrix-meta/messagix/graphql"
	"go.mau.fi/mautrix-meta/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

func (c *Client) makeGraphQLRequest(name string, variables interface{}) (*http.Response, []byte, error) {
	graphQLDoc, ok := graphql.GraphQLDocs[name]
	if !ok {
		return nil, nil, fmt.Errorf("could not find graphql doc by the name of: %s", name)
	}

	vBytes, err := json.Marshal(variables)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal graphql variables to json string: %v", err)
	}

	payload := c.NewHttpQuery()
	payload.FbAPICallerClass = graphQLDoc.CallerClass
	payload.FbAPIReqFriendlyName = graphQLDoc.FriendlyName
	payload.Variables = string(vBytes)
	payload.ServerTimestamps = "true"
	payload.DocID = graphQLDoc.DocId

	form, err := query.Values(payload)
	if err != nil {
		return nil, nil, err
	}

	payloadBytes := []byte(form.Encode())

	headers := c.buildHeaders(true)
	headers.Set("x-fb-friendly-name", graphQLDoc.FriendlyName)
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	headers.Set("origin", c.getEndpoint("base_url"))
	headers.Set("referer", c.getEndpoint("messages")+"/")

	reqUrl := c.getEndpoint("graphql")
	//c.Logger.Info().Any("url", reqUrl).Any("payload", string(payloadBytes)).Any("headers", headers).Msg("Sending graphQL request.")
	return c.MakeRequest(reqUrl, "POST", headers, payloadBytes, types.FORM)
}

func (c *Client) makeLSRequest(variables *graphql.LSPlatformGraphQLLightspeedVariables, reqType int) (*table.LSTable, error) {
	strPayload, err := json.Marshal(&variables)
	if err != nil {
		return nil, err
	}

	lsVariables := &graphql.LSPlatformGraphQLLightspeedRequestPayload{
		DeviceID:              c.configs.browserConfigTable.MqttWebDeviceID.ClientID,
		IncludeChatVisibility: false,
		RequestID:             c.lsRequests,
		RequestPayload:        string(strPayload),
		RequestType:           reqType,
	}
	c.lsRequests++

	var lsRequestQueryName string
	if c.platform.IsMessenger() {
		lsRequestQueryName = "LSGraphQLRequest"
	} else {
		lsRequestQueryName = "LSGraphQLRequestIG"
	}
	_, respBody, err := c.makeGraphQLRequest(lsRequestQueryName, &lsVariables)
	if err != nil {
		return nil, err
	}

	var lightSpeedRes []byte
	var deps interface{}
	if c.platform.IsMessenger() {
		var graphQLData *graphql.LSPlatformGraphQLLightspeedRequestQuery
		err = json.Unmarshal(respBody, &graphQLData)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal LSRequest response bytes into LSPlatformGraphQLLightspeedRequestQuery struct: %v", err)
		}
		lightSpeedRes = []byte(graphQLData.Data.Viewer.LightspeedWebRequest.Payload)
		deps = graphQLData.Data.Viewer.LightspeedWebRequest.Dependencies
	} else {
		var graphQLData *graphql.LSPlatformGraphQLLightspeedRequestForIGDQuery
		err = json.Unmarshal(respBody, &graphQLData)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal LSRequest response bytes into LSPlatformGraphQLLightspeedRequestForIGDQuery struct: %v", err)
		}
		lightSpeedRes = []byte(graphQLData.Data.LightspeedWebRequestForIgd.Payload)
		deps = graphQLData.Data.LightspeedWebRequestForIgd.Dependencies
	}

	var lsData *lightspeed.LightSpeedData
	err = json.Unmarshal([]byte(lightSpeedRes), &lsData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal LSRequest lightspeed payload into lightspeed.LightSpeedData: %v", err)
	}

	dependencies, err := lightspeed.DependenciesToMap(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to convert dependencies to map: %v", err)
	}

	lsTable := &table.LSTable{}
	lsDecoder := lightspeed.NewLightSpeedDecoder(dependencies, lsTable)
	lsDecoder.Decode(lsData.Steps)

	return lsTable, nil
}
