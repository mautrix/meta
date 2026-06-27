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

package slidetypes

import (
	"go.mau.fi/util/jsontime"
)

type StreamControllerPayload struct {
	Data struct {
		XDTDirectRealtimeEvent struct {
			Event string `json:"event"` // usually "patch"
			Data  []struct {
				Op    string `json:"op"` // usually "add"
				Path  string `json:"path"`
				Value string `json:"value"` // json-encoded TypingNotification
			} `json:"data"`
		} `json:"xdt_direct_realtime_event"`
	} `json:"data"`
}

type TypingNotification struct {
	ThreadID string `json:"-"` // This is parsed from the path above

	Timestamp      jsontime.UnixMicro `json:"timestamp"`
	SenderID       int64              `json:"sender_id"`
	TTL            int                `json:"ttl"`
	ActivityStatus int                `json:"activity_status"`
	Attribution    any                `json:"attribution"`
}
