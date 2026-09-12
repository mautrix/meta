package pushcrypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"strconv"
	"time"
)

type NativePushKeys struct {
	AESKey         []byte `json:"aes_key"`
	AESKeyID       int64  `json:"aes_key_id"`
	HPKEPrivateKey []byte `json:"hpke_private_key"`
	HPKEKeyID      string `json:"hpke_key_id"`
}

func NewNativePushKeys(userID int64) (*NativePushKeys, error) {
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	rand.Read(key)
	return &NativePushKeys{
		AESKey:         key,
		AESKeyID:       userID<<8 | 1,
		HPKEPrivateKey: private.Bytes(),
		HPKEKeyID:      strconv.FormatInt(time.Now().UnixMilli(), 10),
	}, nil
}
