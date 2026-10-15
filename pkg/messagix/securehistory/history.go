package securehistory

import (
	"bytes"
	"context"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
)

type EpochRef struct {
	ID     string `json:"epoch_id"`
	AnonID string `json:"epoch_anon_id"`
}

type EpochForwardEdge struct {
	AuthPublicKey      []byte `json:"auth_public_key"`
	EncryptedEntropy   []byte `json:"encrypted_entropy"`
	EntropyFingerprint string `json:"entropy_fingerprint"`
	StoragePublicKey   []byte `json:"epoch_storage_public_key"`
	PSKFingerprint     string `json:"psk_fingerprint"`
}

type EpochEdge struct {
	From            EpochRef          `json:"from_epoch"`
	To              EpochRef          `json:"to_epoch"`
	Backward        []byte            `json:"backward_edge"`
	Forward         *EpochForwardEdge `json:"forward_edge"`
	FromFingerprint string            `json:"from_epoch_fingerprint"`
	ToFingerprint   string            `json:"to_epoch_fingerprint"`
}

type EpochDerivationSet struct {
	Edges []EpochEdge `json:"epoch_edges"`
}

type MessageRangeInfo struct {
	HasMoreBefore *bool `json:"has_more_before"`
	HasMoreAfter  *bool `json:"has_more_after"`
	Before        int64 `json:"next_message_timestamp_ms_before,string"`
	After         int64 `json:"next_message_timestamp_ms_after,string"`
}

type MessageRange struct {
	BackupID      string              `json:"backup_id"`
	Messages      []*ArchiveMessage   `json:"encrypted_messages"`
	Range         *MessageRangeInfo   `json:"message_range_info"`
	Epochs        *EpochDerivationSet `json:"epoch_derivation_set"`
	MinosKeys     []*MinosKeyEntry    `json:"minos_decryption_keys"`
	Exception     *string             `json:"exception_string"`
	ThreadMissing bool                `json:"thread_not_found"`
	DeleteMailbox bool                `json:"should_delete_mailbox"`
}

type messageRangeResponse struct {
	Backup *struct {
		Mailbox *struct {
			Ranges []*MessageRange `json:"messages_from_selected_threads"`
		} `json:"mailbox"`
	} `json:"encrypted_backup"`
}

type ArchiveAttachment struct {
	URL      string `json:"attachment_cdn_url"`
	ObjectID string `json:"attachment_object_id"`
}

type ArchiveProtobuf struct {
	Encrypted         []byte `json:"encrypted_protobuf_stanza"`
	EncryptionVersion string `json:"encryption_version"`
	EpochID           string `json:"epoch_id"`
	EpochAnonID       string `json:"epoch_anon_id"`
	TimestampMS       int64  `json:"protobuf_timestamp_ms,string"`
	SupplementalKey   string `json:"supplemental_key"`
	SupplementalID    string `json:"supplemental_otid"`
	KeyCiphertext     []byte `json:"sk_ciphertext"`
}

type MinosProtobuf struct {
	ActorToken              string          `json:"actor_token"`
	Encrypted               []byte          `json:"encrypted_protobuf"`
	MEKFBID                 string          `json:"mek_fbid"`
	MEKID                   []byte          `json:"mek_id"`
	Timestamp               int64           `json:"protobuf_timestamp,string"`
	TransportSigningPK      []byte          `json:"transport_sender_signing_pk"`
	Signature               []byte          `json:"transport_sender_message_signature"`
	EncryptionVersion       int             `json:"message_encryption_version"`
	MetadataVersion         int             `json:"message_metadata_version"`
	ServerOriginContentType json.RawMessage `json:"server_origin_content_type"`
	SupplementalKey         string          `json:"supplemental_key"`
	SupplementalID          string          `json:"supplemental_otid"`
}

type MinosKeyEntry struct {
	ID   string        `json:"id"`
	Keys *MinosKeyData `json:"keys"`
}

type MinosKeyData struct {
	MEKInfo       *MinosMEKInfo       `json:"mek_info"`
	RecipientInfo *MinosRecipientInfo `json:"recipient_info"`
	CreatorInfo   *MinosCreatorInfo   `json:"mek_creator_info"`
}

type MinosMEKInfo struct {
	ID                []byte `json:"mek_id"`
	RosterHash        []byte `json:"roster_hash"`
	Encrypted         []byte `json:"encrypted_mek"`
	CreationTimestamp int64  `json:"mek_creation_timestamp,string"`
	EncryptionVersion int    `json:"mek_encryption_version"`
}

type MinosRecipientInfo struct {
	EpochID   string `json:"epoch_fbid"`
	EpochAnon []byte `json:"epoch_anon_id"`
	EpochHead []byte `json:"epoch_head"`
}

type MinosCreatorInfo struct {
	Minos *struct {
		AuthPublicKey []byte `json:"sender_auth_pk"`
		EpochHead     []byte `json:"sender_epoch_head"`
	} `json:"minos_keys"`
	Transport *struct {
		EphemeralPublicKey []byte `json:"ephm_hpke_pk"`
		Signature          []byte `json:"ephm_signature"`
		SigningPublicKey   []byte `json:"mek_creator_transport_signing_pk"`
	} `json:"transport_keys"`
	Mandrake json.RawMessage `json:"mandrake_keys"`
}

type EchoDocument struct {
	EchoDocumentString []byte          `json:"echo_document_string"`
	EncryptionVersion  string          `json:"encryption_version"`
	EpochAnonID        string          `json:"epoch_anon_id"`
	EpochID            *string         `json:"epoch_id"`
	EpochFingerprint   json.RawMessage `json:"epoch_fingerprint"`
}

type ArchiveMessage struct {
	ID          string              `json:"otid"`
	Attachments []ArchiveAttachment `json:"attachment_data"`
	Echo        *EchoDocument       `json:"echo_document"`
	Protobufs   *ArchiveProtobufs   `json:"protobuf_stanzas"`
}

type ArchiveProtobufs struct {
	Message       *ArchiveProtobuf   `json:"top_level_protobuf"`
	Supplementals []*ArchiveProtobuf `json:"supplemental_protobufs"`
	Unencrypted   *struct {
		Message     []byte `json:"unencrypted_protobuf"`
		TimestampMS int64  `json:"protobuf_timestamp_ms,string"`
	} `json:"top_level_protobuf_unencrypted"`
	MinosMessage       *MinosProtobuf   `json:"top_level_protobuf_v2"`
	MinosSupplementals []*MinosProtobuf `json:"supplemental_protobufs_v2"`
	Tags               []string         `json:"message_tags"`
}

type DecryptedProtobuf struct {
	Message     *waArmadilloBackupMessage.BackupMessage
	TimestampMS int64
}

type DecryptedMessage struct {
	ID            string
	Message       *DecryptedProtobuf
	Supplementals map[string]*DecryptedProtobuf
	Attachments   []ArchiveAttachment
	Tags          []string
}

func (c *Client) FetchMessages(ctx context.Context, device *Device, threadID string, before bool, timestampMS int64, count int) (*MessageRange, error) {
	if device == nil || device.ID == "" || device.BackupID == "" || len(device.MailboxRootKey) == 0 || len(device.OCMFClientState) == 0 || c.AppID == "" {
		return nil, errors.New("archive device is not enrolled")
	}
	threadKey, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil || threadKey <= 0 || timestampMS < 0 || timestampMS > 1<<48-1 || count <= 0 {
		return nil, errors.New("invalid Messenger archive range")
	}
	epochs := make([]string, 0, len(device.Epochs))
	for id := range device.Epochs {
		epochs = append(epochs, id)
	}
	slices.Sort(epochs)
	direction := 2
	if before {
		direction = 1
	}
	payload, err := json.Marshal(map[string]any{
		"restore_context": map[string]any{
			"act_thread_id":      threadID,
			"site":               "www",
			"source":             -1,
			"tam_thread_subtype": 0,
		},
		"success": map[string]any{
			"device_context": map[string]any{
				"device_id":                device.ID,
				"locally_available_epochs": epochs,
				"raw_tokens": map[string]any{
					"mailbox_root_key":       device.MailboxRootKey,
					"ocmf_client_state_blob": device.OCMFClientState,
				},
			},
			"direction":           direction,
			"query_num_messages":  count,
			"reference_timestamp": strconv.FormatInt(timestampMS, 10),
			"server_thread_key":   threadKey,
			"source":              -1,
		},
	})
	if err != nil {
		return nil, err
	}
	response, err := request[messageRangeResponse](ctx, c.HTTP, "EBMessageRangeQueryForThreadsQuery", "viewer", map[string]any{
		"app_id":                  c.AppID,
		"includeAttachmentData":   true,
		"restore_payload_strings": []string{string(payload)},
		"restore_type":            "RANGE_QUERY_RESTORE",
	})
	if err != nil {
		return nil, err
	} else if response == nil || response.Backup == nil || response.Backup.Mailbox == nil {
		return nil, errors.New("missing Messenger archive range response")
	} else if len(response.Backup.Mailbox.Ranges) != 1 || response.Backup.Mailbox.Ranges[0] == nil {
		return nil, errors.New("missing Messenger archive range response")
	}
	result := response.Backup.Mailbox.Ranges[0]
	if result.DeleteMailbox {
		return nil, errors.New("archive is no longer available")
	} else if result.Exception != nil {
		if result.ThreadMissing {
			return nil, errors.New("archive thread was not found")
		}
		return nil, errors.New("archive server rejected the range request")
	} else if result.BackupID != "" && result.BackupID != device.BackupID {
		return nil, errors.New("archive response belongs to a different backup")
	} else if result.Range == nil || result.Range.HasMoreBefore == nil || result.Range.HasMoreAfter == nil {
		return nil, errors.New("missing Messenger archive pagination")
	}
	return result, nil
}

func archiveEncryptionVersion(version string) (int64, error) {
	switch version {
	case "", "UNKNOWN":
		return 0, nil
	case "ENCRYPTION_KDF_WITH_VERSION":
		return 2, nil
	case "ENCRYPTION_KDF_WITH_NULL_SALT":
		return 3, nil
	case "ENCRYPTION_KDF_WITH_NULL_SALT_DF_CANARY":
		return 4, nil
	default:
		return 0, fmt.Errorf("unsupported Messenger archive encryption version %q", version)
	}
}

func (p *ArchiveProtobuf) decrypt(device *Device, threadID string) (*DecryptedProtobuf, string, error) {
	epoch, ok := device.Epochs[p.EpochID]
	if !ok || string(epoch.AnonID) != p.EpochAnonID {
		return nil, "", errors.New("missing Messenger archive message epoch")
	}
	version, err := archiveEncryptionVersion(p.EncryptionVersion)
	if err != nil {
		return nil, "", err
	}
	message, err := DecryptArchiveMessage(epoch.RootKey, p.EpochAnonID, threadID, version, p.Encrypted)
	if err != nil {
		return nil, "", err
	}
	var supplementalKey string
	if len(p.KeyCiphertext) > 0 {
		key, err := hkdf.Key(sha256.New, epoch.RootKey, make([]byte, 10), "kdf_info:mailbox_root_key_v2>supplemental_enc_key", 32)
		if err != nil {
			return nil, "", err
		}
		decrypted, err := decryptData(key, p.KeyCiphertext, []byte("supplemental_enc_aad"))
		if err != nil {
			return nil, "", fmt.Errorf("failed to decrypt Messenger archive supplemental key: %w", err)
		}
		supplementalKey = string(decrypted)
	}
	return &DecryptedProtobuf{Message: message, TimestampMS: p.TimestampMS}, supplementalKey, nil
}

func (r *MessageRange) Decrypt(device *Device, threadID string, selfUserID int64, isGroup bool) ([]*DecryptedMessage, error) {
	keys := make(map[string]*MinosKeyEntry, len(r.MinosKeys))
	for _, key := range r.MinosKeys {
		if key != nil {
			keys[key.ID] = key
		}
	}
	decryptedKeys := make(map[string][]byte)
	minosThreadID := threadID
	if !isGroup {
		self := strconv.FormatInt(selfUserID, 10)
		if self < threadID {
			minosThreadID = self + ":" + threadID
		} else {
			minosThreadID = threadID + ":" + self
		}
	}
	decryptMinos := func(stanza *MinosProtobuf, messageID string) (*DecryptedProtobuf, error) {
		if stanza == nil || stanza.ActorToken == "" || messageID == "" || threadID == "" || (!isGroup && selfUserID <= 0) {
			return nil, errors.New("missing Messenger Minos message metadata")
		}
		if origin := bytes.TrimSpace(stanza.ServerOriginContentType); len(origin) > 0 && !bytes.Equal(origin, []byte("null")) {
			return nil, errors.New("unsupported Messenger Minos server-origin message")
		}
		timestamp := stanza.Timestamp
		if timestamp < 0 {
			return nil, errors.New("invalid Messenger Minos message timestamp")
		}
		authenticatedTimestamp := uint64(timestamp)
		switch stanza.MetadataVersion {
		case 1:
		case 2:
			otid, err := strconv.ParseUint(messageID, 10, 64)
			if err != nil {
				return nil, errors.New("invalid Messenger Minos message ID")
			}
			authenticatedTimestamp = otid >> 22
		default:
			return nil, errors.New("unsupported Messenger Minos metadata version")
		}
		key := keys[stanza.MEKFBID]
		if key == nil || key.Keys == nil || key.Keys.MEKInfo == nil || !bytes.Equal(stanza.MEKID, key.Keys.MEKInfo.ID) {
			return nil, errors.New("missing or mismatched Messenger Minos message key")
		}
		mek, ok := decryptedKeys[stanza.MEKFBID]
		if !ok {
			var err error
			mek, err = decryptMinosMEK(device, key, stanza.TransportSigningPK)
			if err != nil {
				return nil, err
			}
			decryptedKeys[stanza.MEKFBID] = mek
		}
		plaintext, err := decryptMinosMessage(minosMessageMetadata{
			MEKID: stanza.MEKID, MessageID: messageID, ThreadID: []byte(minosThreadID), Timestamp: authenticatedTimestamp,
		}, stanza, mek)
		if err != nil {
			return nil, err
		}
		var message waArmadilloBackupMessage.BackupMessage
		err = proto.Unmarshal(plaintext, &message)
		if err != nil {
			return nil, fmt.Errorf("invalid Messenger Minos message: %w", err)
		}
		metadata := message.GetMetadata()
		if metadata == nil || metadata.TimestampMS == nil || metadata.GetSenderID() != stanza.ActorToken || metadata.GetTimestampMS() != timestamp {
			return nil, errors.New("mismatched Messenger Minos sender or timestamp")
		}
		return &DecryptedProtobuf{Message: &message, TimestampMS: timestamp}, nil
	}
	messages := make([]*DecryptedMessage, 0, len(r.Messages))
	for _, archived := range r.Messages {
		if archived != nil && archived.Protobufs != nil && slices.Contains(archived.Protobufs.Tags, "EB_VISIBILITY_METADATA") {
			continue
		}
		message, err := archived.decrypt(device, threadID, decryptMinos)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (m *ArchiveMessage) decrypt(device *Device, threadID string, decryptMinos func(*MinosProtobuf, string) (*DecryptedProtobuf, error)) (*DecryptedMessage, error) {
	if m == nil || device == nil || m.ID == "" || (m.Protobufs == nil && m.Echo == nil) {
		return nil, errors.New("missing or unsupported Messenger archive message")
	}
	if m.Echo != nil && (m.Protobufs == nil || (m.Protobufs.Message == nil && m.Protobufs.Unencrypted == nil && m.Protobufs.MinosMessage == nil)) {
		return m.decryptEcho(device, threadID)
	}
	result := &DecryptedMessage{
		ID:            m.ID,
		Attachments:   m.Attachments,
		Tags:          m.Protobufs.Tags,
		Supplementals: make(map[string]*DecryptedProtobuf),
	}
	var err error
	if m.Protobufs.MinosMessage != nil {
		result.Message, err = decryptMinos(m.Protobufs.MinosMessage, m.ID)
	} else if unencrypted := m.Protobufs.Unencrypted; unencrypted != nil {
		var message waArmadilloBackupMessage.BackupMessage
		if err = proto.Unmarshal(unencrypted.Message, &message); err != nil {
			return nil, fmt.Errorf("invalid unencrypted Messenger archive message: %w", err)
		}
		result.Message = &DecryptedProtobuf{Message: &message, TimestampMS: unencrypted.TimestampMS}
	} else if m.Protobufs.Message == nil {
		return nil, errors.New("missing Messenger archive message payload")
	} else {
		result.Message, _, err = m.Protobufs.Message.decrypt(device, threadID)
	}
	if err != nil {
		return nil, err
	}
	for _, supplemental := range m.Protobufs.Supplementals {
		if supplemental == nil {
			return nil, errors.New("missing Messenger archive supplemental")
		}
		message, key, err := supplemental.decrypt(device, threadID)
		if err != nil {
			return nil, err
		} else if key == "" {
			return nil, errors.New("missing Messenger archive supplemental key")
		}
		result.Supplementals[key] = message
	}
	for _, supplemental := range m.Protobufs.MinosSupplementals {
		if supplemental == nil || supplemental.SupplementalKey == "" || supplemental.SupplementalID == "" {
			return nil, errors.New("missing Messenger Minos supplemental metadata")
		}
		message, err := decryptMinos(supplemental, supplemental.SupplementalID)
		if err != nil {
			return nil, err
		}
		result.Supplementals[supplemental.SupplementalKey] = message
	}
	return result, nil
}
