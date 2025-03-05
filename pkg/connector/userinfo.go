package connector

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	return nil, fmt.Errorf("getting user info is not supported")
}

func (m *MetaClient) wrapUserInfo(info types.UserInfo) *bridgev2.UserInfo {
	var identifiers []string
	if m.LoginMeta.Platform == types.Instagram {
		identifiers = append(identifiers, fmt.Sprintf("instagram:%s", info.GetUsername()))
	}

	name := info.GetName()
	if name == "" {
		resp, err := m.Client.ExecuteTasks(&socket.GetContactsFullTask{
			ContactID: info.GetFBID(),
		})
		log := m.UserLogin.Log
		if err != nil {
			log.Trace().Any("ContactID", info.GetFBID()).Any("err", err).Msg("GetContactsFullTask failed")
		}
		if len(resp.LSDeleteThenInsertContact) > 0 {
			name = resp.LSDeleteThenInsertContact[0].Name
		}
	}

	return &bridgev2.UserInfo{
		Identifiers: identifiers,
		Name: ptr.Ptr(m.Main.Config.FormatDisplayname(DisplaynameParams{
			DisplayName: name,
			Username:    info.GetUsername(),
			ID:          info.GetFBID(),
		})),
		Avatar: wrapAvatar(info.GetAvatarURL()),
		IsBot:  nil, // TODO
		ExtraUpdates: func(ctx context.Context, ghost *bridgev2.Ghost) (changed bool) {
			meta := ghost.Metadata.(*metaid.GhostMetadata)
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
			return msgconv.DownloadAvatar(ctx, avatarURL)
		},
	}
}
