package connector

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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
	"go.mau.fi/whatsmeow/util/keys"
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

func (m *MetaClient) uploadSentMessageToHistory(
	ctx context.Context,
	chat waTypes.JID,
	messageID string,
	deletedID string,
	message armadillo.RealMessageApplicationSub,
	metadata *waMsgApplication.MessageApplication_Metadata,
) {
	if !m.canBackfillEncrypted() || !m.LoginMeta.Platform.IsMessenger() || chat.Server != waTypes.MessengerServer || metadata == nil || len(metadata.GetFrankingKey()) == 0 {
		return
	}
	client, historyDevice, waDevice := m.Client, m.historyDevice(), m.WADevice
	if client == nil || historyDevice == nil || waDevice == nil || waDevice.IdentityKey == nil {
		return
	}
	historyClient := client.GetSecureHistoryClient()
	if historyClient.AppID == "" {
		return
	}
	backupID := historyDevice.BackupID
	identity := waDevice.IdentityKey
	log := zerolog.Ctx(ctx).With().Str("action", "upload message to encrypted history").Logger()
	ctx = log.WithContext(context.WithoutCancel(ctx))
	go func() {
		ctx, cancel := context.WithTimeout(ctx, historyUploadTimeout)
		defer cancel()
		err := m.doUploadSentMessageToHistory(ctx, historyClient, backupID, identity, chat, messageID, deletedID, message, metadata)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to upload sent message to encrypted history")
		} else {
			log.Debug().Msg("Uploaded sent message to encrypted history")
		}
	}()
}

func (m *MetaClient) doUploadSentMessageToHistory(
	ctx context.Context,
	historyClient *securehistory.Client,
	backupID string,
	identity *keys.KeyPair,
	chat waTypes.JID,
	messageID string,
	deletedID string,
	message armadillo.RealMessageApplicationSub,
	metadata *waMsgApplication.MessageApplication_Metadata,
) error {
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
	return historyClient.UploadSentMessage(ctx, &securehistory.MinosSentMessage{
		BackupID:       backupID,
		SenderID:       metaid.ParseUserLoginID(m.UserLogin.ID),
		OtherUserID:    otherUserID,
		MessageID:      messageID,
		DeletedID:      deletedID,
		TimestampMS:    otid >> 22,
		TransportBytes: transportBytes,
		FrankingTag:    franking.Sum(nil),
		IdentityPriv:   identity.Priv,
		IdentityPub:    identity.Pub,
	})
}
