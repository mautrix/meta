package metaid

import (
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
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
