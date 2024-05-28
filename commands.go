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
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridge/commands"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
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
		cmdLoginTokens,
		cmdSyncSpace,
		cmdDeleteSession,
		cmdToggleEncryption,
		cmdSetRelay,
		cmdUnsetRelay,
		cmdDeletePortal,
		cmdDeleteAllPortals,
		cmdDeleteThread,
		cmdSearch,
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

var cmdToggleEncryption = &commands.FullHandler{
	Func: wrapCommand(fnToggleEncryption),
	Name: "toggle-encryption",
	Help: commands.HelpMeta{
		Section:     HelpSectionPortalManagement,
		Description: "Toggle Messenger-side encryption for the current room",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnToggleEncryption(ce *WrappedCommandEvent) {
	if !ce.Bridge.Config.Meta.Mode.IsMessenger() && !ce.Bridge.Config.Meta.IGE2EE {
		ce.Reply("Encryption support is not yet enabled in Instagram mode")
		return
	} else if !ce.Portal.IsPrivateChat() {
		ce.Reply("Only private chats can be toggled between encrypted and unencrypted")
		return
	}
	if ce.Portal.ThreadType.IsWhatsApp() {
		ce.Portal.ThreadType = table.ONE_TO_ONE
		ce.Reply("Messages in this room will now be sent unencrypted over Messenger")
	} else {
		if len(ce.Args) == 0 || ce.Args[0] != "--force" {
			resp, err := ce.User.Client.ExecuteTasks(&socket.CreateWhatsAppThreadTask{
				WAJID:            ce.Portal.ThreadID,
				OfflineThreadKey: methods.GenerateEpochId(),
				ThreadType:       table.ENCRYPTED_OVER_WA_ONE_TO_ONE,
				FolderType:       table.INBOX,
				BumpTimestampMS:  time.Now().UnixMilli(),
				TAMThreadSubtype: 0,
			})
			if err != nil {
				ce.ZLog.Err(err).Msg("Failed to create WhatsApp thread")
				ce.Reply("Failed to create WhatsApp thread")
				return
			}
			ce.ZLog.Trace().Any("create_resp", resp).Msg("Create WhatsApp thread response")
			if len(resp.LSIssueNewTask) > 0 {
				tasks := make([]socket.Task, len(resp.LSIssueNewTask))
				for i, task := range resp.LSIssueNewTask {
					ce.ZLog.Trace().Any("task", task).Msg("Create WhatsApp thread response task")
					tasks[i] = task
				}
				resp, err = ce.User.Client.ExecuteTasks(tasks...)
				if err != nil {
					ce.ZLog.Err(err).Msg("Failed to create WhatsApp thread (subtask)")
					ce.Reply("Failed to create WhatsApp thread")
					return
				} else {
					ce.ZLog.Trace().Any("create_resp", resp).Msg("Create thread response")
				}
			}
		}
		ce.Portal.ThreadType = table.ENCRYPTED_OVER_WA_ONE_TO_ONE
		ce.Reply("Messages in this room will now be sent encrypted over WhatsApp")
	}
	err := ce.Portal.Update(ce.Ctx)
	if err != nil {
		ce.ZLog.Err(err).Msg("Failed to update portal in database")
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
	wasLoggedIn := ce.User.IsLoggedIn()
	hadCookies := ce.User.Cookies != nil || ce.User.MetaID != 0
	ce.User.DeleteSession()
	if wasLoggedIn {
		ce.Reply("Disconnected and deleted session")
	} else if hadCookies {
		ce.Reply("Wasn't connected, but deleted session")
	} else {
		ce.Reply("You weren't logged in, but deleted session anyway")
	}
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

var cmdLoginTokens = &commands.FullHandler{
	Func: wrapCommand(fnLoginTokens),
	Name: "login-tokens",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionAuth,
		Description: "Link the bridge to your Meta account using access tokens.",
		Args:        "<datr> <c_user> <sb> <xs>",
	},
}

func fnLoginTokens(ce *WrappedCommandEvent) {
	if ce.User.IsLoggedIn() {
		if ce.User.Client.IsConnected() {
			ce.Reply("You're already logged in")
		} else {
			ce.Reply("You're already logged in, but not connected 🤔")
		}
		return
	}

	data := make(map[string]string)
	if ce.Bridge.Config.Meta.Mode.IsMessenger() {
		if len(ce.Args) == 4 {
			data["datr"] = ce.Args[0]
			data["c_user"] = ce.Args[1]
			data["sb"] = ce.Args[2]
			data["xs"] = ce.Args[3]
		} else {
			ce.Reply("**Usage for facebook**: login-tokens <datr> <c_user> <sb> <xs>")
			return
		}
	}

	if ce.Bridge.Config.Meta.Mode.IsInstagram() {
		if len(ce.Args) == 5 {
			data["sessionid"] = ce.Args[0]
			data["csrftoken"] = ce.Args[1]
			data["mid"] = ce.Args[2]
			data["ig_did"] = ce.Args[3]
			data["ds_user_id"] = ce.Args[4]
		} else {
			ce.Reply("**Usage for instagram**: login-tokens <sessionid> <csrftoken> <mid> <ig_did> <ds_user_id>")
			return
		}
	}

	var newCookies cookies.Cookies
	newCookies.Platform = database.MessagixPlatform

	rawData, _ := json.Marshal(data)
	err := json.Unmarshal(rawData, &newCookies)
	if err != nil {
		ce.Reply("Failed to parse cookies into struct: %v", err)
		return
	}

	// print the cookies
	ce.Reply("Cookies: %s", newCookies.String())

	missingRequiredCookies := newCookies.GetMissingCookieNames()
	if len(missingRequiredCookies) > 0 {
		ce.Reply("Missing some cookies: %v", missingRequiredCookies)
		return
	}

	err = ce.User.Login(ce.Ctx, &newCookies)
	if err != nil {
		ce.Reply("Failed to log in: %v", err)
	} else {
		ce.Reply("Successfully logged in as %d", ce.User.MetaID)
	}
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
	var newCookies cookies.Cookies
	newCookies.Platform = database.MessagixPlatform
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
		err := json.Unmarshal(rawData, &newCookies)
		if err != nil {
			ce.Reply("Failed to parse cookies into struct: %v", err)
			return
		}
	} else {
		err := json.Unmarshal([]byte(ce.RawArgs), &newCookies)
		if err != nil {
			ce.Reply("Failed to parse input as JSON: %v", err)
			return
		}
	}
	missingRequiredCookies := newCookies.GetMissingCookieNames()
	if len(missingRequiredCookies) > 0 {
		ce.Reply("Missing some cookies: %v", missingRequiredCookies)
		return
	}
	err := ce.User.Login(ce.Ctx, &newCookies)
	if err != nil {
		ce.Reply("Failed to log in: %v", err)
	} else {
		ce.Reply("Successfully logged in as %d", ce.User.MetaID)
	}
}

var cmdDeleteThread = &commands.FullHandler{
	Func:    wrapCommand(fnDeleteThread),
	Name:    "delete-chat",
	Aliases: []string{"delete-thread"},
	Help: commands.HelpMeta{
		Section:     HelpSectionPortalManagement,
		Description: "Delete the current chat on Meta and then delete the Matrix room",
	},
	RequiresLogin:  true,
	RequiresPortal: true,
}

func fnDeleteThread(ce *WrappedCommandEvent) {
	if ce.Portal.ThreadType.IsWhatsApp() {
		ce.Reply("Deleting encrypted chats is not yet supported")
		return
	}
	resp, err := ce.User.Client.ExecuteTasks(&socket.DeleteThreadTask{
		ThreadKey:  ce.Portal.ThreadID,
		RemoveType: 0,
		SyncGroup:  1, // 95 for encrypted chats
	})
	ce.ZLog.Trace().Any("response_data", resp).Err(err).Msg("Delete thread response")
	if err != nil {
		ce.ZLog.Err(err).Msg("Failed to delete thread")
		ce.Reply("Failed to delete thread")
	} else {
		ce.Portal.Delete()
		ce.Portal.Cleanup(ce.Ctx, false)
	}
}

var cmdSearch = &commands.FullHandler{
	Func: wrapCommand(fnSearch),
	Name: "search",
	Help: commands.HelpMeta{
		Section:     HelpSectionCreatingPortals,
		Description: "Search for a user on Meta",
		Args:        "<query>",
	},
	RequiresLogin: true,
}

func fnSearch(ce *WrappedCommandEvent) {
	task := &socket.SearchUserTask{
		Query: ce.RawArgs,
		SupportedTypes: []table.SearchType{
			table.SearchTypeContact, table.SearchTypeGroup, table.SearchTypePage, table.SearchTypeNonContact,
			table.SearchTypeIGContactFollowing, table.SearchTypeIGContactNonFollowing,
			table.SearchTypeIGNonContactFollowing, table.SearchTypeIGNonContactNonFollowing,
		},
		SurfaceType: 15,
		Secondary:   false,
	}
	if ce.Bridge.Config.Meta.Mode.IsMessenger() {
		task.SurfaceType = 5
		task.SupportedTypes = append(task.SupportedTypes, table.SearchTypeCommunityMessagingThread)
	}
	taskCopy := *task
	taskCopy.Secondary = true
	secondaryTask := &taskCopy

	go func() {
		time.Sleep(10 * time.Millisecond)
		resp, err := ce.User.Client.ExecuteTasks(secondaryTask)
		ce.ZLog.Trace().Any("response_data", resp).Err(err).Msg("Search secondary response")
		// The secondary response doesn't seem to have anything important, so just ignore it
	}()

	resp, err := ce.User.Client.ExecuteTasks(task)
	ce.ZLog.Trace().Any("response_data", resp).Msg("Search primary response")
	if err != nil {
		ce.ZLog.Err(err).Msg("Failed to search users")
		ce.Reply("Failed to search for users (see logs for more details)")
		return
	}
	puppets := make([]*Puppet, 0, len(resp.LSInsertSearchResult))
	subtitles := make([]string, 0, len(resp.LSInsertSearchResult))
	var wg sync.WaitGroup
	wg.Add(1)
	for _, result := range resp.LSInsertSearchResult {
		if result.ThreadType == table.ONE_TO_ONE && result.CanViewerMessage && result.GetFBID() != 0 {
			puppet := ce.Bridge.GetPuppetByID(result.GetFBID())
			puppets = append(puppets, puppet)
			subtitles = append(subtitles, result.ContextLine)
			wg.Add(1)
			go func(result *table.LSInsertSearchResult) {
				defer wg.Done()
				puppet.UpdateInfo(ce.Ctx, result)
			}(result)
		}
	}
	wg.Done()
	wg.Wait()
	if len(puppets) == 0 {
		ce.Reply("No results")
		return
	}
	var output strings.Builder
	output.WriteString("Results:\n\n")
	for i, puppet := range puppets {
		_, _ = fmt.Fprintf(&output, "* [%s](%s) (`%d`)\n  %s\n", puppet.Name, puppet.MXID.URI().MatrixToURL(), puppet.ID, subtitles[i])
	}
	ce.Reply(output.String())
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
