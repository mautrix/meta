package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/mediaproxy"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

var _ bridgev2.DirectMediableNetwork = (*MetaConnector)(nil)

func (m *MetaConnector) SetUseDirectMedia() {
	m.MsgConv.DirectMedia = true
}

func (m *MetaConnector) Download(ctx context.Context, mediaID networkid.MediaID, params map[string]string) (mediaproxy.GetMediaResponse, error) {
	mediaInfo, err := metaid.ParseMediaID(mediaID)
	if err != nil {
		return nil, err
	}
	zerolog.Ctx(ctx).Trace().Any("mediaInfo", mediaInfo).Any("err", err).Msg("download direct media")

	var msg *database.Message
	if mediaInfo.PartID == "" {
		msg, err = m.Bridge.DB.Message.GetFirstPartByID(ctx, mediaInfo.UserID, mediaInfo.MessageID)
	} else {
		msg, err = m.Bridge.DB.Message.GetPartByID(ctx, mediaInfo.UserID, mediaInfo.MessageID, mediaInfo.PartID)
	}
	if err != nil {
		return nil, nil
	} else if msg == nil {
		return nil, fmt.Errorf("message not found")
	}

	dmm := msg.Metadata.(*metaid.MessageMetadata).DirectMediaMeta
	if dmm == nil {
		return nil, fmt.Errorf("message does not have direct media metadata")
	}

	switch mediaInfo.Type {
	case metaid.DirectMediaTypeMetaV1, metaid.DirectMediaTypeMetaV2:
		var info *msgconv.DirectMediaMeta
		err = json.Unmarshal(dmm, &info)
		if err != nil {
			return nil, err
		}

		size, reader, err := msgconv.DownloadMedia(ctx, info.MimeType, info.URL, m.MsgConv.MaxFileSize)
		if err != nil {
			return nil, err
		}

		return &mediaproxy.GetMediaResponseData{
			Reader:        reader,
			ContentType:   info.MimeType,
			ContentLength: size,
		}, nil
	case metaid.DirectMediaTypeWhatsAppV1, metaid.DirectMediaTypeWhatsAppV2:
		var info *msgconv.DirectMediaWhatsApp
		err = json.Unmarshal(dmm, &info)
		if err != nil {
			return nil, err
		}

		ul := m.Bridge.GetCachedUserLoginByID(mediaInfo.UserID)
		if ul == nil || !ul.Client.IsLoggedIn() {
			return nil, fmt.Errorf("no logged in user found")
		}
		client := ul.Client.(*MetaClient)
		return &mediaproxy.GetMediaResponseFile{
			Callback: func(f *os.File) (*mediaproxy.FileMeta, error) {
				return &mediaproxy.FileMeta{}, client.E2EEClient.DownloadToFile(ctx, info, f)
			},
		}, nil
	}

	return nil, nil
}
