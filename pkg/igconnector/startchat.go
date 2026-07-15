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

package igconnector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exslices"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.IdentifierResolvingNetworkAPI = (*IGClient)(nil)
	_ bridgev2.UserSearchingNetworkAPI       = (*IGClient)(nil)
	_ bridgev2.GroupCreatingNetworkAPI       = (*IGClient)(nil)
	_ bridgev2.IdentifierValidatingNetwork   = (*IGConnector)(nil)
)

func (ic *IGConnector) ValidateUserID(id networkid.UserID) bool {
	parsed := metaid.ParseUserID(id)
	return parsed > 0
}

func (ic *IGClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	if ic.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}

	id, err := metaid.ParseIDFromString(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identifier: %w", err)
	}
	userID := metaid.MakeUserID(id)
	ghost, err := ic.Main.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ghost: %w", err)
	}
	portalKey := ic.makePortalKey(id, false)
	portal, err := ic.Main.Bridge.GetExistingPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing portal: %w", err)
	}
	var chat *bridgev2.CreateChatResponse
	var userInfo *bridgev2.UserInfo
	if portal != nil && portal.MXID != "" {
		chat = &bridgev2.CreateChatResponse{
			PortalKey: portalKey,
			Portal:    portal,
		}
	} else if createChat {
		igChatID, err := ic.ensureIGIDDirect(ctx, id)
		if errors.Is(err, instameow.ErrThreadNotFound) {
			chat = &bridgev2.CreateChatResponse{
				PortalKey:  portalKey,
				Portal:     portal,
				PortalInfo: ic.makeMinimalDMInfo(id),
			}
		} else if err != nil {
			return nil, fmt.Errorf("failed to get DM chat ID for user %d: %w", id, err)
		} else {
			resp, err := ic.Client.GetThread(ctx, slidetypes.MakeGetThreadInfoRequest(igChatID))
			if err != nil {
				return nil, fmt.Errorf("failed to get DM chat info with %d: %w", id, err)
			}
			chatInfo := ic.wrapChatInfo(resp.ThreadInfo.AsIGDirectThread)
			if chatInfo.Members != nil {
				userInfo = chatInfo.Members.MemberMap[userID].UserInfo
			}
			chat = &bridgev2.CreateChatResponse{
				PortalKey:  portalKey,
				Portal:     portal,
				PortalInfo: chatInfo,
			}
		}
	}
	return &bridgev2.ResolveIdentifierResponse{
		UserID:   userID,
		Ghost:    ghost,
		UserInfo: userInfo,
		Chat:     chat,
	}, nil
}

func (ic *IGClient) CreateGroup(ctx context.Context, params *bridgev2.GroupCreateParams) (*bridgev2.CreateChatResponse, error) {
	otid := strconv.FormatInt(methods.GenerateEpochID(), 10)
	ic.pendingGroupCreations.Add(otid)
	defer ic.pendingGroupCreations.Remove(otid)
	resp, err := ic.Client.CreateGroup(ctx, &slidetypes.CreateGroupRequest{
		RecipientFBIDs:     exslices.CastToString[string](params.Participants),
		OfflineThreadingID: otid,
	})
	if err != nil {
		return nil, err
	}
	idInt, err := strconv.ParseInt(resp.Data.ID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread ID: %w", err)
	}
	portal, err := ic.Main.Bridge.GetPortalByKey(ctx, ic.makePortalKey(idInt, true))
	if err != nil {
		return nil, fmt.Errorf("failed to get portal: %w", err)
	}
	var portalInfo *bridgev2.ChatInfo
	if params.RoomID != "" {
		err = portal.UpdateMatrixRoomID(ctx, params.RoomID, bridgev2.UpdateMatrixRoomIDParams{
			OverwriteOldPortal: true,
			TombstoneOldRoom:   true,
			DeleteOldRoom:      true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update room ID after creating group: %w", err)
		}
	}
	_, err = ic.getAndResyncThread(ctx, resp.Data.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to sync thread after creating group: %w", err)
	}
	err = portal.RoomCreated.WaitTimeoutCtx(ctx, 5*time.Second)
	if err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to wait for room creation after creating group")
	}
	return &bridgev2.CreateChatResponse{
		PortalKey:  portal.PortalKey,
		Portal:     portal,
		PortalInfo: portalInfo,
	}, nil
}

func (ic *IGClient) SearchUsers(ctx context.Context, query string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	if ic.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	resp, err := ic.Client.SearchUsers(ctx, query)
	if err != nil {
		return nil, err
	}

	users := make([]*bridgev2.ResolveIdentifierResponse, len(resp.Data.Results))
	for i, result := range resp.Data.Results {
		userID := metaid.MakeUserID(result.InteropMessagingUserFBID)
		ghost, err := ic.Main.Bridge.GetGhostByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		users[i] = &bridgev2.ResolveIdentifierResponse{
			UserID:   userID,
			Ghost:    ghost,
			UserInfo: ic.wrapSearchResultInfo(result),
		}
	}
	return users, nil
}
