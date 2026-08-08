package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"strings"
	"testing"
)

func TestEncryptInstagramAppPasswordRoundTrips(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to create test key: %v", err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal test key: %v", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKey})
	encrypted, err := EncryptInstagramAppPassword(145, base64.StdEncoding.EncodeToString(publicKeyPEM), "secret-password")
	if err != nil {
		t.Fatalf("EncryptInstagramAppPassword returned error for a base64-encoded PEM key: %v", err)
	}
	parts := strings.SplitN(encrypted, ":", 4)
	if len(parts) != 4 || parts[0] != "#PWD_INSTAGRAM" || parts[1] != "4" {
		t.Fatalf("unexpected Instagram app password envelope %q", encrypted)
	}
	payload, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	if len(payload) < 2+12+2+16 {
		t.Fatalf("password payload is too short: %d", len(payload))
	}
	if payload[0] != 1 || payload[1] != 145 {
		t.Fatalf("unexpected payload prefix: %v", payload[:2])
	}

	iv := payload[2:14]
	keyLength := int(binary.LittleEndian.Uint16(payload[14:16]))
	keyEnd := 16 + keyLength
	if keyEnd+16 > len(payload) {
		t.Fatalf("invalid encrypted key length %d", keyLength)
	}
	sessionKey, err := rsa.DecryptPKCS1v15(nil, privateKey, payload[16:keyEnd])
	if err != nil {
		t.Fatalf("failed to decrypt session key: %v", err)
	}
	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		t.Fatalf("failed to create AES cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("failed to create GCM: %v", err)
	}
	tag := payload[keyEnd : keyEnd+aead.Overhead()]
	ciphertext := payload[keyEnd+aead.Overhead():]
	sealed := append(append([]byte(nil), ciphertext...), tag...)
	plaintext, err := aead.Open(nil, iv, sealed, []byte(parts[2]))
	if err != nil {
		t.Fatalf("failed to decrypt password: %v", err)
	}
	if string(plaintext) != "secret-password" {
		t.Fatalf("unexpected password %q", plaintext)
	}
}
