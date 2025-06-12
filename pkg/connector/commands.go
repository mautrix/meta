package connector

import (
	"time"

	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var cmdToggleEncryption = &commands.FullHandler{
	Func: fnToggleEncryption,
	Name: "toggle-encryption",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionChats,
		Description: "Toggle Messenger-side encryption for the current room",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnToggleEncryption(ce *commands.Event) {
	conn := ce.Bridge.Network.(*MetaConnector)
	if !conn.Config.Mode.IsMessenger() && !conn.Config.IGE2EE {
		ce.Reply("Instagram encryption is not enabled in the bridge config")
		return
	} else if ce.Portal.RoomType != database.RoomTypeDM {
		ce.Reply("Only private chats can be toggled between encrypted and unencrypted")
		return
	}
	login, _, err := ce.Portal.FindPreferredLogin(ce.Ctx, ce.User, false)
	if err != nil {
		ce.Reply("Failed to find login for room")
		ce.Log.Err(err).Msg("Failed to find login for room")
		return
	}
	cli := login.Client.(*MetaClient)
	meta := ce.Portal.Metadata.(*metaid.PortalMetadata)
	if meta.ThreadType.IsWhatsApp() {
		meta.ThreadType = table.ONE_TO_ONE
		ce.Reply("Messages in this room will now be sent unencrypted over Messenger")
	} else {
		if len(ce.Args) == 0 || ce.Args[0] != "--force" {
			threadID := metaid.ParseFBPortalID(ce.Portal.ID)
			resp, err := cli.Client.ExecuteTasks(ce.Ctx, &socket.CreateWhatsAppThreadTask{
				WAJID:            threadID,
				OfflineThreadKey: methods.GenerateEpochID(),
				ThreadType:       table.ENCRYPTED_OVER_WA_ONE_TO_ONE,
				FolderType:       table.INBOX,
				BumpTimestampMS:  time.Now().UnixMilli(),
				TAMThreadSubtype: 0,
			})
			if err != nil {
				ce.Log.Err(err).Msg("Failed to create WhatsApp thread")
				ce.Reply("Failed to create WhatsApp thread")
				return
			}
			ce.Log.Trace().Any("create_resp", resp).Msg("Create WhatsApp thread response")
			if len(resp.LSIssueNewTask) > 0 {
				tasks := make([]socket.Task, len(resp.LSIssueNewTask))
				for i, task := range resp.LSIssueNewTask {
					ce.Log.Trace().Any("task", task).Msg("Create WhatsApp thread response task")
					tasks[i] = task
				}
				resp, err = cli.Client.ExecuteTasks(ce.Ctx, tasks...)
				if err != nil {
					ce.Log.Err(err).Msg("Failed to create WhatsApp thread (subtask)")
					ce.Reply("Failed to create WhatsApp thread")
					return
				} else {
					ce.Log.Trace().Any("create_resp", resp).Msg("Create thread response")
				}
			}
		}
		meta.ThreadType = table.ENCRYPTED_OVER_WA_ONE_TO_ONE
		ce.Reply("Messages in this room will now be sent encrypted over WhatsApp")
	}
	err = ce.Portal.Save(ce.Ctx)
	if err != nil {
		ce.Log.Err(err).Msg("Failed to update portal in database")
	}
}
