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
	"fmt"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/whatsmeow/binary/armadillo/waCommon"
	"go.mau.fi/whatsmeow/binary/armadillo/waConsumerApplication"
)

func (mc *MessageConverter) ToWhatsApp(ctx context.Context, evt *event.Event, content *event.MessageEventContent, relaybotFormatted bool) (*waConsumerApplication.ConsumerApplication, error) {
	if content.MsgType == event.MsgEmote && !relaybotFormatted {
		content.Body = "/me " + content.Body
		if content.FormattedBody != "" {
			content.FormattedBody = "/me " + content.FormattedBody
		}
	}
	var waContent waConsumerApplication.ConsumerApplication_Content
	switch content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		// TODO mentions
		waContent.Content = &waConsumerApplication.ConsumerApplication_Content_MessageText{
			MessageText: &waCommon.MessageText{
				Text: content.Body,
			},
		}
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		fallthrough
	case event.MsgLocation:
		fallthrough
	default:
		return nil, fmt.Errorf("%w %s", ErrUnsupportedMsgType, content.MsgType)
	}
	return &waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
				Content: &waContent,
			},
		},
		Metadata: nil,
	}, nil
}
