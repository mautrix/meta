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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"go.mau.fi/util/exmime"
	"go.mau.fi/util/ffmpeg"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

// MediaRefreshMeta contains identifiers needed to refresh expired media URLs
type MediaRefreshMeta struct {
	ExpiresAt jsontime.UnixMilli `json:"expires_at,omitzero"`

	// For blob attachments (message re-fetch):
	AttachmentFBID string `json:"attachment_fbid,omitempty"`
	PartIndex      int    `json:"part_index,omitempty"`

	// For XMA attachments (Instagram API refresh)
	XMATargetID  int64  `json:"xma_target_id,omitempty"`
	XMAShortcode string `json:"xma_shortcode,omitempty"`

	CarouselMediaID string `json:"carousel_media_id,omitempty"`

	// For XMA story attachments (pre-parsed from action URL)
	StoryMediaID string `json:"story_media_id,omitempty"` // story pk
	StoryReelID  string `json:"story_reel_id,omitempty"`  // user pk (for /stories/direct/ type)
}

type DirectMediaMeta struct {
	MediaRefreshMeta
	MimeType string `json:"mime_type,omitempty"`
	URL      string `json:"url"`
}

var ErrURLNotFound = errors.New("url not found")

type ReuploadParams struct {
	AttachmentType table.AttachmentType
	URL            string
	FileName       string
	MimeType       string

	FileSize int
	Width    int
	Height   int
	Duration int

	PreviewWidth  int
	PreviewHeight int

	RefreshMeta *MediaRefreshMeta

	DirectMedia bool
	MaxFileSize int64
}

func ParseExpiryFromURL(urlStr string) time.Time {
	parsedURL, _ := url.Parse(urlStr)
	if parsedURL == nil {
		return time.Time{}
	}
	expiresAtInt, _ := strconv.ParseInt(parsedURL.Query().Get("oe"), 16, 64)
	if expiresAtInt > 0 {
		return time.Unix(expiresAtInt, 0)
	}
	return time.Time{}
}

func ReuploadFileToMatrix(ctx context.Context, params ReuploadParams) (*bridgev2.ConvertedMessagePart, error) {
	if params.URL == "" {
		return nil, ErrURLNotFound
	}
	needMime := params.MimeType == ""
	parsedURL, _ := url.Parse(params.URL)
	if params.MimeType == "" && parsedURL != nil {
		ext := path.Ext(parsedURL.Path)
		if ext == ".webm" {
			params.MimeType = "video/webm"
		} else {
			params.MimeType = mime.TypeByExtension(ext)
		}
	}
	if params.FileName == "" && parsedURL != nil {
		params.FileName = path.Base(parsedURL.Path)
	}

	portal := ctx.Value(ContextKeyPortal).(*bridgev2.Portal)
	content := &event.MessageEventContent{
		Info: &event.FileInfo{},
	}
	extra := map[string]any{}
	eventType := event.EventMessage
	fillMetadata := func() {
		if params.AttachmentType == table.AttachmentTypeAnimatedImage && params.MimeType == "video/mp4" {
			content.Info.MauGIF = true
			extra["info"] = map[string]any{
				"fi.mau.loop":          true,
				"fi.mau.autoplay":      true,
				"fi.mau.hide_controls": true,
				"fi.mau.no_audio":      true,
			}
		}
		switch params.AttachmentType {
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
				Duration: params.Duration,
				Waveform: []int{},
			}
		default:
			switch strings.Split(params.MimeType, "/")[0] {
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
		content.Body = params.FileName
		content.Info.MimeType = params.MimeType
		content.Info.Duration = params.Duration
		content.Info.Width = cmp.Or(params.Width, params.PreviewWidth)
		content.Info.Height = cmp.Or(params.Height, params.PreviewHeight)

		if content.Body == "" {
			content.Body = strings.TrimPrefix(string(content.MsgType), "m.") + exmime.ExtensionFromMimetype(params.MimeType)
		} else if content.MsgType != "" && !strings.ContainsRune(content.Body, '.') {
			content.Body += exmime.ExtensionFromMimetype(params.MimeType)
		}
	}

	if params.DirectMedia {
		msgID := ctx.Value(ContextKeyMsgID).(networkid.MessageID)
		var partID networkid.PartID
		if ctx.Value(ContextKeyPartID) != nil {
			partID = ctx.Value(ContextKeyPartID).(networkid.PartID)
		}
		loginID := portal.Receiver
		login, ok := ctx.Value(ContextKeyUserLogin).(*bridgev2.UserLogin)
		if ok {
			loginID = login.ID
		}
		mediaID := metaid.MakeMediaID(metaid.DirectMediaTypeMetaV2, loginID, msgID, partID)
		var err error
		content.URL, err = portal.Bridge.Matrix.GenerateContentURI(ctx, mediaID)
		if err != nil {
			return nil, err
		}
		if params.RefreshMeta != nil && params.RefreshMeta.ExpiresAt.IsZero() {
			params.RefreshMeta.ExpiresAt = jsontime.UM(ParseExpiryFromURL(params.URL))
		}
		directMediaMeta, err := json.Marshal(&DirectMediaMeta{
			MimeType:         params.MimeType,
			URL:              params.URL,
			MediaRefreshMeta: ptr.Val(params.RefreshMeta),
		})
		if err != nil {
			return nil, err
		}
		content.Info.Size = params.FileSize
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

	size, reader, err := DownloadMedia(ctx, params.MimeType, params.URL, params.MaxFileSize)
	if err != nil {
		if errors.Is(err, ErrTooLargeFile) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", bridgev2.ErrMediaDownloadFailed, err)
	}
	defer reader.Close()
	content.Info.Size = int(size)
	needVoiceConvert := params.AttachmentType == table.AttachmentTypeAudio && params.MimeType != "audio/ogg" && ffmpeg.Supported()
	needImageSize := (params.Width == 0 || params.Height == 0) &&
		(params.AttachmentType == table.AttachmentTypeImage ||
			params.AttachmentType == table.AttachmentTypeEphemeralImage ||
			params.AttachmentType == table.AttachmentTypeSticker ||
			(params.AttachmentType == table.AttachmentTypeNone && strings.HasPrefix(params.MimeType, "image/")))
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
			m, err := mimetype.DetectReader(destRS)
			if err != nil {
				return nil, err
			}
			params.MimeType = m.String()
		}
		var replPath string
		if needVoiceConvert {
			destFile := dest.(*os.File)
			_, err = destFile.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}
			_ = destFile.Close()
			sourceFileName := destFile.Name() + exmime.ExtensionFromMimetype(params.MimeType)
			err = os.Rename(destFile.Name(), sourceFileName)
			if err != nil {
				return nil, err
			}
			replPath, err = ffmpeg.ConvertPath(ctx, sourceFileName, ".ogg", []string{}, []string{"-c:a", "libopus"}, true)
			if err != nil {
				return nil, fmt.Errorf("%w (audio to ogg/opus): %w", bridgev2.ErrMediaConvertFailed, err)
			}
			params.FileName += ".ogg"
			params.MimeType = "audio/ogg"
		} else if needImageSize {
			destRS := dest.(io.ReadSeeker)
			_, err = destRS.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}
			config, _, err := image.DecodeConfig(destRS)
			if err == nil {
				params.Width, params.Height = config.Width, config.Height
			}
		}
		return &bridgev2.FileStreamResult{
			ReplacementFile: replPath,
			FileName:        params.FileName,
			MimeType:        params.MimeType,
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
