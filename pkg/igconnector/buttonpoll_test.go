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
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/exsync"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

type pollSend struct {
	sender networkid.UserID
	room   id.RoomID
	typ    event.Type
	parsed *event.MessageEventContent
	raw    map[string]any
	ts     time.Time
}

type pollMatrixStub struct {
	bridgev2.MatrixConnector
	bridgev2.MatrixAPI
	mu    sync.Mutex
	sends []pollSend
	// sendErr is returned instead of sending while set.
	sendErr error
}

func (m *pollMatrixStub) Init(*bridgev2.Bridge)         {}
func (m *pollMatrixStub) BotIntent() bridgev2.MatrixAPI { return &pollIntentStub{m, "bot"} }
func (m *pollMatrixStub) GhostIntent(userID networkid.UserID) bridgev2.MatrixAPI {
	return &pollIntentStub{m, userID}
}

type pollIntentStub struct {
	*pollMatrixStub
	sender networkid.UserID
}

func (i *pollIntentStub) SendMessage(_ context.Context, room id.RoomID, typ event.Type, content *event.Content, extra *bridgev2.MatrixSendExtra) (*mautrix.RespSendEvent, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sendErr != nil {
		return nil, i.sendErr
	}
	send := pollSend{sender: i.sender, room: room, typ: typ, raw: content.Raw}
	send.parsed, _ = content.Parsed.(*event.MessageEventContent)
	if extra != nil {
		send.ts = extra.Timestamp
	}
	i.sends = append(i.sends, send)
	return &mautrix.RespSendEvent{EventID: id.EventID("$sent" + strconv.Itoa(len(i.sends)))}, nil
}

func (m *pollMatrixStub) sendsOf(typ event.Type) []pollSend {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []pollSend
	for _, send := range m.sends {
		if send.typ == typ {
			out = append(out, send)
		}
	}
	return out
}

func (m *pollMatrixStub) ends() []pollSend { return m.sendsOf(event.EventUnstablePollEnd) }

type buttonPollFixture struct {
	ic        *IGClient
	connector *IGConnector
	matrix    *pollMatrixStub
	portal    *bridgev2.Portal
	poll      *database.Message
	buttons   []*metaid.PostbackButton

	sendMu sync.Mutex
	sent   []*metaid.PostbackButton
	otids  []string
	// sendErr is returned by the stubbed Instagram send while set.
	sendErr atomic.Pointer[error]
	// gate, if set, blocks the stubbed Instagram send until it is closed.
	gate chan struct{}
}

func (f *buttonPollFixture) sends() int {
	f.sendMu.Lock()
	defer f.sendMu.Unlock()
	return len(f.sent)
}

func newButtonPollFixture(t *testing.T) *buttonPollFixture {
	t.Helper()
	db, err := dbutil.NewWithDialect(filepath.Join(t.TempDir(), "bridge.db"), "sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &buttonPollFixture{matrix: &pollMatrixStub{}, connector: &IGConnector{}}
	bridge := bridgev2.NewBridge("test", db, zerolog.Nop(), nil, f.matrix, f.connector, commands.NewProcessor)
	f.connector.Bridge = bridge
	f.portal = &bridgev2.Portal{Bridge: bridge, Portal: &database.Portal{
		PortalKey: networkid.PortalKey{ID: "thread", Receiver: "login"},
		MXID:      "!room:example.com",
		Metadata:  &metaid.PortalMetadata{IGID: "send-thread", IGThreadID: "history-thread"},
	}}
	login := &bridgev2.UserLogin{UserLogin: &database.UserLogin{ID: "login"}}
	f.ic = &IGClient{Main: f.connector, UserLogin: login, pendingPostbackOTIDs: exsync.NewSet[string]()}
	f.ic.sendTap = func(_ context.Context, threadID string, button *metaid.PostbackButton, clientContext string) (string, error) {
		if threadID != "send-thread" {
			t.Errorf("sent to thread %q", threadID)
		}
		if f.gate != nil {
			<-f.gate
		}
		if err := f.sendErr.Load(); err != nil && *err != nil {
			return "", *err
		}
		f.sendMu.Lock()
		defer f.sendMu.Unlock()
		f.sent = append(f.sent, button)
		f.otids = append(f.otids, clientContext)
		return "", nil
	}
	f.buttons = []*metaid.PostbackButton{
		{Title: "Guide", Payload: "payload-a", CTAType: "postback"},
		{Title: "Maybe", Payload: "payload-b", CTAType: metaid.CTATypeQuickReply},
	}
	f.poll = &database.Message{
		ID: "mid:card", PartID: metaid.ButtonPollPartID, MXID: "$poll", Room: f.portal.PortalKey, SenderID: "card-ghost",
		Metadata: &metaid.MessageMetadata{
			ButtonPoll: true, PostbackButtons: f.buttons[:1], QuickReplies: f.buttons[1:],
		},
	}
	return f
}

func (f *buttonPollFixture) vote(txnID string, answers ...string) (*bridgev2.MatrixMessageResponse, error) {
	return f.ic.HandleMatrixPollVote(context.Background(), &bridgev2.MatrixPollVote{
		MatrixMessage: bridgev2.MatrixMessage{MatrixEventBase: bridgev2.MatrixEventBase[*event.MessageEventContent]{
			Portal: f.portal, InputTransactionID: networkid.RawTransactionID(txnID),
		}},
		VoteTo:  f.poll,
		Content: &event.PollResponseEventContent{Response: event.PollResponse{Answers: answers}},
	})
}

func certainStatus(t *testing.T, err error) bridgev2.MessageStatus {
	t.Helper()
	var status bridgev2.MessageStatus
	if !errors.As(err, &status) || !status.IsCertain {
		t.Fatalf("want a certain message status, got %#v", err)
	}
	return status
}

func TestPollVoteTapsOnceAndEndsAsCardSender(t *testing.T) {
	f := newButtonPollFixture(t)
	resp, err := f.vote("12345", "2")
	if err != nil {
		t.Fatal(err)
	}
	if f.sends() != 1 || f.sent[0].Payload != "payload-b" || !f.sent[0].IsQuickReply() || f.otids[0] != "12345" {
		t.Fatalf("sent %+v with OTIDs %v", f.sent, f.otids)
	}
	if !f.ic.pendingPostbackOTIDs.Has("12345") {
		t.Error("the echo of the tap must be correlated by its OTID")
	}
	if resp == nil || resp.DB == nil || resp.Pending || resp.DB.ID != "mid:card" || !isButtonVote(resp.DB) || resp.DB.SenderID != "login" {
		t.Errorf("unsaveable response: %+v", resp)
	}
	ends := f.matrix.ends()
	if len(ends) != 1 {
		t.Fatalf("expected one poll end, got %d", len(ends))
	}
	end := ends[0]
	rel, _ := end.raw["m.relates_to"].(map[string]any)
	if end.sender != "card-ghost" || end.room != "!room:example.com" ||
		rel["rel_type"] != "m.reference" || rel["event_id"] != id.EventID("$poll") {
		t.Errorf("end not sent by the card's ghost against the poll: %+v", end)
	}
	if body, ok := end.raw["org.matrix.msc3381.poll.end"].(map[string]any); !ok || len(body) != 0 {
		t.Errorf("unexpected poll.end body: %#v", end.raw["org.matrix.msc3381.poll.end"])
	}

	for _, answer := range []string{"2", "1"} {
		if _, err = f.vote("", answer); !errors.Is(err, errButtonAlreadySent) {
			t.Errorf("vote %q after the send: %v", answer, err)
		}
	}
	if f.sends() != 1 || len(f.matrix.ends()) != 1 {
		t.Errorf("repeat votes must not send (%d) or end (%d) again", f.sends(), len(f.matrix.ends()))
	}
}

func TestPollVoteMapsEachOptionToItsButton(t *testing.T) {
	f := newButtonPollFixture(t)
	if _, err := f.vote("", "1"); err != nil {
		t.Fatal(err)
	}
	if f.sent[0].Payload != "payload-a" || f.sent[0].IsQuickReply() || f.otids[0] == "" {
		t.Errorf("option 1 sent %+v with OTID %q", f.sent[0], f.otids[0])
	}
}

func TestPollVoteRejectsWhatIsNotAButtonVote(t *testing.T) {
	f := newButtonPollFixture(t)
	f.poll.Metadata = &metaid.MessageMetadata{PollID: 42, PollQuestion: "Lunch?", PostbackButtons: f.buttons}
	if _, err := f.vote("", "1"); !errors.Is(err, errNotButtonPoll) {
		t.Errorf("a real poll with buttons stored: %v", err)
	}
	f.poll.Metadata = &metaid.MessageMetadata{ButtonPoll: true}
	if _, err := f.vote("", "1"); !errors.Is(err, errNotButtonPoll) {
		t.Errorf("a button poll without buttons: %v", err)
	}
	f.poll.Metadata = &metaid.MessageMetadata{ButtonPoll: true, PostbackButtons: f.buttons}
	for _, answers := range [][]string{nil, {"1", "2"}, {"0"}, {"3"}, {"Guide"}, {"01"}} {
		if _, err := f.vote("", answers...); !errors.Is(err, errInvalidButtonVote) {
			t.Errorf("answers %v: %v", answers, err)
		}
	}
	certainStatus(t, errNotButtonPoll)
	if f.sends() != 0 || len(f.matrix.sends) != 0 {
		t.Errorf("nothing may be sent or ended: %d taps, %d events", f.sends(), len(f.matrix.sends))
	}
}

func TestPollStartIsNotBridged(t *testing.T) {
	f := newButtonPollFixture(t)
	resp, err := f.ic.HandleMatrixPollStart(context.Background(), &bridgev2.MatrixPollStart{
		MatrixMessage: bridgev2.MatrixMessage{MatrixEventBase: bridgev2.MatrixEventBase[*event.MessageEventContent]{Portal: f.portal}},
		Content:       &event.PollStartEventContent{PollStart: event.PollStart{MaxSelections: 1}},
	})
	if resp != nil || !errors.Is(err, errPollStartNotBridged) {
		t.Fatalf("resp %+v, err %v", resp, err)
	}
	if status := certainStatus(t, err); !status.SendNotice || !status.ErrorAsMessage {
		t.Errorf("the user must be told why: %+v", status)
	}
	if f.sends() != 0 || len(f.matrix.sends) != 0 {
		t.Error("a poll start must not reach Instagram")
	}
}

func TestPollVoteFailedSendCanBeRetried(t *testing.T) {
	f := newButtonPollFixture(t)
	failure := errors.New("socket closed")
	f.sendErr.Store(&failure)
	if resp, err := f.vote("777", "1"); err == nil || resp != nil {
		t.Fatalf("a failed send must fail the vote, got %+v, %v", resp, err)
	}
	if f.sends() != 0 || len(f.matrix.ends()) != 0 || f.ic.pendingPostbackOTIDs.Has("777") {
		t.Fatalf("failed send left state behind: %d sends, %d ends, pending OTID %v", f.sends(), len(f.matrix.ends()), f.ic.pendingPostbackOTIDs.Has("777"))
	}
	var none error
	f.sendErr.Store(&none)
	if _, err := f.vote("778", "1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if f.sends() != 1 || len(f.matrix.ends()) != 1 {
		t.Errorf("retry: %d sends, %d ends", f.sends(), len(f.matrix.ends()))
	}
}

func TestPollVotesInFlightShareOneSend(t *testing.T) {
	f := newButtonPollFixture(t)
	f.gate = make(chan struct{})
	const voters = 8
	var waiting atomic.Int32
	waitersReady := make(chan struct{})
	f.connector.tapOnce.onWait = func() {
		if waiting.Add(1) == voters-1 {
			close(waitersReady)
		}
	}
	var wg sync.WaitGroup
	var succeeded, rejected atomic.Int32
	for range voters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.vote("", "2"); err == nil {
				succeeded.Add(1)
			} else if errors.Is(err, errButtonAlreadySent) {
				rejected.Add(1)
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	<-waitersReady
	close(f.gate)
	wg.Wait()
	if succeeded.Load() != 1 || rejected.Load() != voters-1 || f.sends() != 1 || len(f.matrix.ends()) != 1 {
		t.Errorf("succeeded %d, rejected %d, sends %d, ends %d", succeeded.Load(), rejected.Load(), f.sends(), len(f.matrix.ends()))
	}
}

func TestPollVoteAndSignedLinkShareOneSend(t *testing.T) {
	signer := igconv.NewTapSigner([]byte("test-only-poll-key"))
	target := func(f *buttonPollFixture, index int, kind igconv.TapKind) igconv.TapTarget {
		return igconv.TapTarget{Portal: f.portal.PortalKey, MessageID: f.poll.ID, Index: index, Kind: kind}
	}
	newHandler := func(f *buttonPollFixture, linkSends *atomic.Int32) *tapHandler {
		return &tapHandler{signer: signer, log: zerolog.Nop(), once: &f.connector.tapOnce, resolve: func(_ context.Context, got igconv.TapTarget) (*tapAction, error) {
			return &tapAction{button: igconv.ButtonAtIndex(f.buttons, got.Index), send: func(context.Context, *metaid.PostbackButton) error {
				linkSends.Add(1)
				return nil
			}}, nil
		}}
	}
	click := func(h *tapHandler, tt igconv.TapTarget) int {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "http://example.com/_ig/tap/"+signer.Sign(tt), nil))
		return w.Code
	}

	t.Run("link first, then vote", func(t *testing.T) {
		f := newButtonPollFixture(t)
		var linkSends atomic.Int32
		h := newHandler(f, &linkSends)
		if click(h, target(f, 2, igconv.TapQuickReply)) != http.StatusOK || linkSends.Load() != 1 {
			t.Fatal("the link must send")
		}
		if _, err := f.vote("", "2"); !errors.Is(err, errButtonAlreadySent) {
			t.Fatalf("vote on the clicked button: %v", err)
		}
		if f.sends() != 0 {
			t.Errorf("the vote sent %d taps to Instagram after the link already sent", f.sends())
		}
		ends := f.matrix.ends()
		if len(ends) != 1 {
			t.Fatalf("the button was sent, so the vote must end the poll: %d ends", len(ends))
		}
		if rel, _ := ends[0].raw["m.relates_to"].(map[string]any); ends[0].sender != "card-ghost" || rel["event_id"] != id.EventID("$poll") {
			t.Errorf("end not sent by the card's ghost against the poll: %+v", ends[0])
		}
		for _, answer := range []string{"2", "1"} {
			if _, err := f.vote("", answer); !errors.Is(err, errButtonAlreadySent) {
				t.Errorf("vote %q on the ended poll: %v", answer, err)
			}
		}
		if f.sends() != 0 || len(f.matrix.ends()) != 1 {
			t.Errorf("votes on the ended poll sent %d taps and %d more ends", f.sends(), len(f.matrix.ends())-1)
		}
		if click(h, target(f, 1, igconv.TapPostback)) != http.StatusOK || linkSends.Load() != 2 {
			t.Error("a link for a different button must still send")
		}
	})
	t.Run("vote first, then link", func(t *testing.T) {
		f := newButtonPollFixture(t)
		var linkSends atomic.Int32
		h := newHandler(f, &linkSends)
		if _, err := f.vote("", "2"); err != nil {
			t.Fatal(err)
		}
		if click(h, target(f, 2, igconv.TapQuickReply)) != http.StatusOK || linkSends.Load() != 0 || f.sends() != 1 {
			t.Errorf("the link sent again: %d link sends, %d sends", linkSends.Load(), f.sends())
		}
		if click(h, target(f, 1, igconv.TapPostback)) != http.StatusOK || linkSends.Load() != 1 {
			t.Error("a link for a different button must still send")
		}
	})
}

func TestMatrixOnlyButtonPartsAreNotUnsent(t *testing.T) {
	f := newButtonPollFixture(t)
	vote := &database.Message{ID: "mid:card", PartID: buttonVotePartPrefix + "1"}
	for name, msg := range map[string]*database.Message{"poll": f.poll, "vote": vote} {
		// The client has no login: reaching the Instagram unsend would panic.
		if err := f.ic.HandleMatrixMessageRemove(context.Background(), &bridgev2.MatrixMessageRemove{TargetMessage: msg}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if isMatrixOnlyButtonPart(&database.Message{ID: "mid:card", Metadata: &metaid.MessageMetadata{QuickReplies: f.buttons}}) {
		t.Error("the card itself is a real Instagram message")
	}
}

func TestInstagramCapsDoNotOfferPollCreation(t *testing.T) {
	var unset event.CapabilitySupportLevel
	for name, caps := range map[string]*event.RoomFeatures{"dm": igCaps, "group": igCapsGroup, "self": igCapsSelf} {
		if caps.Poll != unset || caps.PollEnd != unset || caps.PollMaxOptions != 0 {
			t.Errorf("%s caps advertise polls: %+v", name, caps)
		}
	}
}
