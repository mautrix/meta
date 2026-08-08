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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/instameow"
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

func (ic *IGClient) GetPushConfigs() *bridgev2.PushConfig {
	return pushCfg
}

type DoubleToken struct {
	Unencrypted string `json:"unencrypted"`
	Encrypted   string `json:"encrypted"`
}

func (ic *IGClient) RegisterPushNotifications(ctx context.Context, pushType bridgev2.PushType, token string) error {
	if pushType != bridgev2.PushTypeWeb {
		return fmt.Errorf("unsupported push type %s", pushType)
	}
	meta := ic.UserLogin.Metadata.(*metaid.UserLoginMetadata)
	if meta.PushKeys == nil {
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
	err := cli.RegisterPushNotifications(ctx, token, keys)
	return err
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

	_, err = ic.getAndResyncThread(ctx, meta.IGID)
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
	data, err := ic.LoginMeta.PushKeys.Decrypt(ctx, params.RawData)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to decrypt web push")
	} else if data != nil {
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
	case <-ctx.Done():
		log.Debug().Msg("Closing background connection due to cancellation")
	case <-ic.caughtUp.GetChan():
		log.Debug().Msg("Closing background connection as we caught up to the latest seq ID")
		ic.ensurePushMessageReceived(ctx, data, parsedMsgID)
	}
	return nil
}
