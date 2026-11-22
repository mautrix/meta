package igconnector

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
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
			var calls, resolves atomic.Int32
			var resolveErr error
			var logs strings.Builder
			handler := &tapHandler{signer: signer, log: zerolog.New(&logs), once: newTapOnce(), resolve: func(_ context.Context, got igconv.TapTarget) (*tapAction, error) {
				resolves.Add(1)
				if got != target {
					t.Fatal("wrong button identity resolved")
				}
				if resolveErr != nil {
					return nil, resolveErr
				}
				return &tapAction{button: button, send: func(_ context.Context, sent *metaid.PostbackButton) error {
					calls.Add(1)
					if sent.Title != "Choose <this>" || igconv.ButtonKind(sent) != kind {
						t.Fatal("wrong title or kind sent")
					}
					return nil
				}}, nil
			}}
			request := func(method, token string, header http.Header) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(method, "http://example.com/_ig/tap/"+token, nil)
				for k, vs := range header {
					for _, v := range vs {
						req.Header.Add(k, v)
					}
				}
				handler.ServeHTTP(w, req)
				return w
			}
			token := signer.Sign(target)
			for _, invalid := range []string{"bad", token[:len(token)-3]} {
				if w := request(http.MethodGet, invalid, nil); w.Code != 400 || calls.Load() != 0 || resolves.Load() != 0 {
					t.Fatal("invalid link reached resolver or send")
				}
			}
			if w := request(http.MethodHead, token, nil); w.Code != 204 || calls.Load() != 0 || resolves.Load() != 0 {
				t.Fatal("HEAD must not send")
			}
			w := request(http.MethodGet, token, nil)
			if w.Code != 200 || calls.Load() != 1 || !strings.Contains(w.Body.String(), "Enviado ✓") || strings.Contains(w.Body.String(), "Confirm") || strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "Choose") {
				t.Fatalf("GET must send once and return a minimal page: code=%d calls=%d body=%s", w.Code, calls.Load(), w.Body.String())
			}
			if !strings.Contains(logs.String(), "Instagram button tap sent") || strings.Contains(logs.String(), token) {
				t.Fatal("send must be logged without the token")
			}
			if w := request(http.MethodGet, token, nil); w.Code != 200 || calls.Load() != 1 || !strings.Contains(w.Body.String(), "Enviado ✓") {
				t.Fatal("second GET must not send again")
			}
			if !strings.Contains(logs.String(), "already sent") {
				t.Fatal("duplicate tap must be logged")
			}
			if w := request(http.MethodPost, token, nil); w.Code != 200 || calls.Load() != 1 {
				t.Fatal("POST after GET must not send again")
			}
			if w := request(http.MethodPut, token, nil); w.Code != 405 || w.Header().Get("Allow") != "GET, POST" || calls.Load() != 1 {
				t.Fatal("other methods must be rejected")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Fatal("missing privacy headers")
			}
			if w := request(http.MethodPost, "bad", nil); w.Code != 400 || calls.Load() != 1 {
				t.Fatal("invalid POST sent")
			}
		})
	}
}

func TestTapHTTPPreviewBlocked(t *testing.T) {
	signer := igconv.NewTapSigner([]byte("test-only-preview-key"))
	target := igconv.TapTarget{Portal: networkid.PortalKey{ID: "room", Receiver: "login"}, MessageID: "message", Index: 1, Kind: igconv.TapQuickReply}
	token := signer.Sign(target)
	var calls atomic.Int32
	var logs strings.Builder
	handler := &tapHandler{signer: signer, log: zerolog.New(&logs), once: newTapOnce(), resolve: func(context.Context, igconv.TapTarget) (*tapAction, error) {
		t.Fatal("preview must not resolve")
		return nil, nil
	}}
	sendVia := func() {
		handler.resolve = func(context.Context, igconv.TapTarget) (*tapAction, error) {
			return &tapAction{button: &metaid.PostbackButton{Title: "Yes", CTAType: string(igconv.TapQuickReply)}, send: func(context.Context, *metaid.PostbackButton) error {
				calls.Add(1)
				return nil
			}}, nil
		}
	}
	request := func(method string, header http.Header) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "http://example.com/_ig/tap/"+token, nil)
		for k, vs := range header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		handler.ServeHTTP(w, req)
		return w
	}
	cases := []http.Header{
		{"User-Agent": {"Slackbot 1.0"}},
		{"User-Agent": {"facebookexternalhit/1.1"}},
		{"User-Agent": {"WhatsApp/2.0"}},
		{"User-Agent": {"TelegramBot"}},
		{"User-Agent": {"Twitterbot/1.0"}},
		{"User-Agent": {"Mozilla/5.0 Discordbot/2.0"}},
		{"User-Agent": {"Googlebot/2.1"}},
		{"User-Agent": {"curl/8.5.0"}},
		{"Purpose": {"prefetch"}},
		{"Purpose": {"Prefetch; as=document"}},
		{"Sec-Purpose": {"prefetch"}},
	}
	for _, header := range cases {
		if w := request(http.MethodGet, header); w.Code != 204 || calls.Load() != 0 {
			t.Fatalf("preview was not blocked: header=%v code=%d calls=%d", header, w.Code, calls.Load())
		}
	}
	if w := request(http.MethodHead, nil); w.Code != 204 || calls.Load() != 0 {
		t.Fatal("HEAD preview was not blocked")
	}
	if !strings.Contains(logs.String(), "Ignored Instagram button tap preview") {
		t.Fatal("preview block must be logged")
	}
	if strings.Contains(logs.String(), token) {
		t.Fatal("logs leaked the tap token")
	}
	sendVia()
	if w := request(http.MethodGet, http.Header{"User-Agent": {"Mozilla/5.0"}}); w.Code != 200 || calls.Load() != 1 || !strings.Contains(w.Body.String(), "Enviado ✓") {
		t.Fatal("a normal browser GET must still send")
	}
}

func TestTapHTTPIdempotencyAndRetry(t *testing.T) {
	signer := igconv.NewTapSigner([]byte("test-only-once-key"))
	target := igconv.TapTarget{Portal: networkid.PortalKey{ID: "room", Receiver: "login"}, MessageID: "message", Index: 1, Kind: igconv.TapPostback}
	token := signer.Sign(target)
	var calls atomic.Int32
	var sendErr error
	var resolveErr error
	handler := &tapHandler{signer: signer, log: zerolog.Nop(), once: newTapOnce(), resolve: func(context.Context, igconv.TapTarget) (*tapAction, error) {
		if resolveErr != nil {
			return nil, resolveErr
		}
		return &tapAction{button: &metaid.PostbackButton{Title: "Go", CTAType: string(igconv.TapPostback)}, send: func(context.Context, *metaid.PostbackButton) error {
			calls.Add(1)
			return sendErr
		}}, nil
	}}
	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://example.com/_ig/tap/"+token, nil))
		return w
	}
	resolveErr = errTapNotFound
	if w := get(); w.Code != 404 || calls.Load() != 0 {
		t.Fatal("missing button sent")
	}
	resolveErr = errors.New("upstream unavailable")
	if w := get(); w.Code != 502 || calls.Load() != 0 {
		t.Fatal("failed recovery sent")
	}
	resolveErr = nil
	sendErr = errors.New("send unavailable")
	if w := get(); w.Code != 502 || calls.Load() != 1 {
		t.Fatal("send failure returned wrong status")
	}
	sendErr = nil
	if w := get(); w.Code != 200 || calls.Load() != 2 || !strings.Contains(w.Body.String(), "Enviado ✓") {
		t.Fatal("failed send must be retryable")
	}
	if w := get(); w.Code != 200 || calls.Load() != 2 {
		t.Fatal("successful send must not be retried")
	}

	started := make(chan struct{})
	waiting := make(chan struct{})
	release := make(chan struct{})
	var concurrent atomic.Int32
	handler.once = newTapOnce()
	handler.once.onWait = func() { close(waiting) }
	handler.resolve = func(context.Context, igconv.TapTarget) (*tapAction, error) {
		return &tapAction{button: &metaid.PostbackButton{Title: "Go", CTAType: string(igconv.TapPostback)}, send: func(context.Context, *metaid.PostbackButton) error {
			concurrent.Add(1)
			close(started)
			<-release
			return nil
		}}, nil
	}
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://example.com/_ig/tap/"+token, nil))
			codes[i] = w.Code
		}(i)
	}
	<-started
	select {
	case <-waiting:
	case <-time.After(2 * time.Second):
		t.Fatal("second request did not wait on the in-flight send")
	}
	close(release)
	wg.Wait()
	if concurrent.Load() != 1 || codes[0] != 200 || codes[1] != 200 {
		t.Fatalf("concurrent taps sent %d times, codes=%v", concurrent.Load(), codes)
	}
}
