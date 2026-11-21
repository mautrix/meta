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

package igconv

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

// See https://github.com/mautrix/meta/issues/338 for the postback button protocol.

var (
	ErrPostbackNotFound  = errors.New("no matching button")
	ErrPostbackAmbiguous = errors.New("multiple buttons match")
)

func isURLCTA(cta *slidetypes.CTAButton) bool {
	actionURL, err := url.Parse(cta.ActionURL)
	if err != nil {
		return false
	}
	actionURL = unwrapLinkShim(actionURL)
	return actionURL.Scheme == "https" || actionURL.Scheme == "http"
}

// PostbackButtonsFromCTAs extracts the non-URL buttons that have a payload.
func PostbackButtonsFromCTAs(ctas []*slidetypes.CTAButton) []*metaid.PostbackButton {
	var buttons []*metaid.PostbackButton
	for _, cta := range ctas {
		if cta == nil || cta.Title == "" || isURLCTA(cta) {
			continue
		}
		if payload := cta.Postback(); payload != "" {
			buttons = append(buttons, &metaid.PostbackButton{Title: cta.Title, Payload: payload, CTAType: cta.CTAType})
		}
	}
	return buttons
}

// xmaNeedsPostbackFetch returns true if the GraphQL data seems to be hiding postback payloads:
// a non-URL button without a payload (including placeholders), or a card with no buttons at all
// and no media/link target.
func xmaNeedsPostbackFetch(xma *slidetypes.XMAContent) bool {
	if xma == nil {
		return false
	}
	if len(xma.CTAButtons) == 0 {
		return xma.TargetID == "" && xma.TargetURL == "" && (xma.TitleText != "" || xma.SubtitleText != "" || xma.CaptionBodyText != "")
	}
	for _, cta := range xma.CTAButtons {
		if cta != nil && !isURLCTA(cta) && cta.Postback() == "" {
			return true
		}
	}
	return false
}

func ctaTitles(ctas []*slidetypes.CTAButton) []string {
	var titles []string
	for _, cta := range ctas {
		if cta != nil && cta.Title != "" {
			titles = append(titles, cta.Title)
		}
	}
	return titles
}

// ItemIDFromMessageID returns the Instagram item ID of a bridged message, or an empty string
// if the message ID isn't an Instagram one.
func ItemIDFromMessageID(id networkid.MessageID) string {
	if parsed, ok := metaid.ParseMessageID(id).(metaid.ParsedFBMessageID); ok {
		return parsed.ID
	}
	return ""
}

// PickPostbackButtons finds the postback buttons for a message in the generic_xma items of a
// thread.
//
// If wantItemID matches exactly one item, only that item is used: its buttons are returned,
// or nil if it has none (never another card's). Otherwise, wantTitles must be non-empty,
// and only items with all those button titles are considered; the one with the timestamp
// closest to tsMS wins (or the first one, which is the newest, if timestamps are missing).
// With no item ID match and no titles, nothing is returned.
func PickPostbackButtons(items []*slidetypes.ThreadItem, wantItemID string, wantTitles []string, tsMS int64) []*metaid.PostbackButton {
	if wantItemID != "" {
		var matched *slidetypes.ThreadItem
		matches := 0
		for _, item := range items {
			if item.ItemID == wantItemID {
				matched = item
				matches++
			}
		}
		if matches == 1 {
			return PostbackButtonsFromCTAs(matched.AllCTAButtons())
		}
	}
	if len(wantTitles) == 0 {
		return nil
	}
	var best []*metaid.PostbackButton
	var bestDiff int64 = -1
	for _, item := range items {
		ctas := item.AllCTAButtons()
		buttons := PostbackButtonsFromCTAs(ctas)
		if len(buttons) == 0 {
			continue
		}
		itemTitles := ctaTitles(ctas)
		if !isSubset(wantTitles, itemTitles) {
			continue
		}
		if tsMS == 0 || item.TimestampMS() == 0 {
			if best == nil {
				best = buttons
			}
			continue
		}
		diff := tsMS - item.TimestampMS()
		if diff < 0 {
			diff = -diff
		}
		if bestDiff < 0 || diff < bestDiff {
			best, bestDiff = buttons, diff
		}
	}
	return best
}

func isSubset(want, have []string) bool {
	for _, title := range want {
		if !slices.Contains(have, title) {
			return false
		}
	}
	return true
}

// PostbackThreadID returns the ID to use for the direct_v2 thread fetch: the long Instagram
// Direct thread ID, falling back to the short thread_fbid, which the REST endpoint 404s on.
func PostbackThreadID(meta *metaid.PortalMetadata) string {
	return cmp.Or(meta.IGThreadID, meta.IGID)
}

// recoverPostbackButtons returns the postback buttons of an XMA card, either from the
// GraphQL data or, if that data seems to hide them, with a targeted REST thread fetch.
func (mc *MessageConverter) recoverPostbackButtons(ctx context.Context, xma *slidetypes.XMAContent, msg *slidetypes.Message) []*metaid.PostbackButton {
	if !xmaNeedsPostbackFetch(xma) || !mediadl.ShouldFetchXMA(ctx) {
		return PostbackButtonsFromCTAs(xma.CTAButtons)
	}
	client, _ := ctx.Value(mediadl.ContextKeyIGClient).(*instameow.Client)
	portalMeta := portalMetaFromContext(ctx)
	if client == nil || portalMeta == nil {
		return PostbackButtonsFromCTAs(xma.CTAButtons)
	}
	threadID := PostbackThreadID(portalMeta)
	if threadID == "" {
		return PostbackButtonsFromCTAs(xma.CTAButtons)
	}
	log := zerolog.Ctx(ctx)
	// Workaround from #338: GraphQL doesn't expose the postback payloads.
	items, err := client.FetchThreadGenericXMAItems(ctx, threadID)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch thread to recover XMA postback buttons")
		return PostbackButtonsFromCTAs(xma.CTAButtons)
	}
	var tsMS int64
	var itemID string
	if msg != nil {
		tsMS = msg.TimestampMS.UnixMilli()
		itemID = msg.MessageID
	}
	buttons := PickPostbackButtons(items, itemID, ctaTitles(xma.CTAButtons), tsMS)
	log.Debug().Int("button_count", len(buttons)).Int("item_count", len(items)).Msg("Recovered XMA postback buttons")
	return buttons
}

// ResolvePostbackButton finds a button by 1-based index or by title (exact, then case-insensitive).
func ResolvePostbackButton(buttons []*metaid.PostbackButton, arg string) (*metaid.PostbackButton, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil, fmt.Errorf("%w: no button specified", ErrPostbackNotFound)
	}
	if idx, err := strconv.Atoi(arg); err == nil && isNumeric(arg) {
		if idx < 1 || idx > len(buttons) {
			return nil, fmt.Errorf("%w: button %d (there are %d buttons)", ErrPostbackNotFound, idx, len(buttons))
		}
		return buttons[idx-1], nil
	}
	for _, match := range []func(title string) bool{
		func(title string) bool { return title == arg },
		func(title string) bool { return strings.EqualFold(title, arg) },
	} {
		var found []*metaid.PostbackButton
		for _, button := range buttons {
			if match(button.Title) {
				found = append(found, button)
			}
		}
		if len(found) == 1 {
			return found[0], nil
		} else if len(found) > 1 {
			return nil, fmt.Errorf("%w: %q", ErrPostbackAmbiguous, arg)
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrPostbackNotFound, arg)
}

func portalMetaFromContext(ctx context.Context) *metaid.PortalMetadata {
	portal, _ := ctx.Value(mediadl.ContextKeyPortal).(*bridgev2.Portal)
	if portal == nil {
		return nil
	}
	meta, _ := portal.Metadata.(*metaid.PortalMetadata)
	return meta
}

// attachPostbackButtons stores the recovered buttons in the database metadata of the part.
func attachPostbackButtons(part *bridgev2.ConvertedMessagePart, buttons []*metaid.PostbackButton) {
	if part == nil || len(buttons) == 0 {
		return
	}
	meta, ok := part.DBMetadata.(*metaid.MessageMetadata)
	if !ok || meta == nil {
		meta = &metaid.MessageMetadata{}
		part.DBMetadata = meta
	}
	meta.PostbackButtons = buttons
}
