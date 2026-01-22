package metaid

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"sync/atomic"

	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/random"
	waTypes "go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type MessageMetadata struct {
	EditTimestamp   int64           `json:"edit_timestamp,omitempty"`
	DirectMediaMeta json.RawMessage `json:"direct_media_meta,omitempty"`
}

type GhostMetadata struct {
	Username string `json:"username,omitempty"`
}

type UserLoginMetadata struct {
	Platform   types.Platform   `json:"platform"`
	Cookies    *cookies.Cookies `json:"cookies"`
	WADeviceID uint16           `json:"wa_device_id,omitempty"`
	PushKeys   *PushKeys        `json:"push_keys,omitempty"`
	LoginUA    string           `json:"login_ua,omitempty"`

	// Thread backfill state
	BackfillCompleted bool `json:"backfill_completed,omitempty"`
}

type PushKeys struct {
	P256DH  []byte `json:"p256dh"`
	Auth    []byte `json:"auth"`
	Private []byte `json:"private"`
}

func (m *UserLoginMetadata) GeneratePushKeys() {
	privateKey := exerrors.Must(ecdh.P256().GenerateKey(rand.Reader))
	m.PushKeys = &PushKeys{
		P256DH:  privateKey.Public().(*ecdh.PublicKey).Bytes(),
		Auth:    random.Bytes(16),
		Private: privateKey.Bytes(),
	}
}

type PortalMetadata struct {
	ThreadType     table.ThreadType `json:"thread_type"`
	WhatsAppServer string           `json:"whatsapp_server,omitempty"`

	EphemeralSettingTimestamp int64 `json:"ephemeral_setting_timestamp,omitempty"`

	FetchAttempted atomic.Bool `json:"-"`
}

func (meta *PortalMetadata) JID(id networkid.PortalID) waTypes.JID {
	jid := ParseWAPortalID(id, meta.WhatsAppServer)
	if jid.Server == "" {
		switch meta.ThreadType {
		case table.ENCRYPTED_OVER_WA_GROUP:
			jid.Server = waTypes.GroupServer
		//case table.ENCRYPTED_OVER_WA_ONE_TO_ONE:
		//	jid.Server = waTypes.DefaultUserServer
		default:
			jid.Server = waTypes.MessengerServer
		}
	}
	return jid
}
