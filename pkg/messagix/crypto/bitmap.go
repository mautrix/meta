package crypto

import (
	"bytes"
	"strconv"
	"strings"
)

const charMap = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_"

type Bitmap struct {
	BMap          []int  `json:"-"`
	CompressedStr string `json:"compressed_str"`
}

func NewBitmap() *Bitmap {
	return &Bitmap{BMap: []int{}, CompressedStr: ""}
}

func (b *Bitmap) Update(data []int) *Bitmap {
	maxIndex := -1
	for _, index := range data {
		if index > maxIndex {
			maxIndex = index
		}
	}

	if maxIndex >= len(b.BMap) {
		expanded := make([]int, maxIndex+1)
		copy(expanded, b.BMap)
		b.BMap = expanded
	}

	for _, index := range data {
		if b.BMap[index] == 0 && b.CompressedStr != "" {
			b.CompressedStr = ""
		}
		b.BMap[index] = 1
	}

	return b
}

func (b *Bitmap) ToCompressedString() *Bitmap {
	if len(b.BMap) == 0 {
		return b
	}

	if b.CompressedStr != "" {
		return b
	}

	var buf bytes.Buffer
	count := 1
	lastValue := b.BMap[0]
	buf.WriteString(strconv.Itoa(lastValue))
	for i := 1; i < len(b.BMap); i++ {
		currentValue := b.BMap[i]
		if currentValue == lastValue {
			count++
		} else {
			buf.WriteString(encodeRunLength(count))
			lastValue = currentValue
			count = 1
		}
	}

	if count > 0 {
		buf.WriteString(encodeRunLength(count))
	}

	compressedStr := encodeBase64(buf.String())
	b.CompressedStr = compressedStr
	return b
}

func encodeRunLength(num int) string {
	binaryStr := strconv.FormatInt(int64(num), 2)
	var buf bytes.Buffer
	buf.WriteString(strings.Repeat("0", len(binaryStr)-1))
	buf.WriteString(binaryStr)
	return buf.String()
}

// CUSTOM "BASE64" IMPLEMENTATION, DON'T CHANGE
// TODO change
func encodeBase64(binaryStr string) string {
	for len(binaryStr)%6 != 0 {
		binaryStr += "0"
	}
	var result string
	for i := 0; i < len(binaryStr); i += 6 {
		val, _ := strconv.ParseInt(binaryStr[i:i+6], 2, 64)
		result += string(charMap[val])
	}
	return result
}
