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

package textfmt

import (
	"context"
	"strings"
	"unicode/utf16"

	"github.com/rs/zerolog"
	"go.mau.fi/util/random"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
)

type UTF16String []uint16

func NewUTF16String(s string) UTF16String {
	return utf16.Encode([]rune(s))
}

func (u UTF16String) String() string {
	return string(utf16.Decode(u))
}

type wrappedMention struct {
	socket.Mention
	Placeholder string
	OrigText    string
}

func parseMetaMentions(ctx context.Context, text string, mentions []socket.Mention) (string, []*wrappedMention) {
	if len(mentions) == 0 {
		return text, nil
	}
	utf16Text := NewUTF16String(text)
	outList := make([]*wrappedMention, 0, len(mentions))
	prevEnd := 0
	var output strings.Builder
	for _, mention := range mentions {
		if mention.Offset < prevEnd {
			zerolog.Ctx(ctx).Warn().Msg("Ignoring overlapping mentions in message")
			continue
		} else if mention.Offset >= len(utf16Text) {
			zerolog.Ctx(ctx).Warn().Msg("Ignoring mention outside of message")
			continue
		}
		end := mention.Offset + mention.Length
		if end > len(utf16Text) {
			end = len(utf16Text)
		}
		placeholder := random.String(10)
		output.WriteString(utf16Text[prevEnd:mention.Offset].String())
		output.WriteString(placeholder)
		outList = append(outList, &wrappedMention{
			Mention:     mention,
			Placeholder: placeholder,
			OrigText:    utf16Text[mention.Offset:end].String(),
		})
		prevEnd = end
	}
	output.WriteString(utf16Text[prevEnd:].String())
	return output.String(), outList
}
