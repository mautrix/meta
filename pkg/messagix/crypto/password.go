package crypto

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

var FacebookPubKey = "70425d9c3279f0fd3855dd64cd5588d2dfd0fe77163bd7b650e1304b5f25135b"
var FacebookPubKeyId int = 180
var (
	ErrRandomReadFailed = errors.New("failed to encrypt pw: random read failed")
	ErrAESCreation      = errors.New("failed to encrypt pw: AES cipher creation failed")
	ErrGCMCreation      = errors.New("failed to encrypt pw: GCM mode creation failed")
)

// TO-DO: implement automatic grabbing of pub key from html module config for facebook as insta does
func EncryptPassword(platform int, pubKeyId int, pubKey, password string) (string, error) {
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
