package crypto

import (
	"bytes"
	"slices"
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
	prevLen := len(b.BMap)
	b.BMap = append(b.BMap, data...)
	slices.Sort(b.BMap)
	b.BMap = slices.Compact(b.BMap)
	if len(b.BMap) != prevLen {
		b.CompressedStr = ""
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
	if b.BMap[0] == 0 {
		buf.WriteByte('1')
	} else {
		buf.WriteByte('0')
	}
	prevIndex := -1
	setRun := 0
	for _, index := range b.BMap {
		if index != prevIndex+1 {
			if setRun > 0 {
				buf.WriteString(encodeRunLength(setRun))
			}
			buf.WriteString(encodeRunLength(index - prevIndex - 1))
			setRun = 0
		}
		setRun++
		prevIndex = index
	}
	buf.WriteString(encodeRunLength(setRun))

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
