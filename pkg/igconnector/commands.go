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
	"slices"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

var cmdTap = &commands.FullHandler{
	Func: fnTap,
	Name: "tap",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionChats,
		Description: "Tap a postback or quick reply button (e.g. from a ManyChat bot) on an Instagram message",
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
	button, err := igconv.ResolvePostbackButton(meta.TapButtons(), arg)
	if err != nil {
		ce.Reply("Couldn't tap button: %v", err)
		return
	}
	portalMeta, err := cli.ensureIGID(ce.Ctx, ce.Portal)
	if err != nil {
		ce.Reply("Failed to tap button: %v", err)
		return
	}
	_, err = cli.sendButtonTap(ce.Ctx, portalMeta.IGID, button, "")
	if err != nil {
		ce.Log.Err(err).Bool("quick_reply", button.IsQuickReply()).Msg("Failed to send button tap")
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
		if len(meta.TapButtons()) == 0 {
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
	newest := newestPostbackMessage(recent)
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
	items, err := cli.Client.FetchThreadButtonItems(ctx, igconv.PostbackThreadID(portalMeta))
	if err != nil {
		return fmt.Errorf("failed to fetch buttons: %w", err)
	}
	buttons := igconv.PickButtons(items, igconv.ItemIDFromMessageID(msg.ID), nil, 0)
	if buttons.Empty() {
		return errors.New("the message has no tappable buttons")
	}
	meta.PostbackButtons = buttons.Postbacks
	meta.QuickReplies = buttons.QuickReplies
	if err = ce.Bridge.DB.Message.Update(ctx, msg); err != nil {
		ce.Log.Warn().Err(err).Msg("Failed to save recovered postback buttons")
	}
	return nil
}

var cmdRefresh = &commands.FullHandler{
	Func: fnRefresh,
	Name: "refresh",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionChats,
		Description: "Refresh CTA buttons and links on existing Instagram cards in this room (or the replied-to message) and add the button poll to cards that lack one. Doesn't send anything to Instagram",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

type refreshResult int

const (
	refreshSkipped refreshResult = iota
	refreshUpdated
	refreshFailed
)

func fnRefresh(ce *commands.Event) {
	login, _, err := ce.Portal.FindPreferredLogin(ce.Ctx, ce.User, false)
	if err != nil || login == nil {
		ce.Reply("Failed to find login for room")
		ce.Log.Debug().Str("reason", "login_unavailable").Msg("Failed refresh")
		return
	}
	var targets []*database.Message
	if ce.ReplyTo != "" {
		msg, err := ce.Bridge.DB.Message.GetPartByMXID(ce.Ctx, ce.ReplyTo)
		if err != nil {
			ce.Reply("Failed to look up the replied message")
			return
		} else if msg == nil || msg.Room != ce.Portal.PortalKey {
			ce.Reply("The replied message wasn't found in this chat")
			return
		}
		targets = []*database.Message{cardOfButtonPoll(ce.Ctx, ce.Bridge.DB.Message, ce.Portal.Receiver, msg)}
	} else if targets, err = ce.Bridge.DB.Message.IterPortalMessages(ce.Ctx, ce.Portal.PortalKey).AsList(); err != nil {
		ce.Log.Debug().Str("reason", "message_lookup_failed").Msg("Failed refresh")
		ce.Reply("Failed to look up messages")
		return
	}
	counts := refreshPortal(ce, login, targets)
	ce.Reply("Refreshed %d message(s) (including stored buttons), skipped %d, failed %d", counts[refreshUpdated], counts[refreshSkipped], counts[refreshFailed])
}

// cardOfButtonPoll returns the card that a button poll belongs to, or msg itself if it isn't a
// button poll or the card can't be found. The poll has no card HTML of its own to refresh.
func cardOfButtonPoll(ctx context.Context, db *database.MessageQuery, receiver networkid.UserLoginID, msg *database.Message) *database.Message {
	if !isButtonPoll(msg) {
		return msg
	}
	if card, err := db.GetFirstPartByID(ctx, receiver, msg.ID); err == nil && card != nil && !isButtonPoll(card) {
		return card
	}
	return msg
}

func refreshPortal(ce *commands.Event, login *bridgev2.UserLogin, targets []*database.Message) [3]int {
	var counts [3]int
	// Button polls are Matrix-only. Editing one as if it were a card would corrupt it.
	targets = slices.DeleteFunc(slices.Clone(targets), isButtonPoll)
	if len(targets) == 0 {
		return counts
	}
	cli, ok := login.Client.(*IGClient)
	if !ok || cli.Client == nil {
		ce.Log.Debug().Str("reason", "client_unavailable").Msg("Failed refresh")
		counts[refreshFailed] = len(targets)
		return counts
	}
	portalMeta, _ := ce.Portal.Metadata.(*metaid.PortalMetadata)
	var items []*slidetypes.ThreadItem
	var fetchErr error
	if threadID := igconv.PostbackThreadID(portalMeta); threadID != "" {
		items, fetchErr = cli.Client.FetchAllThreadButtonItems(ce.Ctx, threadID)
		if fetchErr != nil {
			ce.Log.Debug().Str("reason", "thread_history_fetch_failed").Msg("Refresh will use available items and stored buttons")
		}
	} else {
		fetchErr = errors.New("thread ID unavailable")
		ce.Log.Debug().Str("reason", "thread_id_unavailable").Msg("Refresh will use stored buttons without fetching Instagram IDs")
	}
	for _, msg := range targets {
		if ce.Ctx.Err() != nil {
			ce.Log.Debug().Str("reason", "refresh_cancelled").Msg("Failed refresh")
			counts[refreshFailed]++
			continue
		}
		result := refreshCard(ce, login, items, msg)
		attached, attachErr := ce.Bridge.Network.(*IGConnector).attachButtonPoll(ce.Ctx, ce.Portal, login, items, msg)
		switch {
		case attachErr != nil:
			ce.Log.Debug().Str("message_id", string(msg.ID)).Str("reason", "button_poll_attach_failed").Msg("Failed refresh")
			result = refreshFailed
		case attached && result == refreshSkipped:
			result = refreshUpdated
		case result == refreshSkipped && fetchErr != nil:
			ce.Log.Debug().Str("message_id", string(msg.ID)).Str("reason", "history_unavailable_for_skipped_message").Msg("Failed refresh")
			result = refreshFailed
		}
		counts[result]++
	}
	return counts
}

var cmdRefreshAll = &commands.FullHandler{
	Func: fnRefreshAll,
	Name: "refresh-all",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionChats,
		Description: "Refresh cards, and add missing button polls, in every portal of your login. Run in the management room. Only reads Instagram history",
		Args:        "[_login ID_]",
	},
	RequiresLogin: true,
}

func fnRefreshAll(ce *commands.Event) {
	if ce.RoomID != ce.User.ManagementRoom {
		ce.Reply("Run `$cmdprefix refresh-all` in your management room")
		return
	}
	login := ce.User.GetDefaultLogin()
	if len(ce.Args) > 1 {
		ce.Reply("Usage: `$cmdprefix refresh-all [login ID]`")
		return
	}
	if len(ce.Args) == 1 {
		login = nil
		for _, candidate := range ce.User.GetUserLogins() {
			if string(candidate.ID) == ce.Args[0] {
				login = candidate
				break
			}
		}
	}
	if login == nil {
		ce.Reply("No matching Instagram login found")
		return
	}
	memberships, err := ce.Bridge.DB.UserPortal.GetAllForLogin(ce.Ctx, login.UserLogin)
	if err != nil {
		ce.Log.Debug().Str("reason", "portal_lookup_failed").Msg("Failed refresh-all")
		ce.Reply("Failed to look up portals")
		return
	}
	var totals [3]int
	for _, membership := range memberships {
		var counts [3]int
		portal, err := ce.Bridge.GetExistingPortalByKey(ce.Ctx, membership.Portal)
		switch {
		case err != nil:
			ce.Log.Debug().Str("portal_id", string(membership.Portal.ID)).Str("reason", "portal_load_failed").Msg("Failed refresh-all portal")
			counts[refreshFailed]++
		case portal == nil || portal.MXID == "":
			ce.Log.Debug().Str("portal_id", string(membership.Portal.ID)).Str("reason", "portal_has_no_matrix_room").Msg("Skipping refresh-all portal")
			counts[refreshSkipped]++
		default:
			targets, err := ce.Bridge.DB.Message.IterPortalMessages(ce.Ctx, portal.PortalKey).AsList()
			if err != nil {
				ce.Log.Debug().Str("portal_id", string(membership.Portal.ID)).Str("reason", "message_lookup_failed").Msg("Failed refresh-all portal")
				counts[refreshFailed]++
			} else {
				portalEvent := *ce
				portalEvent.Portal = portal
				counts = refreshPortal(&portalEvent, login, targets)
			}
		}
		for result, count := range counts {
			totals[result] += count
		}
		ce.Reply("Portal %s: updated %d, skipped %d, failed %d", membership.Portal.ID, counts[refreshUpdated], counts[refreshSkipped], counts[refreshFailed])
	}
	ce.Reply("Totals across %d portal(s): updated %d, skipped %d, failed %d", len(memberships), totals[refreshUpdated], totals[refreshSkipped], totals[refreshFailed])
}

func sameButtons(a, b []*metaid.PostbackButton) bool {
	return slices.EqualFunc(a, b, func(x, y *metaid.PostbackButton) bool {
		return x.Title == y.Title && x.Payload == y.Payload && x.CTAType == y.CTAType
	})
}

// refreshCard updates the stored buttons and the Matrix event of one bridged card.
// It never sends anything to Instagram, and never needs to decrypt the old Matrix event.
// The button poll is attached separately, see attachButtonPoll.
func refreshCard(ce *commands.Event, login *bridgev2.UserLogin, items []*slidetypes.ThreadItem, msg *database.Message) refreshResult {
	log := ce.Log.With().Str("message_id", string(msg.ID)).Logger()
	meta, ok := msg.Metadata.(*metaid.MessageMetadata)
	if !ok || msg.MXID == "" {
		log.Debug().Str("reason", "no_metadata_or_mxid").Msg("Skipping refresh")
		return refreshSkipped
	}
	itemID := igconv.ItemIDFromMessageID(msg.ID)
	if itemID == "" {
		log.Debug().Str("reason", "no_item_id").Msg("Skipping refresh")
		return refreshSkipped
	}
	var item *slidetypes.ThreadItem
	for _, candidate := range items {
		if candidate.MatchesID(itemID) {
			if item != nil {
				log.Debug().Str("reason", "ambiguous_thread_item").Msg("Skipping refresh")
				return refreshSkipped
			}
			item = candidate
		}
	}
	var ctas []*slidetypes.CTAButton
	var buttons igconv.Buttons
	if item != nil {
		ctas = item.AllCTAButtons()
		buttons = igconv.ButtonsFromItem(item)
	}
	if len(buttons.Postbacks) == 0 {
		buttons.Postbacks = meta.PostbackButtons
	}
	if len(buttons.QuickReplies) == 0 {
		buttons.QuickReplies = meta.QuickReplies
	}
	if item == nil && buttons.Empty() {
		log.Debug().Str("reason", "no_stored_buttons_or_matching_history_item").Msg("Skipping refresh")
		return refreshSkipped
	}
	opts := ce.Bridge.Network.(*IGConnector).tapOptions(msg)
	ctaHTML := igconv.BuildCTAButtonsHTML(ctas, buttons.Postbacks, buttons.QuickReplies, opts)
	if ctaHTML == "" {
		log.Debug().Str("reason", "no_cta_html").Msg("Skipping refresh")
		return refreshSkipped
	}
	metaChanged := !sameButtons(meta.PostbackButtons, buttons.Postbacks) || !sameButtons(meta.QuickReplies, buttons.QuickReplies)
	metaSaveFailed := false
	if metaChanged {
		meta.PostbackButtons = buttons.Postbacks
		meta.QuickReplies = buttons.QuickReplies
		if err := ce.Bridge.DB.Message.Update(ce.Ctx, msg); err != nil {
			log.Debug().Str("reason", "metadata_save_failed").Msg("Failed to save refreshed buttons")
			metaSaveFailed = true
		}
	}
	// Nothing to edit in Matrix: count as updated only if the stored buttons were rewritten.
	noEdit := func(reason string) refreshResult {
		log.Debug().Str("reason", reason).Bool("metadata_changed", metaChanged).Msg("No Matrix edit needed for refresh")
		if metaSaveFailed {
			return refreshFailed
		}
		if metaChanged {
			return refreshUpdated
		}
		return refreshSkipped
	}
	intent, intentErr := messageSenderIntent(ce.Ctx, ce.Bridge, ce.Portal, login, msg)
	if intentErr != nil || intent == nil {
		log.Debug().Str("reason", "matrix_intent_unavailable").Msg("Failed to get intent for refresh edit")
		return refreshFailed
	}
	// The old event is optional: it may be encrypted in a way the puppet can't read.
	evt, getErr := intent.GetEvent(ce.Ctx, ce.Portal.MXID, msg.MXID)
	if getErr != nil {
		log.Debug().Err(getErr).Msg("Couldn't read the old Matrix event, rebuilding the card from the Instagram item")
	}
	cardHTML := igconv.CardHTMLFromThreadItem(item, opts)
	if cardHTML != "" {
		cardHTML = igconv.ReplaceOrAppendCTAButtonsHTML(cardHTML, ctaHTML)
	} else if getErr != nil || evt == nil {
		log.Debug().Str("reason", "rebuilding_from_stored_buttons_only").Msg("Refreshing CTA-only card")
		cardHTML = ctaHTML
	}
	newContent, send := planRefreshEdit(cardHTML, ctaHTML, evt, getErr)
	if !send {
		return noEdit("matrix_content_unchanged_or_unknown")
	}
	newContent.SetEdit(msg.MXID)
	_, err := intent.SendMessage(ce.Ctx, ce.Portal.MXID, event.EventMessage, &event.Content{
		Parsed: newContent,
		Raw:    map[string]any{"com.beeper.dont_render_edited": true},
	}, &bridgev2.MatrixSendExtra{MessageMeta: msg})
	if err != nil {
		log.Debug().Str("reason", "matrix_edit_failed").Msg("Failed to send refresh edit")
		return refreshFailed
	}
	if metaSaveFailed {
		return refreshFailed
	}
	return refreshUpdated
}

// planRefreshEdit decides the content of the refresh edit. cardHTML is the card rebuilt from the
// fresh Instagram item and is the source of truth when set. If the old event couldn't be read
// (getErr) it is sent as plain text. If it could, media msgtypes are kept, and an identical latest
// body means no edit. Without cardHTML the old body is patched with ctaHTML, but only if that body
// is known to be current (see shouldEditRefreshedCard).
func planRefreshEdit(cardHTML, ctaHTML string, evt *event.Event, getErr error) (*event.MessageEventContent, bool) {
	var content *event.MessageEventContent
	bundledEdit := false
	if getErr == nil && evt != nil {
		content, bundledEdit = currentMessageContent(evt)
	}
	newHTML := cardHTML
	if newHTML == "" {
		if content == nil {
			return nil, false
		}
		oldHTML := content.FormattedBody
		if oldHTML == "" {
			if content.MsgType != event.MsgText && content.MsgType != event.MsgNotice {
				return nil, false
			}
			oldHTML = event.TextToHTML(content.Body)
		}
		if !shouldEditRefreshedCard(bundledEdit, oldHTML) {
			return nil, false
		}
		newHTML = igconv.ReplaceOrAppendCTAButtonsHTML(oldHTML, ctaHTML)
	}
	if content != nil && newHTML == content.FormattedBody {
		return nil, false
	}
	var newContent event.MessageEventContent
	if content != nil {
		newContent = *content
	} else {
		newContent.MsgType = event.MsgText
	}
	converted := format.HTMLToContent(newHTML)
	newContent.Body = converted.Body
	newContent.Format = event.FormatHTML
	newContent.FormattedBody = newHTML
	newContent.NewContent = nil
	newContent.RelatesTo = nil
	return &newContent, true
}

// shouldEditRefreshedCard decides whether a refresh may send a Matrix edit. Without a bundled
// latest edit the content is the original event, which may predate an edit that resolved the media
// caption, so it is only edited if it already has a CTA footer to replace.
func shouldEditRefreshedCard(bundledEdit bool, html string) bool {
	return bundledEdit || igconv.HasKnownCTAFooter(html)
}

// currentMessageContent returns the latest content of a message event, using the bundled
// latest edit if the homeserver provided one. The bool is true if the content came from an edit.
func currentMessageContent(evt *event.Event) (*event.MessageEventContent, bool) {
	if edit := evt.Unsigned.Relations.GetLatestEdit(); edit != nil {
		if edit.Content.Parsed == nil {
			_ = edit.Content.ParseRaw(edit.Type)
		}
		if editContent := edit.Content.AsMessage(); editContent != nil && editContent.NewContent != nil {
			return editContent.NewContent, true
		}
	}
	return evt.Content.AsMessage(), false
}
