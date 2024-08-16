package debug

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-colorable"
	zerolog "github.com/rs/zerolog"
)

var colors = map[string]string{
	"text":  "\x1b[38;5;6m%s\x1b[0m",
	"debug": "\x1b[32mDEBUG\x1b[0m",
	"gray":  "\x1b[38;5;8m%s\x1b[0m",
	"info":  "\x1b[38;5;111mINFO\x1b[0m",
	"error": "\x1b[38;5;204mERROR\x1b[0m",
	"fatal": "\x1b[38;5;52mFATAL\x1b[0m",
}

var output = zerolog.ConsoleWriter{
	Out:        colorable.NewColorableStdout(),
	TimeFormat: time.ANSIC,
	FormatLevel: func(i interface{}) string {
		name := fmt.Sprintf("%s", i)
		coloredName := colors[name]
		return coloredName
	},
	FormatMessage: func(i interface{}) string {
		coloredMsg := fmt.Sprintf(colors["text"], i)
		return coloredMsg
	},
	FormatFieldName: func(i interface{}) string {
		name := fmt.Sprintf("%s", i)
		return fmt.Sprintf(colors["gray"], name+"=")
	},
	FormatFieldValue: func(i interface{}) string {
		return fmt.Sprintf("%s", i)
	},
	NoColor: false,
}

func NewLogger() zerolog.Logger {
	return zerolog.New(output).With().Timestamp().Logger()
}

func BeautifyHex(data []byte) string {
	hexStr := hex.EncodeToString(data)
	result := ""
	for i := 0; i < len(hexStr); i += 2 {
		result += hexStr[i:i+2] + " "
	}

	return strings.TrimRight(result, " ")
}
