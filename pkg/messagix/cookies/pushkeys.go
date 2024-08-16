package cookies

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
)

type PushKeysPublic struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

type PushKeys struct {
	Public  PushKeysPublic
	Private string
}

func generatePushKeys() (*PushKeys, error) {
	curve := ecdh.P256()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	publicKey := privateKey.Public().(*ecdh.PublicKey)
	encodedPublicKey := base64.URLEncoding.EncodeToString(publicKey.Bytes())

	privateKeyBytes := privateKey.Bytes()
	encodedPrivateKey := base64.URLEncoding.EncodeToString(privateKeyBytes)

	auth := make([]byte, 16)
	_, err = rand.Read(auth)
	if err != nil {
		return nil, err
	}
	encodedAuth := base64.URLEncoding.EncodeToString(auth)

	pushKeys := &PushKeys{
		Public: PushKeysPublic{
			P256dh: encodedPublicKey,
			Auth:   encodedAuth,
		},
		Private: encodedPrivateKey,
	}
	return pushKeys, nil
}
