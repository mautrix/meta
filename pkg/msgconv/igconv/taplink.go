package igconv

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

type TapKind string

const (
	TapPostback       TapKind = "postback"
	TapQuickReply     TapKind = "quick_reply"
	DefaultTapAddress         = "http://127.0.0.1:29330"
)

// TapTarget identifies a stored button, never its Instagram payload.
type TapTarget struct {
	Portal    networkid.PortalKey `json:"portal"`
	MessageID networkid.MessageID `json:"message_id"`
	Index     int                 `json:"index"`
	Kind      TapKind             `json:"kind"`
}

func (t TapTarget) valid() bool {
	return t.Portal.ID != "" && t.MessageID != "" && t.Index > 0 && (t.Kind == TapPostback || t.Kind == TapQuickReply)
}

func ButtonKind(button *metaid.PostbackButton) TapKind {
	if button.IsQuickReply() {
		return TapQuickReply
	}
	return TapPostback
}

type TapSigner struct{ key []byte }

func NewTapSigner(key []byte) *TapSigner {
	if len(key) == 0 {
		panic("empty tap signing key")
	}
	return &TapSigner{key: append([]byte(nil), key...)}
}

func (s *TapSigner) Sign(target TapTarget) string {
	if s == nil || !target.valid() {
		return ""
	}
	payload, _ := json.Marshal(target)
	unsigned := "v1." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var ErrInvalidTapToken = errors.New("invalid tap token")

func (s *TapSigner) Verify(token string) (TapTarget, error) {
	var target TapTarget
	if s == nil || len(token) > 8192 {
		return target, ErrInvalidTapToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return target, ErrInvalidTapToken
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil {
		return target, ErrInvalidTapToken
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return target, ErrInvalidTapToken
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &target) != nil || !target.valid() {
		return TapTarget{}, ErrInvalidTapToken
	}
	return target, nil
}

type CTAOptions struct {
	BaseURL   string
	Signer    *TapSigner
	Portal    networkid.PortalKey
	MessageID networkid.MessageID
}

func (opts CTAOptions) ButtonURL(index int, button *metaid.PostbackButton) string {
	token := opts.Signer.Sign(TapTarget{Portal: opts.Portal, MessageID: opts.MessageID, Index: index, Kind: ButtonKind(button)})
	if token == "" {
		return ""
	}
	base := opts.BaseURL
	if base == "" {
		base = DefaultTapAddress
	}
	return strings.TrimRight(base, "/") + "/_ig/tap/" + token
}

func (mc *MessageConverter) ctaOptions(ctx context.Context) CTAOptions {
	opts := CTAOptions{BaseURL: mc.TapPublicAddress, Signer: mc.TapSigner}
	opts.MessageID, _ = ctx.Value(mediadl.ContextKeyMsgID).(networkid.MessageID)
	if portal, ok := ctx.Value(mediadl.ContextKeyPortal).(*bridgev2.Portal); ok && portal != nil {
		opts.Portal = portal.PortalKey
	}
	return opts
}
