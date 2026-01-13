package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/crypto/nacl/box"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"

	// We're replacing golang.org/x/crypto with a fork for "legacy chacha20poly1305" (8 byte nonce)
	//"golang.org/x/crypto/chacha20poly1305"
	"github.com/beeper/poly1305/chacha20poly1305"
)

var (
	ErrRandomReadFailed = errors.New("failed to encrypt pw: random read failed")
	ErrAESCreation      = errors.New("failed to encrypt pw: AES cipher creation failed")
	ErrGCMCreation      = errors.New("failed to encrypt pw: GCM mode creation failed")
)

// TO-DO: implement automatic grabbing of pub key from html module config for facebook as insta does
func EncryptPassword(platform int, pubKeyId int, pubKey, password string) (string, error) {
	if platform == int(types.MessengerLite) {
		return encryptPasswordLightspeed(pubKeyId, pubKey, password)
	}

	pubKeyBytes, err := hex.DecodeString(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode pubKey, must be a hex-encoded string: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	ts := []byte(strconv.FormatInt(time.Now().Unix(), 10))
	pwBytes := []byte(password)
	buf.WriteByte(1)
	buf.WriteByte(byte(pubKeyId))

	aesKey, aeadCipher, err := generateAESGCMKey(32)
	if err != nil {
		return "", err
	}

	encryptedData, err := encryptAESGCM(aeadCipher, pwBytes, ts)
	if err != nil {
		return "", err
	}

	sharedSecret, err := encryptWithNaCl(aesKey, pubKeyBytes)
	if err != nil {
		return "", err
	}

	buf.Write([]byte{byte(len(sharedSecret)), byte(len(sharedSecret) >> 8 & 255)})
	buf.Write(sharedSecret)
	buf.Write(encryptedData[len(encryptedData)-16:])
	buf.Write(encryptedData[:len(encryptedData)-16])

	finalString := base64.StdEncoding.EncodeToString(buf.Bytes())

	var formattedStr string
	if platform == 0 {
		formattedStr = fmt.Sprintf("#PWD_INSTAGRAM_BROWSER:10:%s:%s", string(ts), finalString)
	} else {
		formattedStr = fmt.Sprintf("#PWD_BROWSER:5:%s:%s", string(ts), finalString)
	}

	return formattedStr, nil
}

func encryptPasswordLightspeed(pubKeyId int, pubKey, password string) (string, error) {
	pubKeyBytes, err := hex.DecodeString(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode pubKey, must be a hex-encoded string: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	ts := []byte(strconv.FormatInt(time.Now().Unix(), 10))
	pwBytes := []byte(password)
	buf.WriteByte(1)
	buf.WriteByte(byte(pubKeyId))

	encryptionKey := make([]byte, 32)
	if _, err := rand.Read(encryptionKey); err != nil {
		return "", err
	}

	var pubKeyArray [32]byte
	copy(pubKeyArray[:], pubKeyBytes)
	boxed, err := box.SealAnonymous(nil, encryptionKey, &pubKeyArray, rand.Reader)
	if err != nil {
		return "", err
	}
	buf.Write(binary.LittleEndian.AppendUint16(nil, uint16(len(boxed))))
	buf.Write(boxed)

	cipher, err := chacha20poly1305.NewLegacy(encryptionKey)
	if err != nil {
		return "", err
	}

	encrypted_password := cipher.Seal(nil, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, pwBytes, ts)
	// Swap AEAD tag from back to front
	encrypted_password = append(encrypted_password[len(encrypted_password)-16:], encrypted_password[:len(encrypted_password)-16]...)

	buf.Write(encrypted_password)

	finalString := base64.StdEncoding.EncodeToString(buf.Bytes())

	return fmt.Sprintf("#PWD_LIGHTSPEED:3:%s:%s", string(ts), finalString), nil
}
