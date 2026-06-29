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

package mediadl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

// MediaRefreshMeta contains identifiers needed to refresh expired media URLs
type MediaRefreshMeta struct {
	ExpiresAt int64 `json:"expires_at,omitempty"` // Unix seconds when the URL expires

	// For blob attachments (message re-fetch):
	AttachmentFBID string `json:"attachment_fbid,omitempty"`
	PartIndex      int    `json:"part_index,omitempty"`

	// For XMA attachments (Instagram API refresh)
	XMATargetID  int64  `json:"xma_target_id,omitempty"`
	XMAShortcode string `json:"xma_shortcode,omitempty"`

	// For XMA story attachments (pre-parsed from action URL)
	StoryMediaID string `json:"story_media_id,omitempty"` // story pk
	StoryReelID  string `json:"story_reel_id,omitempty"`  // user pk (for /stories/direct/ type)
}

type DirectMediaMeta struct {
	MediaRefreshMeta
	MimeType string `json:"mime_type"`
	URL      string `json:"url"`
}

var ErrURLNotFound = errors.New("url not found")

func ReuploadAttachment(
	ctx context.Context, attachmentType table.AttachmentType,
	url, fileName, mimeType string,
	fileSize, width, height, duration int,
	refreshMeta *MediaRefreshMeta,
	directMedia bool, maxFileSize int64,
) (*bridgev2.ConvertedMessagePart, error) {
	if url == "" {
		return nil, ErrURLNotFound
	}

	portal := ctx.Value(ContextKeyPortal).(*bridgev2.Portal)
	content := &event.MessageEventContent{
		Info: &event.FileInfo{},
	}
	extra := map[string]any{}
	if attachmentType == table.AttachmentTypeAnimatedImage && mimeType == "video/mp4" {
		extra["info"] = map[string]any{
			"fi.mau.gif":           true,
			"fi.mau.loop":          true,
			"fi.mau.autoplay":      true,
			"fi.mau.hide_controls": true,
			"fi.mau.no_audio":      true,
		}
	}
	eventType := event.EventMessage
	fillMetadata := func() {
		switch attachmentType {
		case table.AttachmentTypeSticker:
			eventType = event.EventSticker
		case table.AttachmentTypeImage, table.AttachmentTypeEphemeralImage:
			content.MsgType = event.MsgImage
		case table.AttachmentTypeVideo, table.AttachmentTypeEphemeralVideo:
			content.MsgType = event.MsgVideo
		case table.AttachmentTypeFile:
			content.MsgType = event.MsgFile
		case table.AttachmentTypeAudio:
			content.MsgType = event.MsgAudio
			content.MSC3245Voice = &event.MSC3245Voice{}
			content.MSC1767Audio = &event.MSC1767Audio{
				Duration: duration,
				Waveform: []int{},
			}
		default:
			switch strings.Split(mimeType, "/")[0] {
			case "image":
				content.MsgType = event.MsgImage
			case "video":
				content.MsgType = event.MsgVideo
			case "audio":
				content.MsgType = event.MsgAudio
			default:
				content.MsgType = event.MsgFile
			}
		}
		content.Body = fileName
		content.Info.MimeType = mimeType
		content.Info.Duration = duration
		content.Info.Width = width
		content.Info.Height = height

		if content.Body == "" {
			content.Body = strings.TrimPrefix(string(content.MsgType), "m.") + exmime.ExtensionFromMimetype(mimeType)
		} else if content.MsgType != "" && !strings.ContainsRune(content.Body, '.') {
			content.Body += exmime.ExtensionFromMimetype(mimeType)
		}
	}

	if directMedia {
		msgID := ctx.Value(ContextKeyMsgID).(networkid.MessageID)
		var partID networkid.PartID
		if ctx.Value(ContextKeyPartID) != nil {
			partID = ctx.Value(ContextKeyPartID).(networkid.PartID)
		}
		mediaID := metaid.MakeMediaID(metaid.DirectMediaTypeMetaV2, portal.Receiver, msgID, partID)
		var err error
		content.URL, err = portal.Bridge.Matrix.GenerateContentURI(ctx, mediaID)
		if err != nil {
			return nil, err
		}
		directMediaMeta, err := json.Marshal(&DirectMediaMeta{
			MimeType:         mimeType,
			URL:              url,
			MediaRefreshMeta: ptr.Val(refreshMeta),
		})
		if err != nil {
			return nil, err
		}
		content.Info.Size = fileSize
		fillMetadata()
		return &bridgev2.ConvertedMessagePart{
			ID:      partID,
			Type:    eventType,
			Content: content,
			Extra:   extra,
			DBMetadata: &metaid.MessageMetadata{
				DirectMediaMeta: directMediaMeta,
			},
		}, nil
	}

	size, reader, err := DownloadMedia(ctx, mimeType, url, maxFileSize)
	if err != nil {
		if errors.Is(err, ErrTooLargeFile) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", bridgev2.ErrMediaDownloadFailed, err)
	}
	defer reader.Close()
	content.Info.Size = int(size)
	needVoiceConvert := attachmentType == table.AttachmentTypeAudio && ffmpeg.Supported()
	needMime := mimeType == ""
	needImageSize := (attachmentType == table.AttachmentTypeImage || attachmentType == table.AttachmentTypeEphemeralImage) && (width == 0 || height == 0)
	requireFile := needVoiceConvert || needMime || needImageSize
	intent := ctx.Value(ContextKeyIntent).(bridgev2.MatrixAPI)
	content.URL, content.File, err = intent.UploadMediaStream(ctx, portal.MXID, size, requireFile, func(dest io.Writer) (*bridgev2.FileStreamResult, error) {
		_, err := io.Copy(dest, reader)
		if err != nil {
			return nil, err
		}
		if needMime {
			destRS := dest.(io.ReadSeeker)
			_, err = destRS.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}
			var mime *mimetype.MIME
			mime, err = mimetype.DetectReader(destRS)
			if err != nil {
				return nil, err
			}
			mimeType = mime.String()
		}
		var replPath string
		if needVoiceConvert {
			destFile := dest.(*os.File)
			_, err = destFile.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}
			_ = destFile.Close()
			sourceFileName := destFile.Name() + exmime.ExtensionFromMimetype(mimeType)
			err = os.Rename(destFile.Name(), sourceFileName)
			if err != nil {
				return nil, err
			}
			replPath, err = ffmpeg.ConvertPath(ctx, sourceFileName, ".ogg", []string{}, []string{"-c:a", "libopus"}, true)
			if err != nil {
				return nil, fmt.Errorf("%w (audio to ogg/opus): %w", bridgev2.ErrMediaConvertFailed, err)
			}
			fileName += ".ogg"
			mimeType = "audio/ogg"
		} else if needImageSize {
			destRS := dest.(io.ReadSeeker)
			_, err = destRS.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}
			config, _, err := image.DecodeConfig(destRS)
			if err == nil {
				width, height = config.Width, config.Height
			}
		}
		return &bridgev2.FileStreamResult{
			ReplacementFile: replPath,
			FileName:        fileName,
			MimeType:        mimeType,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	fillMetadata()
	return &bridgev2.ConvertedMessagePart{
		Type:    eventType,
		Content: content,
		Extra:   extra,
	}, nil
}
