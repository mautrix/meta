package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
)

func generateAESGCMKey(keySize int) ([]byte, cipher.AEAD, error) {
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, nil, ErrRandomReadFailed
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, ErrAESCreation
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, ErrGCMCreation
	}

	return key, gcm, nil
}

func encryptAESGCM(aead cipher.AEAD, data, aad []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize()) // has to be zeroed out nonce
	ciphertext := aead.Seal(nil, nonce, data, aad)
	return ciphertext, nil
}
