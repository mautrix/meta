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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/messagix"
)

var avatarHTTPClient http.Client
var MediaReferer string

func DownloadMedia(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	// TODO adjust sec-fetch headers by media type
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Referer", MediaReferer)
	req.Header.Set("Accept", "image/avif,image/webp,*/*")
	req.Header.Set("User-Agent", messagix.UserAgent)
	req.Header.Add("sec-ch-ua", messagix.SecCHUserAgent)
	req.Header.Add("sec-ch-ua-platform", messagix.SecCHPlatform)
	resp, err := avatarHTTPClient.Do(req)
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	} else if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	} else if respData, err := io.ReadAll(resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read response data: %w", err)
	} else {
		return respData, nil
	}
}

func UpdateAvatar(
	ctx context.Context,
	newAvatarURL string,
	avatarID *string, avatarSet *bool, avatarURL *id.ContentURI,
	uploadAvatar func(context.Context, []byte, string) (*mautrix.RespMediaUpload, error),
	setAvatar func(context.Context, id.ContentURI) error,
) bool {
	log := zerolog.Ctx(ctx)
	var newAvatarID string
	if newAvatarURL != "" {
		parsedAvatarURL, _ := url.Parse(newAvatarURL)
		newAvatarID = path.Base(parsedAvatarURL.Path)
	}
	if *avatarID == newAvatarID && (*avatarSet || setAvatar == nil) {
		return false
	}
	*avatarID = newAvatarID
	*avatarSet = false
	*avatarURL = id.ContentURI{}
	if newAvatarID == "" {
		if setAvatar == nil {
			return true
		}
		err := setAvatar(ctx, *avatarURL)
		if err != nil {
			log.Err(err).Msg("Failed to remove avatar")
			return true
		}
		log.Debug().Msg("Avatar removed")
		*avatarSet = true
		return true
	}
	avatarData, err := DownloadMedia(ctx, newAvatarURL)
	if err != nil {
		log.Err(err).
			Str("avatar_id", newAvatarID).
			Msg("Failed to download new avatar")
		return true
	}
	avatarContentType := http.DetectContentType(avatarData)
	resp, err := uploadAvatar(ctx, avatarData, avatarContentType)
	if err != nil {
		log.Err(err).
			Str("avatar_id", newAvatarID).
			Msg("Failed to upload new avatar")
		return true
	}
	*avatarURL = resp.ContentURI
	if setAvatar == nil {
		return true
	}
	err = setAvatar(ctx, *avatarURL)
	if err != nil {
		log.Err(err).Msg("Failed to update avatar")
		return true
	}
	log.Debug().
		Str("avatar_id", newAvatarID).
		Stringer("avatar_mxc", resp.ContentURI).
		Msg("Avatar updated successfully")
	*avatarSet = true
	return true
}
