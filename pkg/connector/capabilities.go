// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2025 Tulir Asokan
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

package connector

import (
	"context"
	"time"

	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
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

func (m *MetaConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return metaGeneralCaps
}

func (m *MetaConnector) GetBridgeInfoVersion() (info, caps int) {
	return 1, 12
}

const MaxTextLength = 20000
const MaxFileSize = 25 * 1000 * 1000
const MaxFileSizeWithE2E = 100 * 1000 * 1000
const MaxImageSize = 8 * 1000 * 1000

func supportedIfFFmpeg() event.CapabilitySupportLevel {
	if ffmpeg.Supported() {
		return event.CapLevelPartialSupport
	}
	return event.CapLevelRejected
}

func capID() string {
	base := "fi.mau.meta.capabilities.2026_01_25"
	if ffmpeg.Supported() {
		return base + "+ffmpeg"
	}
	return base
}

var metaCaps = &event.RoomFeatures{
	ID: capID(),
	Formatting: event.FormattingFeatureMap{
		event.FmtUserLink: event.CapLevelFullySupported,
	},
	File: event.FileFeatureMap{
		event.MsgImage: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"image/jpeg": event.CapLevelFullySupported,
				"image/png":  event.CapLevelFullySupported,
				"image/gif":  event.CapLevelFullySupported,
				"image/webp": event.CapLevelFullySupported,
			},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: MaxTextLength,
			MaxSize:          MaxImageSize,
		},
		event.MsgVideo: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"video/mp4":       event.CapLevelFullySupported,
				"video/quicktime": event.CapLevelFullySupported,
				"video/webm":      event.CapLevelFullySupported,
				"video/ogg":       event.CapLevelFullySupported,
			},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: MaxTextLength,
			MaxSize:          MaxFileSize,
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
		event.MsgFile: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"*/*": event.CapLevelFullySupported,
			},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: MaxTextLength,
			MaxSize:          MaxFileSize,
		},
		event.CapMsgGIF: {
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"image/gif": event.CapLevelFullySupported,
			},
			Caption:          event.CapLevelDropped,
			MaxCaptionLength: MaxTextLength,
			MaxSize:          MaxImageSize,
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
	EditMaxCount:        10,
	EditMaxAge:          ptr.Ptr(jsontime.S(24 * time.Hour)),
	Delete:              event.CapLevelFullySupported,
	DeleteForMe:         false,
	DeleteMaxAge:        ptr.Ptr(jsontime.S(10 * time.Minute)),
	Reaction:            event.CapLevelFullySupported,
	ReactionCount:       1,
	TypingNotifications: true,
	//LocationMessage: event.CapLevelPartialSupport,
	DeleteChat: true,
}

var metaCapsWithThreads *event.RoomFeatures
var metaCapsWithE2E *event.RoomFeatures
var metaCapsWithE2EGroup *event.RoomFeatures
var igCaps *event.RoomFeatures
var igCapsGroup *event.RoomFeatures
var metaCapsGroup *event.RoomFeatures

func init() {
	metaCapsWithThreads = metaCaps.Clone()
	metaCapsWithThreads.ID += "+communitygroup"
	metaCapsWithThreads.Thread = event.CapLevelFullySupported
	metaCapsWithThreads.TypingNotifications = false

	metaCapsWithE2E = metaCaps.Clone()
	metaCapsWithE2E.ID += "+e2e"
	for _, value := range metaCapsWithE2E.File {
		value.MaxSize = MaxFileSizeWithE2E
		// Messenger Web doesn't render captions on images in e2ee chats 3:<
		// (works fine on Messenger iOS and Android though)
		value.Caption = event.CapLevelDropped
	}
	delete(metaCapsWithE2E.File[event.MsgVideo].MimeTypes, "video/webm")
	delete(metaCapsWithE2E.File[event.MsgVideo].MimeTypes, "video/ogg")
	metaCapsWithE2E.DeleteChat = false
	metaCapsWithE2EGroup = metaCapsWithE2E.Clone()
	metaCapsWithE2EGroup.ID += "+group"
	metaCapsWithE2EGroup.MemberActions = map[event.MemberAction]event.CapabilitySupportLevel{
		event.MemberActionInvite: event.CapLevelFullySupported,
		event.MemberActionKick:   event.CapLevelFullySupported,
	}

	metaCapsGroup = metaCaps.Clone()
	metaCapsGroup.ID += "+group"
	metaCapsGroup.State = event.StateFeatureMap{
		event.StateRoomName.Type:   {Level: event.CapLevelFullySupported},
		event.StateRoomAvatar.Type: {Level: event.CapLevelFullySupported},
	}
	metaCapsGroup.MemberActions = metaCapsWithE2EGroup.MemberActions.Clone()

	igCaps = metaCaps.Clone()
	delete(igCaps.File, event.MsgFile)
	for _, value := range igCaps.File {
		value.Caption = event.CapLevelDropped
	}
	igCaps.ID += "+instagram-p2"
	igCapsGroup = igCaps.Clone()
	igCapsGroup.ID += "+instagram-group"
	igCapsGroup.State = event.StateFeatureMap{
		event.StateRoomName.Type: {Level: event.CapLevelFullySupported},
	}
	igCapsGroup.MemberActions = metaCapsWithE2EGroup.MemberActions.Clone()
}

func (m *MetaClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	switch portal.Metadata.(*metaid.PortalMetadata).ThreadType {
	case table.COMMUNITY_GROUP:
		return metaCapsWithThreads
	case table.ENCRYPTED_OVER_WA_ONE_TO_ONE:
		return metaCapsWithE2E
	case table.ENCRYPTED_OVER_WA_GROUP:
		return metaCapsWithE2EGroup
	}
	if m.Client.GetPlatform() == types.Instagram || m.Main.Config.Mode == types.Instagram {
		if portal.RoomType == database.RoomTypeDM {
			return igCaps
		}
		return igCapsGroup
	}
	if portal.RoomType == database.RoomTypeDM {
		return metaCaps
	}
	return metaCapsGroup
}
