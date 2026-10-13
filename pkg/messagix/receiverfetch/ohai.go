package receiverfetch

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/sha256"
	"fmt"
)

func Encapsulate(keyID byte, publicKey, request []byte) ([]byte, func([]byte) ([]byte, error), error) {
	pub, err := ecdh.X25519().NewPublicKey(publicKey)
	if err != nil {
		return nil, nil, err
	}
	key, err := hpke.NewDHKEMPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	enc, sender, err := hpke.NewSender(key, hpke.HKDFSHA256(), hpke.AES128GCM(), []byte("message/bhttp request"))
	if err != nil {
		return nil, nil, err
	}
	header := []byte{keyID, 0, 32, 0, 1, 0, 1}
	ciphertext, err := sender.Seal(header, request)
	if err != nil {
		return nil, nil, err
	}
	decrypt := func(response []byte) ([]byte, error) {
		if len(response) < 16 {
			return nil, fmt.Errorf("encapsulated response too short")
		}
		secret, err := sender.Export("message/bhttp response", 16)
		if err != nil {
			return nil, err
		}
		prk, err := hkdf.Extract(sha256.New, secret, append(append([]byte{}, enc...), response[:16]...))
		if err != nil {
			return nil, err
		}
		key, err := hkdf.Expand(sha256.New, prk, "key", 16)
		if err != nil {
			return nil, err
		}
		nonce, err := hkdf.Expand(sha256.New, prk, "nonce", 12)
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
		return aead.Open(nil, nonce, response[16:], nil)
	}
	return append(append(header, enc...), ciphertext...), decrypt, nil
}
