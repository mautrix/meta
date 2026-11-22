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
	"slices"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/format"

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
	return cta.IsLink()
}

// Buttons are the tappable buttons recovered for a message. Postbacks are sent with
// cta_type=postback, quick replies as plain text.
type Buttons struct {
	Postbacks    []*metaid.PostbackButton
	QuickReplies []*metaid.PostbackButton
}

// Empty reports whether there are no tappable buttons.
func (b Buttons) Empty() bool {
	return len(b.Postbacks) == 0 && len(b.QuickReplies) == 0
}

// All returns the combined list in display order (postbacks first), the one numbers refer to.
func (b Buttons) All() []*metaid.PostbackButton {
	return metaid.TapButtons(b.Postbacks, b.QuickReplies)
}

func (b *Buttons) add(other Buttons) {
	b.Postbacks = append(b.Postbacks, other.Postbacks...)
	b.QuickReplies = append(b.QuickReplies, other.QuickReplies...)
}

// ButtonsFromCTAs classifies the non-link CTAs: postbacks (cta_type postback, GT-PB payload or
// platform_token.postback, with a recovered payload) and quick replies (any other titled non-link
// button, with or without a payload). Links are skipped.
func ButtonsFromCTAs(ctas []*slidetypes.CTAButton) Buttons {
	var out Buttons
	for _, cta := range ctas {
		if cta == nil || cta.Title == "" || isURLCTA(cta) {
			continue
		}
		if cta.IsPostback() {
			if payload := cta.Postback(); payload != "" {
				out.Postbacks = append(out.Postbacks, &metaid.PostbackButton{Title: cta.Title, Payload: payload, CTAType: cta.CTAType})
			}
			continue
		}
		out.QuickReplies = append(out.QuickReplies, &metaid.PostbackButton{Title: cta.Title, Payload: cta.Postback(), CTAType: metaid.CTATypeQuickReply})
	}
	return out
}

// PostbackButtonsFromCTAs extracts only the postback buttons of the CTAs.
func PostbackButtonsFromCTAs(ctas []*slidetypes.CTAButton) []*metaid.PostbackButton {
	return ButtonsFromCTAs(ctas).Postbacks
}

// ButtonsFromQuickReplies converts REST quick_replies entries. An entry whose payload carries
// the GT-PB marker is a postback; everything else is a quick reply.
func ButtonsFromQuickReplies(qrs []*slidetypes.QuickReply) Buttons {
	var out Buttons
	for _, qr := range qrs {
		if qr == nil || qr.Title == "" {
			continue
		}
		payload := qr.PayloadValue()
		if strings.Contains(payload, slidetypes.PostbackPayloadMarker) {
			out.Postbacks = append(out.Postbacks, &metaid.PostbackButton{Title: qr.Title, Payload: payload, CTAType: "postback"})
		} else {
			out.QuickReplies = append(out.QuickReplies, &metaid.PostbackButton{Title: qr.Title, Payload: payload, CTAType: metaid.CTATypeQuickReply})
		}
	}
	return out
}

// ButtonsFromItem returns the buttons of a REST thread item: its CTAs, then its quick replies.
func ButtonsFromItem(item *slidetypes.ThreadItem) Buttons {
	out := ButtonsFromCTAs(item.AllCTAButtons())
	out.add(ButtonsFromQuickReplies(item.AllQuickReplies()))
	return out
}

func itemTitles(item *slidetypes.ThreadItem) []string {
	titles := ctaTitles(item.AllCTAButtons())
	for _, qr := range item.AllQuickReplies() {
		titles = append(titles, qr.Title)
	}
	return titles
}

func isQuickReplyCTAType(ctaType string) bool {
	return strings.EqualFold(ctaType, "quick_reply") || strings.EqualFold(ctaType, "quick_replies")
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
		if cta != nil && !isURLCTA(cta) && cta.Postback() == "" && !isQuickReplyCTAType(cta.CTAType) {
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

// PickPostbackButtons is PickButtons restricted to the postback buttons.
func PickPostbackButtons(items []*slidetypes.ThreadItem, wantItemID string, wantTitles []string, tsMS int64) []*metaid.PostbackButton {
	return PickButtons(items, wantItemID, wantTitles, tsMS).Postbacks
}

// PickButtons finds the buttons (postbacks and quick replies) for a message in the button items
// of a thread.
//
// If wantItemID matches exactly one item, only that item is used: its buttons are returned,
// or none if it has none (never another card's). Otherwise, wantTitles must be non-empty,
// and only items with all those button titles are considered; the one with the timestamp
// closest to tsMS wins (or the first one, which is the newest, if timestamps are missing).
// With no item ID match and no titles, nothing is returned.
func PickButtons(items []*slidetypes.ThreadItem, wantItemID string, wantTitles []string, tsMS int64) Buttons {
	if wantItemID != "" {
		var matched *slidetypes.ThreadItem
		matches := 0
		for _, item := range items {
			if item.MatchesID(wantItemID) {
				matched = item
				matches++
			}
		}
		if matches == 1 {
			return ButtonsFromItem(matched)
		}
	}
	if len(wantTitles) == 0 {
		return Buttons{}
	}
	var best Buttons
	found := false
	var bestDiff int64 = -1
	for _, item := range items {
		buttons := ButtonsFromItem(item)
		if buttons.Empty() || !isSubset(wantTitles, itemTitles(item)) {
			continue
		}
		if tsMS == 0 || item.TimestampMS() == 0 {
			if !found {
				best, found = buttons, true
			}
			continue
		}
		diff := tsMS - item.TimestampMS()
		if diff < 0 {
			diff = -diff
		}
		if bestDiff < 0 || diff < bestDiff {
			best, bestDiff, found = buttons, diff, true
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
	if meta == nil {
		return ""
	}
	return cmp.Or(meta.IGThreadID, meta.IGID)
}

// fetchButtonItems fetches the thread's button items for recovering payloads. It returns false
// if fetching isn't allowed or possible here.
func (mc *MessageConverter) fetchButtonItems(ctx context.Context) ([]*slidetypes.ThreadItem, bool) {
	if !mediadl.ShouldFetchXMA(ctx) {
		return nil, false
	}
	client, _ := ctx.Value(mediadl.ContextKeyIGClient).(*instameow.Client)
	portalMeta := portalMetaFromContext(ctx)
	if client == nil || portalMeta == nil {
		return nil, false
	}
	threadID := PostbackThreadID(portalMeta)
	if threadID == "" {
		return nil, false
	}
	// Workaround from #338: GraphQL doesn't expose the postback payloads.
	items, err := client.FetchThreadButtonItems(ctx, threadID)
	if err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to fetch thread to recover buttons")
		return nil, false
	}
	return items, true
}

// recoverButtons returns the buttons of an XMA card, either from the GraphQL data or, if that
// data seems to hide them, with a targeted REST thread fetch.
func (mc *MessageConverter) recoverButtons(ctx context.Context, xma *slidetypes.XMAContent, msg *slidetypes.Message) Buttons {
	fromGraphQL := ButtonsFromCTAs(xma.CTAButtons)
	if !xmaNeedsPostbackFetch(xma) {
		return fromGraphQL
	}
	items, ok := mc.fetchButtonItems(ctx)
	if !ok {
		return fromGraphQL
	}
	var tsMS int64
	var itemID string
	if msg != nil {
		tsMS = msg.TimestampMS.UnixMilli()
		itemID = msg.MessageID
	}
	buttons := PickButtons(items, itemID, ctaTitles(xma.CTAButtons), tsMS)
	zerolog.Ctx(ctx).Debug().
		Int("postback_count", len(buttons.Postbacks)).
		Int("quick_reply_count", len(buttons.QuickReplies)).
		Int("item_count", len(items)).
		Msg("Recovered XMA buttons")
	return buttons
}

// recoverTextQuickReplies returns the quick replies of a text message. GraphQL has no
// first-class field for them, so they're read from quick_replies in the unrecognized keys of the
// message or its content; if only has_quick_replies is present, the thread is fetched.
func (mc *MessageConverter) recoverTextQuickReplies(ctx context.Context, msg *slidetypes.Message, contentUnrecognized map[string]any) Buttons {
	if msg == nil {
		return Buttons{}
	}
	var out Buttons
	hint := false
	for _, m := range []map[string]any{msg.Unrecognized, contentUnrecognized} {
		if raw, ok := m["quick_replies"]; ok {
			hint = true
			out.add(ButtonsFromQuickReplies(slidetypes.QuickRepliesFromAny(raw)))
		}
		if has, _ := m["has_quick_replies"].(bool); has {
			hint = true
		}
	}
	if !out.Empty() || !hint {
		return out
	}
	items, ok := mc.fetchButtonItems(ctx)
	if !ok {
		return out
	}
	return PickButtons(items, msg.MessageID, nil, 0)
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

// attachButtons stores the recovered buttons in the database metadata of the part.
func attachButtons(part *bridgev2.ConvertedMessagePart, buttons Buttons) {
	if part == nil || buttons.Empty() {
		return
	}
	meta, ok := part.DBMetadata.(*metaid.MessageMetadata)
	if !ok || meta == nil {
		meta = &metaid.MessageMetadata{}
		part.DBMetadata = meta
	}
	meta.PostbackButtons = buttons.Postbacks
	meta.QuickReplies = buttons.QuickReplies
}

// appendButtonsToText appends the button list to a text part and stores the buttons.
func appendButtonsToText(part *bridgev2.ConvertedMessagePart, buttons Buttons, opts CTAOptions) {
	if part == nil || part.Content == nil || buttons.Empty() {
		return
	}
	buttonsHTML := xmaCTAButtonsHTML(nil, buttons.Postbacks, buttons.QuickReplies, opts)
	if buttonsHTML == "" {
		return
	}
	part.Content.EnsureHasHTML()
	part.Content.Body += "\n\n" + format.HTMLToContent(buttonsHTML).Body
	part.Content.FormattedBody += buttonsHTML
	attachButtons(part, buttons)
}

// MatchUserPostback checks whether a message from the user should be a button tap instead of text.
// For replies, the text can be a 1-based number or a title. Otherwise only a unique title matches,
// so that a plain "1" in normal chat is still sent as text. Callers must send the text normally
// when an error is returned.
func MatchUserPostback(buttons []*metaid.PostbackButton, text string, isReply bool) (*metaid.PostbackButton, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("%w: empty text", ErrPostbackNotFound)
	}
	if isReply {
		return ResolvePostbackButton(buttons, text)
	}
	if isNumeric(text) {
		return nil, fmt.Errorf("%w: %q", ErrPostbackNotFound, text)
	}
	return ResolvePostbackButton(buttons, text)
}
