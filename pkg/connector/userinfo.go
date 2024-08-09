package connector

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/messagix/types"
	"go.mau.fi/mautrix-meta/msgconv"
)

func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	return nil, fmt.Errorf("getting user info is not supported")
}

func (m *MetaClient) wrapUserInfo(info types.UserInfo) *bridgev2.UserInfo {
	var identifiers []string
	if m.LoginMeta.Platform == types.Instagram {
		identifiers = append(identifiers, fmt.Sprintf("instagram:%s", info.GetUsername()))
	}
	name := m.Main.Config.FormatDisplayname(DisplaynameParams{
		DisplayName: info.GetName(),
		Username:    info.GetUsername(),
		ID:          info.GetFBID(),
	})
	var avatar *bridgev2.Avatar
	_, isInitialData := info.(*types.CurrentUserInitialData)
	if !isInitialData {
		avatar = wrapAvatar(info.GetAvatarURL())
	}

	return &bridgev2.UserInfo{
		Identifiers: identifiers,
		Name:        &name,
		Avatar:      avatar,
		IsBot:       nil, // TODO
		ExtraUpdates: func(ctx context.Context, ghost *bridgev2.Ghost) (changed bool) {
			meta := ghost.Metadata.(*GhostMetadata)
			if m.LoginMeta.Platform == types.Instagram && meta.Username != info.GetUsername() {
				meta.Username = info.GetUsername()
				changed = true
			}
			return
		},
	}
}

func wrapAvatar(avatarURL string) *bridgev2.Avatar {
	if avatarURL == "" {
		return &bridgev2.Avatar{Remove: true}
	}
	parsedURL, _ := url.Parse(avatarURL)
	avatarID := path.Base(parsedURL.Path)
	return &bridgev2.Avatar{
		ID: networkid.AvatarID(avatarID),
		Get: func(ctx context.Context) ([]byte, error) {
			return msgconv.DownloadMedia(ctx, "image/*", avatarURL, 5*1024*1024)
		},
	}
}
