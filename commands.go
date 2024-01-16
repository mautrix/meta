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
	"net/http"
	"regexp"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridge/commands"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/database"
)

var (
	HelpSectionConnectionManagement = commands.HelpSection{Name: "Connection management", Order: 11}
	HelpSectionCreatingPortals      = commands.HelpSection{Name: "Creating portals", Order: 15}
	HelpSectionPortalManagement     = commands.HelpSection{Name: "Portal management", Order: 20}
	HelpSectionMiscellaneous        = commands.HelpSection{Name: "Miscellaneous", Order: 30}
)

type WrappedCommandEvent struct {
	*commands.Event
	Bridge *MetaBridge
	User   *User
	Portal *Portal
}

func (br *MetaBridge) RegisterCommands() {
	proc := br.CommandProcessor.(*commands.Processor)
	proc.AddHandlers(
		cmdPing,
		cmdLogin,
		cmdSyncSpace,
		cmdDeleteSession,
		cmdSetRelay,
		cmdUnsetRelay,
		cmdDeletePortal,
		cmdDeleteAllPortals,
	)
}

func wrapCommand(handler func(*WrappedCommandEvent)) func(*commands.Event) {
	return func(ce *commands.Event) {
		user := ce.User.(*User)
		var portal *Portal
		if ce.Portal != nil {
			portal = ce.Portal.(*Portal)
		}
		br := ce.Bridge.Child.(*MetaBridge)
		handler(&WrappedCommandEvent{ce, br, user, portal})
	}
}

var cmdSetRelay = &commands.FullHandler{
	Func: wrapCommand(fnSetRelay),
	Name: "set-relay",
	Help: commands.HelpMeta{
		Section:     HelpSectionPortalManagement,
		Description: "Relay messages in this room through your Meta account.",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnSetRelay(ce *WrappedCommandEvent) {
	if !ce.Bridge.Config.Bridge.Relay.Enabled {
		ce.Reply("Relay mode is not enabled on this instance of the bridge")
	} else if ce.Bridge.Config.Bridge.Relay.AdminOnly && !ce.User.Admin {
		ce.Reply("Only bridge admins are allowed to enable relay mode on this instance of the bridge")
	} else {
		ce.Portal.RelayUserID = ce.User.MXID
		err := ce.Portal.Update(ce.Ctx)
		if err != nil {
			ce.ZLog.Err(err).Msg("Failed to save portal")
		}
		// TODO reply with Facebook/Instagram instead of Meta
		ce.Reply("Messages from non-logged-in users in this room will now be bridged through your Meta account")
	}
}

var cmdUnsetRelay = &commands.FullHandler{
	Func: wrapCommand(fnUnsetRelay),
	Name: "unset-relay",
	Help: commands.HelpMeta{
		Section:     HelpSectionPortalManagement,
		Description: "Stop relaying messages in this room.",
	},
	RequiresPortal: true,
}

func fnUnsetRelay(ce *WrappedCommandEvent) {
	if !ce.Bridge.Config.Bridge.Relay.Enabled {
		ce.Reply("Relay mode is not enabled on this instance of the bridge")
	} else if ce.Bridge.Config.Bridge.Relay.AdminOnly && !ce.User.Admin {
		ce.Reply("Only bridge admins are allowed to enable relay mode on this instance of the bridge")
	} else {
		ce.Portal.RelayUserID = ""
		err := ce.Portal.Update(ce.Ctx)
		if err != nil {
			ce.ZLog.Err(err).Msg("Failed to save portal")
		}
		ce.Reply("Messages from non-logged-in users will no longer be bridged in this room")
	}
}

var cmdDeleteSession = &commands.FullHandler{
	Func: wrapCommand(fnDeleteSession),
	Name: "delete-session",
	Help: commands.HelpMeta{
		Section:     HelpSectionConnectionManagement,
		Description: "Disconnect from Meta, clearing sessions but keeping other data. Reconnect with `login`",
	},
}

func fnDeleteSession(ce *WrappedCommandEvent) {
	if !ce.User.IsLoggedIn() {
		ce.Reply("You're not logged in")
		return
	}
	ce.User.DeleteSession()
	ce.Reply("Disconnected and deleted session")
}

var cmdPing = &commands.FullHandler{
	Func: wrapCommand(fnPing),
	Name: "ping",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionAuth,
		Description: "Check your connection to Meta",
	},
}

func fnPing(ce *WrappedCommandEvent) {
	if ce.User.MetaID == 0 {
		ce.Reply("You're not logged in")
	} else if !ce.User.IsLoggedIn() {
		ce.Reply("You were logged in at some point, but are not anymore")
	} else if !ce.User.Client.IsConnected() {
		ce.Reply("You're logged into Meta, but not connected to the server")
	} else {
		ce.Reply("You're logged into Meta and probably connected to the server")
	}
}

var cmdSyncSpace = &commands.FullHandler{
	Func: wrapCommand(fnSyncSpace),
	Name: "sync-space",
	Help: commands.HelpMeta{
		Section:     HelpSectionMiscellaneous,
		Description: "Synchronize your personal filtering space",
	},
	RequiresLogin: true,
}

func fnSyncSpace(ce *WrappedCommandEvent) {
	if !ce.Bridge.Config.Bridge.PersonalFilteringSpaces {
		ce.Reply("Personal filtering spaces are not enabled on this instance of the bridge")
		return
	}
	ctx := ce.Ctx
	dmKeys, err := ce.Bridge.DB.Portal.FindPrivateChatsNotInSpace(ctx, ce.User.MetaID)
	if err != nil {
		ce.ZLog.Err(err).Msg("Failed to get private chat keys")
		ce.Reply("Failed to get private chat IDs from database")
		return
	}
	count := 0
	allPortals := ce.Bridge.GetAllPortalsWithMXID()
	for _, portal := range allPortals {
		if portal.IsPrivateChat() {
			continue
		}
		if ce.Bridge.StateStore.IsInRoom(ctx, portal.MXID, ce.User.MXID) && portal.addToPersonalSpace(ctx, ce.User) {
			count++
		}
	}
	for _, key := range dmKeys {
		portal := ce.Bridge.GetExistingPortalByThreadID(key)
		portal.addToPersonalSpace(ctx, ce.User)
		count++
	}
	plural := "s"
	if count == 1 {
		plural = ""
	}
	ce.Reply("Added %d room%s to space", count, plural)
}

var cmdLogin = &commands.FullHandler{
	Func: wrapCommand(fnLogin),
	Name: "login",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionAuth,
		Description: "Link the bridge to your Meta account.",
	},
}

func fnLogin(ce *WrappedCommandEvent) {
	if ce.User.IsLoggedIn() {
		if ce.User.Client.IsConnected() {
			ce.Reply("You're already logged in")
		} else {
			ce.Reply("You're already logged in, but not connected 🤔")
		}
		return
	}

	ce.Reply("Paste your cookies here (either as a JSON object or cURL request). " +
		"See full instructions at <https://docs.mau.fi/bridges/go/meta/authentication.html>")
	ce.User.commandState = &commands.CommandState{
		Next:   wrappedFnLoginEnterCookies,
		Action: "Login",
	}
}

var wrappedFnLoginEnterCookies = commands.MinimalHandlerFunc(wrapCommand(fnLoginEnterCookies))
var curlCookieRegex = regexp.MustCompile(`-H '[cC]ookie: ([^']*)'`)

func fnLoginEnterCookies(ce *WrappedCommandEvent) {
	newCookies := database.NewCookies()
	ce.Redact()
	if strings.HasPrefix(strings.TrimSpace(ce.RawArgs), "curl") {
		cookieHeader := curlCookieRegex.FindStringSubmatch(ce.RawArgs)
		if len(cookieHeader) != 2 {
			ce.Reply("Couldn't find `-H 'Cookie: ...'` in curl command")
			return
		}
		parsed := (&http.Request{Header: http.Header{"Cookie": {cookieHeader[1]}}}).Cookies()
		data := make(map[string]string)
		for _, cookie := range parsed {
			data[cookie.Name] = cookie.Value
		}
		rawData, _ := json.Marshal(data)
		err := json.Unmarshal(rawData, newCookies)
		if err != nil {
			ce.Reply("Failed to parse cookies into struct: %v", err)
			return
		}
	} else {
		err := json.Unmarshal([]byte(ce.RawArgs), newCookies)
		if err != nil {
			ce.Reply("Failed to parse input as JSON: %v", err)
			return
		}
	}
	if !newCookies.AllCookiesPresent() {
		ce.Reply("Missing some cookies")
		return
	}
	err := ce.User.Login(ce.Ctx, newCookies)
	if err != nil {
		ce.Reply("Failed to log in: %v", err)
	} else {
		ce.Reply("Successfully logged in as %d", ce.User.MetaID)
	}
}

func canDeletePortal(ctx context.Context, portal *Portal, userID id.UserID) bool {
	if len(portal.MXID) == 0 {
		return false
	}

	members, err := portal.MainIntent().JoinedMembers(ctx, portal.MXID)
	if err != nil {
		portal.log.Err(err).
			Str("user_id", userID.String()).
			Msg("Failed to get joined members to check if user can delete portal")
		return false
	}
	for otherUser := range members.Joined {
		_, isPuppet := portal.bridge.ParsePuppetMXID(otherUser)
		if isPuppet || otherUser == portal.bridge.Bot.UserID || otherUser == userID {
			continue
		}
		user := portal.bridge.GetUserByMXID(otherUser)
		if user != nil && user.IsLoggedIn() {
			return false
		}
	}
	return true
}

var cmdDeletePortal = &commands.FullHandler{
	Func: wrapCommand(fnDeletePortal),
	Name: "delete-portal",
	Help: commands.HelpMeta{
		Section:     HelpSectionPortalManagement,
		Description: "Delete the current portal. If the portal is used by other people, this is limited to bridge admins.",
	},
	RequiresPortal: true,
}

func fnDeletePortal(ce *WrappedCommandEvent) {
	if !ce.User.Admin && !canDeletePortal(ce.Ctx, ce.Portal, ce.User.MXID) {
		ce.Reply("Only bridge admins can delete portals with other Matrix users")
		return
	}

	ce.Portal.log.Info().Stringer("user_id", ce.User.MXID).Msg("User requested deletion of portal")
	ce.Portal.Delete()
	ce.Portal.Cleanup(ce.Ctx, false)
}

var cmdDeleteAllPortals = &commands.FullHandler{
	Func: wrapCommand(fnDeleteAllPortals),
	Name: "delete-all-portals",
	Help: commands.HelpMeta{
		Section:     HelpSectionPortalManagement,
		Description: "Delete all portals.",
	},
}

func fnDeleteAllPortals(ce *WrappedCommandEvent) {
	portals := ce.Bridge.GetAllPortalsWithMXID()
	var portalsToDelete []*Portal

	if ce.User.Admin {
		portalsToDelete = portals
	} else {
		portalsToDelete = portals[:0]
		for _, portal := range portals {
			if canDeletePortal(ce.Ctx, portal, ce.User.MXID) {
				portalsToDelete = append(portalsToDelete, portal)
			}
		}
	}
	if len(portalsToDelete) == 0 {
		ce.Reply("Didn't find any portals to delete")
		return
	}

	leave := func(portal *Portal) {
		if len(portal.MXID) > 0 {
			_, _ = portal.MainIntent().KickUser(ce.Ctx, portal.MXID, &mautrix.ReqKickUser{
				Reason: "Deleting portal",
				UserID: ce.User.MXID,
			})
		}
	}
	customPuppet := ce.Bridge.GetPuppetByCustomMXID(ce.User.MXID)
	if customPuppet != nil && customPuppet.CustomIntent() != nil {
		intent := customPuppet.CustomIntent()
		leave = func(portal *Portal) {
			if len(portal.MXID) > 0 {
				_, _ = intent.LeaveRoom(ce.Ctx, portal.MXID)
				_, _ = intent.ForgetRoom(ce.Ctx, portal.MXID)
			}
		}
	}
	ce.Reply("Found %d portals, deleting...", len(portalsToDelete))
	for _, portal := range portalsToDelete {
		portal.Delete()
		leave(portal)
	}
	ce.Reply("Finished deleting portal info. Now cleaning up rooms in background.")

	backgroundCtx := context.TODO()
	go func() {
		for _, portal := range portalsToDelete {
			portal.Cleanup(backgroundCtx, false)
		}
		ce.Reply("Finished background cleanup of deleted portal rooms.")
	}()
}
