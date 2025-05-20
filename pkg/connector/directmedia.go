package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/mediaproxy"

	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

var _ bridgev2.DirectMediableNetwork = (*MetaConnector)(nil)

func (mc *MetaConnector) SetUseDirectMedia() {
	mc.MsgConv.DirectMedia = true
}

func (mc *MetaConnector) Download(ctx context.Context, mediaID networkid.MediaID, params map[string]string) (mediaproxy.GetMediaResponse, error) {
	mediaInfo, err := metaid.ParseMediaID(mediaID)
	if err != nil {
		return nil, err
	}
	zerolog.Ctx(ctx).Trace().Any("mediaInfo", mediaInfo).Any("err", err).Msg("download direct media")

	msg, err := mc.Bridge.DB.Message.GetFirstPartByID(ctx, mediaInfo.UserID, mediaInfo.MessageID)
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
	case metaid.DirectMediaTypeMeta:
		var info *msgconv.DirectMediaMeta
		err = json.Unmarshal(dmm, &info)
		if err != nil {
			return nil, err
		}

		size, reader, err := msgconv.DownloadMedia(ctx, info.MimeType, info.URL, mc.MsgConv.MaxFileSize)
		if err != nil {
			return nil, err
		}

		return &mediaproxy.GetMediaResponseData{
			Reader:        reader,
			ContentType:   info.MimeType,
			ContentLength: size,
		}, nil
	case metaid.DirectMediaTypeWhatsApp:
		var info *msgconv.DirectMediaWhatsApp
		err = json.Unmarshal(dmm, &info)
		if err != nil {
			return nil, err
		}

		ul := mc.Bridge.GetCachedUserLoginByID(mediaInfo.UserID)
		if ul == nil || !ul.Client.IsLoggedIn() {
			return nil, fmt.Errorf("no logged in user found")
		}
		client := ul.Client.(*MetaClient)
		return &mediaproxy.GetMediaResponseFile{
			Callback: func(f *os.File) error {
				return client.E2EEClient.DownloadToFile(ctx, info, f)
			},
		}, nil
	}

	return nil, nil
}
