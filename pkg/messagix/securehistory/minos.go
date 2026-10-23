package securehistory

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/mlkem"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
)

var minosKDFSalt = []byte{0x62, 0xb9, 0x88, 0x19, 0xba, 0xde, 0x10, 0x74, 0xe9, 0x0f, 0x97, 0x2d, 0x80, 0x88, 0x21, 0x0b, 0xcb, 0xf8, 0x62, 0x29, 0xb0, 0xd1, 0x19, 0x0f, 0xb0, 0x96, 0x6a, 0x43, 0xb1, 0x43, 0x91, 0xf3}

func minosKDF(root []byte, info string, length int) ([]byte, error) {
	return hkdf.Key(sha256.New, root, minosKDFSalt, info, length)
}

func minosHPKEExtract(suite string, salt []byte, label string, input []byte) []byte {
	h := hmac.New(sha256.New, salt)
	_, _ = h.Write([]byte("HPKE-v1" + suite + label))
	_, _ = h.Write(input)
	return h.Sum(nil)
}

func minosHPKEExpand(suite string, key []byte, label string, info []byte, length uint16) []byte {
	context := binary.BigEndian.AppendUint16(nil, length)
	context = append(context, "HPKE-v1"+suite+label...)
	context = append(context, info...)
	result, _ := hkdf.Expand(sha256.New, key, string(context), int(length))
	return result
}

func minosMailboxKeys(epoch Epoch) (*ecdh.PrivateKey, *mlkem.DecapsulationKey768, error) {
	epochNumber, err := strconv.ParseUint(string(epoch.AnonID), 10, 64)
	if err != nil || len(epoch.RootKey) != 32 {
		return nil, nil, errors.New("invalid Minos epoch")
	}
	seed, err := minosKDF(epoch.RootKey, fmt.Sprintf("minos.kdf.export_root_key.encryption_keypair:epoch_number=%d", epochNumber), 32)
	if err != nil {
		return nil, nil, err
	}
	prk := minosHPKEExtract("KEM\x00\x20", nil, "dkp_prk", seed)
	x25519Key := minosHPKEExpand("KEM\x00\x20", prk, "sk", nil, 32)
	sk, err := ecdh.X25519().NewPrivateKey(x25519Key)
	if err != nil {
		return nil, nil, err
	}
	d, _ := hkdf.Key(sha256.New, seed, nil, "mlkem768.derive_keypair.d", 32)
	z, _ := hkdf.Key(sha256.New, seed, nil, "mlkem768.derive_keypair.z", 32)
	kemSeed := append(d, z...)
	kem, err := mlkem.NewDecapsulationKey768(kemSeed)
	return sk, kem, err
}

func minosOpenHPKE(sk *ecdh.PrivateKey, kem *mlkem.DecapsulationKey768, authPK, ciphertext, aad []byte) ([]byte, error) {
	if len(authPK) != 33 || authPK[0] != 3 || len(ciphertext) < 1137 || ciphertext[0] != 3 {
		return nil, errors.New("invalid Minos HPKE envelope")
	}
	ephemeralPK, err := ecdh.X25519().NewPublicKey(ciphertext[1:33])
	if err != nil {
		return nil, err
	}
	authKey, err := ecdh.X25519().NewPublicKey(authPK[1:])
	if err != nil {
		return nil, err
	}
	dh, err := sk.ECDH(ephemeralPK)
	if err != nil {
		return nil, err
	}
	authDH, err := sk.ECDH(authKey)
	if err != nil {
		return nil, err
	}
	dhInput := bytes.Join([][]byte{dh, authDH}, nil)
	kemContext := bytes.Join([][]byte{ephemeralPK.Bytes(), sk.PublicKey().Bytes(), authKey.Bytes()}, nil)
	eaePRK := minosHPKEExtract("KEM\x00\x20", nil, "eae_prk", dhInput)
	dhSecret := minosHPKEExpand("KEM\x00\x20", eaePRK, "shared_secret", kemContext, 32)
	kemCiphertext := ciphertext[33:1121]
	kemSecret, err := kem.Decapsulate(kemCiphertext)
	if err != nil {
		return nil, err
	}
	combined := bytes.Join([][]byte{dhSecret, kemSecret, ephemeralPK.Bytes(), kemCiphertext, kem.EncapsulationKey().Bytes()}, nil)
	shared, _ := hkdf.Key(sha256.New, combined, nil, "hawk_final", 32)
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
	return gcm.Open(nil, nonce, ciphertext[1121:], aad)
}

func decryptMinosMEK(device *Device, entry *MinosKeyEntry, transportSigningPK []byte) ([]byte, error) {
	if entry == nil || entry.Keys == nil || entry.Keys.MEKInfo == nil || entry.Keys.RecipientInfo == nil || entry.Keys.CreatorInfo == nil {
		return nil, errors.New("incomplete Minos MEK")
	}
	mek, recipient, creator := entry.Keys.MEKInfo, entry.Keys.RecipientInfo, entry.Keys.CreatorInfo
	if len(mek.ID) != 12 || len(mek.RosterHash) != 32 || mek.EncryptionVersion < 1 || mek.EncryptionVersion > 2 {
		return nil, errors.New("invalid Minos MEK metadata")
	}
	epoch, ok := device.Epochs[recipient.EpochID]
	if !ok || !bytes.Equal(epoch.AnonID, recipient.EpochAnon) {
		return nil, errors.New("missing Minos recipient epoch")
	}
	suffix := ""
	recipientContext := ""
	if mek.EncryptionVersion == 2 {
		if len(recipient.EpochHead) != 32 {
			return nil, errors.New("missing Minos recipient epoch head")
		}
		suffix = "_v2"
		recipientContext = fmt.Sprintf(";recipient_epoch=%x", recipient.EpochHead)
	}
	var authPK []byte
	var aad string
	if creator.Minos != nil && creator.Transport == nil {
		if len(creator.Minos.EpochHead) != 32 {
			return nil, errors.New("invalid Minos sender epoch head")
		}
		authPK = creator.Minos.AuthPublicKey
		aad = fmt.Sprintf("minos.hpke.mek_distribution_from_minos%s:mek_id=%x%s;recipient_roster_hash=%x;sender_epoch=%x", suffix, mek.ID, recipientContext, mek.RosterHash, creator.Minos.EpochHead)
	} else if creator.Transport != nil && creator.Minos == nil {
		transport := creator.Transport
		authPK = transport.EphemeralPublicKey
		signatureData := fmt.Sprintf("minos.structured_signature.mek_distribution.ephemeral_auth:ephemeral_auth_pubkey=%x;mek_id=%x", authPK, mek.ID)
		if err := minosVerifySignature(transportSigningPK, transport.Signature, []byte(signatureData)); err != nil {
			signatureData = fmt.Sprintf("minos.structured_signature.mek_distribution:ephemeral_auth_pubkey=%x;mek_id=%x", authPK, mek.ID)
			if err = minosVerifySignature(transportSigningPK, transport.Signature, []byte(signatureData)); err != nil {
				return nil, fmt.Errorf("invalid Minos transport MEK signature: %w", err)
			}
		}
		aad = fmt.Sprintf("minos.hpke.mek_distribution_from_transport%s:mek_id=%x%s;recipient_roster_hash=%x;sender_transport_pubkey=%x", suffix, mek.ID, recipientContext, mek.RosterHash, transportSigningPK)
	} else {
		return nil, errors.New("unsupported Minos MEK sender")
	}
	sk, kem, err := minosMailboxKeys(epoch)
	if err != nil {
		return nil, err
	}
	return minosOpenHPKE(sk, kem, authPK, mek.Encrypted, []byte(aad))
}
