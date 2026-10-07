package mediadl

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestParseWaveformString(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []int
	}{
		{name: "empty", data: "", want: nil},
		{name: "empty list", data: "[]", want: nil},
		{name: "bare list", data: "[0, 0.25, 0.5, 1]", want: []int{0, 64, 128, 256}},
		{
			name: "object with amplitudes",
			data: `{"amplitudes":[0,0.5,1],"sampling_frequency":9}`,
			want: []int{0, 128, 256},
		},
		{name: "out of range values are clamped", data: "[-1, 2]", want: []int{0, 256}},
		{name: "not json", data: "definitely not json", want: nil},
		{name: "wrong element type", data: `["a","b"]`, want: nil},
		{name: "object without amplitudes", data: `{"sampling_frequency":9}`, want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ParseWaveformString(test.data)
			if !slices.Equal(got, test.want) {
				t.Errorf("ParseWaveformString(%q) = %v, want %v", test.data, got, test.want)
			}
		})
	}
}

func TestParseWaveformList(t *testing.T) {
	tests := []struct {
		name string
		data []any
		want []int
	}{
		{name: "nil", data: nil, want: nil},
		{name: "floats", data: []any{0.0, 0.5, 1.0}, want: []int{0, 128, 256}},
		{name: "json numbers", data: []any{json.Number("0.25"), json.Number("1")}, want: []int{64, 256}},
		{name: "unexpected type", data: []any{"loud"}, want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ParseWaveformList(test.data)
			if !slices.Equal(got, test.want) {
				t.Errorf("ParseWaveformList(%v) = %v, want %v", test.data, got, test.want)
			}
		})
	}
}

// The outgoing direction in reuploadFileToMeta divides by WaveformScale, so a waveform
// bridged from Meta and sent back should survive the round trip.
func TestWaveformRoundTrip(t *testing.T) {
	original := []float64{0, 0.25, 0.5, 0.75, 1}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal waveform: %v", err)
	}
	waveform := ParseWaveformString(string(encoded))
	if len(waveform) != len(original) {
		t.Fatalf("Got %d samples, want %d", len(waveform), len(original))
	}
	for i, amp := range waveform {
		back := max(min(float64(amp)/WaveformScale, 1.0), 0.0)
		if back != original[i] {
			t.Errorf("Sample %d round tripped to %v, want %v", i, back, original[i])
		}
	}
}
