// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package igconv

import (
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metadb"
	"go.mau.fi/mautrix-meta/pkg/msgconv/textfmt"
)

type MessageConverter struct {
	Bridge          *bridgev2.Bridge
	MaxFileSize     int64
	AsyncFiles      bool
	DisableViewOnce bool
	BridgeMode      types.Platform
	HTMLParser      *textfmt.MatrixHTMLParser
	DB              *metadb.MetaDB
	DirectMedia     bool
}

func New(br *bridgev2.Bridge, db *metadb.MetaDB) *MessageConverter {
	mc := &MessageConverter{
		Bridge:      br,
		MaxFileSize: 50 * 1024 * 1024,
		DB:          db,
		HTMLParser:  textfmt.NewMatrixParser(br),
	}
	return mc
}

type contextKey int

const (
	contextKeyIGClient contextKey = iota
	contextKeyIntent
	contextKeyPortal
	contextKeyFetchXMA
	contextKeyMsgID
	contextKeyPartID
)
