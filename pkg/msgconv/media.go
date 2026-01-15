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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/exhttp"
	"go.mau.fi/whatsmeow"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

var mediaHTTPClient *http.Client
var BypassOnionForMedia bool

func SetProxy(proxy string) {
	parsedURL := exerrors.Must(url.Parse(proxy))
	mediaHTTPClient.Transport.(*http.Transport).Proxy = http.ProxyURL(parsedURL)
}

func SetHTTP(settings exhttp.ClientSettings) {
	oldClient := mediaHTTPClient
	mediaHTTPClient = settings.WithGlobalTimeout(5 * time.Minute).Compile()
	mediaHTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Hostname() == "video.xx.fbcdn.net" {
			return http.ErrUseLastResponse
		}
		return nil
	}
	if oldClient != nil {
		oldClient.CloseIdleConnections()
	}
}

var ErrTooLargeFile = bridgev2.WrapErrorInStatus(errors.New("too large file")).
	WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)

var ErrForbidden = errors.New("http forbidden")

func addDownloadHeaders(hdr http.Header, mime string) {
	hdr.Set("Accept", "*/*")
	switch strings.Split(mime, "/")[0] {
	case "image":
		hdr.Set("Accept", "image/avif,image/webp,*/*")
		hdr.Set("Sec-Fetch-Dest", "image")
	case "video":
		hdr.Set("Sec-Fetch-Dest", "video")
	case "audio":
		hdr.Set("Sec-Fetch-Dest", "audio")
	default:
		hdr.Set("Sec-Fetch-Dest", "empty")
	}
	hdr.Set("Sec-Fetch-Mode", "no-cors")
	hdr.Set("Sec-Fetch-Site", "cross-site")
	// Setting a referer seems to disable redirects for some reason
	//hdr.Set("Referer", MediaReferer)
	hdr.Set("User-Agent", useragent.UserAgent)
	hdr.Set("sec-ch-ua", useragent.SecCHUserAgent)
	hdr.Set("sec-ch-ua-platform", useragent.SecCHPlatform)
}

func DownloadAvatar(ctx context.Context, url string) ([]byte, error) {
	_, resp, err := downloadMedia(ctx, "image/*", url, 5*1024*1024, "", false)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(resp)
}

func DownloadMedia(ctx context.Context, mime, url string, maxSize int64) (int64, io.ReadCloser, error) {
	return downloadMedia(ctx, mime, url, maxSize, "", true)
}

func downloadMedia(ctx context.Context, mime, url string, maxSize int64, byteRange string, switchToChunked bool) (int64, io.ReadCloser, error) {
	zerolog.Ctx(ctx).Trace().Str("url", url).Msg("Downloading media")
	if BypassOnionForMedia {
		url = strings.ReplaceAll(url, "facebookcooa4ldbat4g7iacswl3p2zrf5nuylvnhxn6kqolvojixwid.onion", "fbcdn.net")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	addDownloadHeaders(req.Header, mime)
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}

	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to send request: %w", err)
	} else if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		_ = resp.Body.Close()
		if resp.StatusCode == 302 && switchToChunked {
			loc, _ := resp.Location()
			if loc != nil && loc.Hostname() == "video.xx.fbcdn.net" {
				return downloadChunkedVideo(ctx, mime, loc.String(), maxSize)
			}
		}
		if resp.StatusCode == 403 {
			return 0, nil, ErrForbidden
		}
		return 0, nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	} else if resp.ContentLength > maxSize {
		_ = resp.Body.Close()
		return resp.ContentLength, nil, fmt.Errorf("%w (%.2f MiB)", ErrTooLargeFile, float64(resp.ContentLength)/1024/1024)
	}
	zerolog.Ctx(ctx).Debug().Int64("content_length", resp.ContentLength).Msg("Got media response")
	return resp.ContentLength, resp.Body, nil
}

type chunkedVideoDownloader struct {
	ctx             context.Context
	mime            string
	url             string
	offset          int64
	totalSize       int64
	inFlightRequest io.ReadCloser
}

const chunkSize = 1 * 1024 * 1024

func (cvd *chunkedVideoDownloader) Read(p []byte) (n int, err error) {
	if cvd.inFlightRequest == nil {
		end := cvd.offset + chunkSize - 1
		if end > cvd.totalSize {
			end = cvd.totalSize - 1
		}
		byteRange := fmt.Sprintf("bytes=%d-%d", cvd.offset, end)
		zerolog.Ctx(cvd.ctx).Debug().Str("range", byteRange).Msg("Downloading chunk")
		_, cvd.inFlightRequest, err = downloadMedia(cvd.ctx, cvd.mime, cvd.url, cvd.totalSize, byteRange, false)
		if err != nil {
			err = fmt.Errorf("failed to start download for chunk %d-%d: %w", cvd.offset, end, err)
			return
		}
		cvd.offset = end + 1
	}
	n, err = cvd.inFlightRequest.Read(p)
	if errors.Is(err, io.EOF) {
		_ = cvd.inFlightRequest.Close()
		cvd.inFlightRequest = nil
		if cvd.offset < cvd.totalSize {
			err = nil
		}
	}
	return
}

func (cvd *chunkedVideoDownloader) Close() error {
	if cvd.inFlightRequest != nil {
		return cvd.inFlightRequest.Close()
	}
	return nil
}

func downloadChunkedVideo(ctx context.Context, mime, url string, maxSize int64) (int64, io.ReadCloser, error) {
	log := zerolog.Ctx(ctx)
	log.Trace().Str("url", url).Msg("Downloading video in chunks")
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	addDownloadHeaders(req.Header, mime)
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to send HEAD request: %w", err)
	} else if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("unexpected status code %d for HEAD request", resp.StatusCode)
	} else if resp.Header.Get("Accept-Ranges") != "bytes" {
		return 0, nil, fmt.Errorf("server does not support byte range requests")
	} else if resp.ContentLength <= 0 {
		return 0, nil, fmt.Errorf("server didn't return media size")
	} else if resp.ContentLength > maxSize {
		return resp.ContentLength, nil, fmt.Errorf("%w (%.2f MiB)", ErrTooLargeFile, float64(resp.ContentLength)/1024/1024)
	}
	log.Debug().Int64("content_length", resp.ContentLength).Msg("Found video size to download in chunks")
	return resp.ContentLength, &chunkedVideoDownloader{
		ctx:       ctx,
		mime:      mime,
		url:       url,
		totalSize: resp.ContentLength,
	}, nil
}

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
