package crypto

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func denseCompressedString(indices []int) string {
	if len(indices) == 0 {
		return ""
	}
	dense := make([]int, slices.Max(indices)+1)
	for _, index := range indices {
		dense[index] = 1
	}
	var buf strings.Builder
	buf.WriteString(strconv.Itoa(dense[0]))
	count := 1
	for i := 1; i < len(dense); i++ {
		if dense[i] == dense[i-1] {
			count++
		} else {
			buf.WriteString(encodeRunLength(count))
			count = 1
		}
	}
	buf.WriteString(encodeRunLength(count))
	return encodeBase64(buf.String())
}

func compressedString(indices []int) string {
	return NewBitmap().Update(indices).ToCompressedString().CompressedStr
}

func TestBitmapCompressedString(t *testing.T) {
	cases := []struct {
		indices  []int
		expected string
	}{
		{nil, ""},
		{[]int{1}, "o"},
		{[]int{0}, "M"},
		{[]int{0, 1, 2, 3}, "A"},
		{[]int{5, 3, 3, 9, 1, 5}, "vK"},
		{[]int{7, 8, 9, 10, 20, 21, 63, 64, 65}, "789gam"},
		{[]int{131071, 2, 65536, 2}, "a00f_Zw03__E"},
	}
	for _, c := range cases {
		if got := denseCompressedString(c.indices); got != c.expected {
			t.Errorf("dense encoding of %v = %q, expected %q", c.indices, got, c.expected)
		}
		if got := compressedString(c.indices); got != c.expected {
			t.Errorf("encoding of %v = %q, expected %q", c.indices, got, c.expected)
		}
	}
}

func TestBitmapMatchesDenseEncoding(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 1000 {
		maxIndex := 1 + rng.IntN(1<<rng.IntN(18))
		indices := make([]int, rng.IntN(500))
		for i := range indices {
			indices[i] = rng.IntN(maxIndex)
		}
		expected := denseCompressedString(indices)
		if got := compressedString(indices); got != expected {
			t.Fatalf("encoding of %v = %q, expected %q", indices, got, expected)
		}
	}
}

func TestBitmapUpdateAfterAppend(t *testing.T) {
	b := NewBitmap().Update([]int{4, 2}).ToCompressedString()
	initial := b.CompressedStr
	if got := b.Update([]int{2, 4}).ToCompressedString().CompressedStr; got != initial {
		t.Errorf("updating with existing indices changed the encoding from %q to %q", initial, got)
	}
	if got := b.Update([]int{9}).ToCompressedString().CompressedStr; got != "bAw" {
		t.Errorf("encoding after adding an index = %q, expected %q", got, "bAw")
	}

	b.BMap = append(b.BMap, 30, 2)
	if got, expected := compressedString(b.BMap), compressedString([]int{2, 4, 9, 30}); got != expected {
		t.Errorf("encoding after appending raw indices = %q, expected %q", got, expected)
	}
}
