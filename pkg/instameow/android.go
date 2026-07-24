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
	"image"
	imgjpeg "image/jpeg"
	"net/http"
	"strconv"

	"github.com/google/go-querystring/query"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

func (c *Client) EditGroupAvatar(ctx context.Context, threadID string, avatar []byte) error {
	detectedType := http.DetectContentType(avatar)
	if detectedType != "image/jpeg" && detectedType != "image/png" {
		img, _, err := image.Decode(bytes.NewReader(avatar))
		if err != nil {
			return fmt.Errorf("failed to decode avatar image: %w", err)
		}
		var buf bytes.Buffer
		if err = imgjpeg.Encode(&buf, img, nil); err != nil {
			return fmt.Errorf("failed to encode avatar image as jpeg: %w", err)
		}
		avatar = buf.Bytes()
		detectedType = "image/jpeg"
	}

	entityID := fmt.Sprintf("%d_0_%d", c.cookies.GetUserID(), methods.GenerateEpochID())
	reqUrl := c.GetEndpoint("rupload_ig") + entityID

	h := c.buildAndroidHeaders()
	h.Set("content-type", "application/octet-stream")
	h.Set("image_type", "FILE_ATTACHMENT")
	h.Set("x-entity-name", entityID)
	h.Set("x-entity-length", strconv.Itoa(len(avatar)))
	h.Set("x-entity-type", detectedType)
	h.Set("offset", "0")
	h.Set("priority", "u=6, i")

	_, respBody, err := c.http.MakeRequest(ctx, reqUrl, http.MethodPost, h, avatar, types.NONE)
	if err != nil {
		c.checkResponseError(err)
		return fmt.Errorf("failed to upload group avatar: %w", err)
	}

	var uploadResp httpclient.RUploadResponse
	err = json.Unmarshal(respBody, &uploadResp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal upload response: %w", err)
	}
	if uploadResp.MediaID == 0 {
		return fmt.Errorf("failed to upload group avatar: no media id received. response: %s", string(respBody))
	}

	igVariables := &graphql.IGEditGroupAvatarGraphQLRequestPayload{
		ThreadID:           threadID,
		OfflineThreadingID: strconv.FormatInt(methods.GenerateEpochID(), 10),
		AttachmentFBID:     strconv.FormatInt(uploadResp.MediaID, 10),
	}

	_, err = c.makeIGraphQLRequest(ctx, "IGDirectUpdateThreadImageMutation", "xig_direct_update_thread_image", igVariables)
	if err != nil {
		return fmt.Errorf("failed to set group avatar: %w", err)
	}

	return nil
}

func (c *Client) RemoveGroupAvatar(ctx context.Context, threadID string) error {
	igVariables := &graphql.IGEditGroupAvatarGraphQLRequestPayload{
		ThreadID:           threadID,
		OfflineThreadingID: strconv.FormatInt(methods.GenerateEpochID(), 10),
	}

	_, err := c.makeIGraphQLRequest(ctx, "IGDirectRemoveThreadImageMutation", "xig_direct_remove_thread_image", igVariables)
	if err != nil {
		return fmt.Errorf("failed to remove group avatar: %w", err)
	}

	return nil
}

func (c *Client) buildAndroidHeaders() http.Header {
	h := http.Header{}
	h.Set("authorization", c.http.GetRUploadToken())
	h.Set("user-agent", useragent.AndroidUserAgent)
	h.Set("ig-intended-user-id", strconv.FormatInt(c.cookies.GetUserID(), 10))
	h.Set("ig-u-ds-user-id", strconv.FormatInt(c.cookies.GetUserID(), 10))
	h.Set("x-mid", c.cookies.Get(cookies.IGCookieMachineID))
	h.Set("x-ig-app-id", useragent.IGAndroidAppID)
	return h
}

func (c *Client) makeIGraphQLRequest(ctx context.Context, docName, rootFieldName string, variables interface{}) ([]byte, error) {
	graphQLDoc, ok := graphql.GraphQLDocs[docName]
	if !ok {
		return nil, fmt.Errorf("graphql doc %s not found", docName)
	}
	h := c.buildAndroidHeaders()
	h.Set("x-fb-friendly-name", graphQLDoc.FriendlyName)
	if rootFieldName != "" {
		h.Set("x-root-field-name", rootFieldName)
	}
	h.Set("x-graphql-client-library", "pando")
	h.Set("x-fb-http-engine", "Tigon/MNS/TCP")

	graphQLUrl := c.GetEndpoint("i_graphql")

	vBytes, err := json.Marshal(variables)
	if err != nil {
		return nil, err
	}

	payload := &httpclient.HTTPQuery{
		Method:                           "post",
		Pretty:                           "false",
		Format:                           "json",
		ServerTimestamps:                 "true",
		Locale:                           "user",
		FbAPIReqFriendlyName:             graphQLDoc.FriendlyName,
		ClientDocID:                      graphQLDoc.ClientDocID,
		EnableCanonicalNaming:            "true",
		EnableCanonicalVariableOverrides: "true",
		EnableCanonicalNamingAmbiguousTypePrefixing: "true",
		Variables: string(vBytes),
	}

	form, err := query.Values(payload)
	if err != nil {
		return nil, err
	}
	payloadBytes := []byte(form.Encode())

	_, respBody, err := c.http.MakeRequest(ctx, graphQLUrl, http.MethodPost, h, payloadBytes, types.FORM)
	if err != nil {
		c.checkResponseError(err)
		return nil, err
	}

	return respBody, nil
}
