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
	"fmt"
	"net/url"
	"path"
	"slices"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

func (ic *IGClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	info, err := ic.Client.GetThread(ctx, slidetypes.MakeGetThreadInfoRequest(strconv.FormatInt(metaid.ParseFBPortalID(portal.ID), 10)))
	if err != nil {
		return nil, err
	}
	return ic.wrapChatInfo(info.ThreadInfo.AsIGDirectThread), nil
}

func (ic *IGClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	// We can't get user info by FBID, and if the IGID is known, we should already have the profile.
	return nil, nil
}

func (ic *IGClient) updateGhostIGID(username, igid string, fbid int64) bridgev2.ExtraUpdater[*bridgev2.Ghost] {
	return func(ctx context.Context, ghost *bridgev2.Ghost) (changed bool) {
		meta := ghost.Metadata.(*metaid.GhostMetadata)
		if meta.Username != username {
			meta.Username = username
			changed = true
		}
		if meta.IGID != igid {
			meta.IGID = igid
			changed = true
			err := ic.Main.DB.PutFBIDForIGUser(ctx, igid, fbid)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).
					Int64("user_fbid", fbid).
					Str("user_igid", igid).
					Msg("Failed to save FBID for IG user")
			}
		}
		return
	}
}

func (ic *IGClient) wrapUserInfo(info *slidetypes.User) *bridgev2.UserInfo {
	return &bridgev2.UserInfo{
		Identifiers: []string{fmt.Sprintf("instagram:%s", info.Username)},
		Name: ptr.Ptr(ic.Main.Config.FormatDisplayname(DisplaynameParams{
			DisplayName: info.FullName,
			Username:    info.Username,
			ID:          info.InteropMessagingUserFBID,
		})),
		Avatar:       wrapAvatar(info.ProfilePicURL),
		IsBot:        ptr.Ptr(info.AIAgentType != ""),
		ExtraUpdates: ic.updateGhostIGID(info.Username, info.ID, info.InteropMessagingUserFBID),
	}
}

func (ic *IGClient) wrapSearchResultInfo(info *slidetypes.SearchResult) *bridgev2.UserInfo {
	return &bridgev2.UserInfo{
		Identifiers: []string{fmt.Sprintf("instagram:%s", info.Username)},
		Name: ptr.Ptr(ic.Main.Config.FormatDisplayname(DisplaynameParams{
			DisplayName: info.FullName,
			Username:    info.Username,
			ID:          info.InteropMessagingUserFBID,
		})),
		Avatar:       wrapAvatar(info.ProfilePicURL),
		ExtraUpdates: ic.updateGhostIGID(info.Username, info.PK, info.InteropMessagingUserFBID),
	}
}

func wrapAvatar(avatarURL string) *bridgev2.Avatar {
	if avatarURL == "" {
		return &bridgev2.Avatar{Remove: true}
	}
	parsedURL, _ := url.Parse(avatarURL)
	avatarID := path.Base(parsedURL.Path)
	return &bridgev2.Avatar{
		ID: networkid.AvatarID(avatarID),
		Get: func(ctx context.Context) ([]byte, error) {
			return mediadl.DownloadAvatar(ctx, avatarURL)
		},
	}
}

const (
	powerDefault = 0
	powerAdmin   = 50
)

func (ic *IGClient) saveThreadMappings(ctx context.Context, info *slidetypes.ThreadInfo) error {
	err := ic.Main.DB.PutFBIDForIGThread(ctx, info.ThreadID, info.ThreadKey, ic.UserLogin.ID)
	if err != nil {
		return fmt.Errorf("failed to save IG thread ID mapping %s<->%d: %w", info.ThreadID, info.ThreadKey, err)
	}
	err = ic.Main.DB.PutFBIDForIGChat(ctx, info.ID, info.ThreadKey, ic.UserLogin.ID)
	if err != nil {
		return fmt.Errorf("failed to save IG chat ID mapping %s<->%d: %w", info.ID, info.ThreadKey, err)
	}
	for _, user := range info.Users {
		err = ic.Main.DB.PutFBIDForIGUser(ctx, user.ID, user.InteropMessagingUserFBID)
		if err != nil {
			return fmt.Errorf("failed to save IG user ID mapping %s<->%d: %w", user.ID, user.InteropMessagingUserFBID, err)
		}
	}
	return nil
}

func (ic *IGClient) makeMinimalDMInfo(userID int64) *bridgev2.ChatInfo {
	memberMap := make(bridgev2.ChatMemberMap, 2)
	return &bridgev2.ChatInfo{
		Members: &bridgev2.ChatMemberList{
			IsFull:                     true,
			ExcludeChangesFromTimeline: true,
			TotalMemberCount:           2,
			OtherUserID:                metaid.MakeUserID(userID),
			MemberMap:                  memberMap,
		},
		Type:                       ptr.Ptr(database.RoomTypeDM),
		ExcludeChangesFromTimeline: true,
	}
}

func (ic *IGClient) wrapChatInfo(info *slidetypes.ThreadInfo) *bridgev2.ChatInfo {
	if info.ThreadFBID != info.ID ||
		(info.IsGroup && strconv.FormatInt(info.ThreadKey, 10) != info.ID) ||
		(!info.IsGroup && len(info.Users) == 1 && info.ThreadKey != info.Users[0].InteropMessagingUserFBID) {
		usersArr := zerolog.Arr()
		for _, user := range info.Users {
			usersArr.Dict(zerolog.Dict().Str("igid", user.ID).Int64("fbid", user.InteropMessagingUserFBID))
		}
		ic.UserLogin.Log.Warn().
			Str("main_id", info.ID).
			Str("thread_fbid", info.ThreadFBID).
			Str("thread_id", info.ThreadID).
			Int64("thread_key", info.ThreadKey).
			Bool("is_group", info.IsGroup).
			Str("subtype", string(info.ThreadSubtype)).
			Array("users", usersArr).
			Msg("Unexpected thread identifiers")
	}
	roomType := database.RoomTypeDM
	var name *string
	var avatar *bridgev2.Avatar
	if info.IsGroup || len(info.Users) > 1 {
		roomType = database.RoomTypeDefault
		name = &info.ThreadTitle
		avatar = wrapAvatar(info.ThreadImageURL)
	}
	var mutedUntil *time.Time
	if ptr.Val(info.IsMuted) {
		mutedUntil = &event.MutedForever
	} else if info.IsMuted != nil {
		mutedUntil = &bridgev2.Unmuted
	}
	var tag *event.RoomTag
	if ptr.Val(info.IsPin) {
		tag = ptr.Ptr(event.RoomTagFavourite)
	} else if info.IsPin != nil {
		tag = ptr.Ptr(event.RoomTag(""))
	}
	members := &bridgev2.ChatMemberList{
		IsFull:                     true,
		ExcludeChangesFromTimeline: true,
		TotalMemberCount:           len(info.Users) + 1,
		MemberMap:                  make(bridgev2.ChatMemberMap, len(info.Users)+1),
		PowerLevels: &bridgev2.PowerLevelOverrides{
			Events: map[event.Type]int{
				event.StateRoomName:                powerDefault,
				event.StateRoomAvatar:              powerDefault,
				event.StateTopic:                   powerDefault,
				event.StateBeeperDisappearingTimer: powerDefault,
			},
		},
	}
	nickMap := make(map[int64]*string, len(info.Nicknames))
	for _, nick := range info.Nicknames {
		nickMap[nick.EIMUID] = &nick.Nickname
	}
	addMember := func(member *slidetypes.User) {
		var powerLevel *int
		if info.AdminUserIDs != nil {
			powerLevel = ptr.Ptr(powerDefault)
			if slices.Contains(info.AdminUserIDs, member.ID) {
				powerLevel = ptr.Ptr(powerAdmin)
			}
		}
		members.MemberMap.Add(bridgev2.ChatMember{
			EventSender: ic.makeEventSender(member.InteropMessagingUserFBID),
			Membership:  event.MembershipJoin,
			Nickname:    nickMap[member.InteropMessagingUserFBID],
			PowerLevel:  powerLevel,
			UserInfo:    ic.wrapUserInfo(member),
		})
	}
	for _, member := range info.Users {
		addMember(member)
	}
	addMember(info.Viewer)
	if len(info.Users) == 1 && info.ThreadKey == info.Users[0].InteropMessagingUserFBID {
		members.OtherUserID = metaid.MakeUserID(info.ThreadKey)
	} else if len(info.Users) == 0 && info.ThreadKey == info.Viewer.InteropMessagingUserFBID {
		members.OtherUserID = metaid.MakeUserID(info.ThreadKey)
		members.MemberMap = makeNoteToSelfMembers(members.OtherUserID, ic.wrapUserInfo(info.Viewer))
	}
	return &bridgev2.ChatInfo{
		Name:      name,
		Avatar:    avatar,
		Members:   members,
		Type:      &roomType,
		Disappear: nil, // TODO
		UserLocal: &bridgev2.UserLocalPortalInfo{
			MutedUntil: mutedUntil,
			Tag:        tag,
		},
		MessageRequest: ptr.Ptr(info.SystemFolder == "PENDING" || info.SystemFolder == "SPAM"),
		CanBackfill:    true,
		ExtraUpdates: func(ctx context.Context, portal *bridgev2.Portal) (changed bool) {
			meta := portal.Metadata.(*metaid.PortalMetadata)
			if info.IsGroup {
				meta.ThreadType = table.GROUP_THREAD
			} else {
				meta.ThreadType = table.ONE_TO_ONE
			}
			if meta.IGThreadID != info.ThreadID {
				meta.IGThreadID = info.ThreadID
				changed = true
				err := ic.Main.DB.PutFBIDForIGThread(ctx, info.ThreadID, info.ThreadKey, ic.UserLogin.ID)
				if err != nil {
					zerolog.Ctx(ctx).Err(err).
						Int64("thread_fbid", info.ThreadKey).
						Str("ig_thread_id", info.ThreadID).
						Msg("Failed to save FBID for IG thread")
				}
			}
			if meta.IGID != info.ID {
				meta.IGID = info.ID
				changed = true
				err := ic.Main.DB.PutFBIDForIGChat(ctx, info.ID, info.ThreadKey, ic.UserLogin.ID)
				if err != nil {
					zerolog.Ctx(ctx).Err(err).
						Int64("thread_fbid", info.ThreadKey).
						Str("thread_igid", info.ID).
						Msg("Failed to save FBID for IG chat")
				}
			}
			return
		},
	}
}

func makeNoteToSelfMembers(otherUserID networkid.UserID, info *bridgev2.UserInfo) map[networkid.UserID]bridgev2.ChatMember {
	// For note to self chats, force the user's ghost to be present by having two members
	// where one only has FromMe and the other only has Sender.
	return map[networkid.UserID]bridgev2.ChatMember{
		"":          {EventSender: bridgev2.EventSender{IsFromMe: true}},
		otherUserID: {EventSender: bridgev2.EventSender{Sender: otherUserID}, UserInfo: info},
	}
}
