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

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func (c *Client) SendMessage(ctx context.Context, req *slidetypes.SendTextRequest) (*slidetypes.SendTextResponse, error) {
	return makeGraphQLRequest[*slidetypes.SendTextResponse](ctx, c, "IGDirectTextSendMutation", req, true)
}

func (c *Client) SendMedia(ctx context.Context, req *slidetypes.SendMediaRequest) (*slidetypes.SendMediaResponse, error) {
	return makeGraphQLRequest[*slidetypes.SendMediaResponse](ctx, c, "IGDirectMediaSendMutation", req, true)
}

func (c *Client) EditMessage(ctx context.Context, req *slidetypes.EditMessageRequest) (*slidetypes.EditMessageResponse, error) {
	return makeGraphQLRequest[*slidetypes.EditMessageResponse](ctx, c, "IGDirectEditMessageMutation", req, true)
}

func (c *Client) UnsendMessage(ctx context.Context, req *slidetypes.UnsendMessageRequest) (*slidetypes.UnsendMessageResponse, error) {
	return makeGraphQLRequest[*slidetypes.UnsendMessageResponse](ctx, c, "IGDMessageUnsendDialogOffMsysMutation", req, true)
}

func (c *Client) SendReaction(ctx context.Context, req *slidetypes.CreateReactionRequest) (*slidetypes.SendReactionResponse, error) {
	return makeGraphQLRequest[*slidetypes.SendReactionResponse](ctx, c, "IGDirectReactionSendMutation", req, true)
}

func (c *Client) MarkRead(ctx context.Context, req *slidetypes.MarkReadRequest) (*slidetypes.MarkReadResponse, error) {
	return makeGraphQLRequest[*slidetypes.MarkReadResponse](ctx, c, "useIGDMarkThreadAsReadMutation", req, true)
}

func (c *Client) MarkReadValidation(ctx context.Context, req *slidetypes.MarkReadRequest) (*slidetypes.MarkReadValidationResponse, error) {
	return makeGraphQLRequest[*slidetypes.MarkReadValidationResponse](ctx, c, "useIGDMarkThreadAsReadValidationMutation", req, false)
}

func (c *Client) DeleteThread(ctx context.Context, req *slidetypes.DeleteThreadRequest) (*slidetypes.DeleteThreadResponse, error) {
	return makeGraphQLRequest[*slidetypes.DeleteThreadResponse](ctx, c, "IGDInboxInfoDeleteThreadDialogOffMsysMutation", req, true)
}

func (c *Client) AcceptMessageRequest(ctx context.Context, req *slidetypes.AcceptMessageRequestRequest) (*slidetypes.AcceptMessageRequestResponse, error) {
	return makeGraphQLRequest[*slidetypes.AcceptMessageRequestResponse](ctx, c, "useIGDirectAcceptMessageRequestMutation", req, true)
}

func (c *Client) MuteThread(ctx context.Context, req *slidetypes.MuteThreadRequest) (*slidetypes.MuteThreadResponse, error) {
	return makeGraphQLRequest[*slidetypes.MuteThreadResponse](ctx, c, "IGDInboxInfoMuteToggleOffMsysMutation", req, true)
}

func (c *Client) PinThread(ctx context.Context, req *slidetypes.PinThreadRequest) (*slidetypes.PinThreadResponse, error) {
	return makeGraphQLRequest[*slidetypes.PinThreadResponse](ctx, c, "useIGDPinThreadMutation", req, true)
}

func (c *Client) PinMessage(ctx context.Context, req *slidetypes.PinMessageRequest) (*slidetypes.PinMessageResponse, error) {
	return makeGraphQLRequest[*slidetypes.PinMessageResponse](ctx, c, "useIGDPinMessageOffMsysMutation", req, true)
}

func (c *Client) UnpinMessage(ctx context.Context, req *slidetypes.PinMessageRequest) (*slidetypes.UnpinMessageResponse, error) {
	return makeGraphQLRequest[*slidetypes.UnpinMessageResponse](ctx, c, "useIGDUnpinMessageOffMsysMutation", req, true)
}

func (c *Client) GetMailbox(ctx context.Context) (*slidetypes.MailboxResponse, error) {
	return makeGraphQLRequest[*slidetypes.MailboxResponse](
		ctx, c, "PolarisDirectInboxQuery",
		slidetypes.MakeMailboxRequest(c.configs.BrowserConfigTable.IGDMqttWebDeviceID.ClientID),
		false,
	)
}

func (c *Client) GetThread(ctx context.Context, req *slidetypes.GetThreadInfoRequest) (*slidetypes.ThreadInfoResponse, error) {
	return makeGraphQLRequest[*slidetypes.ThreadInfoResponse](ctx, c, "IGDThreadDetailQuery", req, true)
}

func (c *Client) PaginateMessages(ctx context.Context, req *slidetypes.PaginateMessagesRequest) (*slidetypes.PaginateMessagesResponse, error) {
	return makeGraphQLRequest[*slidetypes.PaginateMessagesResponse](ctx, c, "IGDMessageListOffMsysQuery", req, true)
}

func (c *Client) GetProfile(ctx context.Context, igid string) (*slidetypes.ProfilePageResponse, error) {
	return makeGraphQLRequest[*slidetypes.ProfilePageResponse](ctx, c, "PolarisProfilePageContentQuery", slidetypes.MakeProfilePageRequest(igid), true)
}

func (c *Client) EditGroupTitle(ctx context.Context, threadID, newTitle string) error {
	_, err := makeGraphQLRequest[noResp](ctx, c, "IGDEditThreadNameDialogOffMsysMutation", &graphql.IGEditGroupTitleGraphQLRequestPayload{
		ThreadID: threadID,
		NewTitle: newTitle,
	}, true)
	return err
}

type noResp = json.RawMessage

func makeGraphQLRequest[T any](ctx context.Context, c *Client, name string, req any, allowReload bool) (resp T, err error) {
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
	} else if allowReload && wrappedResp.ErrorCode == types.ErrPleaseReloadPage.ErrorCode {
		zerolog.Ctx(ctx).Warn().Err(wrappedResp.AsError()).
			Msg("Got please reload page error, reloading index and retrying")
		reloadErr := c.ReloadIndex(ctx)
		if reloadErr != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to reload page to retry GraphQL request")
		} else {
			zerolog.Ctx(ctx).Debug().Msg("Successfully reloaded index, retrying GraphQL request")
			return makeGraphQLRequest[T](ctx, c, name, req, false)
		}
	}
	return wrappedResp.Data, wrappedResp.AsError()
}
