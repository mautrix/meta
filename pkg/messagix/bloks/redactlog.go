package bloks

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/redact"
)

var bloksRedactPolicy = makeBloksRedactPolicy()

// Bloks scripts are program logic, not user data, and their function names and
// arguments are what make a redacted payload readable. Their literals are only
// scrubbed by HashPatterns, one token at a time, so a match can never span
// tokens or leave the script unparseable.
var bloksScriptLiteralPolicy = makeBloksScriptLiteralPolicy()

// bloksScript matches a Lispy Bloks script string: a funcall opening with the
// function reference, in either minified or unminified form.
var bloksScript = regexp.MustCompile(`^[\t ,]*\([\t ,]*[\w.]+[\t )]`)

// bloksScriptToken matches string literals, then names and keywords, then
// numeric literals, mirroring the script parser.
var bloksScriptToken = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|[A-Za-z_][\w.]*|[0-9.-]+`)

var bloksStringEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

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
	return p
}

func makeBloksScriptLiteralPolicy() redact.Policy {
	p := makeBloksRedactPolicy()
	p.ClassifyString = func(string) redact.Decision {
		return redact.Keep
	}
	return p
}

func redactBloksScript(script string) string {
	return bloksScriptToken.ReplaceAllStringFunc(script, func(token string) string {
		var literal string
		switch first := token[0]; {
		case first == '"':
			literal = unquoteBloksString(token)
		case first == '-' || first == '.' || (first >= '0' && first <= '9'):
			literal = token
		default:
			return token
		}
		masked := bloksScriptLiteralPolicy.String(literal)
		if masked == literal {
			return token
		}
		return `"` + bloksStringEscaper.Replace(masked) + `"`
	})
}

func unquoteBloksString(token string) string {
	var parsed BloksScriptLiteral
	if _, err := parsed.Parse(token, 0); err == nil {
		if value, ok := parsed.Value().(string); ok {
			return value
		}
	}
	return token[1 : len(token)-1]
}

func redactBloksValue(value any) any {
	switch val := value.(type) {
	case string:
		if bloksScript.MatchString(val) {
			return redactBloksScript(val)
		}
	case []any:
		for idx := range val {
			val[idx] = redactBloksValue(val[idx])
		}
		return val
	case map[string]any:
		for key, child := range val {
			if bloksRedactPolicy.SensitiveKeys[strings.ToLower(key)] {
				keyed := any(map[string]any{key: child})
				bloksRedactPolicy.Value(&keyed)
				val[key] = keyed.(map[string]any)[key]
			} else {
				val[key] = redactBloksValue(child)
			}
		}
		return val
	}
	bloksRedactPolicy.Value(&value)
	return value
}

func redactBundle(rawBundle []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawBundle))
	decoder.UseNumber()
	var bundle any
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("parsing bloks payload for redaction: %w", err)
	}
	marshaled, err := json.Marshal(redactBloksValue(bundle))
	if err != nil {
		return nil, fmt.Errorf("marshaling redacted bloks payload: %w", err)
	}
	return marshaled, nil
}

func LogRedactedBundle(log *zerolog.Logger, appID string, rawBundle []byte) error {
	marshaled, err := redactBundle(rawBundle)
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
