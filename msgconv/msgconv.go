// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
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

package msgconv

import (
	"context"

	"go.mau.fi/whatsmeow"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/socket"
)

type PortalMethods interface {
	UploadMatrixMedia(ctx context.Context, data []byte, fileName, contentType string) (id.ContentURIString, error)
	DownloadMatrixMedia(ctx context.Context, uri id.ContentURIString) ([]byte, error)
	GetMatrixReply(ctx context.Context, messageID string, replyToUser int64) (replyTo id.EventID, replyTargetSender id.UserID)
	GetMetaReply(ctx context.Context, content *event.MessageEventContent) *socket.ReplyMetaData
	GetUserMXID(ctx context.Context, userID int64) id.UserID
	ShouldFetchXMA(ctx context.Context) bool
	GetThreadURL(ctx context.Context) (string, string)

	GetClient(ctx context.Context) *messagix.Client
	GetE2EEClient(ctx context.Context) *whatsmeow.Client
	GetData(ctx context.Context) *database.Portal
}

type MessageConverter struct {
	PortalMethods

	ConvertVoiceMessages bool
	ConvertGIFToAPNG     bool
	MaxFileSize          int64
	AsyncFiles           bool
}

func (mc *MessageConverter) IsPrivateChat(ctx context.Context) bool {
	return mc.GetData(ctx).IsPrivateChat()
}
