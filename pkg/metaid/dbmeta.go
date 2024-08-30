package metaid

import (
	"sync/atomic"

	waTypes "go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type MessageMetadata struct {
	EditTimestamp int64 `json:"edit_timestamp,omitempty"`
}

type GhostMetadata struct {
	Username string `json:"username,omitempty"`
}

type UserLoginMetadata struct {
	Platform   types.Platform   `json:"platform"`
	Cookies    *cookies.Cookies `json:"cookies"`
	WADeviceID uint16           `json:"wa_device_id,omitempty"`
}

type PortalMetadata struct {
	ThreadType     table.ThreadType `json:"thread_type"`
	WhatsAppServer string           `json:"whatsapp_server,omitempty"`

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
