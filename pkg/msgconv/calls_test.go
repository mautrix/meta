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

package msgconv

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waArmadilloXMA"
	"google.golang.org/protobuf/proto"
	"maunium.net/go/mautrix/event"
)

func TestWACallLogToMatrix(t *testing.T) {
	part := waCallLogToMatrix(&waArmadilloXMA.ExtendedContentMessage{
		TargetType:   waArmadilloXMA.ExtendedContentMessage_RTC_MISSED_VIDEO_CALL.Enum(),
		TitleText:    proto.String("Missed video call"),
		SubtitleText: proto.String("17:11"),
	})
	if part == nil {
		t.Fatal("expected a call log part")
	}
	if part.Content.MsgType != event.MsgNotice || part.Content.Body != "Missed video call · 17:11" {
		t.Errorf("unexpected content: %s %q", part.Content.MsgType, part.Content.Body)
	}
	call := part.Extra["fi.mau.meta.call"].(map[string]any)
	if call["media"] != "video" || call["missed"] != true || call["group"] != false || call["subtitle"] != "17:11" {
		t.Errorf("unexpected call info: %v", call)
	}

	part = waCallLogToMatrix(&waArmadilloXMA.ExtendedContentMessage{
		TargetType: waArmadilloXMA.ExtendedContentMessage_RTC_GROUP_AUDIO_CALL.Enum(),
	})
	if part == nil || part.Content.Body != "Group audio call" {
		t.Errorf("expected a default title, got %+v", part)
	}

	if waCallLogToMatrix(&waArmadilloXMA.ExtendedContentMessage{
		TargetType: waArmadilloXMA.ExtendedContentMessage_FB_STORY_REPLY.Enum(),
	}) != nil {
		t.Error("story reply shouldn't be a call log")
	}
}

func TestCallLogFromText(t *testing.T) {
	cl := CallLogFromText("You missed a video chat", "")
	if !cl.Video || !cl.Missed || cl.Group {
		t.Errorf("unexpected call log: %+v", cl)
	}
}
