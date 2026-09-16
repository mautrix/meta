package bloks

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"regexp"

	"github.com/rs/zerolog"
	"go.mau.fi/util/redact"
)

var bloksRedactPolicy = makeBloksRedactPolicy()

// bloksScript matches a Lispy Bloks script string: a funcall opening with the
// function reference, in either minified or unminified form.
var bloksScript = regexp.MustCompile(`^[\t ,]*\([\t ,]*[\w.]+[\t )]`)

func makeBloksRedactPolicy() redact.Policy {
	p := redact.MakeDefaultPolicy()
	p.KeepProse = true
	// Bloks IDs and script indices are structural; keep enough digits for
	// seconds-since-epoch timestamps before treating a number as sensitive.
	p.MaxDigits = 10
	p.KeepPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^com\.bloks\.`),
		regexp.MustCompile(`^(CAA|caa|INTERNAL)_`),
		regexp.MustCompile(`^i:(caa|com\.bloks)\.`),
		regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,16}dp$`),
	}
	p.KeepURLPathPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^rsrc\.php/`),
	}
	// Bloks scripts are program logic, not user data, and their function names
	// and arguments are what make a redacted payload readable. Keep them whole;
	// HashPatterns still scrubs any PII embedded in the script.
	p.ClassifyString = func(value string) redact.Decision {
		if bloksScript.MatchString(value) {
			return redact.Keep
		}
		return redact.Auto
	}
	return p
}

func LogRedactedBundle(log *zerolog.Logger, appID string, rawBundle []byte) error {
	marshaled, err := bloksRedactPolicy.JSON(rawBundle)
	if err != nil {
		return err
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
