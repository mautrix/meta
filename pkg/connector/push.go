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
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/simplevent"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/pushcrypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.PushableNetworkAPI          = (*MetaClient)(nil)
	_ bridgev2.BackgroundSyncingNetworkAPI = (*MetaClient)(nil)
)

var pushCfg = &bridgev2.PushConfig{
	Web: &bridgev2.WebPushConfig{VapidKey: "BIBn3E_rWTci8Xn6P9Xj3btShT85Wdtne0LtwNUyRQ5XjFNkuTq9j4MPAVLvAFhXrUU1A9UxyxBA7YIOjqDIDHI"},
}

var nativePushCfg = &bridgev2.PushConfig{
	FCM: &bridgev2.FCMPushConfig{SenderID: "622912139302"},
}

func (m *MetaClient) GetPushConfigs() *bridgev2.PushConfig {
	if session := m.LoginMeta.NativeSession; session != nil && session.AccessToken != "" && session.AppID == useragent.MessengerLiteAndroidAppID {
		return nativePushCfg
	}
	return pushCfg
}

type DoubleToken struct {
	Unencrypted string `json:"unencrypted"`
	Encrypted   string `json:"encrypted"`
}

func (m *MetaClient) RegisterPushNotifications(ctx context.Context, pushType bridgev2.PushType, token string) error {
	m.pushRegistrationLock.Lock()
	defer m.pushRegistrationLock.Unlock()
	if token == "" {
		return errors.New("empty push token")
	}
	if pushType == bridgev2.PushTypeFCM {
		return m.registerNativePush(ctx, token)
	} else if pushType != bridgev2.PushTypeWeb {
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
		if m.E2EEClient == nil {
			return ErrNotConnected
		}
		err := m.E2EEClient.RegisterForPushNotifications(ctx, &whatsmeow.WebPushConfig{
			Endpoint: encToken,
			Auth:     meta.PushKeys.Auth,
			P256DH:   meta.PushKeys.P256DH,
		})
		if err != nil {
			return fmt.Errorf("failed to register e2ee notifications: %w", err)
		}
	}
	cli := m.Client
	if cli == nil {
		return messagix.ErrClientIsNil
	}
	err := cli.Facebook.RegisterPushNotifications(ctx, token, keys)
	if errors.Is(err, types.ErrPleaseReloadPage) && m.canReconnect() {
		zerolog.Ctx(ctx).Debug().Msg("Doing full reconnect and retrying push registration")
		m.FullReconnect()
		if cli = m.Client; cli != nil {
			err = cli.Facebook.RegisterPushNotifications(ctx, token, keys)
		}
	}
	return err
}

func (m *MetaClient) registerNativePush(ctx context.Context, token string) error {
	if m.Client == nil || m.LoginMeta.NativeSession == nil {
		return bridgev2.ErrNotLoggedIn
	}
	if err := m.e2eeConnectWaiter.WaitTimeoutCtx(ctx, ConnectWaitTimeout); err != nil {
		return fmt.Errorf("waiting for encrypted push connection: %w", err)
	}
	client := m.Client
	e2eeClient := m.E2EEClient
	device := m.WADevice
	if client == nil || e2eeClient == nil || !e2eeClient.IsLoggedIn() || device == nil {
		return ErrNotConnected
	}
	session := m.LoginMeta.NativeSession
	if device.FacebookUUID != session.DeviceID {
		return errors.New("native push session does not match encrypted device")
	}
	if m.LoginMeta.NativePushKeys == nil {
		keys, err := pushcrypto.NewNativePushKeys(metaid.ParseUserLoginID(m.UserLogin.ID))
		if err != nil {
			return fmt.Errorf("failed to generate native push keys: %w", err)
		}
		m.LoginMeta.NativePushKeys = keys
		if err = m.UserLogin.Save(ctx); err != nil {
			m.LoginMeta.NativePushKeys = nil
			return fmt.Errorf("failed to save native push keys: %w", err)
		}
	}
	if err := client.Facebook.RegisterNativePushNotifications(ctx, session, token, m.LoginMeta.NativePushKeys); err != nil {
		return err
	}
	if err := e2eeClient.RegisterForPushNotifications(ctx, &messagix.NativePushConfig{AppID: session.AppID, DeviceID: session.DeviceID}); err != nil {
		return fmt.Errorf("failed to register encrypted native push: %w", err)
	}
	return nil
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

func (m *MetaClient) ensurePushMessageReceived(ctx context.Context, pd *pushcrypto.DecryptedPushData, parsed *methods.MetaMessageID) error {
	if pd == nil || parsed == nil {
		return nil
	}
	log := zerolog.Ctx(ctx)
	msgID := parsed.String()
	part, err := m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, m.UserLogin.ID, metaid.MakeFBMessageID(msgID))
	if err != nil {
		return fmt.Errorf("failed to look up push message %s: %w", msgID, err)
	} else if part != nil {
		log.Debug().
			Str("message_id", msgID).
			Str("chat_id", string(part.Room.ID)).
			Str("f_param", pd.Params["f"]).
			Stringer("event_id", part.MXID).
			Msg("Confirmed push message was bridged")
		return nil
	}
	threadType := table.ONE_TO_ONE
	chatID := parsed.ChatID
	if parsed.ChatType == 'g' {
		threadType = table.GROUP_THREAD
	} else if parsed.ChatType == 'c' {
		chatID ^= metaid.ParseUserLoginID(m.UserLogin.ID)
	}
	log.Warn().
		Str("message_id", msgID).
		Int64("chat_id", chatID).
		Str("f_param", pd.Params["f"]).
		Msg("Push message wasn't bridged, trying to backfill")
	res := m.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type:         bridgev2.RemoteEventChatResync,
			LogContext:   nil,
			PortalKey:    m.makeFBPortalKey(chatID, threadType),
			CreatePortal: true,
		},
		LatestMessageTS: parsed.Time,
	})
	log.Debug().Any("result", res).Msg("Event handling result for push backfill")
	if !res.Success {
		return errors.Join(errors.New("failed to queue push backfill"), res.Error)
	}
	part, err = m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, m.UserLogin.ID, metaid.MakeFBMessageID(msgID))
	if err != nil {
		return fmt.Errorf("failed to look up push message %s after backfill: %w", msgID, err)
	} else if part != nil {
		log.Debug().
			Str("message_id", msgID).
			Str("chat_id", string(part.Room.ID)).
			Stringer("event_id", part.MXID).
			Msg("Confirmed push message was bridged after backfill")
	} else {
		return fmt.Errorf("push message %s wasn't bridged after backfill", msgID)
	}
	return nil
}

func (m *MetaClient) ConnectBackground(ctx context.Context, params *bridgev2.ConnectBackgroundParams) error {
	log := zerolog.Ctx(ctx)
	var parsedMsgID *methods.MetaMessageID
	var envelope struct {
		PIM  string `json:"pim"`
		Data struct {
			PIM string `json:"pim"`
		} `json:"data"`
	}
	var data *pushcrypto.DecryptedPushData
	var err error
	if json.Unmarshal(params.RawData, &envelope) != nil || (envelope.PIM == "" && envelope.Data.PIM == "") {
		data, err = m.LoginMeta.PushKeys.Decrypt(ctx, params.RawData)
	}
	if err != nil {
		log.Warn().Err(err).Msg("Failed to decrypt web push")
	} else if data != nil {
		parsedMsgID, err = methods.ParseMessageIDFull(data.MessageID)
		if err != nil || parsedMsgID == nil {
			log.Warn().
				Err(err).
				Str("orig_id", data.MessageID).
				Str("ts_param", data.Params["ts"]).
				Str("cc_param", data.Params["cc"]).
				Str("f_param", data.Params["f"]).
				Msg("Failed to parse push message ID")
		} else {
			log.Debug().
				Str("message_id", parsedMsgID.String()).
				Time("message_id_ts", parsedMsgID.Time).
				Int64("message_id_chat", parsedMsgID.ChatID).
				Int32("message_id_chat_type", parsedMsgID.ChatType).
				Int64("message_id_txn_id", parsedMsgID.TxnID).
				Str("orig_id", data.MessageID).
				Str("f_param", data.Params["f"]).
				Msg("Parsed message ID from push notification")
		}
	}

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
			if parsedMsgID == nil && m.LoginMeta.Platform.IsMessenger() && !m.connectBackgroundWAOfflineSync.IsSet() {
				return errors.New("encrypted offline sync did not complete")
			}
			return m.ensurePushMessageReceived(ctx, data, parsedMsgID)
		case <-ctx.Done():
			log.Debug().
				Bool("fb_tables_received", anythingReceived).
				Bool("fb_table_processing", isProcessing).
				Bool("wa_queue_empty", waDone).
				Int("wa_message_count", waCount).
				Msg("Closing background connection due to cancellation")
			return ctx.Err()
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
