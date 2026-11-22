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
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

// keycapButton returns the button that a keycap reaction (1️⃣-🔟) activates on the target
// message, or nil if the emoji isn't a keycap or the message has no button at that index.
func keycapButton(target *database.Message, emoji string) *metaid.PostbackButton {
	idx, ok := igconv.KeycapReactionIndex(emoji)
	if !ok || target == nil {
		return nil
	}
	meta, ok := target.Metadata.(*metaid.MessageMetadata)
	if !ok {
		return nil
	}
	return igconv.ButtonAtIndex(meta.TapButtons(), idx)
}

// handleKeycapReaction activates a button for a keycap reaction instead of sending the
// reaction to Instagram. The returned reaction only exists in Matrix.
func (ic *IGClient) handleKeycapReaction(ctx context.Context, msg *bridgev2.MatrixReaction, portalMeta *metaid.PortalMetadata, button *metaid.PostbackButton) (*database.Reaction, error) {
	otid := strconv.FormatInt(getOTID(msg.InputTransactionID), 10)
	ic.addPendingTapOTID(otid)
	if _, err := ic.sendButtonTap(ctx, portalMeta.IGID, button, otid); err != nil {
		ic.pendingPostbackOTIDs.Remove(otid)
		return nil, wrapInstagramError(err)
	}
	zerolog.Ctx(ctx).Debug().Bool("quick_reply", button.IsQuickReply()).Msg("Sent keycap reaction as a button tap")
	return &database.Reaction{Timestamp: time.Now()}, nil
}

// isKeycapButtonReaction reports whether a Matrix reaction was a keycap that activated a
// button (and so was never sent to Instagram).
func (ic *IGClient) isKeycapButtonReaction(ctx context.Context, portal *bridgev2.Portal, reaction *database.Reaction) bool {
	if _, ok := igconv.KeycapReactionIndex(reaction.Emoji); !ok {
		return false
	}
	target, err := ic.Main.Bridge.DB.Message.GetFirstPartByID(ctx, portal.Receiver, reaction.MessageID)
	if err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to look up reaction target message")
	}
	return skipInstagramKeycapRemove(err, target, reaction.Emoji)
}

// skipInstagramKeycapRemove is true when a keycap reaction should not be deleted on Instagram:
// the target could not be loaded, or the keycap would have activated a button (so the reaction
// was never sent to Instagram).
func skipInstagramKeycapRemove(lookupErr error, target *database.Message, emoji string) bool {
	if lookupErr != nil || target == nil {
		return true
	}
	return keycapButton(target, emoji) != nil
}
