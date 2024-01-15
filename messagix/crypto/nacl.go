package crypto

import (
	"crypto/rand"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/nacl/box"
)

func encryptWithNaCl(aesKey, publicKeyBytes []byte) ([]byte, error) {
	result := make([]byte, 80)
	publicKey, privateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	var peerPublicKey [32]byte
	copy(peerPublicKey[:], publicKeyBytes)

	var sharedKey [32]byte
	box.Precompute(&sharedKey, &peerPublicKey, privateKey)

	hasher, _ := blake2b.New(24, nil)
	hasher.Write(publicKey[:])
	hasher.Write(publicKeyBytes)
	nonceSlice := hasher.Sum(nil)

	var nonce [24]byte
	copy(nonce[:], nonceSlice)

	encrypted := box.SealAfterPrecomputation(nil, aesKey, &nonce, &sharedKey)

	copy(result, publicKey[:])
	copy(result[32:], encrypted)

	for i := range privateKey {
		privateKey[i] = 0
	}

	return result, nil
}
