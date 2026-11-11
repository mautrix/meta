// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package mediadl

import (
	"encoding/json"
	"math"
)

// WaveformScale is the value a fully saturated sample is scaled to when converting
// Meta's normalized amplitudes into the integers used in MSC1767 audio content.
// It's the inverse of the divisor used when sending waveforms in reuploadFileToMeta.
const WaveformScale = 256

type waveformObject struct {
	Amplitudes []float64 `json:"amplitudes"`
}

// ParseWaveformString converts the waveform of a Messenger voice message into the
// integer list used in MSC1767 audio content. The table field holds JSON, which has
// been observed as a bare list of amplitudes, and is accepted as an object with an
// amplitudes key too, since that's the shape used when uploading.
//
// It returns nil when there's no waveform or it can't be parsed, in which case the
// message is bridged without one rather than failing.
func ParseWaveformString(data string) []int {
	if data == "" {
		return nil
	}
	var amplitudes []float64
	if err := json.Unmarshal([]byte(data), &amplitudes); err == nil {
		return scaleWaveform(amplitudes)
	}
	var obj waveformObject
	if err := json.Unmarshal([]byte(data), &obj); err == nil {
		return scaleWaveform(obj.Amplitudes)
	}
	return nil
}

// ParseWaveformList converts the waveform of an Instagram voice message into the
// integer list used in MSC1767 audio content. The thread API returns the amplitudes
// already decoded, but without a concrete element type, so anything that isn't a
// number is skipped.
func ParseWaveformList(data []any) []int {
	if len(data) == 0 {
		return nil
	}
	amplitudes := make([]float64, 0, len(data))
	for _, item := range data {
		switch val := item.(type) {
		case float64:
			amplitudes = append(amplitudes, val)
		case int64:
			amplitudes = append(amplitudes, float64(val))
		case json.Number:
			parsed, err := val.Float64()
			if err != nil {
				return nil
			}
			amplitudes = append(amplitudes, parsed)
		default:
			return nil
		}
	}
	return scaleWaveform(amplitudes)
}

// scaleWaveform turns amplitudes normalized to 0-1 into integers scaled to
// WaveformScale, clamping anything outside the expected range.
func scaleWaveform(amplitudes []float64) []int {
	if len(amplitudes) == 0 {
		return nil
	}
	waveform := make([]int, len(amplitudes))
	for i, amplitude := range amplitudes {
		if math.IsNaN(amplitude) {
			amplitude = 0
		}
		waveform[i] = int(math.Round(max(min(amplitude, 1), 0) * WaveformScale))
	}
	return waveform
}
