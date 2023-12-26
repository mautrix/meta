package methods

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xzer/messagix/table"
	"github.com/google/uuid"
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

	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
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
	return strconv.Itoa(int(time.Now().UnixMilli()))
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
	decodedHex, err := hex.DecodeString(uuidHex)
	if err != nil {
		log.Fatalf("failed to decode traceId string: %s", err)
	}
	return "#" + base64.RawURLEncoding.EncodeToString(decodedHex)
}

func GenerateMachineId() string {
	return strings.ToLower(RandStr(51))
}

func PreprocessJSObject(s string) string {
	return jsObjectRe.ReplaceAllString(s, "$1 \"$2\":")
}

func InterfaceToStructJSON(data interface{}, i interface{}) error {
	b, err := json.Marshal(&data)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, &i)
}

func NeedUpdateSyncGroups(data table.LSTable) bool {
	return len(data.LSExecuteFirstBlockForSyncTransaction) > 0 ||
	len(data.LSUpsertSyncGroupThreadsRange) > 0
}