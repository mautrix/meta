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

package mediadl

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
	"maunium.net/go/mautrix"
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
	WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported).WithStatus(event.MessageStatusFail)

var ErrForbidden = errors.New("http forbidden")
var ErrNoContentLength = errors.New("server didn't return media size")

type HTTPStatusError struct {
	StatusCode int
}

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status code %d", e.StatusCode)
}

// WrapDirectMediaError turns a failed download into an HTTP error for the media proxy, which answers 404
// to any other error. Transport errors are passed through, since the proxy answers 502 for those itself.
func WrapDirectMediaError(err error) error {
	var mapped mautrix.RespError
	statusErr, _ := errors.AsType[HTTPStatusError](err)
	switch {
	case errors.Is(err, ErrForbidden), statusErr.StatusCode == http.StatusUnauthorized:
		mapped = mautrix.MForbidden.WithMessage("Access to the media was denied")
	case statusErr.StatusCode == http.StatusNotFound, statusErr.StatusCode == http.StatusGone:
		mapped = mautrix.MNotFound.WithMessage("Media no longer exists")
	case statusErr.StatusCode == http.StatusTooManyRequests:
		mapped = mautrix.MLimitExceeded.WithMessage("Rate limited while downloading media").WithCanRetry(true)
	case statusErr.StatusCode != 0:
		mapped = mautrix.MUnknown.WithMessage("Failed to download media").WithStatus(http.StatusBadGateway).WithCanRetry(true)
	default:
		return err
	}
	return fmt.Errorf("%w: %w", mapped, err)
}

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
		return 0, nil, HTTPStatusError{StatusCode: resp.StatusCode}
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
	if err == nil {
		// There's no body so close it immediately
		_ = resp.Body.Close()
	}
	if err != nil {
		return 0, nil, fmt.Errorf("failed to send HEAD request: %w", err)
	} else if resp.StatusCode == http.StatusForbidden || (resp.StatusCode == http.StatusOK && resp.Header.Get("Accept-Ranges") != "bytes") {
		log.Warn().Int("status_code", resp.StatusCode).
			Msg("HEAD request failed, falling back to normal download")
		return downloadMedia(ctx, mime, url, maxSize, "", false)
	} else if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("%w for HEAD request", HTTPStatusError{StatusCode: resp.StatusCode})
	} else if resp.ContentLength <= 0 {
		return 0, nil, ErrNoContentLength
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
