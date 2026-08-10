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
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-querystring/query"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type ThreadIGIDs struct {
	LongID  string `json:"thread_igid"`
	ShortID string `json:"thread_fbid"`
}

type routeDefinition struct {
	Payload struct {
		Payload struct {
			Result struct {
				Exports struct {
					RootView struct {
						Props *ThreadIGIDs `json:"props"`
					} `json:"rootView"`
				} `json:"exports"`
			} `json:"result"`
		} `json:"payload"`
	} `json:"payload"`
}

var ErrThreadNotFound = errors.New("instagram thread not found")

func (c *Client) FetchThreadID(ctx context.Context, threadFBID int64) (*ThreadIGIDs, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	payload := c.http.NewHTTPQuery()
	if c.configs.RoutingNamespace == "" {
		return nil, fmt.Errorf("routing namespace is empty")
	}
	payload.RoutingNamespace = c.configs.RoutingNamespace
	payload.Crn = "comet.igweb.PolarisDirectInboxRoute"
	// This is not the real user FBID nor the IGID, it's some third identifier for the Instagram user
	payload.ClientPreviousActorID = c.configs.BrowserConfigTable.PolarisViewer.Data.Fbid

	form, err := query.Values(&payload)
	if err != nil {
		return nil, err
	}
	routeURL := fmt.Sprintf("/direct/t/%d/", threadFBID)
	form.Add("route_url", routeURL)
	payloadBytes := []byte(form.Encode())

	headers := c.http.BuildHeaders(true, false)
	headers.Set("origin", c.GetEndpoint("base_url"))
	headers.Set("referer", c.GetEndpoint("base_url")+routeURL)
	headers.Set("priority", "u=1, i")
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")

	url := c.GetEndpoint("navigation")
	resp, body, err := c.http.MakeRequest(ctx, url, http.MethodPost, headers, payloadBytes, types.FORM)
	if err != nil {
		c.checkResponseError(err)
		if resp != nil && resp.StatusCode == 404 {
			return nil, fmt.Errorf("%w (404 response)", ErrThreadNotFound)
		}
		return nil, err
	}

	responseBody := bytes.TrimPrefix(body, httpclient.AntiJSPrefix)

	var routeDefResp routeDefinition
	err = json.Unmarshal(responseBody, &routeDefResp)
	if err != nil {
		return nil, err
	}
	props := routeDefResp.Payload.Payload.Result.Exports.RootView.Props
	if props == nil {
		return nil, fmt.Errorf("%w (no props in response)", ErrThreadNotFound)
	} else if props.LongID == "" || props.ShortID == "" {
		return nil, fmt.Errorf("%w (missing thread IDs in props)", ErrThreadNotFound)
	}
	return props, nil
}
