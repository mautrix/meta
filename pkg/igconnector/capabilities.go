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

package igconnector

import (
	"context"
	"time"

	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
)

var metaGeneralCaps = &bridgev2.NetworkGeneralCapabilities{
	DisappearingMessages: true,
	AggressiveUpdateInfo: false,
	ImplicitReadReceipts: true,
	Provisioning: bridgev2.ProvisioningCapabilities{
		ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
			CreateDM: true,
			Search:   true,
		},
		GroupCreation: map[string]bridgev2.GroupTypeCapabilities{
			"group": {
				TypeDescription: "a group",
				Participants:    bridgev2.GroupFieldCapability{Allowed: true, Required: true, MinLength: 2, MaxLength: 250},
			},
		},
	},
}

func (ic *IGConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return metaGeneralCaps
}

func (ic *IGConnector) GetBridgeInfoVersion() (info, caps int) {
	return 1, 15
}

const MaxTextLength = 20000
const MaxFileSize = 25 * 1000 * 1000
const MaxImageSize = 8 * 1000 * 1000

func supportedIfFFmpeg() event.CapabilitySupportLevel {
	if ffmpeg.Supported() {
		return event.CapLevelPartialSupport
	}
	return event.CapLevelRejected
}

func capID() string {
	base := "fi.mau.meta.capabilities.2026_07_07"
	if ffmpeg.Supported() {
		return base + "+ffmpeg+instagram"
	}
	return base + "+instagram"
}

var igCaps = &event.RoomFeatures{
	ID: capID(),
	Formatting: event.FormattingFeatureMap{
		event.FmtUserLink:      event.CapLevelFullySupported,
		event.FmtCodeBlock:     event.CapLevelFullySupported,
		event.FmtInlineCode:    event.CapLevelFullySupported,
		event.FmtBold:          event.CapLevelFullySupported,
		event.FmtItalic:        event.CapLevelFullySupported,
		event.FmtStrikethrough: event.CapLevelFullySupported,
		event.FmtBlockquote:    event.CapLevelFullySupported,
	},
	File: event.FileFeatureMap{
		event.MsgImage: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"image/jpeg": event.CapLevelFullySupported,
				"image/png":  event.CapLevelFullySupported,
				"image/gif":  event.CapLevelFullySupported,
				"image/webp": event.CapLevelFullySupported,
			},
			Caption: event.CapLevelDropped,
			MaxSize: MaxImageSize,
		},
		event.MsgVideo: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"video/mp4":       event.CapLevelFullySupported,
				"video/quicktime": event.CapLevelFullySupported,
				"video/webm":      event.CapLevelFullySupported,
				"video/ogg":       event.CapLevelFullySupported,
			},
			Caption: event.CapLevelDropped,
			MaxSize: MaxFileSize,
		},
		event.MsgAudio: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"audio/mpeg": event.CapLevelFullySupported,
				"audio/mp4":  event.CapLevelFullySupported,
				"audio/wav":  event.CapLevelFullySupported,
			},
			Caption: event.CapLevelDropped,
			MaxSize: MaxFileSize,
		},
		event.CapMsgGIF: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"image/gif": event.CapLevelFullySupported,
			},
			Caption: event.CapLevelDropped,
			MaxSize: MaxImageSize,
		},
		event.CapMsgSticker: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"image/png":  event.CapLevelFullySupported,
				"image/webp": event.CapLevelFullySupported,
			},
			Caption: event.CapLevelDropped,
			MaxSize: MaxImageSize,
		},
		event.CapMsgVoice: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"audio/mp4": event.CapLevelFullySupported,
				"audio/ogg": supportedIfFFmpeg(),
			},
			Caption: event.CapLevelDropped,
			MaxSize: MaxFileSize,
		},
	},
	MaxTextLength:       MaxTextLength,
	Reply:               event.CapLevelFullySupported,
	Edit:                event.CapLevelFullySupported,
	EditMaxCount:        5,
	EditMaxAge:          ptr.Ptr(jsontime.S(15 * time.Minute)),
	Delete:              event.CapLevelFullySupported,
	DeleteForMe:         false,
	Reaction:            event.CapLevelFullySupported,
	ReactionCount:       1,
	TypingNotifications: true,
	MessageRequest: &event.MessageRequestFeatures{
		AcceptWithButton: event.CapLevelFullySupported,
	},
	//LocationMessage: event.CapLevelPartialSupport,
	DeleteChat: true,
}

var igCapsGroup *event.RoomFeatures

func init() {
	igCapsGroup = igCaps.Clone()
	igCapsGroup.ID += "+instagram-group"
	igCapsGroup.State = event.StateFeatureMap{
		event.StateRoomName.Type:   {Level: event.CapLevelFullySupported},
		event.StateRoomAvatar.Type: {Level: event.CapLevelFullySupported},
	}
	igCapsGroup.MemberActions = map[event.MemberAction]event.CapabilitySupportLevel{
		event.MemberActionInvite: event.CapLevelFullySupported,
		event.MemberActionKick:   event.CapLevelFullySupported,
	}
}

func (ic *IGClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	if portal.RoomType == database.RoomTypeDM {
		return igCaps
	}
	return igCapsGroup
}
