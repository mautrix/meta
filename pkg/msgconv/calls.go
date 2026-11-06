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
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

// CallLog describes a call log message ("Missed audio call", "Video call · 2 mins").
type CallLog struct {
	Video   bool
	Missed  bool
	Group   bool
	Ongoing bool
	// Title and Subtitle are the texts Meta shows on the call log card, e.g. "Audio call" and "46 secs".
	Title    string
	Subtitle string
}

func (cl CallLog) defaultTitle() string {
	var parts []string
	if cl.Ongoing {
		parts = append(parts, "Ongoing")
	} else if cl.Missed {
		parts = append(parts, "Missed")
	}
	if cl.Group {
		parts = append(parts, "group")
	}
	if cl.Video {
		parts = append(parts, "video call")
	} else {
		parts = append(parts, "audio call")
	}
	title := strings.Join(parts, " ")
	return strings.ToUpper(title[:1]) + title[1:]
}

// ToMatrix converts the call log into a notice. The structured info is in the fi.mau.meta.call field,
// so clients can render it as a call event.
func (cl CallLog) ToMatrix() *bridgev2.ConvertedMessagePart {
	title := strings.TrimSpace(cl.Title)
	if title == "" {
		title = cl.defaultTitle()
	}
	subtitle := strings.TrimSpace(cl.Subtitle)
	body := title
	if subtitle != "" {
		body = title + " · " + subtitle
	}
	media := "audio"
	if cl.Video {
		media = "video"
	}
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    body,
		},
		Extra: map[string]any{
			"fi.mau.meta.call": map[string]any{
				"media":    media,
				"missed":   cl.Missed,
				"group":    cl.Group,
				"ongoing":  cl.Ongoing,
				"title":    title,
				"subtitle": subtitle,
			},
		},
	}
}

// CallLogFromText guesses a call log's details from its text, for call logs that don't have a structured type.
func CallLogFromText(title, subtitle string) CallLog {
	lower := strings.ToLower(title)
	return CallLog{
		Video:    strings.Contains(lower, "video"),
		Missed:   strings.Contains(lower, "missed"),
		Group:    strings.Contains(lower, "group"),
		Ongoing:  strings.Contains(lower, "ongoing") || strings.Contains(lower, "started"),
		Title:    title,
		Subtitle: subtitle,
	}
}
