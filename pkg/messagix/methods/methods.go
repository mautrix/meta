package methods

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/util/exerrors"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

var (
	epochMutex    sync.Mutex
	lastTimestamp int64
	counter       int64
)
var Charset = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz1234567890")
var jsObjectRe = regexp.MustCompile(`(?m)(\s*{|\s*,\s*)\s*([a-zA-Z0-9_]+)\s*:`)

/*
Counter + Mutex logic to ensure unique epoch id for all calls
*/
func GenerateEpochId() int64 {
	epochMutex.Lock()
	defer epochMutex.Unlock()

	timestamp := time.Now().UnixMilli()
	if timestamp == lastTimestamp {
		counter++
	} else {
		lastTimestamp = timestamp
		counter = 0
	}
	id := (timestamp << 22) | (counter << 12) | 42
	return id
}

func RandomInt(min, max int) int64 {
	return int64(min + rand.Intn(max-min+1))
}

func GenerateTimestampString() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func GenerateSessionId() int64 {
	min := int64(2171078810009599)
	max := int64(4613554604867583)
	return rand.Int63n(max-min+1) + min
}

func RandStr(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = Charset[rand.Intn(len(Charset))]
	}
	return string(b)
}

func GenerateWebsessionID(loggedIn bool) string {
	str := ""
	if loggedIn {
		str = RandStr(6) + ":" + RandStr(6) + ":" + RandStr(6)
	} else {
		str = ":" + RandStr(6) + ":" + RandStr(6)
	}
	return strings.ToLower(str)
}

func GenerateTraceId() string {
	uuidHex := strings.ReplaceAll(uuid.NewString(), "-", "")
	decodedHex := exerrors.Must(hex.DecodeString(uuidHex))
	return "#" + base64.RawURLEncoding.EncodeToString(decodedHex)
}

func GenerateMachineId() string {
	return strings.ToLower(RandStr(51))
}

func PreprocessJSObject(s string) string {
	return jsObjectRe.ReplaceAllString(s, "$1 \"$2\":")
}

func NeedUpdateSyncGroups(data *table.LSTable) bool {
	return len(data.LSExecuteFirstBlockForSyncTransaction) > 0 ||
		len(data.LSUpsertSyncGroupThreadsRange) > 0
}

const MetaEpochMS = 1072915200000

func ParseMessageID(messageID string) (int64, error) {
	if !strings.HasPrefix(messageID, "mid.$") || len(messageID) < 10 {
		return 0, fmt.Errorf("invalid message ID prefix")
	}
	rawMessageID, err := base64.RawURLEncoding.DecodeString(messageID[len("mid.$c"):])
	if err != nil {
		return 0, fmt.Errorf("failed to decode message ID: %w", err)
	} else if len(rawMessageID) != 21 {
		return 0, fmt.Errorf("unexpected decoded length %d", len(rawMessageID))
	}
	timestampBuf := make([]byte, 8)
	copy(timestampBuf[3:], rawMessageID[8:13])
	timestamp := binary.BigEndian.Uint64(timestampBuf)
	return int64(timestamp) + MetaEpochMS, nil
}
