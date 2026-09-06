package capture

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

// Decode authenticates every record and requires the encrypted terminal marker.
// Callers must not publish partial plaintext before this function succeeds.
func Decode(input io.Reader, key *rsa.PrivateKey) ([]Record, error) {
	scanner := bufio.NewScanner(io.LimitReader(input, maxFile+1))
	scanner.Buffer(make([]byte, 4096), 8<<20)
	var header Header
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &header) != nil || header.Format != Format {
		return nil, errors.New("invalid capture header")
	}
	secret, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, header.Key, []byte(Format))
	if err != nil {
		return nil, errors.New("capture key mismatch")
	}
	block, err := aes.NewCipher(secret)
	clear(secret)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	var records []Record
	ended := false
	for scanner.Scan() {
		var env Envelope
		if ended || len(records) >= 33 || json.Unmarshal(scanner.Bytes(), &env) != nil || env.Sequence != len(records)+1 || len(env.Nonce) != aead.NonceSize() {
			return nil, errors.New("invalid capture sequence")
		}
		plain, err := aead.Open(nil, env.Nonce, env.Data, []byte(Format+":"+strconv.Itoa(env.Sequence)))
		if err != nil {
			return nil, errors.New("capture authentication failed")
		}
		var record Record
		err = json.Unmarshal(plain, &record)
		clear(plain)
		if err != nil || (record.Kind != "exchange" && record.Kind != "end") {
			return nil, errors.New("invalid capture record")
		}
		records = append(records, record)
		ended = record.Kind == "end"
	}
	if scanner.Err() != nil || !ended {
		return nil, errors.New("capture incomplete; preserve the encrypted file")
	}
	return records, nil
}
