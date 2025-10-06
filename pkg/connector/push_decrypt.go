// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2025 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/util/jsontime"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type webPushData struct {
	Data            []byte `json:"data"`
	ContentEncoding string `json:"content-encoding"`
	CryptoKey       string `json:"crypto-key"`
	Encryption      string `json:"encryption"`
}

type decryptedPushData struct {
	Type              string            `json:"type"`
	Time              jsontime.Unix     `json:"time"`
	Message           string            `json:"message"`
	Href              *string           `json:"href"`
	ThreadName        string            `json:"t"`
	MessageID         string            `json:"n"`
	ProfilePictureURL string            `json:"ppu"`
	Params            map[string]string `json:"params"`
}

func (m *MetaClient) decryptPush(push json.RawMessage) (*decryptedPushData, error) {
	var wpd webPushData
	err := json.Unmarshal(push, &wpd)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal web push data: %w", err)
	} else if wpd.ContentEncoding != "aesgcm" {
		return nil, fmt.Errorf("unsupported content encoding %s", wpd.ContentEncoding)
	}
	cryptoKey := parseField(wpd.CryptoKey)
	encryption := parseField(wpd.Encryption)
	if cryptoKey["dh"] == nil || encryption["salt"] == nil {
		return nil, fmt.Errorf("missing dh or salt in push headers")
	}
	keys := m.UserLogin.Metadata.(*metaid.UserLoginMetadata).PushKeys
	if keys == nil {
		return nil, fmt.Errorf("no push keys available")
	}
	pk, err := ecdh.P256().NewPrivateKey(keys.Private)
	if err != nil {
		return nil, fmt.Errorf("failed to parse push private key: %w", err)
	}
	dh, err := ecdh.P256().NewPublicKey(cryptoKey["dh"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse push server dh public key: %w", err)
	}
	shared, err := pk.ECDH(dh)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate shared secret: %w", err)
	}

	prkKey := hmacSHA256(keys.Auth, shared)
	ikm := hmacSHA256(prkKey, append([]byte("Content-Encoding: auth\x00"), 0x01))
	prk := hmacSHA256(encryption["salt"], ikm)

	keyInfo := buildLegacyInfo("Content-Encoding: aesgcm", pk.PublicKey().Bytes(), dh.Bytes())
	aesKey := hmacSHA256(prk, append(keyInfo, 0x01))[:16]

	nonceInfo := buildLegacyInfo("Content-Encoding: nonce", pk.PublicKey().Bytes(), dh.Bytes())
	gcmNonce := hmacSHA256(prk, append(nonceInfo, 0x01))[:12]

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}
	decrypted, err := aesgcm.Open(nil, gcmNonce, wpd.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open gcm: %w", err)
	}
	padLen := int(binary.BigEndian.Uint16(decrypted[:2]))
	if 2+padLen > len(decrypted) {
		return nil, errors.New("invalid padding length")
	}
	if json.Valid(decrypted[2+padLen:]) {
		m.UserLogin.Log.Trace().RawJSON("push_data", decrypted[2+padLen:]).Msg("Decrypted push data")
	} else {
		m.UserLogin.Log.Trace().Bytes("raw_push_data", decrypted).Msg("Decrypted push data (not JSON?)")
	}
	var dpd decryptedPushData
	err = json.Unmarshal(decrypted[2+padLen:], &dpd)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted push data: %w", err)
	}
	return &dpd, nil
}

func parseField(field string) map[string][]byte {
	parts := strings.Split(field, ";")
	output := make(map[string][]byte)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		subparts := strings.SplitN(part, "=", 2)
		if len(subparts) != 2 {
			continue
		}
		key := subparts[0]
		valB64 := strings.TrimRight(subparts[1], `=`)
		val, err := base64.RawURLEncoding.DecodeString(valB64)
		if err != nil {
			continue
		}
		output[key] = val
	}
	return output
}

func hmacSHA256(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}

func buildLegacyInfo(label string, clientPub, serverPub []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(label)
	buf.WriteByte(0x00)
	buf.WriteString("P-256")
	buf.WriteByte(0x00)

	buf.WriteByte(0)
	buf.WriteByte(65)
	buf.Write(clientPub)
	buf.WriteByte(0)
	buf.WriteByte(65)
	buf.Write(serverPub)
	return buf.Bytes()
}
