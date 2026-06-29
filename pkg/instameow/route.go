// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package instameow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/go-querystring/query"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type ThreadIGIDs struct {
	LongID  string `json:"thread_igid"`
	ShortID string `json:"thread_fbid"`
}

type routeDefinitions struct {
	Payload struct {
		Payloads map[string]struct {
			Result struct {
				Exports struct {
					RootView struct {
						Props *ThreadIGIDs `json:"props"`
					} `json:"root_view"`
				} `json:"exports"`
			} `json:"result"`
		} `json:"payloads"`
	} `json:"payload"`
}

func (c *Client) FetchThreadIDs(ctx context.Context, threadFBIDs ...int64) (map[int64]*ThreadIGIDs, error) {
	payload := c.http.NewHTTPQuery()
	if c.configs.RoutingNamespace == "" {
		return nil, fmt.Errorf("routing namespace is empty")
	}
	payload.RoutingNamespace = c.configs.RoutingNamespace
	payload.Crn = "comet.igweb.PolarisDirectInboxRoute"

	form, err := query.Values(&payload)
	if err != nil {
		return nil, err
	}
	for i, threadFBID := range threadFBIDs {
		form.Add(fmt.Sprintf("route_urls[%d]", i), fmt.Sprintf("/direct/t/%d/", threadFBID))
	}
	payloadBytes := []byte(form.Encode())

	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", c.GetEndpoint("base_url"))
	headers.Set("referer", c.GetEndpoint("messages"))
	headers.Set("priority", "u=1, i")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")

	url := c.GetEndpoint("route_definition")
	_, body, err := c.http.MakeRequest(ctx, url, http.MethodPost, headers, payloadBytes, types.FORM)
	if err != nil {
		return nil, err
	}

	responseBody := bytes.TrimPrefix(body, httpclient.AntiJSPrefix)

	var routeDefResp routeDefinitions
	err = json.Unmarshal(responseBody, &routeDefResp)
	if err != nil {
		return nil, err
	}
	ids := make(map[int64]*ThreadIGIDs, len(threadFBIDs))
	for _, fbid := range threadFBIDs {
		route, ok := routeDefResp.Payload.Payloads[strconv.FormatInt(fbid, 10)]
		if ok {
			ids[fbid] = route.Result.Exports.RootView.Props
		}
	}
	return ids, nil
}
