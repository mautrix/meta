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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"golang.org/x/exp/maps"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/bridge"
	"maunium.net/go/mautrix/bridge/bridgeconfig"
	"maunium.net/go/mautrix/bridge/commands"
	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/pushrules"

	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

const maxConnectAttempts = 5

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

const (
	WADisconnected             status.BridgeStateErrorCode = "wa-transient-disconnect"
	WAPermanentError           status.BridgeStateErrorCode = "wa-unknown-permanent-error"
	MetaConnectionUnauthorized status.BridgeStateErrorCode = "meta-connection-unauthorized"
	MetaPermanentError         status.BridgeStateErrorCode = "meta-unknown-permanent-error"
	MetaCookieRemoved          status.BridgeStateErrorCode = "meta-cookie-removed"
	MetaConnectError           status.BridgeStateErrorCode = "meta-connect-error"
	MetaTransientDisconnect    status.BridgeStateErrorCode = "meta-transient-disconnect"
	IGChallengeRequired        status.BridgeStateErrorCode = "ig-challenge-required"
	IGChallengeRequiredMaybe   status.BridgeStateErrorCode = "ig-challenge-required-maybe"
	MetaServerUnavailable      status.BridgeStateErrorCode = "meta-server-unavailable"
	IGConsentRequired          status.BridgeStateErrorCode = "ig-consent-required"
)

func init() {
	status.BridgeStateHumanErrors.Update(status.BridgeStateErrorMap{
		WADisconnected:             "Disconnected from encrypted chat server. Trying to reconnect.",
		MetaTransientDisconnect:    "Disconnected from server, trying to reconnect",
		MetaConnectionUnauthorized: "Logged out, please relogin to continue",
		MetaCookieRemoved:          "Logged out, please relogin to continue",
		IGChallengeRequired:        "Challenge required, please check the Instagram website to continue",
		IGChallengeRequiredMaybe:   "Connection refused, please check the Instagram website to continue",
		IGConsentRequired:          "Consent required, please check the Instagram website to continue",
		MetaServerUnavailable:      "Connection refused by server",
		MetaConnectError:           "Unknown connection error",
	})
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

	WADevice        *store.Device
	E2EEClient      *whatsmeow.Client
	e2eeConnectLock sync.Mutex

	BridgeState     *bridge.BridgeStateQueue
	bridgeStateLock sync.Mutex

	waState   status.BridgeState
	metaState status.BridgeState

	spaceMembershipChecked bool
	spaceCreateLock        sync.Mutex
	mgmtCreateLock         sync.Mutex

	stopBackfillTask atomic.Pointer[context.CancelFunc]

	InboxPagesFetched int
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

func (user *User) IsE2EEConnected() bool {
	return user.E2EEClient != nil && user.E2EEClient.IsConnected()
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

func (user *User) sendMarkdownBridgeAlert(ctx context.Context, formatString string, args ...interface{}) {
	if user.bridge.Config.Bridge.DisableBridgeAlerts {
		return
	}
	notice := fmt.Sprintf(formatString, args...)
	content := format.RenderMarkdown(notice, true, false)
	_, err := user.bridge.Bot.SendMessageEvent(ctx, user.GetManagementRoom(ctx), event.EventMessage, content)
	if err != nil {
		user.log.Err(err).Str("notice_text", notice).Msg("Failed to send bridge alert")
	}
}

func (user *User) GetManagementRoom(ctx context.Context) id.RoomID {
	if len(user.ManagementRoom) == 0 {
		user.mgmtCreateLock.Lock()
		defer user.mgmtCreateLock.Unlock()
		if len(user.ManagementRoom) > 0 {
			return user.ManagementRoom
		}
		creationContent := make(map[string]interface{})
		if !user.bridge.Config.Bridge.FederateRooms {
			creationContent["m.federate"] = false
		}
		resp, err := user.bridge.Bot.CreateRoom(ctx, &mautrix.ReqCreateRoom{
			Topic:           user.bridge.ProtocolName + " bridge notices",
			IsDirect:        true,
			CreationContent: creationContent,
		})
		if err != nil {
			user.log.Err(err).Msg("Failed to auto-create management room")
		} else {
			user.SetManagementRoom(resp.RoomID)
		}
	}
	return user.ManagementRoom
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

func (user *User) updateChatMute(ctx context.Context, portal *Portal, mutedUntil int64, isNew bool) {
	if portal == nil || len(portal.MXID) == 0 || user.bridge.Config.Bridge.MuteBridging == "never" {
		// If the chat isn't bridged or the mute bridging option is never, don't do anything
		return
	} else if !isNew && user.bridge.Config.Bridge.MuteBridging != "always" {
		// If the chat isn't new and the mute bridging option is on-create, don't do anything
		return
	} else if mutedUntil == 0 && isNew {
		// No need to unmute new chats
		return
	}
	doublePuppet := user.bridge.GetPuppetByCustomMXID(user.MXID)
	if doublePuppet == nil || doublePuppet.CustomIntent() == nil {
		return
	}
	log := user.log.With().
		Str("action", "update chat mute").
		Int64("thread_id", portal.ThreadID).
		Stringer("portal_mxid", portal.MXID).
		Int64("muted_until", mutedUntil).
		Logger()
	intent := doublePuppet.CustomIntent()
	var err error
	now := time.Now().UnixMilli()
	if mutedUntil >= 0 && mutedUntil < now {
		log.Debug().Msg("Unmuting chat")
		err = intent.DeletePushRule(ctx, "global", pushrules.RoomRule, string(portal.MXID))
	} else {
		log.Debug().Msg("Muting chat")
		err = intent.PutPushRule(ctx, "global", pushrules.RoomRule, string(portal.MXID), &mautrix.ReqPutPushRule{
			Actions: []pushrules.PushActionType{pushrules.ActionDontNotify},
		})
	}
	if err != nil && !errors.Is(err, mautrix.MNotFound) {
		log.Err(err).Msg("Failed to update push rule through double puppet")
	}
}

func (user *User) GetMXID() id.UserID {
	return user.MXID
}

var MessagixPlatform types.Platform

func isNotNetworkError(err error) bool {
	if errors.Is(err, messagix.ErrTokenInvalidated) ||
		errors.Is(err, messagix.ErrChallengeRequired) ||
		errors.Is(err, messagix.ErrConsentRequired) {
		return true
	}
	lsErr := &messagix.LSErrorResponse{}
	if errors.As(err, &lsErr) {
		return true
	}
	return false
}

func (user *User) Connect() {
	user.Lock()
	defer user.Unlock()
	user.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	var err error
	user.Client, err = user.unlockedConnectWithCookies(user.Cookies)
	if err != nil {
		user.log.Error().Err(err).Msg("Failed to connect")
		if errors.Is(err, messagix.ErrTokenInvalidated) {
			user.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      MetaCookieRemoved,
			})
			// TODO clear cookies?
		} else if errors.Is(err, messagix.ErrChallengeRequired) {
			user.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGChallengeRequired,
			})
		} else if errors.Is(err, messagix.ErrConsentRequired) {
			user.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGConsentRequired,
			})
		} else if lsErr := (&messagix.LSErrorResponse{}); errors.As(err, &lsErr) {
			user.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      status.BridgeStateErrorCode(fmt.Sprintf("meta-lserror-%d", lsErr.ErrorCode)),
				Message:    lsErr.Error(),
			})
		} else {
			user.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaConnectError,
			})
		}
		go user.sendMarkdownBridgeAlert(context.TODO(), "Failed to connect to %s: %v", user.bridge.ProtocolName, err)
	}
}

func (user *User) Login(ctx context.Context, cookies cookies.Cookies) error {
	user.Lock()
	defer user.Unlock()
	cli, err := user.unlockedConnectWithCookies(cookies)
	if err != nil {
		return err
	}
	user.Client = cli
	user.Cookies = cookies
	err = user.Update(ctx)
	if err != nil {
		user.log.Err(err).Msg("Failed to update user")
		return err
	}
	return nil
}

type respGetProxy struct {
	ProxyURL string `json:"proxy_url"`
}

func (user *User) getProxy(reason string) (string, error) {
	if user.bridge.Config.Meta.GetProxyFrom == "" {
		return user.bridge.Config.Meta.Proxy, nil
	}
	parsed, err := url.Parse(user.bridge.Config.Meta.GetProxyFrom)
	if err != nil {
		return "", fmt.Errorf("failed to parse address: %w", err)
	}
	q := parsed.Query()
	q.Set("reason", reason)
	parsed.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("User-Agent", mautrix.DefaultUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	} else if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	var respData respGetProxy
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return respData.ProxyURL, nil
}

func (user *User) unlockedConnectWithCookies(cookies cookies.Cookies) (*messagix.Client, error) {
	if cookies == nil {
		return nil, fmt.Errorf("no cookies provided")
	}

	log := user.log.With().Str("component", "messagix").Logger()
	proxyAddr, err := user.getProxy("connect")
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	user.log.Debug().Msg("Connecting to Meta")
	// TODO set proxy for media client?
	attempts := 1
	var cli *messagix.Client
	for {
		cli, err = messagix.NewClient(MessagixPlatform, cookies, log, proxyAddr)
		if err != nil {
			if attempts > maxConnectAttempts || isNotNetworkError(err) {
				return nil, fmt.Errorf("failed to prepare client: %w", err)
			}
			attempts += 1
			continue
		}
		break
	}

	if user.bridge.Config.Meta.GetProxyFrom != "" {
		cli.GetNewProxy = user.getProxy
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

func (user *User) handleTable(tbl *table.LSTable) {
	log := user.log.With().Str("action", "handle table").Logger()
	ctx := log.WithContext(context.TODO())
	for _, contact := range tbl.LSDeleteThenInsertContact {
		user.bridge.GetPuppetByID(contact.Id).UpdateInfo(ctx, contact)
	}
	for _, contact := range tbl.LSVerifyContactRowExists {
		user.bridge.GetPuppetByID(contact.ContactId).UpdateInfo(ctx, contact)
	}
	for _, thread := range tbl.LSDeleteThenInsertThread {
		// TODO handle last read watermark in here?
		portal := user.GetPortalByThreadID(thread.ThreadKey, thread.ThreadType)
		portal.UpdateInfo(ctx, thread)
		if portal.MXID == "" {
			err := portal.CreateMatrixRoom(ctx, user)
			if err != nil {
				log.Err(err).Int64("thread_id", thread.ThreadKey).Msg("Failed to create matrix room")
			}
			go user.updateChatMute(ctx, portal, thread.MuteExpireTimeMs, true)
		} else {
			portal.ensureUserInvited(ctx, user)
			go portal.addToPersonalSpace(portal.log.WithContext(context.TODO()), user)
			go user.updateChatMute(ctx, portal, thread.MuteExpireTimeMs, false)
		}
	}
	for _, participant := range tbl.LSAddParticipantIdToGroupThread {
		portal := user.GetExistingPortalByThreadID(participant.ThreadKey)
		if portal != nil && portal.MXID != "" && !portal.IsPrivateChat() {
			puppet := user.bridge.GetPuppetByID(participant.ContactId)
			err := puppet.IntentFor(portal).EnsureJoined(ctx, portal.MXID)
			if err != nil {
				log.Err(err).
					Int64("thread_id", participant.ThreadKey).
					Int64("contact_id", participant.ContactId).
					Msg("Failed to ensure user is joined to thread")
			}
		}
	}
	for _, participant := range tbl.LSRemoveParticipantFromThread {
		portal := user.GetExistingPortalByThreadID(participant.ThreadKey)
		if portal != nil && portal.MXID != "" {
			puppet := user.bridge.GetPuppetByID(participant.ParticipantId)
			_, err := puppet.IntentFor(portal).LeaveRoom(ctx, portal.MXID)
			if err != nil {
				log.Err(err).
					Int64("thread_id", participant.ThreadKey).
					Int64("contact_id", participant.ParticipantId).
					Msg("Failed to leave user from thread")
			}
		}
	}
	for _, thread := range tbl.LSVerifyThreadExists {
		portal := user.GetPortalByThreadID(thread.ThreadKey, thread.ThreadType)
		if portal.MXID != "" {
			portal.ensureUserInvited(ctx, user)
			go portal.addToPersonalSpace(ctx, user)
		} else if !portal.fetchAttempted.Swap(true) {
			log.Debug().Int64("thread_id", thread.ThreadKey).Msg("Sending create thread request for unknown thread in verifyThreadExists")
			go func(thread *table.LSVerifyThreadExists) {
				resp, err := user.Client.ExecuteTasks(
					&socket.CreateThreadTask{
						ThreadFBID:                thread.ThreadKey,
						ForceUpsert:               0,
						UseOpenMessengerTransport: 0,
						SyncGroup:                 1,
						MetadataOnly:              0,
						PreviewOnly:               0,
					},
				)
				if err != nil {
					log.Err(err).Int64("thread_id", thread.ThreadKey).Msg("Failed to execute create thread task for verifyThreadExists of unknown thread")
				} else {
					log.Debug().Int64("thread_id", thread.ThreadKey).Msg("Sent create thread request for unknown thread in verifyThreadExists")
					log.Trace().Any("resp_data", resp).Int64("thread_id", thread.ThreadKey).Msg("Create thread response")
				}
			}(thread)
		} else {
			log.Warn().Int64("thread_id", thread.ThreadKey).Msg("Portal doesn't exist in verifyThreadExists, but fetch was already attempted")
		}
	}
	for _, mute := range tbl.LSUpdateThreadMuteSetting {
		portal := user.GetExistingPortalByThreadID(mute.ThreadKey)
		go user.updateChatMute(ctx, portal, mute.MuteExpireTimeMS, false)
	}
	upsert, insert := tbl.WrapMessages()
	handlePortalEvents(user, maps.Values(upsert))
	handlePortalEvents(user, tbl.LSUpdateExistingMessageRange)
	handlePortalEvents(user, insert)
	for _, msg := range tbl.LSEditMessage {
		user.handleEditEvent(ctx, msg)
	}
	handlePortalEvents(user, tbl.LSSyncUpdateThreadName)
	handlePortalEvents(user, tbl.LSSetThreadImageURL)
	handlePortalEvents(user, tbl.LSUpdateReadReceipt)
	handlePortalEvents(user, tbl.LSMarkThreadRead)
	handlePortalEvents(user, tbl.LSUpdateTypingIndicator)
	handlePortalEvents(user, tbl.LSDeleteMessage)
	handlePortalEvents(user, tbl.LSDeleteThenInsertMessage)
	handlePortalEvents(user, tbl.LSUpsertReaction)
	handlePortalEvents(user, tbl.LSDeleteReaction)
	user.requestMoreInbox(ctx, tbl.LSUpsertInboxThreadsRange)
}

func (user *User) requestMoreInbox(ctx context.Context, itrs []*table.LSUpsertInboxThreadsRange) {
	if !user.bridge.Config.Bridge.Backfill.Enabled {
		return
	}
	maxInboxPages := user.bridge.Config.Bridge.Backfill.InboxFetchPages
	if len(itrs) == 0 || user.InboxFetched || maxInboxPages == 0 {
		return
	}
	log := zerolog.Ctx(ctx)
	if len(itrs) > 1 {
		log.Warn().Any("thread_ranges", itrs).Msg("Got multiple thread ranges in upsertInboxThreadsRange")
	}
	itr := itrs[0]
	user.InboxPagesFetched++
	reachedPageLimit := maxInboxPages > 0 && user.InboxPagesFetched > maxInboxPages
	reachingOnePageLimit := maxInboxPages == 1 && user.InboxPagesFetched >= 1
	logEvt := log.Debug().
		Int("fetched_pages", user.InboxPagesFetched).
		Bool("has_more_before", itr.HasMoreBefore).
		Bool("reached_page_limit", reachedPageLimit).
		Int64("min_thread_key", itr.MinThreadKey).
		Int64("min_last_activity_timestamp_ms", itr.MinLastActivityTimestampMs)
	shouldFetchMore := itr.HasMoreBefore && !reachedPageLimit
	if !shouldFetchMore || reachingOnePageLimit {
		if !shouldFetchMore {
			logEvt.Msg("Finished fetching threads")
		} else {
			log.Debug().Msg("Marking inbox as fetched before requesting extra page of threads")
		}
		user.InboxFetched = true
		err := user.Update(ctx)
		if err != nil {
			log.Err(err).Msg("Failed to save user after marking inbox as fetched")
		}
	}
	if !shouldFetchMore {
		return
	}
	logEvt.Msg("Requesting more threads")
	resp, err := user.Client.ExecuteTasks(&socket.FetchThreadsTask{
		ReferenceThreadKey:         itr.MinThreadKey,
		ReferenceActivityTimestamp: itr.MinLastActivityTimestampMs,
		Cursor:                     user.Client.SyncManager.GetCursor(1),
		SyncGroup:                  1,
	})
	log.Trace().Any("resp", resp).Msg("Fetch threads response data")
	if err != nil {
		log.Err(err).Msg("Failed to fetch more threads")
	} else {
		log.Debug().Msg("Sent more threads request")
	}
}

type ThreadKeyable interface {
	GetThreadKey() int64
}

func handlePortalEvents[T ThreadKeyable](user *User, msgs []T) {
	for _, msg := range msgs {
		user.handlePortalEvent(msg.GetThreadKey(), msg)
	}
}

func (user *User) handleEditEvent(ctx context.Context, evt *table.LSEditMessage) {
	log := zerolog.Ctx(ctx).With().Str("message_id", evt.MessageID).Logger()
	portalKey, err := user.bridge.DB.Message.FindEditTargetPortal(ctx, evt.MessageID, user.MetaID)
	if err != nil {
		log.Err(err).Msg("Failed to get portal of edited message")
		return
	} else if portalKey.ThreadID == 0 {
		log.Warn().Msg("Edit target message not found")
		return
	}
	portal := user.bridge.GetExistingPortalByThreadID(portalKey)
	if portal == nil {
		log.Warn().Int64("thread_id", portalKey.ThreadID).Msg("Portal for edit target message not found")
		return
	}
	portal.metaMessages <- portalMetaMessage{user: user, evt: evt}
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
	if state.StateEvent == status.StateConnected {
		if user.waState.StateEvent != "" && user.waState.StateEvent != status.StateConnected {
			state = user.waState
		}
		if user.metaState.StateEvent != "" && user.metaState.StateEvent != status.StateConnected {
			state = user.metaState
		}
	}
	return state
}

func (user *User) connectE2EE() error {
	user.e2eeConnectLock.Lock()
	defer user.e2eeConnectLock.Unlock()
	if user.E2EEClient != nil {
		return fmt.Errorf("already connected to e2ee")
	}
	var err error
	if user.WADevice == nil && user.WADeviceID != 0 {
		user.WADevice, err = user.bridge.DeviceStore.GetDevice(waTypes.JID{User: strconv.FormatInt(user.MetaID, 10), Device: user.WADeviceID, Server: waTypes.MessengerServer})
		if err != nil {
			return fmt.Errorf("failed to get whatsmeow device: %w", err)
		} else if user.WADevice == nil {
			user.log.Warn().Uint16("device_id", user.WADeviceID).Msg("Existing device not found in store")
		}
	}
	isNew := false
	if user.WADevice == nil {
		isNew = true
		user.WADevice = user.bridge.DeviceStore.NewDevice()
	}
	user.Client.SetDevice(user.WADevice)

	ctx := user.log.With().Str("component", "e2ee").Logger().WithContext(context.TODO())
	if isNew {
		user.log.Info().Msg("Registering new e2ee device")
		err = user.Client.RegisterE2EE(ctx, user.MetaID)
		if err != nil {
			return fmt.Errorf("failed to register e2ee device: %w", err)
		}
		user.WADeviceID = user.WADevice.ID.Device
		err = user.WADevice.Save()
		if err != nil {
			return fmt.Errorf("failed to save whatsmeow device store: %w", err)
		}
		err = user.Update(ctx)
		if err != nil {
			return fmt.Errorf("failed to save device ID to user: %w", err)
		}
	}
	user.E2EEClient = user.Client.PrepareE2EEClient()
	user.E2EEClient.AddEventHandler(user.e2eeEventHandler)
	err = user.E2EEClient.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to e2ee socket: %w", err)
	}
	return nil
}

func (user *User) e2eeEventHandler(rawEvt any) {
	switch evt := rawEvt.(type) {
	case *events.FBMessage:
		log := user.log.With().
			Str("action", "handle whatsapp message").
			Stringer("chat_jid", evt.Info.Chat).
			Stringer("sender_jid", evt.Info.Sender).
			Str("message_id", evt.Info.ID).
			Logger()
		ctx := log.WithContext(context.TODO())
		threadID := int64(evt.Info.Chat.UserInt())
		if threadID == 0 {
			log.Warn().Msg("Ignoring encrypted message with unsupported jid")
			return
		}
		var expectedType table.ThreadType
		switch evt.Info.Chat.Server {
		case waTypes.GroupServer:
			expectedType = table.ENCRYPTED_OVER_WA_GROUP
		case waTypes.MessengerServer, waTypes.DefaultUserServer:
			expectedType = table.ENCRYPTED_OVER_WA_ONE_TO_ONE
		default:
			log.Warn().Msg("Unexpected chat server in encrypted message")
			return
		}
		portal := user.GetPortalByThreadID(threadID, expectedType)
		changed := false
		if portal.ThreadType != expectedType {
			log.Info().
				Int64("old_thread_type", int64(portal.ThreadType)).
				Int64("new_thread_type", int64(expectedType)).
				Msg("Updating thread type")
			portal.ThreadType = expectedType
			changed = true
		}
		if portal.WhatsAppServer != evt.Info.Chat.Server {
			log.Info().
				Str("old_server", portal.WhatsAppServer).
				Str("new_server", evt.Info.Chat.Server).
				Msg("Updating WhatsApp server")
			portal.WhatsAppServer = evt.Info.Chat.Server
			changed = true
		}
		if changed {
			err := portal.Update(ctx)
			if err != nil {
				log.Err(err).Msg("Failed to update portal")
			}
		}
		portal.metaMessages <- portalMetaMessage{user: user, evt: evt}
	case *events.Receipt:
		portal := user.GetExistingPortalByThreadID(int64(evt.Chat.UserInt()))
		if portal != nil {
			portal.metaMessages <- portalMetaMessage{user: user, evt: evt}
		}
	case *events.Connected:
		user.log.Debug().Msg("Connected to WhatsApp socket")
		user.waState = status.BridgeState{StateEvent: status.StateConnected}
		user.BridgeState.Send(user.waState)
	case *events.Disconnected:
		user.log.Debug().Msg("Disconnected from WhatsApp socket")
		user.waState = status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      WADisconnected,
		}
		user.BridgeState.Send(user.waState)
	case events.PermanentDisconnect:
		user.waState = status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      WAPermanentError,
			Message:    evt.PermanentDisconnectDescription(),
		}
		user.BridgeState.Send(user.waState)
		go user.sendMarkdownBridgeAlert(context.TODO(), "Error in WhatsApp connection: %s", evt.PermanentDisconnectDescription())
	default:
		user.log.Debug().Type("event_type", rawEvt).Msg("Unhandled WhatsApp event")
	}
}

func (user *User) eventHandler(rawEvt any) {
	switch evt := rawEvt.(type) {
	case *messagix.Event_PublishResponse:
		user.log.Trace().Any("table", &evt.Table).Msg("Got new event")
		user.handleTable(evt.Table)
	case *messagix.Event_Ready:
		var newFBID int64
		// TODO figure out why the contact IDs for self is different than the fbid in the ready event
		for _, row := range evt.Table.LSVerifyContactRowExists {
			if row.IsSelf && row.ContactId != newFBID {
				if newFBID != 0 {
					// Hopefully this won't happen
					user.log.Warn().Int64("prev_fbid", newFBID).Int64("new_fbid", row.ContactId).Msg("Got multiple fbids for self")
				} else {
					user.log.Debug().Int64("fbid", row.ContactId).Msg("Found own fbid")
				}
				newFBID = row.ContactId
			}
		}
		if newFBID == 0 {
			newFBID = evt.CurrentUser.GetFBID()
			user.log.Warn().Int64("fbid", newFBID).Msg("Own contact entry not found, falling back to fbid in current user object")
		}
		if user.MetaID != newFBID {
			user.bridge.usersLock.Lock()
			user.MetaID = newFBID
			// TODO check if there's another user?
			user.bridge.usersByMetaID[user.MetaID] = user
			user.bridge.usersLock.Unlock()
			err := user.Update(context.TODO())
			if err != nil {
				user.log.Err(err).Msg("Failed to save user after getting meta ID")
			}
		}
		puppet := user.bridge.GetPuppetByID(user.MetaID)
		puppet.UpdateInfo(context.TODO(), evt.CurrentUser)
		user.log.Debug().Msg("Initial connect to Meta socket completed")
		user.metaState = status.BridgeState{StateEvent: status.StateConnected}
		user.BridgeState.Send(user.metaState)
		user.tryAutomaticDoublePuppeting()
		user.handleTable(evt.Table)
		if user.bridge.Config.Meta.Mode.IsMessenger() || user.bridge.Config.Meta.IGE2EE {
			go func() {
				err := user.connectE2EE()
				if err != nil {
					user.log.Err(err).Msg("Error connecting to e2ee")
				}
			}()
		}
		go user.BackfillLoop()
	case *messagix.Event_SocketError:
		user.log.Debug().Err(evt.Err).Msg("Disconnected from Meta socket")
		user.metaState = status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      MetaTransientDisconnect,
		}
		user.BridgeState.Send(user.metaState)
	case *messagix.Event_Reconnected:
		user.log.Debug().Msg("Reconnected to Meta socket")
		user.metaState = status.BridgeState{StateEvent: status.StateConnected}
		user.BridgeState.Send(user.metaState)
	case *messagix.Event_PermanentError:
		if errors.Is(evt.Err, messagix.CONNECTION_REFUSED_UNAUTHORIZED) {
			user.metaState = status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      MetaConnectionUnauthorized,
			}
		} else if errors.Is(evt.Err, messagix.CONNECTION_REFUSED_SERVER_UNAVAILABLE) {
			if user.bridge.Config.Meta.Mode.IsMessenger() {
				user.metaState = status.BridgeState{
					StateEvent: status.StateBadCredentials,
					Error:      MetaServerUnavailable,
				}
			} else {
				user.metaState = status.BridgeState{
					StateEvent: status.StateBadCredentials,
					Error:      IGChallengeRequiredMaybe,
				}
			}
		} else {
			user.metaState = status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaPermanentError,
				Message:    evt.Err.Error(),
			}
		}
		user.BridgeState.Send(user.metaState)
		go user.sendMarkdownBridgeAlert(context.TODO(), "Error in %s connection: %v", user.bridge.ProtocolName, evt.Err)
		user.StopBackfillLoop()
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

func (user *User) unlockedDisconnect() {
	if user.Client != nil {
		user.Client.Disconnect()
	}
	if user.E2EEClient != nil {
		user.E2EEClient.Disconnect()
	}
	user.StopBackfillLoop()
	user.Client = nil
	user.E2EEClient = nil
	user.waState = status.BridgeState{}
	user.metaState = status.BridgeState{}
}

func (user *User) Disconnect() error {
	user.Lock()
	defer user.Unlock()
	user.unlockedDisconnect()
	return nil
}

func (user *User) DeleteSession() {
	user.Lock()
	defer user.Unlock()
	user.unlockedDisconnect()
	if user.WADevice != nil {
		err := user.WADevice.Delete()
		if err != nil {
			user.log.Err(err).Msg("Failed to delete whatsmeow device")
		}
	}
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
