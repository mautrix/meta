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
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type attachFixture struct {
	connector *IGConnector
	matrix    *pollMatrixStub
	portal    *bridgev2.Portal
	login     *bridgev2.UserLogin
	card      *database.Message
	items     []*slidetypes.ThreadItem
	instagram atomic.Int32
}

func newAttachFixture(t *testing.T) *attachFixture {
	t.Helper()
	ctx := context.Background()
	db, err := dbutil.NewWithDialect(filepath.Join(t.TempDir(), "bridge.db"), "sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &attachFixture{matrix: &pollMatrixStub{}, connector: &IGConnector{}}
	bridge := bridgev2.NewBridge("test", db, zerolog.Nop(), nil, f.matrix, f.connector, commands.NewProcessor)
	bridge.BackgroundCtx = ctx
	f.connector.Bridge = bridge
	if err = bridge.DB.Upgrade(ctx); err != nil {
		t.Fatal(err)
	}
	f.portal = &bridgev2.Portal{Bridge: bridge, Portal: &database.Portal{
		PortalKey: networkid.PortalKey{ID: "thread"}, MXID: "!room:example.com",
		Metadata: &metaid.PortalMetadata{IGID: "send-thread", IGThreadID: "history-thread"},
	}}
	if err = bridge.DB.Portal.Insert(ctx, f.portal.Portal); err != nil {
		t.Fatal(err)
	}
	if err = bridge.DB.Ghost.Insert(ctx, &database.Ghost{ID: "card-ghost"}); err != nil {
		t.Fatal(err)
	}
	f.login = &bridgev2.UserLogin{UserLogin: &database.UserLogin{ID: "login"}}
	f.login.Client = &IGClient{Main: f.connector, UserLogin: f.login, sendTap: func(context.Context, string, *metaid.PostbackButton, string) (string, error) {
		f.instagram.Add(1)
		return "", nil
	}}
	f.card = &database.Message{
		ID: metaid.MakeFBMessageID("77"), MXID: "$card", Room: f.portal.PortalKey, SenderID: "card-ghost",
		Timestamp: time.Unix(1700000000, 0),
		Metadata: &metaid.MessageMetadata{
			PostbackButtons: []*metaid.PostbackButton{{Title: "Guide", Payload: "payload-a", CTAType: "postback"}},
			QuickReplies:    []*metaid.PostbackButton{{Title: "Maybe", Payload: "payload-b", CTAType: metaid.CTATypeQuickReply}},
		},
	}
	if err = bridge.DB.Message.Insert(ctx, f.card); err != nil {
		t.Fatal(err)
	}
	f.items = []*slidetypes.ThreadItem{{ItemID: "77", Text: "Pick one"}}
	return f
}

func (f *attachFixture) attach(card *database.Message) (bool, error) {
	return f.connector.attachButtonPoll(context.Background(), f.portal, f.login, f.items, card)
}

func (f *attachFixture) polls() []pollSend { return f.matrix.sendsOf(event.EventUnstablePollStart) }

func (f *attachFixture) parts(t *testing.T) []*database.Message {
	t.Helper()
	parts, err := f.connector.Bridge.DB.Message.GetAllPartsByID(context.Background(), "", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return parts
}

func TestAttachButtonPollAddsExactlyOne(t *testing.T) {
	f := newAttachFixture(t)
	attached, err := f.attach(f.card)
	if err != nil || !attached {
		t.Fatalf("attach = %v, %v", attached, err)
	}
	polls := f.polls()
	if len(polls) != 1 || polls[0].sender != "card-ghost" || polls[0].room != "!room:example.com" || !polls[0].ts.After(f.card.Timestamp) {
		t.Fatalf("expected one poll from the card's ghost right after the card, got %+v", polls)
	}
	start, ok := polls[0].raw["org.matrix.msc3381.poll.start"].(event.PollStart)
	if !ok || start.Question.Text != "Pick one" || start.MaxSelections != 1 || len(start.Answers) != 2 || start.Answers[0].Text != "Guide" || start.Answers[1].Text != "Maybe" {
		t.Errorf("unexpected poll: %+v", polls[0].raw)
	}
	if polls[0].parsed == nil || polls[0].parsed.Body == "" {
		t.Error("the poll needs a text fallback")
	}
	parts := f.parts(t)
	if len(parts) != 2 || parts[1].PartID != metaid.ButtonPollPartID || parts[1].MXID != "$sent1" || parts[1].SenderID != "card-ghost" || !isButtonPoll(parts[1]) {
		t.Fatalf("the poll must be saved next to the card: %+v", parts)
	}
	if poll := parts[1].Metadata.(*metaid.MessageMetadata); len(poll.PostbackButtons) != 1 || len(poll.QuickReplies) != 1 || poll.PollID != 0 {
		t.Errorf("unexpected poll metadata: %+v", poll)
	}

	if again, err := f.attach(f.card); again || err != nil {
		t.Errorf("second attach = %v, %v", again, err)
	}
	f.connector.pollAttached = nil // a restart: only the database knows about the poll now
	if again, err := f.attach(f.card); again || err != nil {
		t.Errorf("attach after a restart = %v, %v", again, err)
	}
	if len(f.polls()) != 1 || len(f.parts(t)) != 2 {
		t.Errorf("a card must never get a second poll: %d sent, %d parts", len(f.polls()), len(f.parts(t)))
	}
	if len(f.matrix.ends()) != 0 || f.instagram.Load() != 0 {
		t.Errorf("attaching must not end a poll (%d) or send to Instagram (%d)", len(f.matrix.ends()), f.instagram.Load())
	}
}

func TestAttachButtonPollSkips(t *testing.T) {
	ctx := context.Background()
	t.Run("poll already stored", func(t *testing.T) {
		f := newAttachFixture(t)
		existing := *f.card
		existing.PartID, existing.MXID = metaid.ButtonPollPartID, "$old-poll"
		existing.Metadata = &metaid.MessageMetadata{ButtonPoll: true}
		if err := f.connector.Bridge.DB.Message.Insert(ctx, &existing); err != nil {
			t.Fatal(err)
		}
		if attached, err := f.attach(f.card); attached || err != nil || len(f.matrix.sends) != 0 {
			t.Errorf("attach = %v, %v with %d sends", attached, err, len(f.matrix.sends))
		}
	})
	t.Run("nothing to tap or nowhere to send", func(t *testing.T) {
		f := newAttachFixture(t)
		for name, card := range map[string]*database.Message{
			"no buttons":  {ID: f.card.ID, MXID: "$x", Room: f.card.Room, Metadata: &metaid.MessageMetadata{}},
			"no event":    {ID: f.card.ID, Room: f.card.Room, Metadata: f.card.Metadata},
			"the poll":    {ID: f.card.ID, PartID: metaid.ButtonPollPartID, MXID: "$x", Room: f.card.Room, Metadata: &metaid.MessageMetadata{ButtonPoll: true, QuickReplies: f.card.Metadata.(*metaid.MessageMetadata).QuickReplies}},
			"a vote":      {ID: f.card.ID, PartID: buttonVotePartPrefix + "1", MXID: "$x", Room: f.card.Room, Metadata: f.card.Metadata},
			"no metadata": {ID: f.card.ID, MXID: "$x", Room: f.card.Room},
		} {
			if attached, err := f.attach(card); attached || err != nil {
				t.Errorf("%s: attach = %v, %v", name, attached, err)
			}
		}
		if len(f.matrix.sends) != 0 {
			t.Errorf("sent %d events", len(f.matrix.sends))
		}
	})
	t.Run("question falls back without history", func(t *testing.T) {
		f := newAttachFixture(t)
		f.items = nil
		if _, err := f.attach(f.card); err != nil {
			t.Fatal(err)
		}
		if start := f.polls()[0].raw["org.matrix.msc3381.poll.start"].(event.PollStart); start.Question.Text != "Responder" {
			t.Errorf("question %q", start.Question.Text)
		}
	})
}

func TestAttachButtonPollFailureIsRetryable(t *testing.T) {
	f := newAttachFixture(t)
	f.matrix.sendErr = errors.New("homeserver down")
	if attached, err := f.attach(f.card); attached || err == nil {
		t.Fatalf("attach = %v, %v", attached, err)
	}
	if len(f.parts(t)) != 1 {
		t.Fatal("a failed send left a poll row")
	}
	f.matrix.sendErr = nil
	if attached, err := f.attach(f.card); !attached || err != nil || len(f.polls()) != 1 {
		t.Fatalf("retry = %v, %v with %d polls", attached, err, len(f.polls()))
	}
}

func TestAttachButtonPollConcurrentRefreshesSendOnce(t *testing.T) {
	f := newAttachFixture(t)
	var wg sync.WaitGroup
	var attached atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := f.attach(f.card)
			if err != nil {
				t.Error(err)
			} else if ok {
				attached.Add(1)
			}
		}()
	}
	wg.Wait()
	if attached.Load() != 1 || len(f.polls()) != 1 || len(f.parts(t)) != 2 {
		t.Errorf("attached %d times, %d polls sent, %d parts", attached.Load(), len(f.polls()), len(f.parts(t)))
	}
}

const (
	// A database that rejects the row of the button poll, like a busy or full one.
	failPollRowInserts  = `CREATE TRIGGER fail_poll_rows BEFORE INSERT ON message WHEN NEW.part_id = 'buttonpoll' BEGIN SELECT RAISE(ABORT, 'injected insert failure'); END`
	allowPollRowInserts = `DROP TRIGGER fail_poll_rows`
	failKVWrites        = `CREATE TRIGGER fail_kv_writes BEFORE INSERT ON kv_store BEGIN SELECT RAISE(ABORT, 'injected kv failure'); END`
)

func (f *attachFixture) exec(t *testing.T, query string) {
	t.Helper()
	if _, err := f.connector.Bridge.DB.Exec(context.Background(), query); err != nil {
		t.Fatal(err)
	}
}

// restart drops every bit of in-memory state: only the bridge database is left.
func (f *attachFixture) restart() {
	f.connector = &IGConnector{Bridge: f.connector.Bridge}
}

func setInsertRetryWait(t *testing.T, wait func(context.Context, int)) {
	t.Helper()
	old := waitBeforeButtonPollInsertRetry
	waitBeforeButtonPollInsertRetry = wait
	t.Cleanup(func() { waitBeforeButtonPollInsertRetry = old })
}

func (f *attachFixture) markerValue() (string, error) {
	return f.connector.Bridge.DB.KV.GetWithError(context.Background(), pendingButtonPollKey(f.card))
}

func TestAttachButtonPollFailedInsertAfterSendIsRememberedAcrossRestart(t *testing.T) {
	ctx := context.Background()
	f := newAttachFixture(t)
	f.exec(t, failPollRowInserts)
	var retries int
	setInsertRetryWait(t, func(context.Context, int) { retries++ })

	if attached, err := f.attach(f.card); attached || err == nil {
		t.Fatalf("attach with a failing insert = %v, %v", attached, err)
	}
	if len(f.polls()) != 1 || retries != buttonPollInsertAttempts-1 || len(f.parts(t)) != 1 {
		t.Fatalf("%d polls sent, %d retries, %d parts", len(f.polls()), retries, len(f.parts(t)))
	}
	pending, err := f.connector.loadPendingButtonPoll(ctx, f.card)
	if err != nil || pending == nil || pending.EventID != "$sent1" || pending.Metadata == nil || !pending.Metadata.ButtonPoll ||
		len(pending.Metadata.PostbackButtons) != 1 || len(pending.Metadata.QuickReplies) != 1 {
		t.Fatalf("the sent poll must be remembered durably: %+v, %v", pending, err)
	}

	f.restart()
	if attached, err := f.attach(f.card); attached || err == nil {
		t.Fatalf("attach after a restart with the database still failing = %v, %v", attached, err)
	}
	if len(f.polls()) != 1 || len(f.parts(t)) != 1 {
		t.Fatalf("a restart must not send a second poll: %d sent, %d parts", len(f.polls()), len(f.parts(t)))
	}
	if _, err = f.markerValue(); err != nil {
		t.Fatalf("the record must stay until the row is saved: %v", err)
	}

	f.exec(t, allowPollRowInserts)
	f.restart()
	if attached, err := f.attach(f.card); !attached || err != nil {
		t.Fatalf("attach once the database works = %v, %v", attached, err)
	}
	parts := f.parts(t)
	if len(f.polls()) != 1 || len(parts) != 2 {
		t.Fatalf("%d polls sent, %d parts", len(f.polls()), len(parts))
	}
	poll := parts[1]
	meta, _ := poll.Metadata.(*metaid.MessageMetadata)
	if poll.PartID != metaid.ButtonPollPartID || poll.MXID != "$sent1" || poll.SenderID != "card-ghost" || poll.Room != f.card.Room ||
		!poll.Timestamp.Equal(f.card.Timestamp) || !isButtonPoll(poll) || len(meta.PostbackButtons) != 1 || len(meta.QuickReplies) != 1 {
		t.Errorf("the saved poll isn't the one that was sent: %+v (%+v)", poll, meta)
	}
	if _, err = f.markerValue(); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("the record must be gone once the row is saved: %v", err)
	}
	if again, err := f.attach(f.card); again || err != nil || len(f.polls()) != 1 || len(f.parts(t)) != 2 {
		t.Errorf("a later attach = %v, %v with %d polls, %d parts", again, err, len(f.polls()), len(f.parts(t)))
	}
}

func TestAttachButtonPollRetriesInsertBeforeParkingIt(t *testing.T) {
	f := newAttachFixture(t)
	f.exec(t, failPollRowInserts)
	var retries int
	setInsertRetryWait(t, func(context.Context, int) {
		if retries++; retries == 1 {
			f.exec(t, allowPollRowInserts)
		}
	})
	if attached, err := f.attach(f.card); !attached || err != nil {
		t.Fatalf("attach = %v, %v", attached, err)
	}
	if len(f.polls()) != 1 || len(f.parts(t)) != 2 || retries != 1 {
		t.Errorf("%d polls sent, %d parts, %d retries", len(f.polls()), len(f.parts(t)), retries)
	}
	if _, err := f.markerValue(); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("nothing needed parking: %v", err)
	}
}

func TestAttachButtonPollNeverResendsInProcessEvenIfNothingCouldBeSaved(t *testing.T) {
	f := newAttachFixture(t)
	f.exec(t, failPollRowInserts)
	f.exec(t, failKVWrites)
	setInsertRetryWait(t, func(context.Context, int) {})
	if attached, err := f.attach(f.card); attached || err == nil {
		t.Fatalf("attach = %v, %v", attached, err)
	}
	if attached, err := f.attach(f.card); attached || err != nil || len(f.polls()) != 1 {
		t.Errorf("second attach = %v, %v with %d polls", attached, err, len(f.polls()))
	}
}

func TestAttachButtonPollUnreadablePendingRecordNeverResends(t *testing.T) {
	f := newAttachFixture(t)
	f.connector.Bridge.DB.KV.Set(context.Background(), pendingButtonPollKey(f.card), "not json")
	if attached, err := f.attach(f.card); attached || err == nil {
		t.Fatalf("attach = %v, %v", attached, err)
	}
	f.connector.Bridge.DB.KV.Set(context.Background(), pendingButtonPollKey(f.card), `{"event_id":"$x"}`)
	if attached, err := f.attach(f.card); attached || err == nil {
		t.Fatalf("attach with an incomplete record = %v, %v", attached, err)
	}
	if len(f.matrix.sends) != 0 || len(f.parts(t)) != 1 {
		t.Errorf("%d events sent, %d parts: the poll may already exist, so nothing may be sent", len(f.matrix.sends), len(f.parts(t)))
	}
}
