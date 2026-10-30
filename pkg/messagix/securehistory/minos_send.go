package securehistory

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"

	"go.mau.fi/libsignal/ecc"
)

const (
	minosMEKVersion      = 2
	minosMessageVersion  = 2
	minosMetadataVersion = 2
)

type MinosRecipient struct {
	ParticipantID  string
	MailboxKeyFBID string
	EpochHead      []byte
	EncryptionPK   []byte
}

type MinosMEK struct {
	Key        []byte
	ID         []byte
	RosterHash []byte
}

type MinosMEKEnvelope struct {
	ParticipantID  string
	MailboxKeyFBID string
	EncryptedMEK   []byte
}

type MinosMEKDistribution struct {
	MEK                *MinosMEK
	Envelopes          []MinosMEKEnvelope
	EphemeralAuthPK    []byte
	EphemeralSignature []byte
	TransportSigningPK []byte
}

func MinosTransportSigningPK(identityPub *[32]byte) []byte {
	return append([]byte{4}, identityPub[:]...)
}

func minosSign(identityPriv *[32]byte, message []byte) []byte {
	signature := ecc.CalculateSignature(ecc.NewDjbECPrivateKey(*identityPriv), message)
	return append([]byte{4}, signature[:]...)
}

func minosRosterHash(epochHeads [][]byte) []byte {
	sorted := slices.Clone(epochHeads)
	slices.SortFunc(sorted, bytes.Compare)
	hash := sha256.New()
	for _, head := range sorted {
		hash.Write(head)
	}
	return hash.Sum(nil)
}

func GenerateMinosMEK(recipients []MinosRecipient) (*MinosMEK, error) {
	heads := make([][]byte, len(recipients))
	for i, recipient := range recipients {
		if len(recipient.EpochHead) != 32 {
			return nil, fmt.Errorf("invalid Minos epoch head for %s", recipient.ParticipantID)
		}
		heads[i] = recipient.EpochHead
	}
	key := make([]byte, 33)
	key[0] = 2
	if _, err := rand.Read(key[1:]); err != nil {
		return nil, err
	}
	rosterHash := minosRosterHash(heads)
	id, err := minosKDF(key, fmt.Sprintf("minos.kdf.mek.mek_id:recipient_roster_hash=%x", rosterHash), 12)
	if err != nil {
		return nil, err
	}
	return &MinosMEK{Key: key, ID: id, RosterHash: rosterHash}, nil
}

func minosSealHPKE(recipientPK []byte, authSK *ecdh.PrivateKey, plaintext, aad []byte) ([]byte, error) {
	if len(recipientPK) != 1217 || recipientPK[0] != 3 {
		return nil, errors.New("invalid Minos mailbox encryption key")
	}
	recipientKey, err := ecdh.X25519().NewPublicKey(recipientPK[1:33])
	if err != nil {
		return nil, err
	}
	encapsulationKey, err := mlkem.NewEncapsulationKey768(recipientPK[33:])
	if err != nil {
		return nil, err
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	dh, err := ephemeral.ECDH(recipientKey)
	if err != nil {
		return nil, err
	}
	authDH, err := authSK.ECDH(recipientKey)
	if err != nil {
		return nil, err
	}
	dhInput := bytes.Join([][]byte{dh, authDH}, nil)
	kemContext := bytes.Join([][]byte{ephemeral.PublicKey().Bytes(), recipientKey.Bytes(), authSK.PublicKey().Bytes()}, nil)
	eaePRK := minosHPKEExtract("KEM\x00\x20", nil, "eae_prk", dhInput)
	dhSecret := minosHPKEExpand("KEM\x00\x20", eaePRK, "shared_secret", kemContext, 32)
	kemSecret, kemCiphertext := encapsulationKey.Encapsulate()
	combined := bytes.Join([][]byte{dhSecret, kemSecret, ephemeral.PublicKey().Bytes(), kemCiphertext, encapsulationKey.Bytes()}, nil)
	shared, err := hkdf.Key(sha256.New, combined, nil, "hawk_final", 32)
	if err != nil {
		return nil, err
	}
	const suite = "HPKE\x00\x02\x00\x01\x00\x02"
	context := bytes.Join([][]byte{{2}, minosHPKEExtract(suite, nil, "psk_id_hash", nil), minosHPKEExtract(suite, nil, "info_hash", nil)}, nil)
	secret := minosHPKEExtract(suite, shared, "secret", nil)
	key := minosHPKEExpand(suite, secret, "key", context, 32)
	nonce := minosHPKEExpand(suite, secret, "base_nonce", context, 12)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	envelope := make([]byte, 0, 1121+len(plaintext)+gcm.Overhead())
	envelope = append(envelope, 3)
	envelope = append(envelope, ephemeral.PublicKey().Bytes()...)
	envelope = append(envelope, kemCiphertext...)
	return gcm.Seal(envelope, nonce, plaintext, aad), nil
}

func DistributeMinosMEKFromTransport(mek *MinosMEK, recipients []MinosRecipient, identityPriv, identityPub *[32]byte) (*MinosMEKDistribution, error) {
	authSK, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	authPK := append([]byte{3}, authSK.PublicKey().Bytes()...)
	signingPK := MinosTransportSigningPK(identityPub)
	signature := minosSign(identityPriv, fmt.Appendf(nil, "minos.structured_signature.mek_distribution.ephemeral_auth:ephemeral_auth_pubkey=%x;mek_id=%x", authPK, mek.ID))
	envelopes := make([]MinosMEKEnvelope, 0, len(recipients))
	for _, recipient := range recipients {
		aad := fmt.Sprintf("minos.hpke.mek_distribution_from_transport_v2:mek_id=%x;recipient_epoch=%x;recipient_roster_hash=%x;sender_transport_pubkey=%x", mek.ID, recipient.EpochHead, mek.RosterHash, signingPK)
		encrypted, err := minosSealHPKE(recipient.EncryptionPK, authSK, mek.Key, []byte(aad))
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt Minos MEK for %s: %w", recipient.ParticipantID, err)
		}
		envelopes = append(envelopes, MinosMEKEnvelope{
			ParticipantID:  recipient.ParticipantID,
			MailboxKeyFBID: recipient.MailboxKeyFBID,
			EncryptedMEK:   encrypted,
		})
	}
	return &MinosMEKDistribution{
		MEK:                mek,
		Envelopes:          envelopes,
		EphemeralAuthPK:    authPK,
		EphemeralSignature: signature,
		TransportSigningPK: signingPK,
	}, nil
}

func minosEncryptDNDK(key, plaintext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	var input [16]byte
	if _, err = rand.Read(input[:15]); err != nil {
		return nil, err
	}
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
	block, err = aes.NewCipher(derived[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	var nonce [12]byte
	if _, err = rand.Read(nonce[:9]); err != nil {
		return nil, err
	}
	output := make([]byte, 24, 24+len(plaintext)+aead.Overhead()+32)
	copy(output[:15], input[:15])
	copy(output[15:24], nonce[:9])
	output = aead.Seal(output, nonce[:], plaintext, additionalData)
	return append(output, commitment[:]...), nil
}

func EncryptMinosMessage(mek *MinosMEK, messageID string, threadID string, timestamp uint64, identityPriv, identityPub *[32]byte, plaintext []byte) (ciphertext, signature []byte, err error) {
	signingPK := MinosTransportSigningPK(identityPub)
	key, err := minosKDF(mek.Key, fmt.Sprintf("minos.kdf.mek.single_message_key:;message_id=%s;thread_id=%x;timestamp=%d", messageID, []byte(threadID), timestamp), len(mek.Key)-1)
	if err != nil {
		return nil, nil, err
	}
	additionalData := fmt.Sprintf("minos.symmetric_enc.message_v2:mek_id=%x;message_id=%s;sender_transport_pk=%x;thread_id=%x;timestamp=%d", mek.ID, messageID, signingPK, []byte(threadID), timestamp)
	ciphertext, err = minosEncryptDNDK(key, plaintext, []byte(additionalData))
	if err != nil {
		return nil, nil, err
	}
	signature = minosSign(identityPriv, append([]byte("minos.unstructured_signature.message:"), ciphertext...))
	return ciphertext, signature, nil
}
