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

package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mau.fi/whatsmeow"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var _ bridgev2.PushableNetworkAPI = (*MetaClient)(nil)

var pushCfg = &bridgev2.PushConfig{
	Web: &bridgev2.WebPushConfig{VapidKey: "BIBn3E_rWTci8Xn6P9Xj3btShT85Wdtne0LtwNUyRQ5XjFNkuTq9j4MPAVLvAFhXrUU1A9UxyxBA7YIOjqDIDHI"},
}

func (m *MetaClient) GetPushConfigs() *bridgev2.PushConfig {
	return pushCfg
}

type DoubleToken struct {
	Unencrypted string `json:"unencrypted"`
	Encrypted   string `json:"encrypted"`
}

func (m *MetaClient) RegisterPushNotifications(ctx context.Context, pushType bridgev2.PushType, token string) error {
	if pushType != bridgev2.PushTypeWeb {
		return fmt.Errorf("unsupported push type %s", pushType)
	}
	meta := m.UserLogin.Metadata.(*metaid.UserLoginMetadata)
	if meta.PushKeys == nil {
		meta.GeneratePushKeys()
		err := m.UserLogin.Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save push key: %w", err)
		}
	}
	keys := messagix.PushKeys{
		P256DH: meta.PushKeys.P256DH,
		Auth:   meta.PushKeys.Auth,
	}
	var encToken string
	if token[0] == '{' && token[len(token)-1] == '}' {
		var dt DoubleToken
		err := json.Unmarshal([]byte(token), &dt)
		if err != nil {
			return fmt.Errorf("failed to unmarshal double token: %w", err)
		}
		token = dt.Unencrypted
		encToken = dt.Encrypted
	}
	if encToken != "" {
		err := m.E2EEClient.RegisterForPushNotifications(ctx, &whatsmeow.WebPushConfig{
			Endpoint: encToken,
			Auth:     meta.PushKeys.Auth,
			P256DH:   meta.PushKeys.P256DH,
		})
		if err != nil {
			return fmt.Errorf("failed to register e2ee notifications: %w", err)
		}
	}
	if m.Client.Platform.IsMessenger() {
		return m.Client.Facebook.RegisterPushNotifications(token, keys)
	} else {
		return m.Client.Instagram.RegisterPushNotifications(token, keys)
	}
}
