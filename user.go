// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
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

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/bridge"
	"maunium.net/go/mautrix/bridge/bridgeconfig"
	"maunium.net/go/mautrix/bridge/commands"
	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

var (
	ErrNotConnected = errors.New("not connected")
	ErrNotLoggedIn  = errors.New("not logged in")
)

func (br *MetaBridge) GetUserByMXID(userID id.UserID) *User {
	return br.maybeGetUserByMXID(userID, &userID)
}

func (br *MetaBridge) GetUserByMXIDIfExists(userID id.UserID) *User {
	return br.maybeGetUserByMXID(userID, nil)
}

func (br *MetaBridge) maybeGetUserByMXID(userID id.UserID, userIDPtr *id.UserID) *User {
	if userID == br.Bot.UserID || br.IsGhost(userID) {
		return nil
	}
	br.usersLock.Lock()
	defer br.usersLock.Unlock()

	user, ok := br.usersByMXID[userID]
	if !ok {
		dbUser, err := br.DB.User.GetByMXID(context.TODO(), userID)
		if err != nil {
			br.ZLog.Err(err).Msg("Failed to get user from database")
			return nil
		}
		return br.loadUser(context.TODO(), dbUser, userIDPtr)
	}
	return user
}

func (br *MetaBridge) GetUserByMetaID(id int64) *User {
	br.usersLock.Lock()
	defer br.usersLock.Unlock()

	user, ok := br.usersByMetaID[id]
	if !ok {
		dbUser, err := br.DB.User.GetByMetaID(context.TODO(), id)
		if err != nil {
			br.ZLog.Err(err).Msg("Failed to get user from database")
			return nil
		}
		return br.loadUser(context.TODO(), dbUser, nil)
	}
	return user
}

func (br *MetaBridge) GetAllLoggedInUsers() []*User {
	br.usersLock.Lock()
	defer br.usersLock.Unlock()

	dbUsers, err := br.DB.User.GetAllLoggedIn(context.TODO())
	if err != nil {
		br.ZLog.Err(err).Msg("Error getting all logged in users")
		return nil
	}
	users := make([]*User, len(dbUsers))

	for idx, dbUser := range dbUsers {
		user, ok := br.usersByMXID[dbUser.MXID]
		if !ok {
			user = br.loadUser(context.TODO(), dbUser, nil)
		}
		users[idx] = user
	}
	return users
}

func (br *MetaBridge) loadUser(ctx context.Context, dbUser *database.User, mxid *id.UserID) *User {
	if dbUser == nil {
		if mxid == nil {
			return nil
		}
		dbUser = br.DB.User.New()
		dbUser.MXID = *mxid
		err := dbUser.Insert(ctx)
		if err != nil {
			br.ZLog.Err(err).Msg("Error creating user %s")
			return nil
		}
	}

	user := br.NewUser(dbUser)
	br.usersByMXID[user.MXID] = user
	if user.MetaID != 0 {
		br.usersByMetaID[user.MetaID] = user
	}
	if user.ManagementRoom != "" {
		br.managementRoomsLock.Lock()
		br.managementRooms[user.ManagementRoom] = user
		br.managementRoomsLock.Unlock()
	}
	return user
}

func (br *MetaBridge) NewUser(dbUser *database.User) *User {
	user := &User{
		User:   dbUser,
		bridge: br,
		log:    br.ZLog.With().Stringer("user_id", dbUser.MXID).Logger(),

		PermissionLevel: br.Config.Bridge.Permissions.Get(dbUser.MXID),
	}
	user.Admin = user.PermissionLevel >= bridgeconfig.PermissionLevelAdmin
	user.BridgeState = br.NewBridgeStateQueue(user)
	return user
}

type User struct {
	*database.User

	sync.Mutex

	bridge *MetaBridge
	log    zerolog.Logger

	Admin           bool
	PermissionLevel bridgeconfig.PermissionLevel

	commandState *commands.CommandState

	Client *messagix.Client

	BridgeState     *bridge.BridgeStateQueue
	bridgeStateLock sync.Mutex

	spaceMembershipChecked bool
	spaceCreateLock        sync.Mutex
}

var (
	_ bridge.User              = (*User)(nil)
	_ status.BridgeStateFiller = (*User)(nil)
)

func (user *User) GetPermissionLevel() bridgeconfig.PermissionLevel {
	return user.PermissionLevel
}

func (user *User) IsLoggedIn() bool {
	user.Lock()
	defer user.Unlock()

	return user.Client != nil
}

func (user *User) GetManagementRoomID() id.RoomID {
	return user.ManagementRoom
}

func (user *User) SetManagementRoom(roomID id.RoomID) {
	user.bridge.managementRoomsLock.Lock()
	defer user.bridge.managementRoomsLock.Unlock()

	existing, ok := user.bridge.managementRooms[roomID]
	if ok {
		existing.ManagementRoom = ""
		err := existing.Update(context.TODO())
		if err != nil {
			existing.log.Err(err).Msg("Failed to update user when removing management room")
		}
	}

	user.ManagementRoom = roomID
	user.bridge.managementRooms[user.ManagementRoom] = user
	err := user.Update(context.TODO())
	if err != nil {
		user.log.Error().Err(err).Msg("Error setting management room")
	}
}

func (user *User) GetCommandState() *commands.CommandState {
	return user.commandState
}

func (user *User) SetCommandState(state *commands.CommandState) {
	user.commandState = state
}

func (user *User) GetIDoublePuppet() bridge.DoublePuppet {
	p := user.bridge.GetPuppetByCustomMXID(user.MXID)
	if p == nil || p.CustomIntent() == nil {
		return nil
	}
	return p
}

func (user *User) GetIGhost() bridge.Ghost {
	p := user.bridge.GetPuppetByID(user.MetaID)
	if p == nil {
		return nil
	}
	return p
}

func (user *User) ensureInvited(ctx context.Context, intent *appservice.IntentAPI, roomID id.RoomID, isDirect bool) (ok bool) {
	log := user.log.With().Str("action", "ensure_invited").Stringer("room_id", roomID).Logger()
	if user.bridge.StateStore.IsMembership(ctx, roomID, user.MXID, event.MembershipJoin) {
		ok = true
		return
	}
	extraContent := make(map[string]interface{})
	if isDirect {
		extraContent["is_direct"] = true
	}
	customPuppet := user.bridge.GetPuppetByCustomMXID(user.MXID)
	if customPuppet != nil && customPuppet.CustomIntent() != nil {
		log.Debug().Msg("adding will_auto_accept to invite content")
		extraContent["fi.mau.will_auto_accept"] = true
	} else {
		log.Debug().Msg("NOT adding will_auto_accept to invite content")
	}
	_, err := intent.InviteUser(ctx, roomID, &mautrix.ReqInviteUser{UserID: user.MXID}, extraContent)
	var httpErr mautrix.HTTPError
	if err != nil && errors.As(err, &httpErr) && httpErr.RespError != nil && strings.Contains(httpErr.RespError.Err, "is already in the room") {
		err = user.bridge.StateStore.SetMembership(ctx, roomID, user.MXID, event.MembershipJoin)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to update membership in state store")
		}
		ok = true
		return
	} else if err != nil {
		log.Warn().Err(err).Msg("Failed to invite user to room")
	} else {
		ok = true
	}

	if customPuppet != nil && customPuppet.CustomIntent() != nil {
		log.Debug().Msg("ensuring custom puppet is joined")
		err = customPuppet.CustomIntent().EnsureJoined(ctx, roomID, appservice.EnsureJoinedParams{IgnoreCache: true})
		if err != nil {
			log.Warn().Err(err).Msg("Failed to auto-join custom puppet")
			ok = false
		} else {
			ok = true
		}
	}
	return
}

func (user *User) GetSpaceRoom(ctx context.Context) id.RoomID {
	if !user.bridge.Config.Bridge.PersonalFilteringSpaces {
		return ""
	}

	if len(user.SpaceRoom) == 0 {
		user.spaceCreateLock.Lock()
		defer user.spaceCreateLock.Unlock()
		if len(user.SpaceRoom) > 0 {
			return user.SpaceRoom
		}

		resp, err := user.bridge.Bot.CreateRoom(ctx, &mautrix.ReqCreateRoom{
			Visibility: "private",
			Name:       user.bridge.ProtocolName,
			Topic:      "Your " + user.bridge.ProtocolName + " bridged chats",
			InitialState: []*event.Event{{
				Type: event.StateRoomAvatar,
				Content: event.Content{
					Parsed: &event.RoomAvatarEventContent{
						URL: user.bridge.Config.AppService.Bot.ParsedAvatar,
					},
				},
			}},
			CreationContent: map[string]interface{}{
				"type": event.RoomTypeSpace,
			},
			PowerLevelOverride: &event.PowerLevelsEventContent{
				Users: map[id.UserID]int{
					user.bridge.Bot.UserID: 9001,
					user.MXID:              50,
				},
			},
		})

		if err != nil {
			user.log.Err(err).Msg("Failed to auto-create space room")
		} else {
			user.SpaceRoom = resp.RoomID
			err = user.Update(context.TODO())
			if err != nil {
				user.log.Err(err).Msg("Failed to save user in database after creating space room")
			}
			user.ensureInvited(ctx, user.bridge.Bot, user.SpaceRoom, false)
		}
	} else if !user.spaceMembershipChecked {
		user.ensureInvited(ctx, user.bridge.Bot, user.SpaceRoom, false)
	}
	user.spaceMembershipChecked = true

	return user.SpaceRoom
}

func (user *User) syncChatDoublePuppetDetails(portal *Portal, justCreated bool) {
	doublePuppet := portal.bridge.GetPuppetByCustomMXID(user.MXID)
	if doublePuppet == nil {
		return
	}
	if doublePuppet == nil || doublePuppet.CustomIntent() == nil || len(portal.MXID) == 0 {
		return
	}

	// TODO: Get chat setting and sync them here
	//if justCreated || !user.bridge.Config.Bridge.TagOnlyOnCreate {
	//	chat, err := user.Client.Store.ChatSettings.GetChatSettings(portal.Key().ChatID)
	//	if err != nil {
	//		user.log.Warn().Err(err).Msgf("Failed to get settings of %s", portal.Key().ChatID)
	//		return
	//	}
	//	intent := doublePuppet.CustomIntent()
	//	if portal.Key.JID == types.StatusBroadcastJID && justCreated {
	//		if user.bridge.Config.Bridge.MuteStatusBroadcast {
	//			user.updateChatMute(intent, portal, time.Now().Add(365*24*time.Hour))
	//		}
	//		if len(user.bridge.Config.Bridge.StatusBroadcastTag) > 0 {
	//			user.updateChatTag(intent, portal, user.bridge.Config.Bridge.StatusBroadcastTag, true)
	//		}
	//		return
	//	} else if !chat.Found {
	//		return
	//	}
	//	user.updateChatMute(intent, portal, chat.MutedUntil)
	//	user.updateChatTag(intent, portal, user.bridge.Config.Bridge.ArchiveTag, chat.Archived)
	//	user.updateChatTag(intent, portal, user.bridge.Config.Bridge.PinnedTag, chat.Pinned)
	//}
}

func (user *User) GetMXID() id.UserID {
	return user.MXID
}

var MessagixPlatform types.Platform

func (user *User) Connect() {
	user.Lock()
	defer user.Unlock()
	user.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	var err error
	user.Client, err = user.unlockedConnectWithCookies(user.Cookies)
	if err != nil {
		user.log.Error().Err(err).Msg("Failed to connect")
		user.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      "meta-connect-error",
			Message:    err.Error(),
		})
	}
}
func (user *User) Login(ctx context.Context, cookies cookies.Cookies) error {
	user.Lock()
	defer user.Unlock()
	cli, err := user.unlockedConnectWithCookies(cookies)
	if err != nil {
		return err
	}
	user.MetaID = cookies.GetUserID()
	user.Client = cli
	user.Cookies = cookies
	err = user.Update(ctx)
	if err != nil {
		user.log.Err(err).Msg("Failed to update user")
		return err
	}
	return nil
}

func (user *User) unlockedConnectWithCookies(cookies cookies.Cookies) (*messagix.Client, error) {
	if cookies == nil {
		return nil, fmt.Errorf("no cookies provided")
	}

	user.log.Debug().Msg("Connecting to Meta")
	log := user.log.With().Str("component", "messagix").Logger()
	cli, err := messagix.NewClient(MessagixPlatform, cookies, log, "")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare client: %w", err)
	}
	cli.SetEventHandler(user.eventHandler)
	err = cli.Connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return cli, nil
}

func (br *MetaBridge) StartUsers() {
	br.ZLog.Debug().Msg("Starting users")

	usersWithToken := br.GetAllLoggedInUsers()
	for _, u := range usersWithToken {
		go u.Connect()
	}
	if len(usersWithToken) == 0 {
		br.SendGlobalBridgeState(status.BridgeState{StateEvent: status.StateUnconfigured}.Fill(nil))
	}

	br.ZLog.Debug().Msg("Starting custom puppets")
	for _, customPuppet := range br.GetAllPuppetsWithCustomMXID() {
		go func(puppet *Puppet) {
			br.ZLog.Debug().Stringer("user_id", puppet.CustomMXID).Msg("Starting custom puppet")

			if err := puppet.StartCustomMXID(true); err != nil {
				puppet.log.Error().Err(err).Msg("Failed to start custom puppet")
			}
		}(customPuppet)
	}
}

func (user *User) handleTable(table *table.LSTable) {
	ctx := user.log.With().Str("action", "handle table").Logger().WithContext(context.TODO())
	for _, contact := range table.LSVerifyContactRowExists {
		user.bridge.GetPuppetByID(contact.ContactId).UpdateInfo(ctx, contact)
	}
	for _, thread := range table.LSDeleteThenInsertThread {
		portal := user.GetPortalByThreadID(thread.ThreadKey, thread.ThreadType)
		portal.UpdateInfo(ctx, thread)
		if portal.MXID == "" {
			err := portal.CreateMatrixRoom(ctx, user)
			if err != nil {
				user.log.Err(err).Int64("thread_id", thread.ThreadKey).Msg("Failed to create matrix room")
			}
		} else {
			portal.ensureUserInvited(ctx, user)
			go portal.addToPersonalSpace(portal.log.WithContext(context.TODO()), user)
		}
	}
	for _, participant := range table.LSAddParticipantIdToGroupThread {
		portal := user.GetExistingPortalByThreadID(participant.ThreadKey)
		if portal != nil && portal.MXID != "" && !portal.IsPrivateChat() {
			puppet := user.bridge.GetPuppetByID(participant.ContactId)
			err := puppet.IntentFor(portal).EnsureJoined(ctx, portal.MXID)
			if err != nil {
				user.log.Err(err).
					Int64("thread_id", participant.ThreadKey).
					Int64("contact_id", participant.ContactId).
					Msg("Failed to ensure user is joined to thread")
			}
		}
	}
	for _, participant := range table.LSRemoveParticipantFromThread {
		portal := user.GetExistingPortalByThreadID(participant.ThreadKey)
		if portal != nil && portal.MXID != "" {
			puppet := user.bridge.GetPuppetByID(participant.ParticipantId)
			_, err := puppet.IntentFor(portal).LeaveRoom(ctx, portal.MXID)
			if err != nil {
				user.log.Err(err).
					Int64("thread_id", participant.ThreadKey).
					Int64("contact_id", participant.ParticipantId).
					Msg("Failed to leave user from thread")
			}
		}
	}
	for _, thread := range table.LSVerifyThreadExists {
		portal := user.GetPortalByThreadID(thread.ThreadKey, thread.ThreadType)
		// TODO if there's some way to fetch thread info, the portal could be created here
		if portal.MXID != "" {
			portal.ensureUserInvited(ctx, user)
			go portal.addToPersonalSpace(ctx, user)
		}
	}
	for _, msg := range table.LSInsertMessage {
		user.handlePortalEvent(msg.ThreadKey, msg)
	}
	for _, msg := range table.LSDeleteMessage {
		user.handlePortalEvent(msg.ThreadKey, msg)
	}
	for _, msg := range table.LSUpsertReaction {
		user.handlePortalEvent(msg.ThreadKey, msg)
	}
	for _, msg := range table.LSDeleteReaction {
		user.handlePortalEvent(msg.ThreadKey, msg)
	}
}

func (user *User) handlePortalEvent(threadKey int64, evt any) {
	portal := user.GetExistingPortalByThreadID(threadKey)
	if portal != nil {
		portal.metaMessages <- portalMetaMessage{user: user, evt: evt}
	} else {
		user.log.Warn().
			Int64("thread_id", threadKey).
			Type("evt_type", evt).
			Msg("Received event for unknown thread")
	}
}

func (user *User) GetRemoteID() string {
	return strconv.FormatInt(user.MetaID, 10)
}

func (user *User) GetRemoteName() string {
	if user.MetaID != 0 {
		puppet := user.bridge.GetPuppetByID(user.MetaID)
		if puppet != nil {
			return puppet.Name
		}
		return user.GetRemoteID()
	}
	return ""
}

func (user *User) FillBridgeState(state status.BridgeState) status.BridgeState {
	return state
}

func (user *User) eventHandler(rawEvt any) {
	switch evt := rawEvt.(type) {
	case *messagix.Event_PublishResponse:
		user.log.Trace().Any("table", &evt.Table).Msg("Got new event")
		user.handleTable(evt.Table)
	case *messagix.Event_Ready:
		puppet := user.bridge.GetPuppetByID(user.MetaID)
		puppet.UpdateInfo(context.TODO(), evt.CurrentUser)
		user.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
		user.tryAutomaticDoublePuppeting()
		user.handleTable(evt.Table)
	case *messagix.Event_SocketError:
		user.BridgeState.Send(status.BridgeState{StateEvent: status.StateTransientDisconnect, Message: evt.Err.Error()})
	default:
		user.log.Warn().Type("event_type", evt).Msg("Unrecognized event type from messagix")
	}
}

func (user *User) GetExistingPortalByThreadID(threadID int64) *Portal {
	return user.GetPortalByThreadID(threadID, table.UNKNOWN_THREAD_TYPE)
}

func (user *User) GetPortalByThreadID(threadID int64, threadType table.ThreadType) *Portal {
	return user.bridge.GetPortalByThreadID(database.PortalKey{
		ThreadID: threadID,
		Receiver: user.MetaID,
	}, threadType)
}

func (user *User) Disconnect() error {
	user.Lock()
	defer user.Unlock()
	if user.Client != nil {
		user.Client.Disconnect()
	}
	return nil
}

func (user *User) DeleteSession() {
	user.Lock()
	defer user.Unlock()
	if user.Client != nil {
		user.Client.Disconnect()
	}
	user.Client = nil
	user.Cookies = nil
	user.MetaID = 0
	doublePuppet := user.bridge.GetPuppetByCustomMXID(user.MXID)
	if doublePuppet != nil {
		doublePuppet.ClearCustomMXID()
	}
	err := user.Update(context.TODO())
	if err != nil {
		user.log.Err(err).Msg("Failed to delete session")
	}
}

func (user *User) AddDirectChat(ctx context.Context, roomID id.RoomID, userID id.UserID) {
	if !user.bridge.Config.Bridge.SyncDirectChatList {
		return
	}

	puppet := user.bridge.GetPuppetByMXID(user.MXID)
	if puppet == nil {
		return
	}

	intent := puppet.CustomIntent()
	if intent == nil {
		return
	}

	user.log.Debug().Msg("Updating m.direct list on homeserver")
	chats := map[id.UserID][]id.RoomID{}
	err := intent.GetAccountData(ctx, event.AccountDataDirectChats.Type, &chats)
	if err != nil {
		user.log.Warn().Err(err).Msg("Failed to get m.direct event to update it")
		return
	}
	chats[userID] = []id.RoomID{roomID}

	err = intent.SetAccountData(ctx, event.AccountDataDirectChats.Type, &chats)
	if err != nil {
		user.log.Warn().Err(err).Msg("Failed to update m.direct event")
	}
}
