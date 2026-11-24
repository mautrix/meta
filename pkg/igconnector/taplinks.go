package igconnector

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/igconv"
)

const tapSecretKey database.Key = "ig_tap_hmac_secret"

type tapKeyStore interface {
	GetWithError(context.Context, database.Key) (string, error)
	SetWithError(context.Context, database.Key, string) error
}

func loadTapSigner(ctx context.Context, configured string, store tapKeyStore) (*igconv.TapSigner, error) {
	if configured != "" {
		return igconv.NewTapSigner([]byte(configured)), nil
	}
	// Database trace logs include bound parameters. A disabled logger would fall back to the database logger.
	ctx = zerolog.New(io.Discard).Level(zerolog.InfoLevel).WithContext(ctx)
	stored, err := store.GetWithError(ctx, tapSecretKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("failed to read tap signing key")
	}
	if stored == "" {
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, errors.New("failed to generate tap signing key")
		}
		stored = base64.RawURLEncoding.EncodeToString(key)
		if err = store.SetWithError(ctx, tapSecretKey, stored); err != nil {
			return nil, errors.New("failed to persist tap signing key")
		}
	}
	key, err := base64.RawURLEncoding.Strict().DecodeString(stored)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid stored tap signing key")
	}
	return igconv.NewTapSigner(key), nil
}

func (c *Config) tapAddresses() (listen, public string, err error) {
	listen = c.TapListen
	if listen == "" {
		listen = "127.0.0.1:29330"
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" || port == "0" {
		return "", "", errors.New("tap_listen must be a host and nonzero port")
	}
	public = c.TapPublicAddress
	if public == "" {
		public = "http://" + net.JoinHostPort("127.0.0.1", port)
	}
	u, err := url.Parse(public)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", errors.New("tap_public_address must be an http(s) base URL without credentials, query or fragment")
	}
	return listen, strings.TrimRight(public, "/"), nil
}

func (ic *IGConnector) startTapListener(ctx context.Context) error {
	listen, public, err := ic.Config.tapAddresses()
	if err != nil {
		return err
	}
	signer, err := loadTapSigner(ctx, ic.Config.TapHMACSecret, ic.Bridge.DB.KV)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("failed to start tap listener: %w", err)
	}
	ic.MsgConv.TapSigner = signer
	ic.MsgConv.TapPublicAddress = public
	ic.tapServer = &http.Server{
		Handler:           &tapHandler{signer: signer, resolve: ic.resolveTap, log: ic.Bridge.Log, once: &ic.tapOnce},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	go func() {
		if err := ic.tapServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			ic.Bridge.Log.Error().Msg("Tap listener stopped unexpectedly")
		}
	}()
	return nil
}

func (ic *IGConnector) Stop() {
	if ic.tapServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ic.tapServer.Shutdown(ctx); err != nil {
		_ = ic.tapServer.Close()
	}
}

type tapAction struct {
	button *metaid.PostbackButton
	send   func(context.Context, *metaid.PostbackButton) error
}

type tapHandler struct {
	signer  *igconv.TapSigner
	resolve func(context.Context, igconv.TapTarget) (*tapAction, error)
	log     zerolog.Logger
	once    *tapOnce
}

var errTapNotFound = errors.New("tap message or button not found")

// tapOnce makes a successful send for a key happen at most once in this
// process. A failed send can be retried. Concurrent requests for the same
// key share a single in-flight send, so a double click cannot fire twice.
// The zero value is ready to use.
// The record is in-memory: a bridge restart allows one more send. That is
// deliberate (no durable one-shot store); preview user-agents and prefetch
// headers are blocked first so a link preview should never consume the slot.
type tapOnce struct {
	mu       sync.Mutex
	done     map[string]struct{}
	inflight map[string]*tapFlight
	onWait   func() // test hook: called when a request waits for an in-flight send
}

type tapFlight struct {
	done chan struct{}
	err  error
}

func newTapOnce() *tapOnce {
	return &tapOnce{done: make(map[string]struct{}), inflight: make(map[string]*tapFlight)}
}

// tapKey identifies one button of one message. Signed links and poll votes share it, so
// whichever activates the button first is the only one that sends. The kind is part of the
// key because a link whose kind doesn't match the stored button must still be rejected, not
// reported as sent. Index 0 is not a button (they are 1-based) and keys the card's poll as a whole.
func tapKey(portal networkid.PortalKey, messageID networkid.MessageID, index int, kind igconv.TapKind) string {
	return strings.Join([]string{string(portal.ID), string(portal.Receiver), string(messageID), strconv.Itoa(index), string(kind)}, "\x00")
}

// do runs fn at most once per token on success. already is true when this
// call did not invoke fn because a previous send already succeeded (or it
// waited for an in-flight success).
func (o *tapOnce) do(token string, fn func() error) (already bool, err error) {
	o.mu.Lock()
	if o.done == nil {
		o.done, o.inflight = make(map[string]struct{}), make(map[string]*tapFlight)
	}
	if _, ok := o.done[token]; ok {
		o.mu.Unlock()
		return true, nil
	}
	if f, ok := o.inflight[token]; ok {
		onWait := o.onWait
		o.mu.Unlock()
		if onWait != nil {
			onWait()
		}
		<-f.done
		if f.err != nil {
			return false, f.err
		}
		return true, nil
	}
	f := &tapFlight{done: make(chan struct{})}
	o.inflight[token] = f
	o.mu.Unlock()

	err = fn()

	o.mu.Lock()
	delete(o.inflight, token)
	f.err = err
	if err == nil {
		o.done[token] = struct{}{}
	}
	close(f.done)
	o.mu.Unlock()
	return false, err
}

var previewUserAgents = []string{
	"slackbot", "facebookexternalhit", "whatsapp", "telegrambot",
	"twitterbot", "discordbot", "googlebot", "curl",
}

func isTapPreview(r *http.Request) bool {
	if r.Method == http.MethodHead {
		return true
	}
	purpose := strings.ToLower(strings.TrimSpace(r.Header.Get("Purpose")))
	if purpose == "prefetch" || strings.HasPrefix(purpose, "prefetch;") {
		return true
	}
	if strings.TrimSpace(r.Header.Get("Sec-Purpose")) != "" {
		return true
	}
	ua := strings.ToLower(r.UserAgent())
	for _, bot := range previewUserAgents {
		if strings.Contains(ua, bot) {
			return true
		}
	}
	return false
}

const tapSentHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Enviado ✓</title><script>window.close()</script></head><body><p>Enviado ✓</p></body></html>`

func writeTapSent(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, tapSentHTML)
}

func (h *tapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, ok := strings.CutPrefix(r.URL.Path, "/_ig/tap/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if isTapPreview(r) {
		h.log.Info().
			Str("method", r.Method).
			Str("user_agent", r.UserAgent()).
			Str("purpose", r.Header.Get("Purpose")).
			Str("sec_purpose", r.Header.Get("Sec-Purpose")).
			Msg("Ignored Instagram button tap preview")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target, err := h.signer.Verify(token)
	if err != nil {
		http.Error(w, "Invalid tap link", http.StatusBadRequest)
		return
	}
	if h.once == nil {
		h.once = newTapOnce()
	}
	already, err := h.once.do(tapKey(target.Portal, target.MessageID, target.Index, target.Kind), func() error {
		ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
		defer cancel()
		action, err := h.resolve(ctx, target)
		if err != nil {
			return err
		}
		if err = action.send(ctx, action.button); err != nil {
			return err
		}
		h.log.Info().
			Str("portal_id", string(target.Portal.ID)).
			Str("message_id", string(target.MessageID)).
			Int("index", target.Index).
			Str("kind", string(target.Kind)).
			Str("title", action.button.Title).
			Msg("Instagram button tap sent")
		return nil
	})
	if already && err == nil {
		h.log.Info().
			Str("portal_id", string(target.Portal.ID)).
			Int("index", target.Index).
			Str("kind", string(target.Kind)).
			Msg("Instagram button tap already sent")
		writeTapSent(w)
		return
	}
	if errors.Is(err, errTapNotFound) {
		http.Error(w, "Message or button not found", http.StatusNotFound)
		return
	}
	if err != nil {
		// resolve failures and send failures both surface as 502 except not-found.
		// Distinguish resolve vs send? Keep 502 for both after a failed do().
		// But previously resolve not-found is 404 (above), resolve other is 502, send is 502.
		http.Error(w, "Could not send button", http.StatusBadGateway)
		return
	}
	writeTapSent(w)
}

func (ic *IGConnector) resolveTap(ctx context.Context, target igconv.TapTarget) (*tapAction, error) {
	portal, err := ic.Bridge.GetExistingPortalByKey(ctx, target.Portal)
	if err != nil {
		return nil, err
	}
	if portal == nil {
		return nil, errTapNotFound
	}
	msg, err := ic.Bridge.DB.Message.GetFirstPartByID(ctx, target.Portal.Receiver, target.MessageID)
	if err != nil {
		return nil, err
	}
	if msg == nil || msg.Room != target.Portal {
		return nil, errTapNotFound
	}
	logins, err := ic.Bridge.GetUserLoginsInPortal(ctx, target.Portal)
	if err != nil {
		return nil, err
	}
	var login *bridgev2.UserLogin
	for _, candidate := range logins {
		if target.Portal.Receiver != "" && candidate.ID != target.Portal.Receiver {
			continue
		}
		if login != nil {
			return nil, errors.New("ambiguous tap login")
		}
		login = candidate
	}
	if login == nil {
		return nil, errTapNotFound
	}
	cli, ok := login.Client.(*IGClient)
	if !ok || cli.Client == nil {
		return nil, errors.New("Instagram client unavailable")
	}
	portalMeta, err := cli.ensureIGID(ctx, portal)
	if err != nil {
		return nil, err
	}
	meta, ok := msg.Metadata.(*metaid.MessageMetadata)
	if !ok || meta == nil {
		return nil, errTapNotFound
	}
	button := igconv.ButtonAtIndex(meta.TapButtons(), target.Index)
	if button == nil || igconv.ButtonKind(button) != target.Kind {
		items, err := cli.Client.FetchAllThreadButtonItems(ctx, igconv.PostbackThreadID(portalMeta))
		if err != nil {
			return nil, err
		}
		buttons := igconv.PickButtons(items, igconv.ItemIDFromMessageID(msg.ID), nil, 0)
		button = igconv.ButtonAtIndex(buttons.All(), target.Index)
		if button == nil || igconv.ButtonKind(button) != target.Kind {
			return nil, errTapNotFound
		}
		meta.PostbackButtons, meta.QuickReplies = buttons.Postbacks, buttons.QuickReplies
		if err = ic.Bridge.DB.Message.Update(ctx, msg); err != nil {
			return nil, err
		}
	}
	return &tapAction{button: button, send: func(ctx context.Context, button *metaid.PostbackButton) error {
		_, err := cli.sendButtonTap(ctx, portalMeta.IGID, button, "")
		return err
	}}, nil
}

func (ic *IGConnector) tapOptions(msg *database.Message) igconv.CTAOptions {
	return igconv.CTAOptions{Signer: ic.MsgConv.TapSigner, BaseURL: ic.MsgConv.TapPublicAddress, Portal: msg.Room, MessageID: msg.ID}
}
