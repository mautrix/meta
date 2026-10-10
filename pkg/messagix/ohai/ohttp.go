package ohai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	requestLabel  = "message/bhttp request"
	responseLabel = "message/bhttp response"
)

type KeyConfig struct {
	KeyID     uint8
	KEMID     uint16
	KDFID     uint16
	AEADID    uint16
	PublicKey []byte
	ExpiresAt time.Time
}

type ResponseContext struct {
	sender *hpke.Sender
	enc    []byte
	aeadID uint16
}

func (kc *KeyConfig) header() []byte {
	hdr := []byte{kc.KeyID}
	hdr = binary.BigEndian.AppendUint16(hdr, kc.KEMID)
	hdr = binary.BigEndian.AppendUint16(hdr, kc.KDFID)
	return binary.BigEndian.AppendUint16(hdr, kc.AEADID)
}

func (kc *KeyConfig) Encapsulate(request []byte) ([]byte, *ResponseContext, error) {
	if kc.KDFID != hpke.HKDFSHA256().ID() {
		return nil, nil, fmt.Errorf("unsupported ohttp kdf %d", kc.KDFID)
	}
	kem, err := hpke.NewKEM(kc.KEMID)
	if err != nil {
		return nil, nil, err
	}
	aead, err := hpke.NewAEAD(kc.AEADID)
	if err != nil {
		return nil, nil, err
	}
	pub, err := kem.NewPublicKey(kc.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid ohttp public key: %w", err)
	}
	enc, sender, err := hpke.NewSender(pub, hpke.HKDFSHA256(), aead, []byte(requestLabel))
	if err != nil {
		return nil, nil, err
	}
	hdr := kc.header()
	ct, err := sender.Seal(hdr, request)
	if err != nil {
		return nil, nil, err
	}
	out := append(append(hdr, enc...), ct...)
	return out, &ResponseContext{sender: sender, enc: enc, aeadID: kc.AEADID}, nil
}

func newResponseAEAD(id uint16, key []byte) (cipher.AEAD, error) {
	switch id {
	case hpke.AES128GCM().ID(), hpke.AES256GCM().ID():
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case hpke.ChaCha20Poly1305().ID():
		return chacha20poly1305.New(key)
	default:
		return nil, fmt.Errorf("unsupported ohttp aead %d", id)
	}
}

func responseKeySize(id uint16) int {
	switch id {
	case hpke.AES128GCM().ID():
		return 16
	default:
		return 32
	}
}

func (rc *ResponseContext) Decapsulate(response []byte) ([]byte, error) {
	keySize := responseKeySize(rc.aeadID)
	const nonceSize = 12
	nonceLen := max(keySize, nonceSize)
	if len(response) < nonceLen {
		return nil, fmt.Errorf("encapsulated response too short")
	}
	secret, err := rc.sender.Export(responseLabel, keySize)
	if err != nil {
		return nil, err
	}
	salt := append(append([]byte{}, rc.enc...), response[:nonceLen]...)
	prk, err := hkdf.Extract(sha256.New, secret, salt)
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Expand(sha256.New, prk, "key", keySize)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "nonce", nonceSize)
	if err != nil {
		return nil, err
	}
	aead, err := newResponseAEAD(rc.aeadID, key)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, response[nonceLen:], nil)
}
