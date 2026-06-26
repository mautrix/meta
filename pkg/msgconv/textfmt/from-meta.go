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

package textfmt

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type GetBasicUserInfoFunc func(ctx context.Context, user networkid.UserID) (id.UserID, string, error)

func MetaToMatrixText(
	ctx context.Context,
	text string,
	mentions []socket.Mention,
	getBasicUserInfo GetBasicUserInfoFunc,
) (content *event.MessageEventContent) {
	origText := text
	text, wrappedMentions := parseMetaMentions(ctx, text, mentions)
	html := metaFormatToHTML(text)
	matrixMentions := &event.Mentions{}
	for _, mention := range wrappedMentions {
		mentionHTML := event.TextToHTML(mention.OrigText)
		switch mention.Type {
		case socket.MentionTypePerson:
			mxid, displayname, err := getBasicUserInfo(ctx, metaid.MakeUserID(mention.ID))
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to get basic user info for mention")
			} else {
				mentionHTML = fmt.Sprintf(`<a href="%s">%s</a>`, mxid.URI().MatrixToURL(), displayname)
			}
		case socket.MentionTypeThread:
			matrixMentions.Room = true
		}
		html = strings.Replace(html, mention.Placeholder, mentionHTML, 1)
	}
	if html == event.TextToHTML(origText) {
		return &event.MessageEventContent{
			MsgType:  event.MsgText,
			Body:     origText,
			Mentions: matrixMentions,
		}
	}
	return &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          origText,
		Format:        event.FormatHTML,
		FormattedBody: html,
		Mentions:      matrixMentions,
	}
}
