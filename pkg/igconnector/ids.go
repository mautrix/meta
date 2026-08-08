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
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (ic *IGClient) selfEventSender() bridgev2.EventSender {
	return bridgev2.EventSender{
		IsFromMe:    true,
		Sender:      networkid.UserID(ic.UserLogin.ID),
		SenderLogin: ic.UserLogin.ID,
	}
}

func (ic *IGClient) makeEventSender(id int64) bridgev2.EventSender {
	return bridgev2.EventSender{
		IsFromMe:    metaid.MakeUserLoginID(id) == ic.UserLogin.ID,
		Sender:      metaid.MakeUserID(id),
		SenderLogin: metaid.MakeUserLoginID(id),
	}
}

func (ic *IGClient) makeUncertainPortalKey(threadID int64) networkid.PortalKey {
	return networkid.PortalKey{ID: metaid.MakeFBPortalID(threadID), Receiver: ic.UserLogin.ID}
}

func (ic *IGClient) makePortalKey(threadID int64, isGroup bool) networkid.PortalKey {
	key := networkid.PortalKey{ID: metaid.MakeFBPortalID(threadID)}
	if ic.Main.Bridge.Config.SplitPortals || !isGroup {
		key.Receiver = ic.UserLogin.ID
	}
	return key
}
