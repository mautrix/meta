package igconnector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

type refreshMatrixStub struct {
	bridgev2.MatrixConnector
	bridgev2.MatrixAPI
	edits   []*event.MessageEventContent
	replies []string
	polls   []*event.Content
}

func (m *refreshMatrixStub) Init(*bridgev2.Bridge)                           {}
func (m *refreshMatrixStub) BotIntent() bridgev2.MatrixAPI                   { return m }
func (m *refreshMatrixStub) GhostIntent(networkid.UserID) bridgev2.MatrixAPI { return m }
func (m *refreshMatrixStub) GetMXID() id.UserID                              { return "@bridge:example.com" }
func (m *refreshMatrixStub) GetEvent(context.Context, id.RoomID, id.EventID) (*event.Event, error) {
	return nil, errors.New("can't decrypt the event")
}
func (m *refreshMatrixStub) SendMessage(_ context.Context, _ id.RoomID, typ event.Type, content *event.Content, _ *bridgev2.MatrixSendExtra) (*mautrix.RespSendEvent, error) {
	if typ == event.EventUnstablePollStart {
		m.polls = append(m.polls, content)
		return &mautrix.RespSendEvent{EventID: id.EventID(fmt.Sprintf("$poll%d", len(m.polls)))}, nil
	}
	if typ != event.EventMessage {
		return nil, fmt.Errorf("refresh sent a %s event", typ.Type)
	}
	if content.AsMessage().NewContent != nil {
		m.edits = append(m.edits, content.AsMessage())
	} else {
		m.replies = append(m.replies, content.AsMessage().Body)
	}
	return &mautrix.RespSendEvent{EventID: "$edit"}, nil
}

type refreshTransport func(*http.Request) (*http.Response, error)

func (f refreshTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRefreshAllStoredMessagesWithoutDecrypting(t *testing.T) {
	ctx := context.Background()
	db, err := dbutil.NewWithDialect(filepath.Join(t.TempDir(), "bridge.db"), "sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	matrix := &refreshMatrixStub{}
	connector := &IGConnector{}
	bridge := bridgev2.NewBridge("test", db, zerolog.Nop(), nil, matrix, connector, commands.NewProcessor)
	bridge.BackgroundCtx = ctx
	if err := bridge.DB.Upgrade(ctx); err != nil {
		t.Fatal(err)
	}
	t.Run("persisted signing key is excluded from database trace logs", func(t *testing.T) {
		var logs bytes.Buffer
		logger := zerolog.New(&logs).Level(zerolog.TraceLevel)
		oldLog := db.Log
		db.Log = dbutil.ZeroLogger(logger, dbutil.ZeroLogSettings{TraceLogAllQueries: true})
		defer func() { db.Log = oldLog }()
		if _, err := loadTapSigner(logger.WithContext(ctx), "", bridge.DB.KV); err != nil {
			t.Fatal("key persistence failed")
		}
		if logs.Len() != 0 {
			t.Fatal("signing-key database calls were logged")
		}
	})
	connector.MsgConv.TapSigner = igconv.NewTapSigner([]byte("test-only-refresh-key"))
	portal := &bridgev2.Portal{Bridge: bridge, Portal: &database.Portal{PortalKey: networkid.PortalKey{ID: "thread"}, MXID: "!room:example.com", Metadata: &metaid.PortalMetadata{IGID: "send-thread", IGThreadID: "history-thread"}}}
	if err := bridge.DB.Portal.Insert(ctx, portal.Portal); err != nil {
		t.Fatal(err)
	}
	if err := bridge.DB.Ghost.Insert(ctx, &database.Ghost{ID: "sender"}); err != nil {
		t.Fatal(err)
	}
	var items []*slidetypes.ThreadItem
	for i := 0; i < 125; i++ {
		itemID := fmt.Sprint(i + 1)
		msg := &database.Message{ID: metaid.MakeFBMessageID(itemID), MXID: id.EventID("$" + itemID), Room: portal.PortalKey, SenderID: "sender", Timestamp: time.Unix(int64(i+1), 0), Metadata: &metaid.MessageMetadata{}}
		if err := bridge.DB.Message.Insert(ctx, msg); err != nil {
			t.Fatal(err)
		}
		items = append(items, &slidetypes.ThreadItem{ItemID: itemID, Text: "Card", QuickReplies: []*slidetypes.QuickReply{{Title: "Reply"}}})
	}
	body, err := json.Marshal(&slidetypes.ThreadDetailsResponse{Status: "ok", Thread: &slidetypes.ThreadDetails{Items: items}})
	if err != nil {
		t.Fatal(err)
	}
	gets := 0
	client := instameow.NewClient(instameow.ClientParams{Log: zerolog.Nop(), Cookies: &cookies.Cookies{Platform: types.Instagram}})
	client.GetHTTP().HTTP.Transport = refreshTransport(func(r *http.Request) (*http.Response, error) {
		gets++
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/api/v1/direct_v2/threads/") {
			t.Error("refresh attempted an Instagram write")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})
	login := &bridgev2.UserLogin{UserLogin: &database.UserLogin{ID: "login"}}
	login.Client = &IGClient{Main: connector, Client: client, UserLogin: login}
	logger := zerolog.Nop()
	ce := &commands.Event{Ctx: ctx, Log: &logger, Bridge: bridge, Portal: portal}
	targets, err := bridge.DB.Message.IterPortalMessages(ctx, portal.PortalKey).AsList()
	if err != nil {
		t.Fatal(err)
	}
	counts := refreshPortal(ce, login, targets)
	if counts != [3]int{0, 125, 0} || len(matrix.edits) != 125 || gets != 1 {
		t.Fatalf("counts %v, edits %d, GETs %d", counts, len(matrix.edits), gets)
	}
	for _, edit := range matrix.edits {
		if edit.NewContent == nil || !strings.Contains(edit.NewContent.FormattedBody, `<a href="http://127.0.0.1:29330/_ig/tap/`) {
			t.Fatal("refresh edit has no tap link")
		}
	}
	stored, err := bridge.DB.Message.GetFirstPartByID(ctx, "", targets[0].ID)
	if err != nil || len(stored.Metadata.(*metaid.MessageMetadata).QuickReplies) != 1 {
		t.Fatal("refreshed button not persisted")
	}
	if len(matrix.polls) != 125 {
		t.Fatalf("expected one button poll per card, got %d", len(matrix.polls))
	}
	for _, poll := range matrix.polls {
		start, ok := poll.Raw["org.matrix.msc3381.poll.start"].(event.PollStart)
		if !ok || start.Question.Text != "Card" || start.MaxSelections != 1 || len(start.Answers) != 1 || start.Answers[0].Text != "Reply" {
			t.Fatalf("unexpected poll: %+v", poll.Raw)
		}
	}
	for _, target := range targets {
		parts, err := bridge.DB.Message.GetAllPartsByID(ctx, "", target.ID)
		if err != nil || len(parts) != 2 || parts[1].PartID != metaid.ButtonPollPartID || !isButtonPoll(parts[1]) || parts[1].MXID == "" {
			t.Fatalf("card %s has parts %+v, %v", target.ID, parts, err)
		}
	}
	t.Run("refreshing again never adds a second poll", func(t *testing.T) {
		before := len(matrix.polls)
		counts := refreshPortal(ce, login, targets)
		if counts[refreshFailed] != 0 || len(matrix.polls) != before {
			t.Fatalf("counts %v, polls %d -> %d", counts, before, len(matrix.polls))
		}
	})
	t.Run("stored buttons survive missing history", func(t *testing.T) {
		if result := refreshCard(ce, login, nil, stored); result != refreshUpdated {
			t.Fatal("stored card not rebuilt")
		}
	})
	t.Run("missing thread IDs never trigger route lookup", func(t *testing.T) {
		meta := portal.Metadata
		portal.Metadata = &metaid.PortalMetadata{}
		defer func() { portal.Metadata = meta }()
		beforeGets := gets
		if counts := refreshPortal(ce, login, []*database.Message{stored}); counts != [3]int{0, 1, 0} || gets != beforeGets {
			t.Fatal("refresh without thread ID must use stored buttons without Instagram requests")
		}
	})

	t.Run("button polls are never refreshed as cards", func(t *testing.T) {
		parts, err := bridge.DB.Message.GetAllPartsByID(ctx, "", stored.ID)
		if err != nil || len(parts) != 2 || !isButtonPoll(parts[1]) {
			t.Fatalf("the card has no stored poll: %+v, %v", parts, err)
		}
		poll := parts[1]
		beforeEdits, beforeGets := len(matrix.edits), gets
		if counts := refreshPortal(ce, login, []*database.Message{poll}); counts != [3]int{} || len(matrix.edits) != beforeEdits || gets != beforeGets {
			t.Fatalf("poll row was refreshed: counts %v, edits %d, GETs %d", counts, len(matrix.edits)-beforeEdits, gets-beforeGets)
		}
		if got := cardOfButtonPoll(ctx, bridge.DB.Message, portal.Receiver, poll); got.PartID != "" || got.ID != stored.ID {
			t.Fatalf("reply to the poll must refresh the card, got part %q", got.PartID)
		}
		if got := cardOfButtonPoll(ctx, bridge.DB.Message, portal.Receiver, stored); got != stored {
			t.Fatal("a card must refresh itself")
		}
	})
	t.Run("missing stored and fetched buttons are counted", func(t *testing.T) {
		missing := *targets[0]
		missing.Metadata = &metaid.MessageMetadata{}
		if result := refreshCard(ce, login, nil, &missing); result != refreshSkipped {
			t.Fatal("missing card not skipped")
		}
	})
	t.Run("management refresh-all stays within login portals", func(t *testing.T) {
		userRow := &database.User{MXID: "@owner:example.com", ManagementRoom: "!management:example.com"}
		if err := bridge.DB.User.Insert(ctx, userRow); err != nil {
			t.Fatal(err)
		}
		loginRow := &database.UserLogin{ID: "login", UserMXID: userRow.MXID, Metadata: &metaid.UserLoginMetadata{}}
		if err := bridge.DB.UserLogin.Insert(ctx, loginRow); err != nil {
			t.Fatal(err)
		}
		user, err := bridge.GetUserByMXID(ctx, userRow.MXID)
		if err != nil {
			t.Fatal(err)
		}
		selected := user.GetDefaultLogin()
		if selected == nil {
			t.Fatal("fixture login not loaded")
		}
		defer selected.BridgeState.Destroy()
		selected.Client.(*IGClient).Client = client
		selected.Client.(*IGClient).LoginMeta.Cookies = &cookies.Cookies{Platform: types.Instagram}
		second := &database.Portal{PortalKey: networkid.PortalKey{ID: "second"}, MXID: "!second:example.com", Metadata: &metaid.PortalMetadata{IGID: "second-send", IGThreadID: "second-history"}}
		if err := bridge.DB.Portal.Insert(ctx, second); err != nil {
			t.Fatal(err)
		}
		extra := &database.Message{ID: metaid.MakeFBMessageID("999"), MXID: "$999", Room: second.PortalKey, SenderID: "sender", Timestamp: time.Unix(999, 0), Metadata: &metaid.MessageMetadata{QuickReplies: []*metaid.PostbackButton{{Title: "Reply", CTAType: "quick_reply"}}}}
		if err := bridge.DB.Message.Insert(ctx, extra); err != nil {
			t.Fatal(err)
		}
		for _, key := range []networkid.PortalKey{portal.PortalKey, second.PortalKey} {
			if err := bridge.DB.UserPortal.Put(ctx, &database.UserPortal{UserMXID: userRow.MXID, LoginID: loginRow.ID, Portal: key}); err != nil {
				t.Fatal(err)
			}
		}
		outside := &database.Portal{PortalKey: networkid.PortalKey{ID: "other-login-portal"}, MXID: "!other:example.com", Metadata: &metaid.PortalMetadata{}}
		if err := bridge.DB.Portal.Insert(ctx, outside); err != nil {
			t.Fatal(err)
		}
		matrix.edits = nil
		matrix.replies = nil
		beforeGets := gets
		management := &commands.Event{Ctx: ctx, Log: &logger, Bridge: bridge, Bot: matrix, User: user, RoomID: userRow.ManagementRoom, OrigRoomID: userRow.ManagementRoom}
		fnRefreshAll(management)
		if len(matrix.edits) != 126 || gets-beforeGets != 2 || len(matrix.replies) != 3 {
			t.Fatalf("edits %d, GETs %d, replies %d", len(matrix.edits), gets-beforeGets, len(matrix.replies))
		}
		if !strings.Contains(matrix.replies[2], "Totals across 2 portal(s): updated 126, skipped 0, failed 0") {
			t.Fatal("incorrect totals")
		}
		if cmdRefreshAll.RequiresPortal || !cmdRefreshAll.RequiresLogin {
			t.Fatal("incorrect refresh-all requirements")
		}
		management.RoomID = "!not-management:example.com"
		beforeGets = gets
		fnRefreshAll(management)
		if gets != beforeGets {
			t.Fatal("refresh-all ran outside management room")
		}
		t.Run("tap resolves and recovers stored metadata", func(t *testing.T) {
			stored.Metadata = &metaid.MessageMetadata{}
			if err := bridge.DB.Message.Update(ctx, stored); err != nil {
				t.Fatal(err)
			}
			target := igconv.TapTarget{Portal: portal.PortalKey, MessageID: stored.ID, Index: 1, Kind: igconv.TapQuickReply}
			sends := 0
			handler := &tapHandler{signer: connector.MsgConv.TapSigner, resolve: func(ctx context.Context, target igconv.TapTarget) (*tapAction, error) {
				action, err := connector.resolveTap(ctx, target)
				if err != nil {
					return nil, err
				}
				action.send = func(_ context.Context, button *metaid.PostbackButton) error {
					sends++
					if button.Title != "Reply" || !button.IsQuickReply() {
						t.Fatal("wrong recovered button sent")
					}
					return nil
				}
				return action, nil
			}}
			request := func(target igconv.TapTarget) int {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "http://example.com/_ig/tap/"+connector.MsgConv.TapSigner.Sign(target), nil))
				return w.Code
			}
			beforeGets := gets
			if request(target) != 200 || sends != 1 || gets != beforeGets+1 {
				t.Fatal("missing metadata was not recovered by POST before sending once")
			}
			beforeGets = gets
			if request(target) != 200 || sends != 1 || gets != beforeGets {
				t.Fatal("second tap must be idempotent and not fetch again")
			}
			beforeGets = gets
			if _, err := connector.resolveTap(ctx, target); err != nil || gets != beforeGets {
				t.Fatal("stored metadata was not reused")
			}
			target.Kind = igconv.TapPostback
			if request(target) != 404 || sends != 1 {
				t.Fatal("kind mismatch sent a button")
			}
			target.Kind = igconv.TapQuickReply
			target.Portal = second.PortalKey
			if request(target) != 404 || sends != 1 {
				t.Fatal("cross-portal message sent a button")
			}
		})
	})

}
