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
	"go.mau.fi/whatsmeow"
)

type DirectMediaMeta struct {
	MimeType  string `json:"mime_type"`
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at,omitempty"` // Unix ms timestamp

	// For blob attachments (message re-fetch):
	AttachmentFbid string `json:"attachment_fbid,omitempty"`
	PartIndex      int    `json:"part_index,omitempty"`

	// For XMA attachments (Instagram API refresh):
	XMATargetID  int64  `json:"xma_target_id,omitempty"`
	XMAShortcode string `json:"xma_shortcode,omitempty"`

	// For XMA story attachments (parsed from action URL):
	StoryMediaID string `json:"story_media_id,omitempty"` // story pk
	StoryReelID  string `json:"story_reel_id,omitempty"`  // user pk (for /stories/direct/ type)
}

type DirectMediaWhatsApp struct {
	Key        []byte              `json:"key"`
	Type       whatsmeow.MediaType `json:"type"`
	SHA256     []byte              `json:"sha256"`
	EncSHA256  []byte              `json:"enc_sha256"`
	DirectPath string              `json:"direct_path"`
}

func (f *DirectMediaWhatsApp) GetDirectPath() string {
	return f.DirectPath
}

func (f *DirectMediaWhatsApp) GetMediaType() whatsmeow.MediaType {
	return f.Type
}

func (f *DirectMediaWhatsApp) GetMediaKey() []byte {
	return f.Key
}

func (f *DirectMediaWhatsApp) GetFileSHA256() []byte {
	return f.SHA256
}

func (f *DirectMediaWhatsApp) GetFileEncSHA256() []byte {
	return f.EncSHA256
}

var (
	_ whatsmeow.DownloadableMessage = (*DirectMediaWhatsApp)(nil)
	_ whatsmeow.MediaTypeable       = (*DirectMediaWhatsApp)(nil)
)
