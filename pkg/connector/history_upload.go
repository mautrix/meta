package connector

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	armadillo "go.mau.fi/whatsmeow/proto"
	"go.mau.fi/whatsmeow/proto/waArmadilloApplication"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	waTypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const historyUploadTimeout = 2 * time.Minute

func marshalSentFBMessage(message armadillo.RealMessageApplicationSub, metadata *waMsgApplication.MessageApplication_Metadata) ([]byte, error) {
	var subproto waMsgApplication.MessageApplication_SubProtocolPayload
	subproto.FutureProof = waCommon.FutureProofBehavior_PLACEHOLDER.Enum()
	switch typedMsg := message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		payload, err := proto.Marshal(typedMsg)
		if err != nil {
			return nil, err
		}
		subproto.SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{
			ConsumerMessage: &waCommon.SubProtocol{Payload: payload, Version: proto.Int32(whatsmeow.FBConsumerMessageVersion)},
		}
	case *waArmadilloApplication.Armadillo:
		payload, err := proto.Marshal(typedMsg)
		if err != nil {
			return nil, err
		}
		subproto.SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_Armadillo{
			Armadillo: &waCommon.SubProtocol{Payload: payload, Version: proto.Int32(whatsmeow.FBArmadilloMessageVersion)},
		}
	default:
		return nil, fmt.Errorf("unsupported message type %T", message)
	}
	return proto.Marshal(&waMsgApplication.MessageApplication{
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{SubProtocol: &subproto},
		},
		Metadata: metadata,
	})
}

func (m *MetaClient) getHistoryBackupID(ctx context.Context) (string, error) {
	m.historyUploadLock.Lock()
	defer m.historyUploadLock.Unlock()
	if device := m.historyDevice(); device != nil && device.BackupID != "" {
		return device.BackupID, nil
	} else if m.historyBackupChecked {
		return m.historyBackupID, nil
	}
	backup, err := m.Client.GetSecureHistoryClient().GetBackup(ctx)
	if errors.Is(err, securehistory.ErrNoPINBackup) {
		m.historyBackupChecked = true
		return "", nil
	} else if err != nil {
		return "", err
	}
	m.historyBackupID = backup.ID
	m.historyBackupChecked = true
	return backup.ID, nil
}

func (m *MetaClient) uploadSentMessageToHistory(
	ctx context.Context,
	chat waTypes.JID,
	messageID string,
	message armadillo.RealMessageApplicationSub,
	metadata *waMsgApplication.MessageApplication_Metadata,
) {
	if !m.LoginMeta.Platform.IsMessenger() || chat.Server != waTypes.MessengerServer || metadata == nil || len(metadata.GetFrankingKey()) == 0 {
		return
	}
	log := zerolog.Ctx(ctx).With().Str("action", "upload message to encrypted history").Logger()
	ctx = log.WithContext(context.WithoutCancel(ctx))
	go func() {
		ctx, cancel := context.WithTimeout(ctx, historyUploadTimeout)
		defer cancel()
		err := m.doUploadSentMessageToHistory(ctx, chat, messageID, message, metadata)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to upload sent message to encrypted history")
		} else {
			log.Debug().Msg("Uploaded sent message to encrypted history")
		}
	}()
}

func (m *MetaClient) doUploadSentMessageToHistory(
	ctx context.Context,
	chat waTypes.JID,
	messageID string,
	message armadillo.RealMessageApplicationSub,
	metadata *waMsgApplication.MessageApplication_Metadata,
) error {
	backupID, err := m.getHistoryBackupID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get backup ID: %w", err)
	} else if backupID == "" {
		zerolog.Ctx(ctx).Debug().Msg("User doesn't have an encrypted history backup, not uploading")
		return nil
	}
	otherUserID, err := strconv.ParseInt(chat.User, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}
	otid, err := strconv.ParseInt(messageID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid message ID: %w", err)
	}
	transportBytes, err := marshalSentFBMessage(message, metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal sent message: %w", err)
	}
	franking := hmac.New(sha256.New, metadata.GetFrankingKey())
	franking.Write(transportBytes)
	identity := m.WADevice.IdentityKey
	return m.Client.GetSecureHistoryClient().UploadSentMessage(ctx, &securehistory.MinosSentMessage{
		BackupID:       backupID,
		SenderID:       metaid.ParseUserLoginID(m.UserLogin.ID),
		OtherUserID:    otherUserID,
		MessageID:      messageID,
		TimestampMS:    otid >> 22,
		TransportBytes: transportBytes,
		FrankingTag:    franking.Sum(nil),
		IdentityPriv:   identity.Priv,
		IdentityPub:    identity.Pub,
	})
}
