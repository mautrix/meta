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
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waWa6"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

func (c *Client) PrepareE2EEClient() (*whatsmeow.Client, error) {
	if c == nil {
		return nil, ErrClientIsNil
	} else if c.device == nil {
		return nil, fmt.Errorf("PrepareE2EEClient called without device")
	}
	e2eeClient := whatsmeow.NewClient(c.device, waLog.Zerolog(c.Logger.With().Str("component", "whatsmeow").Logger()))
	e2eeClient.GetClientPayload = c.getClientPayload
	e2eeClient.MessengerConfig = &whatsmeow.MessengerConfig{
		UserAgent:    useragent.UserAgent,
		BaseURL:      c.GetEndpoint("base_url"),
		WebsocketURL: c.GetEndpoint("e2ee_ws_url"),
	}
	e2eeClient.RefreshCAT = c.refreshCAT
	e2eeClient.EnableAutoReconnect = true
	e2eeClient.InitialAutoReconnect = true
	return e2eeClient, nil
}

type refreshCATResponseGraphQL struct {
	types.ErrorResponse
	Ar  int    `json:"__ar,omitempty"`
	LID string `json:"lid,omitempty"`

	Data struct {
		SecureMessageOverWACATQuery types.CryptoAuthToken `json:"secure_message_over_wa_cat_query"`
	} `json:"data"`
	Extensions struct {
		IsFinal bool `json:"is_final"`
	} `json:"extensions"`
}

func (c *Client) refreshCAT(ctx context.Context) error {
	if c == nil {
		return ErrClientIsNil
	}
	c.catRefreshLock.Lock()
	defer c.catRefreshLock.Unlock()
	currentExpiration := time.Unix(c.configs.BrowserConfigTable.MessengerWebInitData.CryptoAuthToken.ExpirationTimeInSeconds, 0)
	if time.Until(currentExpiration) > 23*time.Hour {
		c.unnecessaryCATRequests++
		logEvt := c.Logger.Warn().Time("expiration", currentExpiration).Int("unnecessary_requests", c.unnecessaryCATRequests)
		switch c.unnecessaryCATRequests {
		case 1, 3:
			// Don't throw an error if it's just called again times accidentally
			logEvt.Msg("Not refreshing CAT")
			return nil
		case 2:
			// If it's called twice for no reason, there might actually be a reason, so refresh it again to be safe
			logEvt.Msg("Refreshing CAT unnecessarily")
		default:
			logEvt.Msg("Failing CAT refresh call due to repeated requests")
			// If it's called more than 4 times in total, fail
			return fmt.Errorf("repeated unnecessary CAT refreshes")
		}
	} else {
		c.unnecessaryCATRequests = 0
	}
	c.Logger.Info().Time("prev_expiration", currentExpiration).Msg("Refreshing crypto auth token")
	_, respData, err := c.makeGraphQLRequest(ctx, "MAWCatQuery", struct{}{})
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	var parsedResp refreshCATResponseGraphQL
	err = json.Unmarshal(respData, &parsedResp)
	if err != nil {
		c.Logger.Debug().Str("resp_data", base64.StdEncoding.EncodeToString(respData)).Msg("Response data for failed CAT query")
		return fmt.Errorf("failed to parse response: %w", err)
	} else if len(parsedResp.Data.SecureMessageOverWACATQuery.EncryptedSerializedCat) == 0 {
		c.Logger.Debug().RawJSON("resp_data", respData).Msg("Response data for failed CAT query")
		if parsedResp.ErrorCode != 0 {
			return fmt.Errorf("graphql error in CAT query %w", &parsedResp.ErrorResponse)
		}
		return fmt.Errorf("didn't get CAT in response")
	}
	c.Logger.Info().Msg("Successfully refreshed crypto auth token")
	c.configs.BrowserConfigTable.MessengerWebInitData.CryptoAuthToken = parsedResp.Data.SecureMessageOverWACATQuery
	return nil
}

func (c *Client) getClientPayload() *waWa6.ClientPayload {
	userID, _ := strconv.ParseUint(c.device.ID.User, 10, 64)
	platform := waWa6.ClientPayload_UserAgent_BLUE_WEB
	if !c.Platform.IsMessenger() {
		platform = waWa6.ClientPayload_UserAgent_WEB
	}
	return &waWa6.ClientPayload{
		Device:      proto.Uint32(uint32(c.device.ID.Device)),
		FbCat:       []byte(c.configs.BrowserConfigTable.MessengerWebInitData.CryptoAuthToken.EncryptedSerializedCat),
		FbUserAgent: []byte(useragent.UserAgent),
		Product:     waWa6.ClientPayload_MESSENGER.Enum(),
		Username:    proto.Uint64(userID),

		ConnectReason: waWa6.ClientPayload_USER_ACTIVATED.Enum(),
		ConnectType:   waWa6.ClientPayload_WIFI_UNKNOWN.Enum(),
		Passive:       proto.Bool(false),
		Pull:          proto.Bool(true),
		UserAgent: &waWa6.ClientPayload_UserAgent{
			Device: proto.String(useragent.BrowserName),
			AppVersion: &waWa6.ClientPayload_UserAgent_AppVersion{
				Primary:   proto.Uint32(301),
				Secondary: proto.Uint32(0),
				Tertiary:  proto.Uint32(2),
			},
			LocaleCountryIso31661Alpha2: proto.String("en"),
			LocaleLanguageIso6391:       proto.String("en"),
			//Hardware:                    proto.String("Linux"),
			Manufacturer:  proto.String(useragent.OSName),
			Mcc:           proto.String("000"),
			Mnc:           proto.String("000"),
			OsBuildNumber: proto.String(""),
			OsVersion:     proto.String(""),
			//SimMcc: proto.String("000"),
			//SimMnc: proto.String("000"),

			Platform:       platform.Enum(),
			ReleaseChannel: waWa6.ClientPayload_UserAgent_DEBUG.Enum(),
		},
	}
}
