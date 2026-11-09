package securehistory

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
)

func decryptArchivePayload(epochRootKey []byte, epochAnonID, threadID string, version int64, envelope []byte) ([]byte, error) {
	if len(epochRootKey) != 32 || epochAnonID == "" || threadID == "" {
		return nil, errors.New("missing or invalid archive epoch or thread")
	}
	info := "message_key_in_epoch_" + epochAnonID
	salt := []byte(threadID)
	switch version {
	case 0:
	case 2:
		info += "_cipher_version_2"
	case 3, 4:
		info += fmt.Sprintf("_cipher_version_%d_thread_%s", version, threadID)
		salt = make([]byte, 10)
	default:
		return nil, fmt.Errorf("unsupported archive encryption version %d", version)
	}
	key, err := hkdf.Key(sha256.New, epochRootKey, salt, info, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to derive archive message key: %w", err)
	}
	plaintext, err := decryptData(key, envelope, []byte("message_thread_"+threadID))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt archive message: %w", err)
	}
	return plaintext, nil
}

func decryptData(key, ciphertext, aad []byte) ([]byte, error) {
	if len(ciphertext) < 44 {
		return nil, errors.New("ciphertext is too short")
	}
	key, err := chacha20.HChaCha20(key, ciphertext[:16])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, ciphertext[16:28], ciphertext[28:], aad)
	if err != nil {
		return nil, err
	}
	if len(plaintext) < 4 {
		return nil, errors.New("plaintext is missing its length prefix")
	}
	length := binary.BigEndian.Uint32(plaintext[:4])
	if uint64(length) > uint64(len(plaintext)-4) {
		return nil, errors.New("length prefix exceeds plaintext")
	}
	return plaintext[4 : 4+length], nil
}

func DecryptArchiveMessage(epochRootKey []byte, epochAnonID, threadID string, version int64, envelope []byte) (*waArmadilloBackupMessage.BackupMessage, error) {
	plaintext, err := decryptArchivePayload(epochRootKey, epochAnonID, threadID, version, envelope)
	if err != nil {
		return nil, err
	}
	var message waArmadilloBackupMessage.BackupMessage
	if err = proto.Unmarshal(plaintext, &message); err != nil {
		return nil, fmt.Errorf("failed to decode archive message: %w", err)
	}
	return &message, nil
}
