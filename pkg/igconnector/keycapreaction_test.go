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

package igconnector

import (
	"testing"

	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func TestKeycapButton(t *testing.T) {
	follow := &metaid.PostbackButton{Title: "Following", Payload: "payload-b", CTAType: "postback"}
	maybe := &metaid.PostbackButton{Title: "Maybe later", CTAType: "quick_reply"}
	card := &database.Message{Metadata: &metaid.MessageMetadata{
		PostbackButtons: []*metaid.PostbackButton{follow}, QuickReplies: []*metaid.PostbackButton{maybe}}}
	if keycapButton(card, "1️⃣") != follow || keycapButton(card, "2⃣") != maybe {
		t.Error("keycaps must index the combined list")
	}
	for _, emoji := range []string{"3️⃣", "❤️", "🔟", "0️⃣"} {
		if keycapButton(card, emoji) != nil {
			t.Errorf("%q must not activate a button", emoji)
		}
	}
	if keycapButton(nil, "1️⃣") != nil || keycapButton(&database.Message{Metadata: &metaid.MessageMetadata{}}, "1️⃣") != nil {
		t.Error("message without buttons must not match")
	}
}
