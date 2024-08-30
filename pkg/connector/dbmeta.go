package connector

import (
	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaConnector) GetDBMetaTypes() database.MetaTypes {
	return database.MetaTypes{
		Portal: func() any {
			return &metaid.PortalMetadata{}
		},
		Ghost: func() any {
			return &metaid.GhostMetadata{}
		},
		Message: func() any {
			return &metaid.MessageMetadata{}
		},
		Reaction: nil,
		UserLogin: func() any {
			return &metaid.UserLoginMetadata{}
		},
	}
}
