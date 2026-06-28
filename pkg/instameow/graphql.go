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
	"context"
	"encoding/json"
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
)

func (c *Client) SendMessage(ctx context.Context, req *slidetypes.SendTextRequest) (*slidetypes.SendTextResponse, error) {
	return makeGraphQLRequest[*slidetypes.SendTextResponse](ctx, c, "IGDirectTextSendMutation", req)
}

func (c *Client) SendMedia(ctx context.Context, req *slidetypes.SendMediaRequest) (*slidetypes.SendMediaResponse, error) {
	return makeGraphQLRequest[*slidetypes.SendMediaResponse](ctx, c, "IGDirectMediaSendMutation", req)
}

func (c *Client) EditMessage(ctx context.Context, req *slidetypes.EditMessageRequest) (*slidetypes.EditMessageResponse, error) {
	return makeGraphQLRequest[*slidetypes.EditMessageResponse](ctx, c, "IGDirectEditMessageMutation", req)
}

func (c *Client) UnsendMessage(ctx context.Context, req *slidetypes.UnsendMessageRequest) (*slidetypes.UnsendMessageResponse, error) {
	return makeGraphQLRequest[*slidetypes.UnsendMessageResponse](ctx, c, "IGDMessageUnsendDialogOffMsysMutation", req)
}

func (c *Client) SendReaction(ctx context.Context, req *slidetypes.CreateReactionRequest) (*slidetypes.SendReactionResponse, error) {
	return makeGraphQLRequest[*slidetypes.SendReactionResponse](ctx, c, "IGDirectReactionSendMutation", req)
}

func (c *Client) MarkRead(ctx context.Context, req *slidetypes.MarkReadRequest) (*slidetypes.MarkReadResponse, error) {
	return makeGraphQLRequest[*slidetypes.MarkReadResponse](ctx, c, "useIGDMarkThreadAsReadMutation", req)
}

func (c *Client) MarkReadValidation(ctx context.Context, req *slidetypes.MarkReadRequest) (*slidetypes.MarkReadValidationResponse, error) {
	return makeGraphQLRequest[*slidetypes.MarkReadValidationResponse](ctx, c, "useIGDMarkThreadAsReadValidationMutation", req)
}

func (c *Client) DeleteThread(ctx context.Context, req *slidetypes.DeleteThreadRequest) (*slidetypes.DeleteThreadResponse, error) {
	return makeGraphQLRequest[*slidetypes.DeleteThreadResponse](ctx, c, "IGDInboxInfoDeleteThreadDialogOffMsysMutation", req)
}

func (c *Client) MuteThread(ctx context.Context, req *slidetypes.MuteThreadRequest) (*slidetypes.MuteThreadResponse, error) {
	return makeGraphQLRequest[*slidetypes.MuteThreadResponse](ctx, c, "IGDInboxInfoMuteToggleOffMsysMutation", req)
}

func (c *Client) GetMailbox(ctx context.Context) (*slidetypes.MailboxResponse, error) {
	return makeGraphQLRequest[*slidetypes.MailboxResponse](
		ctx, c, "PolarisDirectInboxQuery",
		slidetypes.MakeMailboxRequest(c.configs.BrowserConfigTable.IGDMqttWebDeviceID.ClientID),
	)
}

func (c *Client) GetThread(ctx context.Context, req *slidetypes.GetThreadInfoRequest) (*slidetypes.ThreadInfoResponse, error) {
	return makeGraphQLRequest[*slidetypes.ThreadInfoResponse](ctx, c, "IGDThreadDetailQuery", req)
}

func (c *Client) GetProfile(ctx context.Context, igid string) (*slidetypes.ProfilePageResponse, error) {
	return makeGraphQLRequest[*slidetypes.ProfilePageResponse](ctx, c, "PolarisProfilePageContentQuery", slidetypes.MakeProfilePageRequest(igid))
}

func (c *Client) EditGroupTitle(ctx context.Context, threadID, newTitle string) error {
	_, err := makeGraphQLRequest[noResp](ctx, c, "IGEditGroupTitle", &graphql.IGEditGroupTitleGraphQLRequestPayload{
		ThreadID: threadID,
		NewTitle: newTitle,
	})
	return err
}

type noResp = json.RawMessage

func makeGraphQLRequest[T any](ctx context.Context, c *Client, name string, req any) (resp T, err error) {
	if c == nil {
		err = ErrClientIsNil
		return
	}
	_, respData, err := c.http.MakeGraphQLRequest(ctx, name, req)
	if err != nil {
		return
	}
	var wrappedResp graphql.Response[T]
	err = json.Unmarshal(respData, &wrappedResp)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal %T: %w", resp, err)
		return
	}
	return wrappedResp.Data, wrappedResp.AsError()
}
