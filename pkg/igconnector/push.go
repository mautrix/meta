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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/pushcrypto"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.PushableNetworkAPI          = (*IGClient)(nil)
	_ bridgev2.BackgroundSyncingNetworkAPI = (*IGClient)(nil)
)

var pushCfg = &bridgev2.PushConfig{
	Web: &bridgev2.WebPushConfig{VapidKey: "BIBn3E_rWTci8Xn6P9Xj3btShT85Wdtne0LtwNUyRQ5XjFNkuTq9j4MPAVLvAFhXrUU1A9UxyxBA7YIOjqDIDHI"},
}

var nativePushCfg = &bridgev2.PushConfig{
	FCM: &bridgev2.FCMPushConfig{SenderID: "390017741467"},
}

func (ic *IGClient) GetPushConfigs() *bridgev2.PushConfig {
	if session := ic.LoginMeta.InstagramNativeSession; session != nil && session.Authorization != "" {
		return nativePushCfg
	}
	return pushCfg
}

func (ic *IGClient) RegisterPushNotifications(ctx context.Context, pushType bridgev2.PushType, token string) error {
	ic.pushRegistrationLock.Lock()
	defer ic.pushRegistrationLock.Unlock()
	if token == "" {
		return errors.New("empty push token")
	} else if pushType == bridgev2.PushTypeFCM {
		return ic.registerNativePush(ctx, token)
	} else if pushType != bridgev2.PushTypeWeb {
		return fmt.Errorf("unsupported push type %s", pushType)
	}
	meta := ic.UserLogin.Metadata.(*metaid.UserLoginMetadata)
	newKeys := meta.PushKeys == nil
	if newKeys {
		meta.GeneratePushKeys()
		err := ic.UserLogin.Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save push key: %w", err)
		}
	}
	keys := instameow.PushKeys{
		P256DH: meta.PushKeys.P256DH,
		Auth:   meta.PushKeys.Auth,
	}
	cli := ic.Client
	if cli == nil {
		return instameow.ErrClientIsNil
	}
	if err := cli.RegisterPushNotifications(ctx, token, keys); err != nil {
		return err
	}
	return ic.Main.DB.InitIGWebPush(ctx, ic.UserLogin.ID, time.Now(), newKeys)
}

func (ic *IGClient) registerNativePush(ctx context.Context, token string) error {
	client := ic.Client
	if client == nil {
		return instameow.ErrClientIsNil
	}
	if ic.LoginMeta.NativePushKeys == nil {
		keys, err := pushcrypto.NewNativePushKeys(metaid.ParseUserLoginID(ic.UserLogin.ID))
		if err != nil {
			return fmt.Errorf("failed to generate native push keys: %w", err)
		}
		ic.LoginMeta.NativePushKeys = keys
		if err = ic.UserLogin.Save(ctx); err != nil {
			ic.LoginMeta.NativePushKeys = nil
			return fmt.Errorf("failed to save native push keys: %w", err)
		}
	}
	if err := client.RegisterNativePushNotifications(ctx, token, ic.LoginMeta.NativePushKeys); err != nil {
		return err
	}
	if err := ic.Main.DB.DeleteIGWebPush(ctx, ic.UserLogin.ID); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to clear web push state after native push registration")
	}
	session := client.GetInstagramNativeSession()
	if ic.LoginMeta.InstagramNativeSession == nil || *session != *ic.LoginMeta.InstagramNativeSession {
		previous := ic.LoginMeta.InstagramNativeSession
		ic.LoginMeta.InstagramNativeSession = session
		if err := ic.UserLogin.Save(ctx); err != nil {
			ic.LoginMeta.InstagramNativeSession = previous
			return fmt.Errorf("failed to save native push session: %w", err)
		}
	}
	return nil
}

func (ic *IGClient) igPushToMessageID(ctx context.Context, pd *pushcrypto.DecryptedPushData) (*methods.MetaMessageID, error) {
	ts, err := strconv.ParseInt(pd.Params["ts"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ts param: %w", err)
	}
	cc, err := strconv.ParseInt(pd.Params["cc"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cc param: %w", err)
	}
	isGroup := pd.Params["aud_gn"] != "" || strings.Contains(pd.Params["network_classification"], "group")
	var chatType rune
	var chatID int64
	if isGroup {
		chatID, err = strconv.ParseInt(pd.Params["f"], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse f param: %w", err)
		}
		chatType = 'g'
	} else {
		otherUserFBID, err := ic.Main.DB.GetFBIDForIGUser(ctx, pd.Params["u"])
		if err != nil {
			return nil, err
		} else if otherUserFBID == 0 {
			return nil, fmt.Errorf("fbid for %s not found", pd.Params["u"])
		}
		chatType = 'c'
		// For DMs, the chat ID is our ID XOR their ID
		chatID = otherUserFBID ^ metaid.ParseUserLoginID(ic.UserLogin.ID)
	}
	return &methods.MetaMessageID{
		ChatType: chatType,
		ChatID:   chatID,
		Time:     time.UnixMilli(ts),
		TxnID:    cc,
	}, nil
}

func (ic *IGClient) ensurePushMessageReceived(ctx context.Context, pd *pushcrypto.DecryptedPushData, parsed *methods.MetaMessageID) {
	if pd == nil || parsed == nil {
		return
	}
	log := zerolog.Ctx(ctx)
	msgID := parsed.String()
	part, err := ic.Main.Bridge.DB.Message.GetFirstPartByID(ctx, ic.UserLogin.ID, metaid.MakeFBMessageID(msgID))
	if err != nil {
		log.Err(err).Str("message_id", msgID).
			Msg("Failed to look up push message in database")
		return
	} else if part != nil {
		log.Debug().
			Str("message_id", msgID).
			Str("chat_id", string(part.Room.ID)).
			Str("f_param", pd.Params["f"]).
			Stringer("event_id", part.MXID).
			Msg("Confirmed push message was bridged")
		return
	}
	chatID := parsed.ChatID
	isGroup := false
	if parsed.ChatType == 'g' {
		isGroup = true
	} else if parsed.ChatType == 'c' {
		chatID ^= metaid.ParseUserLoginID(ic.UserLogin.ID)
	}
	log.Warn().
		Str("message_id", msgID).
		Int64("chat_id", chatID).
		Str("f_param", pd.Params["f"]).
		Msg("Push message wasn't bridged, trying to backfill")

	portalKey := ic.makePortalKey(chatID, isGroup)
	portal, err := ic.Main.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		log.Err(err).
			Int64("chat_id", chatID).
			Msg("Failed to look up portal for push message")
		return
	}
	meta, err := ic.ensureIGID(ctx, portal)
	if err != nil {
		log.Err(err).
			Int64("chat_id", chatID).
			Msg("Failed to ensure portal has IGID for push message")
		return
	}

	_, err = ic.getAndResyncThread(ctx, meta.IGID, "push message")
	if err != nil {
		log.Err(err).
			Int64("chat_id", chatID).
			Str("igid", meta.IGID).
			Msg("Failed to backfill for push message")
		return
	}

	part, err = ic.Main.Bridge.DB.Message.GetFirstPartByID(ctx, ic.UserLogin.ID, metaid.MakeFBMessageID(msgID))
	if err != nil {
		log.Err(err).Str("message_id", msgID).
			Msg("Failed to look up push message in database after backfill")
	} else if part != nil {
		log.Debug().
			Str("message_id", msgID).
			Str("chat_id", string(part.Room.ID)).
			Stringer("event_id", part.MXID).
			Msg("Confirmed push message was bridged after backfill")
	} else {
		log.Warn().
			Str("message_id", msgID).
			Msg("Push message still wasn't bridged after backfill")
	}
}

func (ic *IGClient) ConnectBackground(ctx context.Context, params *bridgev2.ConnectBackgroundParams) error {
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
		data, err = ic.LoginMeta.PushKeys.Decrypt(ctx, params.RawData)
	}
	if err != nil {
		log.Warn().Err(err).Msg("Failed to decrypt web push")
	} else if data != nil {
		if err = ic.Main.DB.PutIGWebPushReceived(ctx, ic.UserLogin.ID, time.Now()); err != nil {
			log.Err(err).Msg("Failed to save web push receive time")
		}
		if strings.HasPrefix(data.MessageID, "mid.") {
			parsedMsgID, err = methods.ParseMessageIDFull(data.MessageID)
		} else if strings.HasPrefix(data.Params["mid"], "mid.") {
			parsedMsgID, err = methods.ParseMessageIDFull(data.Params["mid"])
		} else {
			parsedMsgID, err = ic.igPushToMessageID(ctx, data)
		}
		if err != nil || parsedMsgID == nil {
			log.Warn().
				Err(err).
				Str("orig_id", data.MessageID).
				Str("ts_param", data.Params["ts"]).
				Str("cc_param", data.Params["cc"]).
				Str("f_param", data.Params["f"]).
				Str("u_param", data.Params["u"]).
				Str("mid_param", data.Params["mid"]).
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
				Str("mid_param", data.Params["mid"]).
				Msg("Parsed message ID from push notification")
		}
	}

	ic.caughtUp.Clear()
	go ic.Connect(ctx)
	defer ic.Disconnect()

	select {
	case <-time.After(15 * time.Second):
		log.Debug().Msg("Closing background connection due to timeout")
		ic.ensurePushMessageReceived(ctx, data, parsedMsgID)
		if !ic.caughtUp.IsSet() {
			return errors.New("instagram background sync timed out")
		}
	case <-ctx.Done():
		log.Debug().Msg("Closing background connection due to cancellation")
		return ctx.Err()
	case <-ic.caughtUp.GetChan():
		log.Debug().Msg("Closing background connection as we caught up to the latest seq ID")
		ic.ensurePushMessageReceived(ctx, data, parsedMsgID)
	}
	return ctx.Err()
}

const (
	webPushStaleAfter    = 72 * time.Hour
	webPushMissedMessage = time.Hour
)

func (ic *IGClient) usesWebPush() bool {
	session := ic.LoginMeta.InstagramNativeSession
	return ic.LoginMeta.PushKeys != nil && (session == nil || session.Authorization == "")
}

func (ic *IGClient) isWebPushStale(ctx context.Context) bool {
	if !ic.usesWebPush() {
		return false
	}
	_, stale, err := ic.Main.DB.GetIGWebPush(ctx, ic.UserLogin.ID)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get web push state")
	}
	return stale
}

func (ic *IGClient) sendWebPushStaleState() {
	ic.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateBadCredentials,
		Error:      IGWebPushStale,
		UserAction: status.UserActionRelogin,
	})
}

func (ic *IGClient) checkWebPushStale(portalKey networkid.PortalKey, msg *slidetypes.Message) {
	if msg == nil || !ic.usesWebPush() ||
		time.Since(msg.TimestampMS.Time) < webPushMissedMessage ||
		msg.SenderFBID == metaid.ParseUserLoginID(ic.UserLogin.ID) ||
		!ic.webPushStaleReported.CompareAndSwap(false, true) {
		return
	}
	go ic.confirmWebPushStale(portalKey, msg.TimestampMS.Time)
}

func (ic *IGClient) confirmWebPushStale(portalKey networkid.PortalKey, messageTS time.Time) {
	log := ic.UserLogin.Log.With().Str("action", "check web push").Logger()
	ctx := log.WithContext(ic.Main.Bridge.BackgroundCtx)
	receivedBefore := time.Now().Add(-webPushStaleAfter)
	lastReceived, _, err := ic.Main.DB.GetIGWebPush(ctx, ic.UserLogin.ID)
	if err != nil {
		log.Err(err).Msg("Failed to get web push state")
		return
	} else if lastReceived.IsZero() || lastReceived.After(receivedBefore) {
		return
	}
	cli := ic.Client
	if cli == nil {
		return
	}
	resp, err := cli.GetMailbox(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to fetch inbox to check web push state")
		return
	}
	var thread *slidetypes.ThreadInfo
	if mailbox := resp.GetMailbox(); mailbox != nil {
		for _, edge := range mailbox.ThreadsByFolder.Edges {
			info := edge.Node.AsIGDirectThread
			if info != nil && ic.makePortalKey(info.ThreadKey, info.IsGroup).ID == portalKey.ID {
				thread = info
				break
			}
		}
	}
	if thread == nil || ptr.Val(thread.IsMuted) || thread.SystemFolder == "PENDING" || thread.SystemFolder == "SPAM" {
		log.Debug().
			Bool("found", thread != nil).
			Msg("Late message is in a chat that doesn't get pushes, not marking web push as stale")
		return
	}
	changed, err := ic.Main.DB.SetIGWebPushStale(ctx, ic.UserLogin.ID, receivedBefore)
	if err != nil {
		log.Err(err).Msg("Failed to save web push stale state")
		return
	} else if !changed {
		return
	}
	log.Warn().
		Time("last_web_push", lastReceived).
		Time("message_ts", messageTS).
		Msg("Caught up a message that never got a web push, asking for relogin")
	ic.sendWebPushStaleState()
}
