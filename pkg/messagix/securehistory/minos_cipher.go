package securehistory

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/subtle"
	"fmt"

	"go.mau.fi/libsignal/ecc"
)

type minosMessageMetadata struct {
	MEKID     []byte
	MessageID string
	ThreadID  []byte
	Timestamp uint64
}

func minosVerifySignature(publicKey, signature, message []byte) error {
	if len(publicKey) != 33 || publicKey[0] != 4 || len(signature) != 65 || signature[0] != 4 {
		return fmt.Errorf("invalid Minos signing key or signature")
	}
	public, err := ecdh.X25519().NewPublicKey(publicKey[1:])
	if err != nil {
		return err
	}
	private, err := ecdh.X25519().NewPrivateKey(make([]byte, 32))
	if err != nil {
		return err
	}
	if _, err = private.ECDH(public); err != nil {
		return fmt.Errorf("invalid Minos signing public key: %w", err)
	}
	if !ecc.VerifySignature(ecc.NewDjbECPublicKey(*(*[32]byte)(publicKey[1:])), message, *(*[64]byte)(signature[1:])) {
		return fmt.Errorf("invalid Minos signature")
	}
	return nil
}

func decryptMinosMessage(metadata minosMessageMetadata, stanza *MinosProtobuf, mek []byte) ([]byte, error) {
	if len(mek) == 0 || ((mek[0] != 1 || len(mek) != 17) && (mek[0] != 2 || len(mek) != 33)) {
		return nil, fmt.Errorf("invalid Minos message encryption key")
	}
	if stanza.EncryptionVersion != 1 && stanza.EncryptionVersion != 2 {
		return nil, fmt.Errorf("unsupported Minos message encryption version %d", stanza.EncryptionVersion)
	}
	if err := minosVerifySignature(stanza.TransportSigningPK, stanza.Signature, append([]byte("minos.unstructured_signature.message:"), stanza.Encrypted...)); err != nil {
		return nil, err
	}
	info := "minos.kdf.mek.sender_key"
	additionalData := fmt.Sprintf("minos.symmetric_enc.message:mek_id=%x;message_id=%s;thread_id=%x;timestamp=%d", metadata.MEKID, metadata.MessageID, metadata.ThreadID, metadata.Timestamp)
	if stanza.EncryptionVersion == 2 {
		info = fmt.Sprintf("minos.kdf.mek.single_message_key:;message_id=%s;thread_id=%x;timestamp=%d", metadata.MessageID, metadata.ThreadID, metadata.Timestamp)
		additionalData = fmt.Sprintf("minos.symmetric_enc.message_v2:mek_id=%x;message_id=%s;sender_transport_pk=%x;thread_id=%x;timestamp=%d", metadata.MEKID, metadata.MessageID, stanza.TransportSigningPK, metadata.ThreadID, metadata.Timestamp)
	}
	key, err := minosKDF(mek, info, len(mek)-1)
	if err != nil {
		return nil, err
	}
	if mek[0] == 2 {
		return minosDecryptDNDK(key, stanza.Encrypted, []byte(additionalData))
	}
	if len(stanza.Encrypted) < 28 {
		return nil, fmt.Errorf("invalid Minos AES-GCM ciphertext length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, stanza.Encrypted[:12], stanza.Encrypted[12:], []byte(additionalData))
}

func minosDecryptDNDK(key, ciphertext, additionalData []byte) ([]byte, error) {
	if len(key) != 32 || len(ciphertext) < 72 {
		return nil, fmt.Errorf("invalid Minos DNDK key or ciphertext length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	var input [16]byte
	copy(input[:15], ciphertext[:15])
	var blocks [5][16]byte
	for i := range blocks {
		input[15] = byte(224 + i)
		block.Encrypt(blocks[i][:], input[:])
	}
	var derived, commitment [32]byte
	for i := range 16 {
		derived[i] = blocks[0][i] ^ blocks[1][i]
		derived[i+16] = blocks[0][i] ^ blocks[2][i]
		commitment[i] = blocks[0][i] ^ blocks[3][i]
		commitment[i+16] = blocks[0][i] ^ blocks[4][i]
	}
	if subtle.ConstantTimeCompare(commitment[:], ciphertext[len(ciphertext)-32:]) != 1 {
		return nil, fmt.Errorf("minos DNDK key commitment mismatch")
	}
	block, err = aes.NewCipher(derived[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	var nonce [12]byte
	copy(nonce[:9], ciphertext[15:24])
	return aead.Open(nil, nonce[:], ciphertext[24:len(ciphertext)-32], additionalData)
}
