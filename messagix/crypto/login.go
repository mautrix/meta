package crypto

import (
	"math"
	"math/rand"
	"strings"
	"time"
)

var b64Charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

const (
	b                      = rune(32)
	f                      = rune(48)
	g                      = rune(57)
	h                      = rune(58)
	i                      = rune(63)
	d                      = rune(65)
	e                      = rune(90)
	j                      = rune(91)
	k                      = rune(94)
	stdDevKeyPressDuration = 20
	avgKeyPressDuration    = 100
	avgTypingInterval      = 150
	stdDevTypingInterval   = 50
)

type ABTestData struct {
	result []float64
	l      float64
	m      float64
	n      float64
	o      float64
	q      int64
	s      []byte
	t      []byte
	u      []byte
	r      []byte
}

func NewABTestData() *ABTestData {
	return &ABTestData{
		result: make([]float64, 0),
		l:      0,
		m:      0,
		n:      0,
		o:      0,
		s:      make([]byte, 20),
		t:      make([]byte, 10),
		u:      make([]byte, 10),
		r:      make([]byte, 10),
	}
}

func humanLikeKeyPressInterval() int64 {
	interval := int64(rand.NormFloat64()*stdDevTypingInterval + avgTypingInterval)
	if rand.Float64() < 0.05 {
		pause := int64(rand.Float64()*400) + 800
		interval += pause
	}
	return interval
}

func humanLikeKeyPressDuration() int64 {
	return int64(rand.NormFloat64()*stdDevKeyPressDuration + avgKeyPressDuration)
}

func (ab *ABTestData) GenerateAbTestData(strings []string) string {
	var prevTime int64

	for _, s := range strings {
		for _, char := range []byte(s) {
			keyCode := getKeyCode(string(char))
			keyCode %= 128
			ab.mimickTernary(rune(keyCode))

			if prevTime == 0 {
				prevTime = time.Now().UnixMilli()
			} else {
				prevTime += humanLikeKeyPressInterval()
			}
			timeNow := prevTime

			keyDuration := humanLikeKeyPressDuration()
			timeNow2 := timeNow + keyDuration

			if ab.q != 0 {
				r := timeNow - ab.q
				ab.mimickTernary2(r, rune(keyCode))
			}
			ab.q = timeNow2

			b := timeNow2 - timeNow
			if b >= 50 && b < 250 {
				ab.r[(b-50)/20]++
			}
		}
	}

	concatted := append(ab.s, ab.t...)
	concatted = append(concatted, ab.u...)
	transformed := transform(concatted)

	transformedR := transform(ab.r)

	ab.result = append(ab.result, transformed...)
	ab.result = append(ab.result, transformedR...)
	ab.result = append(ab.result, []float64{ab.l / 2, ab.m / 2, ab.n / 2, ab.o / 2}...)
	return ab.encodeResult()
}

func (ab *ABTestData) encodeResult() string {
	var result strings.Builder
	for _, val := range ab.result {
		intVal := int(val)
		if intVal > 63 || intVal <= 0 {
			intVal = 0
		}

		charIndex := intVal & 63
		if charIndex < len(b64Charset) {
			result.WriteByte(b64Charset[charIndex])
		}
	}
	return result.String()
}

func transform(input []uint8) []float64 {
	maxVal := float64(findMax(input))
	output := make([]float64, len(input))
	for i, val := range input {
		if maxVal != 0 {
			output[i] = math.Floor((float64(val) * 63.0) / maxVal)
		} else {
			output[i] = float64(val)
		}
	}
	return output
}

func findMax(input []uint8) uint8 {
	maxVal := input[0]
	for _, val := range input[1:] {
		if val > maxVal {
			maxVal = val
		}
	}
	return maxVal
}

func (ab *ABTestData) mimickTernary(keyCode rune) {
	if keyCode >= d && keyCode <= e || keyCode == b {
		ab.l++
	} else if keyCode >= f && keyCode <= g {
		ab.m++
	} else if (keyCode >= h && keyCode <= i) || (keyCode >= j && keyCode <= k) {
		ab.n++
	} else {
		ab.o++
	}
}

func (ab *ABTestData) mimickTernary2(r int64, keyCode rune) {
	if r >= 0 {
		if keyCode >= d && keyCode <= e || keyCode == b {
			if r < 400 {
				ab.s[r/20]++
			} else if r < 1000 {
				ab.t[(r-400)/60]++
			} else if r < 3000 {
				ab.u[(r-1000)/200]++
			}
		}
	}
}

func getKeyCode(key string) byte {
	keyCodeMap := map[rune]byte{
		'@': 50,
		'.': 190,
		'#': 163,
		'$': 164,
		'%': 165,
	}
	charRune := rune(key[0])

	if keyCode, exists := keyCodeMap[charRune]; exists {
		return keyCode
	}

	return byte(strings.ToUpper(key)[0])
}
