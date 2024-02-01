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

package messagix

import (
	"strconv"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func (c *Client) PrepareE2EEClient() *whatsmeow.Client {
	if c.device == nil {
		panic("PrepareE2EEClient called without device")
	}
	e2eeClient := whatsmeow.NewClient(c.device, waLog.Zerolog(c.Logger.With().Str("component", "whatsmeow").Logger()))
	e2eeClient.GetClientPayload = c.getClientPayload
	e2eeClient.MessengerConfig = &whatsmeow.MessengerConfig{
		UserAgent: UserAgent,
		BaseURL:   c.getEndpoint("base_url"),
	}
	return e2eeClient
}

func (c *Client) getClientPayload() *waProto.ClientPayload {
	userID, _ := strconv.ParseUint(c.device.ID.User, 10, 64)
	return &waProto.ClientPayload{
		Device:      proto.Uint32(uint32(c.device.ID.Device)),
		FbCat:       []byte(c.configs.browserConfigTable.MessengerWebInitData.CryptoAuthToken.EncryptedSerializedCat),
		FbUserAgent: []byte(UserAgent),
		Product:     waProto.ClientPayload_MESSENGER.Enum(),
		Username:    proto.Uint64(userID),

		ConnectReason: waProto.ClientPayload_USER_ACTIVATED.Enum(),
		ConnectType:   waProto.ClientPayload_WIFI_UNKNOWN.Enum(),
		Passive:       proto.Bool(false),
		Pull:          proto.Bool(true),
		UserAgent: &waProto.ClientPayload_UserAgent{
			Device: proto.String("Firefox"),
			AppVersion: &waProto.ClientPayload_UserAgent_AppVersion{
				Primary:   proto.Uint32(301),
				Secondary: proto.Uint32(0),
				Tertiary:  proto.Uint32(2),
			},
			LocaleCountryIso31661Alpha2: proto.String("en"),
			LocaleLanguageIso6391:       proto.String("en"),
			//Hardware:                    proto.String("Linux"),
			Manufacturer:  proto.String("Linux"),
			Mcc:           proto.String("000"),
			Mnc:           proto.String("000"),
			OsBuildNumber: proto.String("6.0.0"),
			OsVersion:     proto.String("6.0.0"),
			//SimMcc: proto.String("000"),
			//SimMnc: proto.String("000"),

			Platform:       waProto.ClientPayload_UserAgent_WEB.Enum(), // or BLUE_WEB?
			ReleaseChannel: waProto.ClientPayload_UserAgent_DEBUG.Enum(),
		},
	}
}
