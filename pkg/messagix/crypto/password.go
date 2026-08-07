package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
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
func EncryptPassword(platform types.Platform, pubKeyId int, pubKey, password string) (string, error) {
	if platform.IsMessengerLite() {
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
	if platform.IsInstagram() {
		formattedStr = fmt.Sprintf("#PWD_INSTAGRAM_BROWSER:10:%s:%s", string(ts), finalString)
	} else {
		formattedStr = fmt.Sprintf("#PWD_BROWSER:5:%s:%s", string(ts), finalString)
	}

	return formattedStr, nil
}

// EncryptInstagramAppPassword creates the password envelope used by the
// first-party Instagram Android API. The app endpoint publishes an RSA public
// key in the response headers from /api/v1/qe/sync/.
func EncryptInstagramAppPassword(pubKeyID int, publicKey, password string) (string, error) {
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode Instagram app public key: %w", err)
	}
	if block, _ := pem.Decode(publicKeyBytes); block != nil {
		publicKeyBytes = block.Bytes
	}
	parsedKey, err := x509.ParsePKIXPublicKey(publicKeyBytes)
	if err != nil {
		if pkcs1Key, pkcs1Err := x509.ParsePKCS1PublicKey(publicKeyBytes); pkcs1Err == nil {
			parsedKey = pkcs1Key
		} else {
			return "", fmt.Errorf("failed to parse Instagram app public key: %w", err)
		}
	}
	rsaKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("instagram app public key is not RSA")
	}

	sessionKey := make([]byte, 32)
	if _, err = rand.Read(sessionKey); err != nil {
		return "", ErrRandomReadFailed
	}
	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return "", ErrAESCreation
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrGCMCreation
	}
	iv := make([]byte, aead.NonceSize())
	if _, err = rand.Read(iv); err != nil {
		return "", ErrRandomReadFailed
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	encryptedSessionKey, err := rsa.EncryptPKCS1v15(rand.Reader, rsaKey, sessionKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt Instagram app session key: %w", err)
	}
	encryptedPassword := aead.Seal(nil, iv, []byte(password), []byte(timestamp))
	tagOffset := len(encryptedPassword) - aead.Overhead()

	buf := bytes.NewBuffer(nil)
	buf.WriteByte(1)
	buf.WriteByte(byte(pubKeyID))
	buf.Write(iv)
	buf.Write(binary.LittleEndian.AppendUint16(nil, uint16(len(encryptedSessionKey))))
	buf.Write(encryptedSessionKey)
	buf.Write(encryptedPassword[tagOffset:])
	buf.Write(encryptedPassword[:tagOffset])

	return fmt.Sprintf(
		"#PWD_INSTAGRAM:4:%s:%s",
		timestamp,
		base64.StdEncoding.EncodeToString(buf.Bytes()),
	), nil
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
