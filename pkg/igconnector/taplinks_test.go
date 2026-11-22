package igconnector

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

type memoryTapKeyStore struct {
	value             string
	writes            int
	readErr, writeErr error
}

func (s *memoryTapKeyStore) GetWithError(context.Context, database.Key) (string, error) {
	if s.readErr != nil {
		return "", s.readErr
	}
	if s.value == "" {
		return "", sql.ErrNoRows
	}
	return s.value, nil
}
func (s *memoryTapKeyStore) SetWithError(_ context.Context, _ database.Key, value string) error {
	s.writes++
	if s.writeErr != nil {
		return s.writeErr
	}
	s.value = value
	return nil
}

func TestTapKeyPersistence(t *testing.T) {
	store := &memoryTapKeyStore{}
	first, err := loadTapSigner(context.Background(), "", store)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadTapSigner(context.Background(), "", store)
	if err != nil {
		t.Fatal(err)
	}
	target := igconv.TapTarget{Portal: networkid.PortalKey{ID: "room", Receiver: "login"}, MessageID: "message", Index: 1, Kind: igconv.TapPostback}
	if store.writes != 1 || first.Sign(target) != second.Sign(target) {
		t.Fatal("restart changed signing key")
	}
	for _, broken := range []*memoryTapKeyStore{{readErr: errors.New("read unavailable")}, {writeErr: errors.New("write unavailable")}, {value: "malformed"}} {
		if _, err := loadTapSigner(context.Background(), "", broken); err == nil {
			t.Fatal("invalid persistence accepted")
		}
	}
	configured := &memoryTapKeyStore{readErr: errors.New("must not read")}
	if _, err := loadTapSigner(context.Background(), "test-only-configured-key", configured); err != nil || configured.writes != 0 {
		t.Fatal("configured key used storage")
	}
}

func TestTapAddressesAndUnknownConfig(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("unknown_live_field: true\ntap_listen: 127.0.0.1:31000\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	listen, public, err := cfg.tapAddresses()
	if err != nil || listen != "127.0.0.1:31000" || public != "http://127.0.0.1:31000" {
		t.Fatal("custom listener fallback failed")
	}
	listen, public, err = (&Config{}).tapAddresses()
	if err != nil || listen != "127.0.0.1:29330" || public != igconv.DefaultTapAddress {
		t.Fatal("default addresses failed")
	}
	for _, bad := range []string{"javascript:alert(1)", "https://example.com/?query=1", "https://user@example.com/", "/relative"} {
		if _, _, err := (&Config{TapPublicAddress: bad}).tapAddresses(); err == nil {
			t.Fatal("invalid public URL accepted")
		}
	}
}

func TestTapHTTPHandler(t *testing.T) {
	signer := igconv.NewTapSigner([]byte("test-only-http-key"))
	for _, kind := range []igconv.TapKind{igconv.TapPostback, igconv.TapQuickReply} {
		t.Run(string(kind), func(t *testing.T) {
			target := igconv.TapTarget{Portal: networkid.PortalKey{ID: "room", Receiver: "login"}, MessageID: "message", Index: 2, Kind: kind}
			button := &metaid.PostbackButton{Title: "Choose <this>", CTAType: string(kind)}
			calls, resolves := 0, 0
			var resolveErr, sendErr error
			handler := &tapHandler{signer: signer, resolve: func(_ context.Context, got igconv.TapTarget) (*tapAction, error) {
				resolves++
				if got != target {
					t.Fatal("wrong button identity resolved")
				}
				if resolveErr != nil {
					return nil, resolveErr
				}
				return &tapAction{button: button, send: func(context.Context) error {
					calls++
					if button.Title != "Choose <this>" || igconv.ButtonKind(button) != kind {
						t.Fatal("wrong title or kind sent")
					}
					return sendErr
				}}, nil
			}}
			request := func(method, token string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(method, "http://example.com/_ig/tap/"+token, nil))
				return w
			}
			token := signer.Sign(target)
			for _, invalid := range []string{"bad", token[:len(token)-3]} {
				if w := request(http.MethodGet, invalid); w.Code != 400 || calls != 0 || resolves != 0 {
					t.Fatal("invalid link reached resolver or send")
				}
			}
			if w := request(http.MethodHead, token); w.Code != 405 || calls != 0 {
				t.Fatal("HEAD must not send")
			}
			w := request(http.MethodGet, token)
			if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), "Sent: Choose &lt;this&gt;") || strings.Contains(w.Body.String(), token) {
				t.Fatal("valid tap response failed")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Fatal("missing privacy headers")
			}
			resolveErr = errTapNotFound
			if w := request(http.MethodGet, token); w.Code != 404 || calls != 1 {
				t.Fatal("missing button sent")
			}
			resolveErr = errors.New("upstream unavailable")
			if w := request(http.MethodGet, token); w.Code != 502 || calls != 1 {
				t.Fatal("failed recovery sent")
			}
			resolveErr = nil
			sendErr = errors.New("send unavailable")
			if w := request(http.MethodGet, token); w.Code != 502 || calls != 2 {
				t.Fatal("send failure retried or returned wrong status")
			}
		})
	}
}
