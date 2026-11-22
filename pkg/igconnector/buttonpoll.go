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
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

var _ bridgev2.PollHandlingNetworkAPI = (*IGClient)(nil)

// buttonVotePartPrefix starts the part ID of the database row that records a vote on a button
// poll. The row belongs to the card's message ID, so a read receipt on the vote event still
// resolves to a real Instagram message, but it is Matrix-only: nothing may be sent to Instagram
// when it is removed.
const buttonVotePartPrefix = "buttonvote:"

var (
	errPollStartNotBridged = bridgev2.WrapErrorInStatus(errors.New("polls started in this room aren't bridged to Instagram")).
				WithIsCertain(true).WithErrorAsMessage().WithErrorReason(event.MessageStatusUnsupported).WithSendNotice(true)
	errNotButtonPoll = bridgev2.WrapErrorInStatus(errors.New("only the polls made from Instagram buttons can be voted on; the vote wasn't sent to Instagram")).
				WithIsCertain(true).WithErrorAsMessage().WithErrorReason(event.MessageStatusUnsupported).WithSendNotice(true)
	errInvalidButtonVote = bridgev2.WrapErrorInStatus(errors.New("choose exactly one of the buttons; the vote wasn't sent to Instagram")).
				WithIsCertain(true).WithErrorAsMessage().WithErrorReason(event.MessageStatusUnsupported)
	errButtonAlreadySent = bridgev2.WrapErrorInStatus(errors.New("this button was already sent to Instagram")).
				WithIsCertain(true).WithErrorAsMessage().WithErrorReason(event.MessageStatusUnsupported)
)

// isButtonVote reports whether the message is the row of a vote on a button poll.
func isButtonVote(msg *database.Message) bool {
	return msg != nil && strings.HasPrefix(string(msg.PartID), buttonVotePartPrefix)
}

// isMatrixOnlyButtonPart reports whether the message only exists in Matrix: a button poll or a
// vote on one. Its ID is the card's, so deleting it must not unsend the card on Instagram.
func isMatrixOnlyButtonPart(msg *database.Message) bool {
	return msg != nil && (isButtonPoll(msg) || isButtonVote(msg))
}

// HandleMatrixPollStart refuses polls created in the room: Instagram has no polls to receive them.
func (ic *IGClient) HandleMatrixPollStart(context.Context, *bridgev2.MatrixPollStart) (*bridgev2.MatrixMessageResponse, error) {
	return nil, errPollStartNotBridged
}

// buttonForPollVote returns the button that a vote selects and its 1-based index, or an error if
// the poll isn't a button poll or the vote doesn't select exactly one existing button.
func buttonForPollVote(poll *database.Message, content *event.PollResponseEventContent) (*metaid.PostbackButton, int, error) {
	var meta *metaid.MessageMetadata
	if poll != nil {
		meta, _ = poll.Metadata.(*metaid.MessageMetadata)
	}
	if meta == nil || !meta.ButtonPoll || len(meta.TapButtons()) == 0 {
		return nil, 0, errNotButtonPoll
	}
	if content == nil || len(content.Response.Answers) != 1 {
		return nil, 0, errInvalidButtonVote
	}
	buttons := meta.TapButtons()
	idx, ok := igconv.ButtonPollIndex(content.Response.Answers[0], len(buttons))
	if !ok || buttons[idx-1] == nil {
		return nil, 0, errInvalidButtonVote
	}
	return buttons[idx-1], idx, nil
}

// HandleMatrixPollVote taps the button that a vote on a button poll selects, then ends the poll.
// Votes never reach Instagram as poll votes, and only a button that was successfully sent closes
// the poll, so a failed send can be retried. A button is sent at most once, even if a signed link
// got there first: the vote then sends nothing, but still ends the poll and gets errButtonAlreadySent.
func (ic *IGClient) HandleMatrixPollVote(ctx context.Context, msg *bridgev2.MatrixPollVote) (*bridgev2.MatrixMessageResponse, error) {
	button, idx, err := buttonForPollVote(msg.VoteTo, msg.Content)
	if err != nil {
		return nil, err
	}
	once := &ic.Main.tapOnce
	var otid string
	var sentBefore bool
	pollAlreadyDone, err := once.do(tapKey(msg.Portal.PortalKey, msg.VoteTo.ID, 0, ""), func() error {
		buttonAlreadySent, err := once.do(tapKey(msg.Portal.PortalKey, msg.VoteTo.ID, idx, igconv.ButtonKind(button)), func() error {
			portalMeta, err := ic.ensureIGID(ctx, msg.Portal)
			if err != nil {
				return err
			}
			otid = strconv.FormatInt(getOTID(msg.InputTransactionID), 10)
			ic.addPendingTapOTID(otid)
			if _, err = ic.sendButtonTap(ctx, portalMeta.IGID, button, otid); err != nil {
				ic.pendingPostbackOTIDs.Remove(otid)
				return wrapInstagramError(err)
			}
			zerolog.Ctx(ctx).Info().
				Str("portal_id", string(msg.Portal.ID)).
				Str("message_id", string(msg.VoteTo.ID)).
				Int("index", idx).
				Str("kind", string(igconv.ButtonKind(button))).
				Str("title", button.Title).
				Msg("Instagram button tap sent from poll vote")
			return nil
		})
		if err != nil {
			return err
		}
		// The button is sent exactly once, by this vote or by a signed link that got there
		// first. Either way the poll is done. Returning nil also records that, so later votes
		// don't end it again.
		sentBefore = buttonAlreadySent
		ic.endButtonPoll(ctx, msg.Portal, msg.VoteTo)
		return nil
	})
	if err != nil {
		return nil, err
	} else if pollAlreadyDone || sentBefore {
		return nil, errButtonAlreadySent
	}
	return &bridgev2.MatrixMessageResponse{DB: &database.Message{
		ID:        msg.VoteTo.ID,
		PartID:    networkid.PartID(buttonVotePartPrefix + otid),
		SenderID:  networkid.UserID(ic.UserLogin.ID),
		Metadata:  &metaid.MessageMetadata{},
		Timestamp: time.Now(),
	}}, nil
}

// endButtonPoll closes the poll as the Matrix user that created it. The tap is already sent, so a
// failure here is only logged: the poll just stays open, and later votes are ignored.
func (ic *IGClient) endButtonPoll(ctx context.Context, portal *bridgev2.Portal, poll *database.Message) {
	log := zerolog.Ctx(ctx).With().Str("message_id", string(poll.ID)).Logger()
	intent, err := messageSenderIntent(ctx, ic.Main.Bridge, portal, ic.UserLogin, poll)
	if err != nil || intent == nil {
		log.Warn().Err(err).Msg("Failed to get the poll creator's intent to end the button poll")
		return
	}
	_, err = intent.SendMessage(ctx, portal.MXID, event.EventUnstablePollEnd, &event.Content{Raw: map[string]any{
		"m.relates_to": map[string]any{
			"rel_type": string(event.RelReference),
			"event_id": poll.MXID,
		},
		"org.matrix.msc3381.poll.end": map[string]any{},
	}}, nil)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to end the button poll")
	}
}

// messageSenderIntent returns the Matrix user that sent the bridged message: a double puppet if
// it was sent by one, otherwise the ghost of its sender. Unlike Portal.GetIntentFor it never
// refreshes the Instagram profile, so refreshing cards only reads thread history.
func messageSenderIntent(ctx context.Context, br *bridgev2.Bridge, portal *bridgev2.Portal, login *bridgev2.UserLogin, msg *database.Message) (bridgev2.MatrixAPI, error) {
	if msg.SenderMXID != "" {
		return portal.Internal().GetIntentForMXID(ctx, msg.SenderMXID)
	}
	if msg.SenderID == "" {
		return br.Bot, nil
	}
	if senderLogin := br.GetCachedUserLoginByID(networkid.UserLoginID(msg.SenderID)); senderLogin != nil && (senderLogin.ID == login.ID || portal.Receiver == "") {
		if intent := senderLogin.User.DoublePuppet(ctx); intent != nil {
			return intent, nil
		}
	}
	return br.Matrix.GhostIntent(msg.SenderID), nil
}
