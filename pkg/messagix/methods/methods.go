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
	"go.mau.fi/util/random"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

var (
	epochMutex    sync.Mutex
	lastTimestamp int64
	counter       int64
)
var jsObjectRe = regexp.MustCompile(`(?m)(\s*{|\s*,\s*)\s*([a-zA-Z0-9_]+)\s*:`)

func GenerateEpochID() int64 {
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

func GenerateTimestampString() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func GenerateSessionID() int64 {
	min := int64(2171078810009599)
	max := int64(4613554604867583)
	return rand.Int63n(max-min+1) + min
}

func GenerateWebsessionID(loggedIn bool) string {
	str := ""
	if loggedIn {
		str = random.String(6) + ":" + random.String(6) + ":" + random.String(6)
	} else {
		str = ":" + random.String(6) + ":" + random.String(6)
	}
	return strings.ToLower(str)
}

func GenerateTraceID() string {
	uuidHex := strings.ReplaceAll(uuid.NewString(), "-", "")
	decodedHex := exerrors.Must(hex.DecodeString(uuidHex))
	return "#" + base64.RawURLEncoding.EncodeToString(decodedHex)
}

func GenerateMachineID() string {
	return strings.ToLower(random.String(51))
}

func PreprocessJSObject(s string) string {
	return jsObjectRe.ReplaceAllString(s, "$1 \"$2\":")
}

func NeedUpdateSyncGroups(data *table.LSTable) bool {
	return len(data.LSExecuteFirstBlockForSyncTransaction) > 0 ||
		len(data.LSUpsertSyncGroupThreadsRange) > 0
}

const MetaEpochMS = 1072915200000

// Message ID format:
// Prefix: mid.$
// Chat type (1 character):
// - c: DM
// - g: Group
// - o, m: ??? (only old FB messages?)
// Body: 21 bytes, base64url encoded without padding
// - 8 bytes: chat ID (for DMs, our ID XOR their ID)
// - 5 bytes: timestamp (milliseconds since Meta epoch)
// - 8 bytes: client-provided transaction ID (aka offline threading ID on FB, client context on IG)
//
// Some old FB messages have also been observed in the formats:
// * 16 bytes of unpadded standard base64 with no prefix
// * "id." + unknown number
// * "mid." + unix milliseconds + 5 or 9 bytes of random? hex

type MetaMessageID struct {
	ChatType rune
	ChatID   int64
	Time     time.Time
	TxnID    int64
}

func (mmi *MetaMessageID) String() string {
	if mmi == nil {
		return ""
	}
	body := make([]byte, 21)
	binary.BigEndian.PutUint64(body[5:13], uint64(mmi.Time.UnixMilli()-MetaEpochMS))
	binary.BigEndian.PutUint64(body[0:8], uint64(mmi.ChatID))
	binary.BigEndian.PutUint64(body[13:21], uint64(mmi.TxnID))
	return fmt.Sprintf("mid.$%c%s", mmi.ChatType, base64.RawURLEncoding.EncodeToString(body))
}

func ParseMessageIDFull(messageID string) (*MetaMessageID, error) {
	if !strings.HasPrefix(messageID, "mid.$") || len(messageID) < 6 {
		return nil, fmt.Errorf("invalid message ID prefix")
	}
	chatType := rune(messageID[5])
	payload, err := base64.RawURLEncoding.DecodeString(messageID[6:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode message ID: %w", err)
	} else if len(payload) != 21 {
		return nil, fmt.Errorf("unexpected decoded length %d", len(payload))
	}
	chatID := int64(binary.BigEndian.Uint64(payload[0:8]))
	timestampBuf := make([]byte, 8)
	copy(timestampBuf[3:], payload[8:13])
	timestamp := int64(binary.BigEndian.Uint64(timestampBuf)) + MetaEpochMS
	txnID := int64(binary.BigEndian.Uint64(payload[13:21]))
	return &MetaMessageID{
		ChatType: chatType,
		ChatID:   chatID,
		Time:     time.UnixMilli(timestamp),
		TxnID:    txnID,
	}, nil
}

func ParseMessageID(messageID string) (int64, error) {
	id, err := ParseMessageIDFull(messageID)
	if err != nil {
		return 0, err
	}
	return id.Time.UnixMilli(), nil
}
