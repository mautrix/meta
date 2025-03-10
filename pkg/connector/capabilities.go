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
	"maps"
	"time"

	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var metaGeneralCaps = &bridgev2.NetworkGeneralCapabilities{
	DisappearingMessages: false,
	AggressiveUpdateInfo: false,
}

func (m *MetaConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return metaGeneralCaps
}

func (m *MetaConnector) GetBridgeInfoVersion() (info, caps int) {
	return 1, 4
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
	base := "fi.mau.meta.capabilities.2025_02_18"
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
				"audio/m4a":  event.CapLevelFullySupported,
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
			Caption:          event.CapLevelFullySupported,
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
	MaxTextLength: MaxTextLength,
	Reply:         event.CapLevelFullySupported,
	Edit:          event.CapLevelFullySupported,
	EditMaxCount:  10,
	EditMaxAge:    ptr.Ptr(jsontime.S(24 * time.Hour)),
	Delete:        event.CapLevelFullySupported,
	DeleteForMe:   false,
	DeleteMaxAge:  ptr.Ptr(jsontime.S(10 * time.Minute)),
	Reaction:      event.CapLevelFullySupported,
	ReactionCount: 1,
	//LocationMessage: event.CapLevelPartialSupport,
}

var metaCapsWithThreads *event.RoomFeatures
var igCaps *event.RoomFeatures

func init() {
	metaCapsWithThreads = ptr.Clone(metaCaps)
	metaCapsWithThreads.ID += "+communitygroup"
	metaCapsWithThreads.Thread = event.CapLevelFullySupported

	igCaps = ptr.Clone(metaCaps)
	igCaps.File = maps.Clone(igCaps.File)
	delete(igCaps.File, event.MsgFile)
	for key, value := range igCaps.File {
		igCaps.File[key] = ptr.Clone(value)
		igCaps.File[key].Caption = event.CapLevelDropped
	}
	igCaps.ID += "+instagram-p2"
}

func (m *MetaClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	switch portal.Metadata.(*metaid.PortalMetadata).ThreadType {
	case table.COMMUNITY_GROUP:
		return metaCapsWithThreads
	}
	if (m.Client != nil && m.Client.Platform == types.Instagram) || m.Main.Config.Mode == types.Instagram {
		return igCaps
	}
	return metaCaps
}
