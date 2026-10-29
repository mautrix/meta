package securehistory

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupCommon"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"google.golang.org/protobuf/proto"
)

const (
	minosBackupActionUpsert = 1
	minosPayloadVersion     = 2
)

type minosContactKeys struct {
	Typename string `json:"__typename"`
	Contacts []struct {
		ContactID string `json:"contact_id"`
		Keys      *struct {
			EncryptionPK string `json:"enc_pk_b64"`
			EpochHead    string `json:"epoch_head_b64"`
			FBID         string `json:"fbid"`
		} `json:"mailbox_public_keys"`
	} `json:"contact_id_to_minos_params"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (c *Client) FetchMinosRecipients(ctx context.Context, userIDs ...int64) ([]MinosRecipient, error) {
	contactIDs := make([]string, len(userIDs))
	for i, id := range userIDs {
		contactIDs[i] = strconv.FormatInt(id, 10)
	}
	resp, err := request[minosContactKeys](ctx, c.HTTP, "EBMinosFetchContactKeysQuery", "xfb_minos_fetch_mailbox_public_keys", map[string]any{
		"input": map[string]any{
			"contact_ids":  contactIDs,
			"request_uuid": uuid.NewString(),
			"use_case":     "SPECIFIC_CONTACTS",
		},
	})
	if err != nil {
		return nil, err
	} else if resp == nil {
		return nil, errors.New("empty Minos contact key response")
	} else if resp.Code != "" || resp.Message != "" {
		return nil, fmt.Errorf("failed to fetch Minos contact keys: %s %s", resp.Code, resp.Message)
	}
	recipients := make([]MinosRecipient, 0, len(resp.Contacts))
	for _, contact := range resp.Contacts {
		if contact.Keys == nil || !slices.Contains(contactIDs, contact.ContactID) {
			continue
		}
		encryptionPK, err := base64.StdEncoding.DecodeString(contact.Keys.EncryptionPK)
		if err != nil {
			return nil, fmt.Errorf("invalid Minos encryption key for %s: %w", contact.ContactID, err)
		}
		epochHead, err := base64.StdEncoding.DecodeString(contact.Keys.EpochHead)
		if err != nil {
			return nil, fmt.Errorf("invalid Minos epoch head for %s: %w", contact.ContactID, err)
		}
		recipients = append(recipients, MinosRecipient{
			ParticipantID:  contact.ContactID,
			MailboxKeyFBID: contact.Keys.FBID,
			EpochHead:      epochHead,
			EncryptionPK:   encryptionPK,
		})
	}
	return recipients, nil
}

type MinosSentMessage struct {
	BackupID       string
	SenderID       int64
	OtherUserID    int64
	MessageID      string
	TimestampMS    int64
	TransportBytes []byte
	FrankingTag    []byte
	IdentityPriv   *[32]byte
	IdentityPub    *[32]byte
}

type minosIdentifiers struct {
	ActThreadID string `json:"act_thread_id"`
	BackupID    string `json:"backup_id"`
	OTID        string `json:"otid"`
	Timestamp   int64  `json:"timestamp"`
}

type minosTaskMetadata struct {
	EnqueueTimestampMS       int64  `json:"enqueue_timestamp_ms"`
	FirstExecutedTimestampMS int64  `json:"first_executed_timestamp_ms"`
	QueueName                string `json:"queue_name"`
	TaskID                   int    `json:"task_id"`
}

type minosUploadPayload struct {
	Context struct {
		Identifiers  minosIdentifiers  `json:"identifiers_v2"`
		TaskMetadata minosTaskMetadata `json:"task_metadata"`
	} `json:"context"`
	Failure *string `json:"failure"`
	Success string  `json:"success"`
}

type minosRawIdentifiers struct {
	OTID       string `json:"otid"`
	Timestamp  int64  `json:"timestamp"`
	WAThreadID string `json:"wa_thread_id"`
}

type minosMEKEnvelopePayload struct {
	EncryptedMEK   []byte `json:"encrypted_mek_base64"`
	MailboxKeyFBID string `json:"mailbox_key_fbid"`
	ParticipantID  string `json:"participant_id"`
}

type minosMEKRegistration struct {
	ActThreadID          string                    `json:"act_thread_id"`
	Envelopes            []minosMEKEnvelopePayload `json:"envelopes"`
	MEKEncryptionVersion int                       `json:"mek_encryption_version"`
	MEKID                []byte                    `json:"mek_id_base64"`
	RosterHash           []byte                    `json:"mek_roster_base64"`
	EphemeralPK          []byte                    `json:"sender_ephemeral_hpke_pub_key_base64"`
	EphemeralSignature   []byte                    `json:"sender_ephemeral_key_signature_base64"`
	TransportSigningPK   []byte                    `json:"sender_transport_signing_pub_key_base64"`
}

type minosUploadSuccess struct {
	EncryptedProtobuf    []byte                `json:"encrypted_protobuf"`
	IsEphemeral          bool                  `json:"is_ephemeral"`
	KeyValueBackupAction int                   `json:"key_value_backup_action"`
	LastEpochID          *string               `json:"last_epoch_id"`
	MEKRegistration      *minosMEKRegistration `json:"mek_registration_payload"`
	MessageEncryptionVer int                   `json:"message_encryption_version"`
	MessageTags          string                `json:"message_tags"`
	RawIdentifiers       minosRawIdentifiers   `json:"raw_identifiers"`
	RequestID            string                `json:"request_id"`
	ValueSecretReference string                `json:"value_secret_ref"`
}

type minosValueSecretReference struct {
	FrankingTag              []byte `json:"franking_tag"`
	MEKFBID                  string `json:"mek_fbid"`
	MEKID                    []byte `json:"mek_id"`
	MessageMetadataVersion   int    `json:"message_metadata_version"`
	ReportingTag             []byte `json:"reporting_tag"`
	TransportSenderSignature []byte `json:"transport_sender_message_signature"`
	TransportSenderSigningPK []byte `json:"transport_sender_signing_pk"`
}

type minosUploadResponse struct {
	Labyrinth *struct {
		Success                bool `json:"is_success"`
		MEKRegistrationResults []struct {
			MEKID     string `json:"mek_id_base64"`
			MEKFBID   string `json:"mek_fbid"`
			Success   bool   `json:"is_success"`
			ErrorCode any    `json:"error_code"`
		} `json:"mek_registration_results"`
	} `json:"labyrinth_1_1"`
}

func minosDMThreadID(a, b int64) string {
	ids := []string{strconv.FormatInt(a, 10), strconv.FormatInt(b, 10)}
	slices.Sort(ids)
	return strings.Join(ids, ":")
}

func marshalJSONString(v any) (string, error) {
	data, err := json.Marshal(v)
	return string(data), err
}

func (c *Client) UploadSentMessage(ctx context.Context, msg *MinosSentMessage) error {
	if msg.BackupID == "" || msg.SenderID <= 0 || msg.OtherUserID <= 0 || msg.MessageID == "" || msg.IdentityPriv == nil || msg.IdentityPub == nil || c.AppID == "" {
		return errors.New("missing Messenger history upload parameters")
	}
	otid, err := strconv.ParseUint(msg.MessageID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid Messenger message ID: %w", err)
	}
	recipients, err := c.FetchMinosRecipients(ctx, msg.SenderID, msg.OtherUserID)
	if err != nil {
		return err
	} else if len(recipients) == 0 {
		return errors.New("no Minos recipients found")
	}
	mek, err := GenerateMinosMEK(recipients)
	if err != nil {
		return err
	}
	distribution, err := DistributeMinosMEKFromTransport(mek, recipients, msg.IdentityPriv, msg.IdentityPub)
	if err != nil {
		return err
	}
	senderID := strconv.FormatInt(msg.SenderID, 10)
	plaintext, err := proto.Marshal(&waArmadilloBackupMessage.BackupMessage{
		Metadata: &waArmadilloBackupCommon.Metadata{
			SenderID:    proto.String(senderID),
			MessageID:   proto.String(msg.MessageID),
			TimestampMS: proto.Int64(msg.TimestampMS),
			FrankingMetadata: &waArmadilloBackupCommon.FrankingMetadata{
				FrankingTag: msg.FrankingTag,
			},
			PayloadVersion: proto.Int32(minosPayloadVersion),
		},
		Payload: &waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage{
			EncryptedTransportMessage: msg.TransportBytes,
		},
	})
	if err != nil {
		return err
	}
	threadID := strconv.FormatInt(msg.OtherUserID, 10)
	ciphertext, signature, err := EncryptMinosMessage(mek, msg.MessageID, minosDMThreadID(msg.SenderID, msg.OtherUserID), otid>>22, msg.IdentityPriv, msg.IdentityPub, plaintext)
	if err != nil {
		return err
	}
	envelopes := make([]minosMEKEnvelopePayload, len(distribution.Envelopes))
	for i, envelope := range distribution.Envelopes {
		envelopes[i] = minosMEKEnvelopePayload{
			EncryptedMEK:   envelope.EncryptedMEK,
			MailboxKeyFBID: envelope.MailboxKeyFBID,
			ParticipantID:  envelope.ParticipantID,
		}
	}
	valueSecretRef, err := marshalJSONString(&minosValueSecretReference{
		FrankingTag:              msg.FrankingTag,
		MEKFBID:                  "0",
		MEKID:                    mek.ID,
		MessageMetadataVersion:   minosMetadataVersion,
		TransportSenderSignature: signature,
		TransportSenderSigningPK: distribution.TransportSigningPK,
	})
	if err != nil {
		return err
	}
	success, err := marshalJSONString(&minosUploadSuccess{
		EncryptedProtobuf:    ciphertext,
		KeyValueBackupAction: minosBackupActionUpsert,
		MEKRegistration: &minosMEKRegistration{
			ActThreadID:          threadID,
			Envelopes:            envelopes,
			MEKEncryptionVersion: minosMEKVersion,
			MEKID:                mek.ID,
			RosterHash:           mek.RosterHash,
			EphemeralPK:          distribution.EphemeralAuthPK,
			EphemeralSignature:   distribution.EphemeralSignature,
			TransportSigningPK:   distribution.TransportSigningPK,
		},
		MessageEncryptionVer: minosMessageVersion,
		MessageTags:          `{"message_tags":[]}`,
		RawIdentifiers: minosRawIdentifiers{
			OTID:       msg.MessageID,
			Timestamp:  msg.TimestampMS,
			WAThreadID: threadID,
		},
		ValueSecretReference: valueSecretRef,
	})
	if err != nil {
		return err
	}
	var payload minosUploadPayload
	payload.Context.Identifiers = minosIdentifiers{
		ActThreadID: threadID,
		BackupID:    msg.BackupID,
		OTID:        msg.MessageID,
		Timestamp:   msg.TimestampMS,
	}
	payload.Context.TaskMetadata = minosTaskMetadata{
		EnqueueTimestampMS:       msg.TimestampMS,
		FirstExecutedTimestampMS: msg.TimestampMS,
		QueueName:                "eb_upload_queue",
	}
	payload.Success = success
	encodedPayload, err := marshalJSONString(&payload)
	if err != nil {
		return err
	}
	resp, err := request[minosUploadResponse](ctx, c.HTTP, "EBMinosUploadMessagesMutation", "xfb_upload_encrypted_msg_to_backup", map[string]any{
		"input": map[string]any{
			"app_id_str":        c.AppID,
			"labyrinth_version": 1,
			"payload": map[string]any{
				"msg_payloads": []string{encodedPayload},
			},
		},
	})
	if err != nil {
		return err
	} else if resp == nil || resp.Labyrinth == nil || !resp.Labyrinth.Success {
		return errors.New("Messenger history upload was not accepted")
	}
	mekID := base64.StdEncoding.EncodeToString(mek.ID)
	for _, result := range resp.Labyrinth.MEKRegistrationResults {
		if result.MEKID == mekID {
			if !result.Success {
				return fmt.Errorf("Messenger history key registration failed: %v", result.ErrorCode)
			}
			return nil
		}
	}
	return errors.New("Messenger history upload response is missing the key registration result")
}
