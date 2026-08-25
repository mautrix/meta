package bloks

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"
)

func LogRedactedBundle(log *zerolog.Logger, appID string, rawBundle []byte) error {
	var redacted BloksBundle
	if err := json.Unmarshal(rawBundle, &redacted); err != nil {
		return fmt.Errorf("parsing bloks payload for redaction: %w", err)
	}
	redacted.Redact()
	marshaled, err := json.Marshal(redacted)
	if err != nil {
		return fmt.Errorf("marshaling redacted bloks payload: %w", err)
	}
	var compressed bytes.Buffer
	compressor := gzip.NewWriter(&compressed)
	if _, err = compressor.Write(marshaled); err != nil {
		return fmt.Errorf("compressing redacted bloks payload: %w", err)
	}
	if err = compressor.Close(); err != nil {
		return fmt.Errorf("compressing redacted bloks payload: %w", err)
	}
	enc := base64.StdEncoding.AppendEncode(nil, compressed.Bytes())
	log.Debug().Str("bloks_app", appID).Bytes("resp_gz", enc).Msg("Logging redacted Bloks response")
	return nil
}
