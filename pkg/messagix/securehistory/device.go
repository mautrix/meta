package securehistory

import (
	"context"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
)

type EncryptedSecrets struct {
	EpochAnonID              []byte `json:"encrypted_epoch_anon_id"`
	EpochRootKey             []byte `json:"encrypted_epoch_root_key"`
	EpochStoragePrivateKey   []byte `json:"encrypted_epoch_storage_private_key"`
	MailboxRootKey           []byte `json:"encrypted_mailbox_root_key_blob"`
	ObliviousValidationToken []byte `json:"encrypted_oblivious_validation_token_blob"`
	OCMFClientState          []byte `json:"encrypted_ocmf_client_state"`
	ORFClientState           []byte `json:"encrypted_orf_client_state_v2"`
}

type MinosEpoch struct {
	Head          []byte `json:"epoch_head"`
	SelfSignature []byte `json:"self_epoch_signature"`
}

type SyncEpoch struct {
	ID            string `json:"epoch_id"`
	BackupID      string `json:"backup_id"`
	AnonID        string `json:"epoch_anon_id"`
	AuthPublicKey []byte `json:"epoch_auth_public_key"`
	EncryptedKey  []byte `json:"encrypted_epoch_key"`
	Active        bool   `json:"is_active_epoch"`
	NextID        string `json:"next_epoch_id"`
	PreviousID    string `json:"prev_epoch_id"`
	EncryptedData *struct {
		AnonID  []byte `json:"epoch_anon_id"`
		RootKey []byte `json:"epoch_root_key"`
	} `json:"epoch_data_v2"`
	Minos *MinosEpoch `json:"minos_epoch"`
}

type VirtualDeviceInfo struct {
	Secrets               *EncryptedSecrets `json:"encrypted_secret_values"`
	EpochID               string            `json:"epoch_id"`
	EpochStoragePublicKey []byte            `json:"epoch_storage_public_key"`
	LastEpochID           string            `json:"last_epoch_id"`
	Epochs                []SyncEpoch       `json:"sync_epochs"`
	Minos                 *MinosEpoch       `json:"minos_epoch"`
	Edges                 []struct {
		From struct {
			ID string `json:"epoch_id"`
		} `json:"from_epoch"`
		BackwardEdge []byte `json:"backward_edge"`
	} `json:"epoch_edges"`
}

type VirtualDevice struct {
	ID      []byte
	BlobKey []byte
	Info    *VirtualDeviceInfo
}

type virtualDeviceResponse struct {
	OK    bool `json:"ok"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
	Data *VirtualDeviceInfo `json:"data"`
}

type Epoch struct {
	ID      string `json:"id"`
	AnonID  []byte `json:"anon_id"`
	RootKey []byte `json:"root_key"`
}

type Device struct {
	ID                string           `json:"id"`
	BackupID          string           `json:"backup_id"`
	StoragePublicKey  []byte           `json:"storage_public_key"`
	StoragePrivateKey []byte           `json:"storage_private_key"`
	MailboxRootKey    []byte           `json:"mailbox_root_key"`
	OCMFClientState   []byte           `json:"ocmf_client_state"`
	Epochs            map[string]Epoch `json:"epochs"`
}

func (d *VirtualDevice) decryptSecret(ciphertext []byte, name string) ([]byte, error) {
	for _, aad := range []string{"virtual_device:" + name, "secret_values_metadata"} {
		value, err := decryptData(d.BlobKey, ciphertext, []byte(aad))
		if err != nil {
			continue
		}
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(value)))
		n, err := base64.StdEncoding.Decode(decoded, value)
		if err == nil {
			return decoded[:n], nil
		}
	}
	return nil, fmt.Errorf("failed to decrypt Messenger backup %s", name)
}

func validEpochAnonID(id []byte) bool {
	if len(id) == 0 || len(id) > 11 {
		return false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (c *Client) GetVirtualDevice(ctx context.Context, backup *Backup, userID int64, code []byte) (*VirtualDevice, error) {
	if backup == nil || backup.ID == "" {
		return nil, ErrNoPINBackup
	} else if userID <= 0 && backup.DelegatorID == "" {
		return nil, errors.New("invalid Messenger recovery account")
	}
	ownerID := backup.DelegatorID
	if ownerID == "" {
		ownerID = strconv.FormatInt(userID, 10)
	}
	id, key, err := deriveVirtualDevice(code, ownerID)
	if err != nil {
		return nil, err
	}
	response, err := request[virtualDeviceResponse](ctx, c.HTTP, "EBAddDeviceGraphQLEBFetchVirtualDeviceInfoQuery", "fetch_virtual_device_info_for_device_addition_v2", map[string]any{
		"request_uuid":      ownerID,
		"virtual_device_id": base64.StdEncoding.EncodeToString(id),
	})
	if err != nil {
		return nil, err
	} else if response == nil || !response.OK || response.Data == nil {
		if response != nil && response.Error != nil {
			return nil, fmt.Errorf("backup device query failed: %s", response.Error.Code)
		}
		return nil, errors.New("backup device query failed")
	}
	return &VirtualDevice{ID: id, BlobKey: key, Info: response.Data}, nil
}

func deriveVirtualDevice(code []byte, userID string) ([]byte, []byte, error) {
	if len(code) != 40 || (code[0] != '1' && code[0] != '2') || userID == "" {
		return nil, nil, errors.New("invalid Messenger recovery code or account")
	}
	secretLen, idLen := 34, 16
	if code[0] == '1' {
		secretLen, idLen = 35, 8
	}
	info := fmt.Sprintf("BackupRecoveryCode_v%c_%c_%s", code[0], code[1], userID)
	derived, err := hkdf.Key(sha256.New, code[2:2+secretLen], nil, info, idLen+32)
	if err != nil {
		return nil, nil, err
	}
	return derived[:idLen:idLen], derived[idLen:], nil
}
