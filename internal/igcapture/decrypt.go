package capture

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

// Reassemble selects one capture ID from newline-delimited JSON log events.
// Grafana may reorder or duplicate entries; missing or conflicting chunks fail.
// The returned ciphertext must still pass Decode before any plaintext is used.
func Reassemble(input io.Reader, id string) ([]byte, error) {
	if raw, err := hex.DecodeString(id); err != nil || len(raw) != 16 {
		return nil, errors.New("invalid capture ID")
	}
	type event struct {
		ID     string `json:"capture_id"`
		Chunk  int    `json:"capture_chunk"`
		Data   string `json:"capture_data"`
		Chunks int    `json:"capture_chunks"`
		Bytes  int    `json:"capture_bytes"`
		SHA256 string `json:"capture_sha256"`
	}
	const maxChunks = maxFile/chunkSize + 34
	parts := make(map[int][]byte)
	var end event
	total, scanned := 0, 0
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 16<<10)
	for scanner.Scan() {
		scanned += len(scanner.Bytes())
		if scanned > 4*maxFile {
			return nil, errors.New("capture log input too large")
		}
		var e event
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			return nil, errors.New("expected newline-delimited JSON capture logs")
		}
		if e.ID != id {
			continue
		}
		if e.Chunk == 0 {
			if e.Chunks < 1 || e.Chunks > maxChunks || e.Bytes < 1 || e.Bytes > maxFile || len(e.SHA256) != 64 || e.Data != "" || (end.ID != "" && end != e) {
				return nil, errors.New("invalid capture completion")
			}
			end = e
			continue
		}
		part, err := base64.StdEncoding.DecodeString(e.Data)
		if err != nil || len(part) == 0 || len(part) > chunkSize || e.Chunk < 1 || e.Chunk > maxChunks || e.Chunks != 0 || e.SHA256 != "" || e.Bytes != 0 {
			return nil, errors.New("invalid capture chunk")
		}
		if previous, exists := parts[e.Chunk]; exists {
			if !bytes.Equal(previous, part) {
				return nil, errors.New("conflicting capture chunk")
			}
			continue
		}
		total += len(part)
		if total > maxFile {
			return nil, errors.New("capture exceeds output limit")
		}
		parts[e.Chunk] = part
	}
	if scanner.Err() != nil || end.ID == "" || len(parts) != end.Chunks || total != end.Bytes {
		return nil, errors.New("capture log chunks incomplete")
	}
	data := make([]byte, 0, total)
	for i := 1; i <= end.Chunks; i++ {
		part, ok := parts[i]
		if !ok {
			return nil, errors.New("capture chunk missing")
		}
		data = append(data, part...)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != end.SHA256 {
		return nil, errors.New("capture log checksum mismatch")
	}
	return data, nil
}

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
