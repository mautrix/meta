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
	"strings"

	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

var cmdTap = &commands.FullHandler{
	Func: fnTap,
	Name: "tap",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionChats,
		Description: "Tap a button (e.g. from a ManyChat bot) on an Instagram message",
		Args:        "<_button number or title_>",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

// How many recent messages are searched for postback buttons when the command isn't a reply.
const tapSearchMessageCount = 50

func fnTap(ce *commands.Event) {
	arg := strings.Join(ce.Args, " ")
	if strings.TrimSpace(arg) == "" {
		ce.Reply("Usage: `$cmdprefix tap <button number or title>`")
		return
	}
	login, _, err := ce.Portal.FindPreferredLogin(ce.Ctx, ce.User, false)
	if err != nil || login == nil {
		ce.Reply("Failed to find login for room")
		ce.Log.Err(err).Msg("Failed to find login for room")
		return
	}
	cli, ok := login.Client.(*IGClient)
	if !ok || cli.Client == nil {
		ce.Reply("Instagram client is not available")
		return
	}
	target, err := findPostbackMessage(ce, cli)
	if err != nil {
		ce.Reply("%v", err)
		return
	}
	meta := target.Metadata.(*metaid.MessageMetadata)
	button, err := igconv.ResolvePostbackButton(meta.PostbackButtons, arg)
	if err != nil {
		ce.Reply("Couldn't tap button: %v", err)
		return
	}
	portalMeta, err := cli.ensureIGID(ce.Ctx, ce.Portal)
	if err != nil {
		ce.Reply("Failed to tap button: %v", err)
		return
	}
	_, err = cli.Client.SendPostback(ce.Ctx, portalMeta.IGID, button.Title, button.Payload)
	if err != nil {
		ce.Log.Err(err).Str("button_title", button.Title).Msg("Failed to send postback")
		ce.Reply("Failed to tap button %q: %v", button.Title, err)
		return
	}
	ce.Reply("Sent a tap for button %q", button.Title)
}

// findPostbackMessage finds the message whose buttons should be tapped: the message being
// replied to, or otherwise the newest recent message with stored postback buttons.
func findPostbackMessage(ce *commands.Event, cli *IGClient) (*database.Message, error) {
	if ce.ReplyTo != "" {
		msg, err := ce.Bridge.DB.Message.GetPartByMXID(ce.Ctx, ce.ReplyTo)
		if err != nil {
			return nil, fmt.Errorf("failed to look up the replied message: %w", err)
		} else if msg == nil || msg.Room != ce.Portal.PortalKey {
			return nil, errors.New("the replied message wasn't found in this chat")
		}
		meta, ok := msg.Metadata.(*metaid.MessageMetadata)
		if !ok {
			return nil, errors.New("the replied message has no buttons")
		}
		if len(meta.PostbackButtons) == 0 {
			// Metadata is missing, so a targeted fetch is allowed (see #338)
			if err = fetchStoredPostbackButtons(ce.Ctx, ce, cli, msg, meta); err != nil {
				return nil, err
			}
		}
		return msg, nil
	}
	recent, err := ce.Bridge.DB.Message.GetLastNInPortal(ce.Ctx, ce.Portal.PortalKey, tapSearchMessageCount)
	if err != nil {
		return nil, fmt.Errorf("failed to look up recent messages: %w", err)
	}
	var newest *database.Message
	for _, msg := range recent {
		meta, ok := msg.Metadata.(*metaid.MessageMetadata)
		if !ok || len(meta.PostbackButtons) == 0 {
			continue
		}
		if newest == nil || msg.Timestamp.After(newest.Timestamp) {
			newest = msg
		}
	}
	if newest == nil {
		return nil, errors.New("no message with tappable buttons found. Reply to the message with the buttons")
	}
	return newest, nil
}

func fetchStoredPostbackButtons(ctx context.Context, ce *commands.Event, cli *IGClient, msg *database.Message, meta *metaid.MessageMetadata) error {
	portalMeta, err := cli.ensureIGID(ctx, ce.Portal)
	if err != nil {
		return fmt.Errorf("failed to find the thread ID: %w", err)
	}
	items, err := cli.Client.FetchThreadGenericXMAItems(ctx, igconv.PostbackThreadID(portalMeta))
	if err != nil {
		return fmt.Errorf("failed to fetch buttons: %w", err)
	}
	buttons := igconv.PickPostbackButtons(items, igconv.ItemIDFromMessageID(msg.ID), nil, 0)
	if len(buttons) == 0 {
		return errors.New("the message has no tappable buttons")
	}
	meta.PostbackButtons = buttons
	if err = ce.Bridge.DB.Message.Update(ctx, msg); err != nil {
		ce.Log.Warn().Err(err).Msg("Failed to save recovered postback buttons")
	}
	return nil
}
