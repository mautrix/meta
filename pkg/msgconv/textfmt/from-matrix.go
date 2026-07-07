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
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type MatrixHTMLParser struct {
	f *format.HTMLParser
	b *bridgev2.Bridge
}

func NewMatrixParser(b *bridgev2.Bridge) *MatrixHTMLParser {
	mhp := &MatrixHTMLParser{b: b}
	mhp.f = &format.HTMLParser{
		TabsToSpaces:   4,
		Newline:        "\n",
		HorizontalLine: "\n---\n",
		PillConverter:  mhp.convertPill,
		BoldConverter: func(text string, ctx format.Context) string {
			return "*" + text + "*"
		},
		ItalicConverter: func(text string, ctx format.Context) string {
			return "_" + text + "_"
		},
		StrikethroughConverter: func(text string, ctx format.Context) string {
			return "~" + text + "~"
		},
		MonospaceConverter: func(text string, ctx format.Context) string {
			return "`" + text + "`"
		},
		MonospaceBlockConverter: func(code, language string, ctx format.Context) string {
			if metaCodeBlockLanguages[language] {
				return "```" + language + "\n" + code + "\n```"
			}
			return "```\n" + code + "\n```"
		},
	}
	return mhp
}

func (mhp *MatrixHTMLParser) Parse(ctx context.Context, content *event.MessageEventContent, portal *bridgev2.Portal) (string, socket.Mentions) {
	if content.Format != event.FormatHTML {
		return content.Body, nil
	}
	mentions := make([]*MetaMention, 0)

	parseCtx := format.NewContext(ctx)
	parseCtx.ReturnData["mentions"] = &mentions
	parseCtx.ReturnData["portal"] = portal
	parsed := mhp.f.Parse(content.FormattedBody, parseCtx)

	var socketMentions socket.Mentions

	for _, mention := range mentions {
		mentionIndex := strings.Index(parsed, mention.Locator)
		if mentionIndex == -1 {
			zerolog.Ctx(ctx).Warn().Any("mention", mention).Msg("Mention not found in parsed body")
			continue
		}

		parsed = parsed[:mentionIndex] + "@" + mention.Name + parsed[mentionIndex+len(mention.Locator):]

		socketMentions = append(socketMentions, socket.Mention{
			ID:     mention.UserID,
			Offset: mentionIndex,
			Length: len("@" + mention.Name),
			Type:   socket.MentionTypePerson,
		})
	}

	return parsed, socketMentions
}

func (mhp *MatrixHTMLParser) ParseWhatsApp(ctx context.Context, content *event.MessageEventContent, portal *bridgev2.Portal) (string, []string) {
	if content.Format != event.FormatHTML {
		return content.Body, nil
	}
	mentions := make([]string, 0)

	parseCtx := format.NewContext(ctx)
	parseCtx.ReturnData["waMentions"] = &mentions
	parseCtx.ReturnData["portal"] = portal
	parsed := mhp.f.Parse(content.FormattedBody, parseCtx)

	return parsed, mentions
}

const mentionLocator = "meta_mention_"

type MetaMention struct {
	Locator string
	Name    string
	UserID  int64
}

func NewMetaMention(userID int64, name string) *MetaMention {
	return &MetaMention{
		Locator: mentionLocator + random.String(16),
		Name:    name,
		UserID:  userID,
	}
}

func (mhp *MatrixHTMLParser) convertPill(displayname, mxid, eventID string, ctx format.Context) string {
	if len(mxid) == 0 || mxid[0] != '@' {
		return format.DefaultPillConverter(displayname, mxid, eventID, ctx)
	}
	var userID int64
	var username string
	ghost, err := mhp.b.GetGhostByMXID(ctx.Ctx, id.UserID(mxid))
	if err != nil {
		zerolog.Ctx(ctx.Ctx).Err(err).Str("mxid", mxid).Msg("Failed to get user for mention")
		return displayname
	} else if ghost != nil {
		username = ghost.Metadata.(*metaid.GhostMetadata).Username
		if username == "" {
			username = ghost.Name
		}
		userID = metaid.ParseUserID(ghost.ID)
	} else if user, err := mhp.b.GetExistingUserByMXID(ctx.Ctx, id.UserID(mxid)); err != nil {
		zerolog.Ctx(ctx.Ctx).Err(err).Str("mxid", mxid).Msg("Failed to get user for mention")
		return displayname
	} else if user != nil {
		portal := ctx.ReturnData["portal"].(*bridgev2.Portal)
		login, _, _ := portal.FindPreferredLogin(ctx.Ctx, user, false)
		if login == nil {
			return displayname
		}
		userID = metaid.ParseUserLoginID(login.ID)
		if login.Metadata.(*metaid.UserLoginMetadata).Platform.IsMessenger() || login.RemoteProfile.Username == "" {
			username = login.RemoteProfile.Name
		} else {
			username = login.RemoteProfile.Username
		}
	} else {
		return displayname
	}
	if waMentions, ok := ctx.ReturnData["waMentions"].(*[]string); ok {
		mentionedJID := types.NewJID(strconv.FormatInt(userID, 10), types.MessengerServer).String()
		*waMentions = append(*waMentions, mentionedJID)
		return "@" + mentionedJID
	}
	mention := NewMetaMention(userID, username)
	mentions := ctx.ReturnData["mentions"].(*[]*MetaMention)
	*mentions = append(*mentions, mention)
	return mention.Locator
}
