package igconnector

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

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
		Handler:           &tapHandler{signer: signer, resolve: ic.resolveTap},
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
	send   func(context.Context) error
}

type tapHandler struct {
	signer  *igconv.TapSigner
	resolve func(context.Context, igconv.TapTarget) (*tapAction, error)
}

var errTapNotFound = errors.New("tap message or button not found")

func (h *tapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, ok := strings.CutPrefix(r.URL.Path, "/_ig/tap/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	target, err := h.signer.Verify(token)
	if err != nil {
		http.Error(w, "Invalid tap link", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	action, err := h.resolve(ctx, target)
	if errors.Is(err, errTapNotFound) {
		http.Error(w, "Message or button not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Could not load button", http.StatusBadGateway)
		return
	}
	if err = action.send(ctx); err != nil {
		http.Error(w, "Could not send button", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><html><head><title>Button sent</title></head><body><p>Sent: %s</p></body></html>", html.EscapeString(action.button.Title))
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
	if len(meta.TapButtons()) == 0 {
		items, err := cli.Client.FetchThreadButtonItems(ctx, igconv.PostbackThreadID(portalMeta))
		if err != nil {
			return nil, err
		}
		buttons := igconv.PickButtons(items, igconv.ItemIDFromMessageID(msg.ID), nil, 0)
		if buttons.Empty() {
			return nil, errTapNotFound
		}
		meta.PostbackButtons, meta.QuickReplies = buttons.Postbacks, buttons.QuickReplies
		if err = ic.Bridge.DB.Message.Update(ctx, msg); err != nil {
			return nil, err
		}
	}
	button := igconv.ButtonAtIndex(meta.TapButtons(), target.Index)
	if button == nil || igconv.ButtonKind(button) != target.Kind {
		return nil, errTapNotFound
	}
	return &tapAction{button: button, send: func(ctx context.Context) error {
		_, err := cli.sendButtonTap(ctx, portalMeta.IGID, button, "")
		return err
	}}, nil
}

func (ic *IGConnector) tapOptions(msg *database.Message) igconv.CTAOptions {
	return igconv.CTAOptions{Signer: ic.MsgConv.TapSigner, BaseURL: ic.MsgConv.TapPublicAddress, Portal: msg.Room, MessageID: msg.ID}
}
