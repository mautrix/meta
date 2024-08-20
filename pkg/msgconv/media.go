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
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix"
)

var mediaHTTPClient = http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ForceAttemptHTTP2:     true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Hostname() == "video.xx.fbcdn.net" {
			return http.ErrUseLastResponse
		}
		return nil
	},
	Timeout: 120 * time.Second,
}
var BypassOnionForMedia bool

var ErrTooLargeFile = errors.New("too large file")

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
	hdr.Set("User-Agent", messagix.UserAgent)
	hdr.Set("sec-ch-ua", messagix.SecCHUserAgent)
	hdr.Set("sec-ch-ua-platform", messagix.SecCHPlatform)
}

func downloadChunkedVideo(ctx context.Context, mime, url string, maxSize int64) ([]byte, error) {
	log := zerolog.Ctx(ctx)
	log.Trace().Str("url", url).Msg("Downloading video in chunks")
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	addDownloadHeaders(req.Header, mime)
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HEAD request: %w", err)
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d for HEAD request", resp.StatusCode)
	} else if resp.Header.Get("Accept-Ranges") != "bytes" {
		return nil, fmt.Errorf("server does not support byte range requests")
	} else if resp.ContentLength <= 0 {
		return nil, fmt.Errorf("server didn't return media size")
	} else if resp.ContentLength > maxSize {
		return nil, fmt.Errorf("%w (%.2f MiB)", ErrTooLargeFile, float64(resp.ContentLength)/1024/1024)
	}
	log.Debug().Int64("content_length", resp.ContentLength).Msg("Found video size to download in chunks")

	const chunkSize = 1 * 1024 * 1024
	fullData := make([]byte, resp.ContentLength)
	for i := int64(0); i < resp.ContentLength; i += chunkSize {
		end := i + chunkSize - 1
		if end > resp.ContentLength {
			end = resp.ContentLength - 1
		}
		byteRange := fmt.Sprintf("bytes=%d-%d", i, end)
		log.Debug().Str("range", byteRange).Msg("Downloading chunk")
		_, err = downloadMedia(ctx, mime, url, maxSize, byteRange, false, fullData[i:end+1])
		if err != nil {
			return nil, fmt.Errorf("failed to download chunk %d-%d: %w", i, end, err)
		}
	}
	log.Debug().Int("data_length", len(fullData)).Msg("Download complete")
	return fullData, nil
}

func DownloadMedia(ctx context.Context, mime, url string, maxSize int64) ([]byte, error) {
	return downloadMedia(ctx, mime, url, maxSize, "", true, nil)
}

func downloadMedia(ctx context.Context, mime, url string, maxSize int64, byteRange string, switchToChunked bool, readInto []byte) ([]byte, error) {
	zerolog.Ctx(ctx).Trace().Str("url", url).Msg("Downloading media")
	if BypassOnionForMedia {
		url = strings.ReplaceAll(url, "facebookcooa4ldbat4g7iacswl3p2zrf5nuylvnhxn6kqolvojixwid.onion", "fbcdn.net")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	addDownloadHeaders(req.Header, mime)
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}

	resp, err := mediaHTTPClient.Do(req)
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	} else if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		if resp.StatusCode == 302 && switchToChunked {
			loc, _ := resp.Location()
			if loc != nil && loc.Hostname() == "video.xx.fbcdn.net" {
				return downloadChunkedVideo(ctx, mime, loc.String(), maxSize)
			}
		}
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	} else if resp.ContentLength > maxSize {
		return nil, fmt.Errorf("%w (%.2f MiB)", ErrTooLargeFile, float64(resp.ContentLength)/1024/1024)
	}
	zerolog.Ctx(ctx).Debug().Int64("content_length", resp.ContentLength).Msg("Got media response, reading data")
	if readInto != nil {
		if resp.ContentLength != int64(len(readInto)) {
			return nil, fmt.Errorf("buffer size (%d) does not match content length (%d)", len(readInto), resp.ContentLength)
		}
		_, err = io.ReadFull(resp.Body, readInto)
		if err != nil {
			return nil, fmt.Errorf("failed to read response data into buffer: %w", err)
		}
		return readInto, nil
	} else if respData, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+2)); err != nil {
		return nil, fmt.Errorf("failed to read response data: %w", err)
	} else if int64(len(respData)) > maxSize {
		return nil, ErrTooLargeFile
	} else {
		zerolog.Ctx(ctx).Debug().Int("data_length", len(respData)).Msg("Media download complete")
		return respData, nil
	}
}
