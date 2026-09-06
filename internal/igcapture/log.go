package capture

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

const chunkSize = 4096 // Base64 plus metadata stays below 8 KiB per log event.

type logOutput struct {
	log     zerolog.Logger
	digest  hash.Hash
	chunks  int
	written int
	closed  bool
}

func newLogOutput(log zerolog.Logger, source string) (*logOutput, error) {
	if !log.Info().Enabled() {
		return nil, errors.New("capture info logging unavailable")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	return &logOutput{
		log:    log.With().Str("capture_id", hex.EncodeToString(id[:])).Str("capture_source", source).Logger(),
		digest: sha256.New(),
	}, nil
}

func (w *logOutput) Write(data []byte) (int, error) {
	if w.closed || len(data) > maxFile-w.written {
		return 0, errors.New("capture log output limit")
	}
	n := len(data)
	for len(data) > 0 {
		part := data[:min(len(data), chunkSize)]
		w.chunks++
		w.log.Info().Int("capture_chunk", w.chunks).
			Str("capture_data", base64.StdEncoding.EncodeToString(part)).
			Msg("Temporary encrypted login capture chunk")
		_, _ = w.digest.Write(part)
		w.written += len(part)
		data = data[len(part):]
	}
	return n, nil
}

func (w *logOutput) Sync() error { return nil }

func (w *logOutput) Close() error {
	if !w.closed {
		w.closed = true
		// This confirms emission, not ingestion. Decode still requires an authenticated end.
		w.log.Info().Int("capture_chunks", w.chunks).Int("capture_bytes", w.written).
			Str("capture_sha256", hex.EncodeToString(w.digest.Sum(nil))).
			Msg("Temporary encrypted login capture export complete")
	}
	return nil
}

// ExportExisting only reads the old fixed filename. It never claims a new login,
// deletes evidence or prints unvalidated file contents. Only the previous scoped
// collector created this file; no arbitrary path or user input is accepted.
func ExportExisting(log zerolog.Logger) {
	deadline, err := time.Parse(time.RFC3339, temporaryExpires)
	if err != nil || !time.Now().Before(deadline) {
		return
	}
	err = exportExisting(temporaryFilename, log)
	if errors.Is(err, os.ErrNotExist) {
		log.Info().Msg("Temporary encrypted login capture file absent; next scoped login will emit chunks")
	} else if err != nil {
		log.Warn().Msg("Temporary encrypted login capture file not exported; next scoped login will emit chunks")
	}
}

func exportExisting(path string, log zerolog.Logger) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFile {
		return errors.New("invalid capture file")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil || len(data) > maxFile {
		return errors.New("capture read failed")
	}
	defer clear(data)
	// Re-encode only schema-checked binary fields. Unknown strings, trailing bytes
	// or partial records cannot turn this into a plaintext file-to-log exporter.
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) < 2 || len(lines) > 34 {
		return errors.New("invalid capture record count")
	}
	strict := func(data []byte, target any) bool {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		return dec.Decode(target) == nil && dec.Decode(new(any)) == io.EOF
	}
	var header Header
	if !strict(lines[0], &header) || header.Format != Format || len(header.Key) != 384 {
		return errors.New("invalid capture header")
	}
	var canonical bytes.Buffer
	enc := json.NewEncoder(&canonical)
	_ = enc.Encode(header)
	for i, line := range lines[1:] {
		var env Envelope
		if !strict(line, &env) || env.Sequence != i+1 || len(env.Nonce) != 12 || len(env.Data) < 16 {
			return errors.New("invalid capture envelope")
		}
		_ = enc.Encode(env)
	}
	w, err := newLogOutput(log, "existing_file")
	if err != nil {
		return err
	}
	if _, err = w.Write(canonical.Bytes()); err != nil {
		return err
	}
	return w.Close()
}
