// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
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

package msgconv

import (
	"context"
	"strings"
	"unicode/utf16"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type UTF16String []uint16

func NewUTF16String(s string) UTF16String {
	return utf16.Encode([]rune(s))
}

func (u UTF16String) String() string {
	return string(utf16.Decode(u))
}

func (mc *MessageConverter) MetaToMatrixText(ctx context.Context, text string, rawMentions *socket.MentionData, portal *bridgev2.Portal) (content *event.MessageEventContent) {
	content = &event.MessageEventContent{
		MsgType:  event.MsgText,
		Body:     text,
		Mentions: &event.Mentions{},
	}
	mentions, err := rawMentions.Parse()
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to parse mentions")
	}
	if mentions == nil {
		return
	}
	utf16Text := NewUTF16String(text)
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
		var mentionLink string
		switch mention.Type {
		case socket.MentionTypePerson:
			mxid, _, err := mc.getBasicUserInfo(ctx, metaid.MakeUserID(mention.ID))
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to get user info for mention")
				continue
			}
			content.Mentions.Add(mxid)
			mentionLink = mxid.URI().MatrixToURL()
		case socket.MentionTypeThread:
			// TODO: how does one send thread mentions?
		}
		if mentionLink == "" {
			continue
		}
		output.WriteString(event.TextToHTML(utf16Text[prevEnd:mention.Offset].String()))
		output.WriteString(`<a href="`)
		output.WriteString(mentionLink)
		output.WriteString(`">`)
		output.WriteString(event.TextToHTML(utf16Text[mention.Offset:end].String()))
		output.WriteString(`</a>`)
		prevEnd = end
	}
	output.WriteString(event.TextToHTML(utf16Text[prevEnd:].String()))
	content.Format = event.FormatHTML
	content.FormattedBody = output.String()
	return content
}
