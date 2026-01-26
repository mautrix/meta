package messagix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-querystring/query"
	"github.com/google/uuid"
	"go.mau.fi/util/exslices"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

// This has some overlap with makeGraphQLRequest but it's really a
// completely different API that takes a ton of different parameters
// and is used by a different client, despite also being called
// "graphql" in the url.
func (c *Client) makeBloksRequest(ctx context.Context, doc *bloks.BloksDoc, variables *bloks.BloksRequestOuter) (*bloks.BloksBundle, error) {
	appID := variables.Params.AppID
	c.Logger.Debug().Str("bloks_app", appID).Msg("Making Bloks request")

	vBytes, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal bloks variables to json string: %w", err)
	}
	c.Logger.Trace().Str("bloks_app", appID).Bytes("req", vBytes).Msg("Logging raw Bloks request")

	payload := &HttpQuery{}
	payload.Method = "post"
	payload.Pretty = "false"
	payload.Format = "json"
	payload.ServerTimestamps = "true"
	payload.Locale = "en_US"
	payload.Purpose = "fetch"
	payload.FbAPIReqFriendlyName = doc.Name + "-" + appID
	payload.ClientDocID = doc.ClientDocID
	payload.EnableCanonicalNaming = "true"
	payload.EnableCanonicalVariableOverrides = "true"
	payload.EnableCanonicalNamingAmbiguousTypePrefixing = "true"
	payload.Variables = string(vBytes)

	form, err := query.Values(payload)
	if err != nil {
		return nil, err
	}

	payloadBytes := []byte(form.Encode())

	headers, err := c.buildMessengerLiteHeaders()
	if err != nil {
		return nil, err
	}

	headers.Set("x-fb-friendly-name", doc.Name+"-"+appID)
	headers.Set("x-root-field-name", doc.RootField)
	headers.Set("x-graphql-request-purpose", "fetch")
	headers.Set("x-graphql-client-library", "pando")
	headers.Set("x-fb-client-ip", "True")
	headers.Set("x-fb-server-cluster", "True")
	headers.Set("x-fb-conn-uuid-client", strings.Replace(uuid.New().String(), "-", "", -1))
	headers.Set("x-fb-http-engine", "Tigon/MNS/TCP")
	headers.Set("x-fb-rmd", "fail=Server:INVALID_MAP,Default:INVALID_MAP;v=;ip=;tkn=;reqTime=0;recvTime=0")

	headers.Set("Authorization", "OAuth "+useragent.MessengerLiteAccessToken)

	reqUrl := c.GetEndpoint("graph_graphql") // graph.facebook.com vs /api/graphql
	_, respData, err := c.MakeRequest(ctx, reqUrl, "POST", headers, payloadBytes, types.FORM)

	if err != nil {
		return nil, err
	}

	c.Logger.Trace().Str("bloks_app", appID).Bytes("resp", respData).Msg("Logging raw Bloks response")

	var respOuter bloks.BloksResponse
	err = json.Unmarshal(respData, &respOuter)
	if err != nil {
		return nil, fmt.Errorf("parsing outer bloks payload: %w", err)
	}

	if len(respOuter.Errors) > 0 {
		errors := []string{}
		for _, e := range respOuter.Errors {
			errors = append(errors, e.Summary)
		}
		return nil, fmt.Errorf("bloks error: %s", strings.Join(errors, "; "))
	}

	innerData := ""
	if respOuter.Data.BloksApp != nil {
		innerData = respOuter.Data.BloksApp.Screen.Component.Bundle.Tree
	}
	if respOuter.Data.BloksAction != nil {
		innerData = respOuter.Data.BloksAction.Action.Bundle.BundleAction
	}

	if innerData == "" {
		c.Logger.Trace().Bytes("response", respData).Msg("failed to find inner bloks payload")
		return nil, fmt.Errorf("couldn't find inner bloks payload")
	}

	var respInner bloks.BloksBundle
	err = json.Unmarshal([]byte(innerData), &respInner)
	if err != nil {
		c.Logger.Trace().Bytes("response", respData).Msg("failed to parse inner bloks payload")
		return nil, fmt.Errorf("parsing inner bloks payload: %w", err)
	}

	return &respInner, nil
}

func (c *Client) makeGraphQLRequest(ctx context.Context, name string, variables interface{}) (*http.Response, []byte, error) {
	graphQLDoc, ok := graphql.GraphQLDocs[name]
	if !ok {
		return nil, nil, fmt.Errorf("could not find graphql doc by the name of: %s", name)
	}

	vBytes, err := json.Marshal(variables)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal graphql variables to json string: %w", err)
	}

	payload := c.newHTTPQuery()
	payload.FbAPICallerClass = graphQLDoc.CallerClass
	payload.FbAPIReqFriendlyName = graphQLDoc.FriendlyName
	payload.Variables = string(vBytes)
	payload.ServerTimestamps = "true"
	payload.DocID = graphQLDoc.DocId
	payload.Jssesw = graphQLDoc.Jsessw

	form, err := query.Values(payload)
	if err != nil {
		return nil, nil, err
	}

	payloadBytes := []byte(form.Encode())

	headers := c.buildHeaders(true, false)
	headers.Set("x-fb-friendly-name", graphQLDoc.FriendlyName)
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	headers.Set("origin", c.GetEndpoint("base_url"))
	headers.Set("referer", c.GetEndpoint("messages")+"/")

	reqUrl := c.GetEndpoint("graphql")
	//c.Logger.Info().Any("url", reqUrl).Any("payload", string(payloadBytes)).Any("headers", headers).Msg("Sending graphQL request.")
	resp, respData, err := c.MakeRequest(ctx, reqUrl, "POST", headers, payloadBytes, types.FORM)
	if err == nil && resp != nil {
		c.cookies.UpdateFromResponse(resp)
	}
	respData = bytes.TrimPrefix(respData, antiJSPrefix)
	return resp, respData, err
}

func (c *Client) makeLSRequest(ctx context.Context, variables *graphql.LSPlatformGraphQLLightspeedVariables, reqType int) (*table.LSTable, error) {
	strPayload, err := json.Marshal(&variables)
	if err != nil {
		return nil, err
	}

	lsVariables := &graphql.LSPlatformGraphQLLightspeedRequestPayload{
		DeviceID:              c.configs.BrowserConfigTable.MqttWebDeviceID.ClientID,
		IncludeChatVisibility: false,
		RequestID:             c.lsRequests,
		RequestPayload:        string(strPayload),
		RequestType:           reqType,
	}
	c.lsRequests++

	var lsRequestQueryName string
	if c.Platform.IsMessenger() {
		lsRequestQueryName = "LSGraphQLRequest"
	} else {
		lsRequestQueryName = "LSGraphQLRequestIG"
	}
	_, respBody, err := c.makeGraphQLRequest(ctx, lsRequestQueryName, &lsVariables)
	if err != nil {
		return nil, err
	}

	var graphQLData *graphql.LSPlatformGraphQLLightspeedRequestQuery
	err = json.Unmarshal(respBody, &graphQLData)
	if err != nil {
		if len(respBody) < 4096 {
			c.Logger.Debug().Str("respBody", base64.StdEncoding.EncodeToString(respBody)).Msg("Errored LS response bytes")
		} else {
			c.Logger.Debug().Str("respBody", base64.StdEncoding.EncodeToString(respBody[:4096])).Msg("Errored LS response bytes (truncated)")
		}
		return nil, fmt.Errorf("failed to unmarshal LSRequest response bytes into LSPlatformGraphQLLightspeedRequestQuery struct: %w", err)
	}
	if graphQLData.ErrorCode != 0 {
		c.Logger.Warn().
			Str("error_description", graphQLData.ErrorDescription).
			Str("error_summary", graphQLData.ErrorSummary).
			Int("error_code", graphQLData.ErrorCode).
			Msg("GraphQL error in lightspeed request")
		if graphQLData.Data == nil {
			return nil, fmt.Errorf("graphql error %w", &graphQLData.ErrorResponse)
		}
	} else if graphQLData.Data == nil {
		c.Logger.Debug().RawJSON("respBody", respBody).Msg("LS response with no data and no error")
		return nil, fmt.Errorf("graphql request didn't return data")
	}
	var lightSpeedRes []byte
	var deps lightspeed.DependencyList
	if graphQLData.Data.LightspeedWebRequestForIG != nil {
		lightSpeedRes = []byte(graphQLData.Data.LightspeedWebRequestForIG.Payload)
		deps = graphQLData.Data.LightspeedWebRequestForIG.Dependencies
	} else if graphQLData.Data.Viewer.LightspeedWebRequest != nil {
		lightSpeedRes = []byte(graphQLData.Data.Viewer.LightspeedWebRequest.Payload)
		deps = graphQLData.Data.Viewer.LightspeedWebRequest.Dependencies
	} else {
		if graphQLData.Errors != nil {
			return nil, errors.Join(exslices.CastFunc(graphQLData.Errors, func(from types.GraphQLError) error {
				return &from
			})...)
		}
		c.Logger.Debug().RawJSON("respBody", respBody).Msg("LS response with no lightspeed response data and no error")
		return nil, fmt.Errorf("graphql request didn't return LS data")
	}
	var lsData *lightspeed.LightSpeedData
	err = json.Unmarshal(lightSpeedRes, &lsData)
	if err != nil {
		c.Logger.Debug().RawJSON("respBody", respBody).Msg("Response data for errored inner response")
		return nil, fmt.Errorf("failed to unmarshal LSRequest lightspeed payload into lightspeed.LightSpeedData: %w", err)
	}

	lsTable := &table.LSTable{}
	lsDecoder := lightspeed.NewLightSpeedDecoder(deps.ToMap(), lsTable)
	lsDecoder.Decode(lsData.Steps)

	return lsTable, nil
}
