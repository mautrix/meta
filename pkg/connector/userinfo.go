package connector

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

func (m *MetaClient) getFBIDForIGUser(ctx context.Context, igid string) (int64, error) {
	if fbid := m.igUserIDs[igid]; fbid != 0 {
		return fbid, nil
	}
	fbid, err := m.Main.DB.GetFBIDForIGUser(ctx, igid)
	if err != nil {
		return 0, err
	}
	m.igUserIDs[igid] = fbid
	m.igUserIDsReverse[fbid] = igid
	return fbid, nil
}

func (m *MetaClient) getIGUserForFBID(ctx context.Context, fbid int64) (string, error) {
	if igid := m.igUserIDsReverse[fbid]; igid != "" {
		return igid, nil
	}
	igid, err := m.Main.DB.GetIGUserForFBID(ctx, fbid)
	if err != nil {
		return "", err
	}
	m.igUserIDs[igid] = fbid
	m.igUserIDsReverse[fbid] = igid
	return igid, nil
}

func (m *MetaClient) getFBIDForIGThread(ctx context.Context, igid string) (int64, error) {
	if fbid := m.igThreadIDs[igid]; fbid != 0 {
		return fbid, nil
	}
	fbid, err := m.Main.DB.GetFBIDForIGThread(ctx, igid)
	if err != nil {
		return 0, err
	}
	m.igThreadIDs[igid] = fbid
	return fbid, nil
}

func (m *MetaClient) putFBIDForIGUser(ctx context.Context, igid string, fbid int64) error {
	if m.igUserIDs[igid] == fbid {
		return nil // already saved
	}
	m.igUserIDs[igid] = fbid
	m.igUserIDsReverse[fbid] = igid
	return m.Main.DB.PutFBIDForIGUser(ctx, igid, fbid)
}

func (m *MetaClient) putFBIDForIGThread(ctx context.Context, igid string, fbid int64) error {
	if m.igThreadIDs[igid] == fbid {
		return nil // already saved
	}
	m.igThreadIDs[igid] = fbid
	return m.Main.DB.PutFBIDForIGThread(ctx, igid, fbid)
}

func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	if ghost.Name == "" {
		contactID := metaid.ParseUserID(ghost.ID)
		resp, err := m.Client.ExecuteTasks(ctx, &socket.GetContactsFullTask{
			ContactID: contactID,
		})
		if err != nil {
			return nil, err
		}
		for _, info := range resp.LSDeleteThenInsertIGContactInfo {
			err := m.putFBIDForIGUser(ctx, info.IgId, info.ContactId)
			if err != nil {
				zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to save FBID for IG user")
			}
		}
		if len(resp.LSDeleteThenInsertContact) > 0 {
			return m.wrapUserInfo(resp.LSDeleteThenInsertContact[0]), nil
		} else {
			return nil, fmt.Errorf("user info not found via GetContactsFullTask")
		}
	}
	return nil, nil
}

const (
	MetaAIInstagramID = 656175869434325
	MetaAIMessengerID = 156025504001094
)

func (m *MetaClient) wrapUserInfo(info types.UserInfo) *bridgev2.UserInfo {
	var identifiers []string
	if m.LoginMeta.Platform == types.Instagram {
		identifiers = append(identifiers, fmt.Sprintf("instagram:%s", info.GetUsername()))
	}

	return &bridgev2.UserInfo{
		Identifiers: identifiers,
		Name: ptr.Ptr(m.Main.Config.FormatDisplayname(DisplaynameParams{
			DisplayName: info.GetName(),
			Username:    info.GetUsername(),
			ID:          info.GetFBID(),
		})),
		Avatar: wrapAvatar(info.GetAvatarURL()),
		IsBot:  ptr.Ptr(info.GetFBID() == MetaAIInstagramID || info.GetFBID() == MetaAIMessengerID), // TODO do this in a less hardcoded way?
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
