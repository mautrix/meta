package securehistory

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"strconv"
)

func deriveEpochKey(epoch Epoch, name string, size int) ([]byte, error) {
	if len(epoch.RootKey) != 32 || !validEpochAnonID(epoch.AnonID) {
		return nil, errors.New("invalid Messenger backup epoch")
	}
	return hkdf.Key(sha256.New, epoch.RootKey, nil, name+"_"+base64.StdEncoding.EncodeToString(epoch.AnonID), size)
}

func (d *VirtualDevice) DeriveEpochs() (map[string]Epoch, error) {
	if d.Info == nil || d.Info.Secrets == nil || d.Info.EpochID == "" || d.Info.LastEpochID == "" {
		return nil, errors.New("missing Messenger backup epoch metadata")
	}
	anonID, err := d.decryptSecret(d.Info.Secrets.EpochAnonID, "epoch_anon_id")
	if err != nil {
		return nil, err
	} else if !validEpochAnonID(anonID) {
		return nil, errors.New("invalid Messenger backup epoch anonymous ID")
	}
	rootKey, err := d.decryptSecret(d.Info.Secrets.EpochRootKey, "epoch_root_key")
	if err != nil {
		return nil, err
	} else if len(rootKey) != 32 {
		return nil, errors.New("invalid Messenger backup epoch root key")
	}
	serverEpochs := make(map[string]SyncEpoch, len(d.Info.Epochs))
	for _, epoch := range d.Info.Epochs {
		serverEpochs[epoch.ID] = epoch
	}
	base := Epoch{ID: d.Info.EpochID, AnonID: anonID, RootKey: rootKey}
	epochs := map[string]Epoch{base.ID: base}
	if base.ID != d.Info.LastEpochID {
		privateKey, err := d.decryptSecret(d.Info.Secrets.EpochStoragePrivateKey, "epoch_storage_private_key")
		if err != nil {
			return nil, err
		}
		for current := base; current.ID != d.Info.LastEpochID; {
			server, ok := serverEpochs[current.ID]
			if !ok || server.NextID == "" {
				return nil, errors.New("missing Messenger backup forward epoch edge")
			} else if _, exists := epochs[server.NextID]; exists {
				return nil, errors.New("cycle in Messenger backup epoch chain")
			}
			next, ok := serverEpochs[server.NextID]
			if !ok {
				return nil, errors.New("missing Messenger backup forward epoch")
			}
			current, err = deriveNextEpoch(current, next, d.Info.EpochStoragePublicKey, privateKey, "")
			if err != nil {
				return nil, err
			}
			epochs[current.ID] = current
		}
	}
	current := base
	for {
		server, ok := serverEpochs[current.ID]
		if !ok || server.PreviousID == "" {
			break
		} else if _, exists := epochs[server.PreviousID]; exists {
			return nil, errors.New("cycle in Messenger backup epoch chain")
		}
		previous, ok := serverEpochs[server.PreviousID]
		if !ok || server.EncryptedData == nil {
			return nil, errors.New("missing or unsupported Messenger backup epoch edge")
		}
		key, err := deriveEpochKey(current, "epoch_data_storage", 32)
		if err != nil {
			return nil, err
		}
		anonID, err := decryptEpochValue(key, server.EncryptedData.AnonID)
		if err != nil {
			return nil, err
		}
		rootKey, err := decryptEpochValue(key, server.EncryptedData.RootKey)
		if err != nil {
			return nil, err
		} else if !validEpochAnonID(anonID) || len(rootKey) != 32 {
			return nil, errors.New("invalid recovered Messenger backup epoch")
		}
		current = Epoch{ID: previous.ID, AnonID: anonID, RootKey: rootKey}
		epochs[current.ID] = current
	}
	return epochs, nil
}

func deriveNextEpoch(current Epoch, next SyncEpoch, publicKey, privateKey []byte, pskFingerprint string) (Epoch, error) {
	if !validEpochAnonID([]byte(next.AnonID)) || len(current.RootKey) != 32 {
		return Epoch{}, errors.New("invalid Messenger backup forward epoch")
	}
	chain, err := hkdf.Key(sha256.New, current.RootKey, nil, "epoch_chaining_"+string(current.AnonID)+"_"+current.ID, 64)
	if err != nil {
		return Epoch{}, err
	}
	if pskFingerprint != "" && !matchesFingerprint(chain[32:], "MEBCryptoPreSharedKey", pskFingerprint) {
		return Epoch{}, errors.New("backup epoch PSK fingerprint mismatch")
	}
	entropy, err := decryptPublic(publicKey, privateKey, next.AuthPublicKey, chain[32:], []byte("epoch__"+next.AnonID), next.EncryptedKey)
	if err != nil {
		return Epoch{}, fmt.Errorf("failed to decrypt Messenger backup epoch entropy: %w", err)
	}
	root, err := hkdf.Key(sha256.New, entropy, chain[:32], "epoch_root_key", 32)
	if err != nil {
		return Epoch{}, err
	}
	return Epoch{ID: next.ID, AnonID: []byte(next.AnonID), RootKey: root}, nil
}

func matchesFingerprint(key []byte, name, expected string) bool {
	fingerprint, err := hkdf.Key(sha256.New, key, nil, "meb/debug/fingerprint/"+name, 18)
	return err == nil && base64.StdEncoding.EncodeToString(fingerprint) == expected
}

func (d *Device) WithEpochs(set *EpochDerivationSet) (*Device, error) {
	updated := *d
	updated.Epochs = maps.Clone(d.Epochs)
	if err := updated.applyEpochs(set); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (d *Device) applyEpochs(set *EpochDerivationSet) error {
	if set == nil {
		return nil
	}
	for _, edge := range set.Edges {
		source, ok := d.Epochs[edge.From.ID]
		if !ok || base64.StdEncoding.EncodeToString(source.AnonID) != edge.From.AnonID ||
			!matchesFingerprint(source.RootKey, "MEBEpochRootKey", edge.FromFingerprint) {
			return errors.New("backup epoch source mismatch")
		}
		anon, err := base64.StdEncoding.DecodeString(edge.To.AnonID)
		if err != nil || !validEpochAnonID(anon) || edge.To.ID == "" {
			return errors.New("invalid Messenger backup epoch destination")
		}
		fromSequence, err := strconv.ParseUint(string(source.AnonID), 10, 64)
		if err != nil {
			return err
		}
		toSequence, err := strconv.ParseUint(string(anon), 10, 64)
		if err != nil {
			return err
		}
		var target Epoch
		if edge.Forward != nil {
			if toSequence != fromSequence+1 || !bytes.Equal(d.StoragePublicKey, edge.Forward.StoragePublicKey) || edge.Forward.PSKFingerprint == "" {
				return errors.New("backup forward epoch mismatch")
			}
			target, err = deriveNextEpoch(source, SyncEpoch{
				ID:            edge.To.ID,
				AnonID:        string(anon),
				AuthPublicKey: edge.Forward.AuthPublicKey,
				EncryptedKey:  edge.Forward.EncryptedEntropy,
			}, d.StoragePublicKey, d.StoragePrivateKey, edge.Forward.PSKFingerprint)
		} else {
			if fromSequence != toSequence+1 || len(edge.Backward) == 0 {
				return errors.New("backup backward epoch mismatch")
			}
			key, keyErr := deriveEpochKey(source, "epoch_data_storage", 32)
			if keyErr != nil {
				return keyErr
			}
			target = Epoch{ID: edge.To.ID, AnonID: anon}
			target.RootKey, err = decryptEpochValue(key, edge.Backward)
		}
		if err != nil {
			return err
		} else if len(target.RootKey) != 32 || !matchesFingerprint(target.RootKey, "MEBEpochRootKey", edge.ToFingerprint) {
			return errors.New("backup epoch destination fingerprint mismatch")
		}
		d.Epochs[target.ID] = target
	}
	return nil
}

func decryptPublic(publicKey, privateKey, senderKey, psk, aad, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 49 || ciphertext[0] != 1 || len(publicKey) != 32 || len(psk) > 32 {
		return nil, errors.New("invalid or unsupported Messenger backup public-key envelope")
	}
	curve := ecdh.X25519()
	private, err := curve.NewPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	sender, err := curve.NewPublicKey(senderKey)
	if err != nil {
		return nil, err
	}
	ephemeral, err := curve.NewPublicKey(ciphertext[1:33])
	if err != nil {
		return nil, err
	}
	shared, err := private.ECDH(sender)
	if err != nil {
		return nil, err
	}
	fresh, err := private.ECDH(ephemeral)
	if err != nil {
		return nil, err
	}
	secret := append(shared, fresh...)
	innerAAD := append([]byte{1}, senderKey...)
	innerAAD = append(innerAAD, publicKey...)
	innerAAD = append(innerAAD, ciphertext[1:33]...)
	innerAAD = append(innerAAD, aad...)
	key, err := hkdf.Key(sha256.New, secret, psk, string(innerAAD), 32)
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
	return aead.Open(nil, make([]byte, 12), ciphertext[33:], innerAAD)
}

func decryptEpochValue(key, ciphertext []byte) ([]byte, error) {
	value, err := decryptData(key, ciphertext, []byte("epoch_data_metadata"))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt Messenger backup epoch: %w", err)
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(value)))
	n, err := base64.StdEncoding.Decode(decoded, value)
	if err != nil {
		return nil, err
	}
	return decoded[:n], nil
}
