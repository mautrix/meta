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
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.PushableNetworkAPI          = (*MetaClient)(nil)
	_ bridgev2.BackgroundSyncingNetworkAPI = (*MetaClient)(nil)
)

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
		return m.Client.Facebook.RegisterPushNotifications(ctx, token, keys)
	} else {
		return m.Client.Instagram.RegisterPushNotifications(ctx, token, keys)
	}
}

func (m *MetaClient) notifyBackgroundConnAboutEvent(isProcessing bool) {
	if ch := m.connectBackgroundEvt; ch != nil {
		select {
		case ch <- connectBackgroundEvent{isProcessing}:
		default:
		}
	}
}

type connectBackgroundEvent struct {
	isProcessing bool
}

func (m *MetaClient) ConnectBackground(ctx context.Context, params *bridgev2.ConnectBackgroundParams) error {
	log := zerolog.Ctx(ctx)

	evtChan := make(chan connectBackgroundEvent, 8)
	m.connectBackgroundWAOfflineSync.Clear()
	waOfflineSyncChan := m.connectBackgroundWAOfflineSync.GetChan()
	m.connectBackgroundEvt = evtChan
	defer func() {
		m.connectBackgroundEvt = nil
	}()

	go m.Connect(ctx)
	defer m.Disconnect()

	timer := time.NewTimer(10 * time.Second)
	anythingReceived := false
	isProcessing := false
	waCount := 0
	waDone := false
	for {
		select {
		case <-timer.C:
			log.Debug().
				Bool("fb_tables_received", anythingReceived).
				Bool("fb_table_processing", isProcessing).
				Bool("wa_queue_empty", waDone).
				Int("wa_message_count", waCount).
				Msg("Closing background connection due to timeout")
			return nil
		case <-ctx.Done():
			log.Debug().
				Bool("fb_tables_received", anythingReceived).
				Bool("fb_table_processing", isProcessing).
				Bool("wa_queue_empty", waDone).
				Int("wa_message_count", waCount).
				Msg("Closing background connection due to cancellation")
			return nil
		case <-waOfflineSyncChan:
			waOfflineSyncChan = nil
			waDone = true
			waCount = int(m.connectBackgroundWAEventCount.Load())
			if (anythingReceived || waCount > 0) && !isProcessing {
				log.Debug().Msg("Extending background connection timeout by 1 second now that whatsapp offline sync is complete and we've received an event")
				timer.Reset(1 * time.Second)
			}
		case evt := <-evtChan:
			anythingReceived = true
			if evt.isProcessing {
				isProcessing = true
				log.Debug().Msg("Extending background connection timeout by 10 seconds due to starting processing an event")
				timer.Reset(10 * time.Second)
			} else {
				isProcessing = false
				if waDone {
					log.Debug().Msg("Extending background connection timeout by 2 seconds after finishing processing an event")
					timer.Reset(2 * time.Second)
				} else {
					log.Debug().Msg("Extending background connection timeout by 10 seconds after finishing processing an event")
					timer.Reset(10 * time.Second)
				}
			}
		}
	}
}
