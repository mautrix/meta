package messagix

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	waBinary "go.mau.fi/whatsmeow/binary"

	"go.mau.fi/mautrix-meta/pkg/messagix/pushcrypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type NativePushConfig struct {
	AppID    string
	DeviceID uuid.UUID
}

func (cfg *NativePushConfig) GetPushConfigAttrs() waBinary.Attrs {
	return waBinary.Attrs{
		"platform": "fb",
		"appid":    cfg.AppID,
		"deviceid": cfg.DeviceID.String(),
	}
}

func (fb *FacebookMethods) RegisterNativePushNotifications(ctx context.Context, session *types.NativeSession, token string, keys *pushcrypto.NativePushKeys) error {
	if session == nil || session.AccessToken == "" || session.AppID != useragent.MessengerLiteAndroidAppID || session.DeviceID == uuid.Nil || session.FamilyDeviceID == uuid.Nil {
		return errors.New("missing native Messenger session")
	}
	userID := fb.client.GetCookies().GetUserID()
	if userID <= 0 || token == "" {
		return errors.New("missing native push account or token")
	}
	if keys == nil || len(keys.AESKey) != 32 || keys.AESKeyID>>8 != userID || keys.HPKEKeyID == "" {
		return errors.New("invalid native push keys")
	}
	private, err := ecdh.P256().NewPrivateKey(keys.HPKEPrivateKey)
	if err != nil {
		return fmt.Errorf("invalid native push private key: %w", err)
	}
	deviceID := session.DeviceID.String()
	androidID := sha256.Sum256([]byte(deviceID))
	protocol, err := json.Marshal(map[string]any{
		"url":              "https://fcm.googleapis.com/fcm/send",
		"token":            token,
		"device_id":        deviceID,
		"family_device_id": strings.ToUpper(session.FamilyDeviceID.String()),
		"sub_type":         0,
		"encryption": map[string]any{
			"key":       base64.StdEncoding.EncodeToString(keys.AESKey) + "\n",
			"key_id":    keys.AESKeyID,
			"algorithm": "AES_GCM",
		},
		"hpke_params": map[string]string{
			"hpke_ciphersuite": "1001000010000",
			"hpke_pubkey":      base64.StdEncoding.EncodeToString(private.PublicKey().Bytes()),
			"hpke_keystore_id": keys.HPKEKeyID,
		},
		"typed_os_settings": map[string]int{"notificationEnabled": 1},
		"extra_data": map[string]int{
			"android_build":          345613452,
			"android_setting_mask":   240,
			"orca_muted_until_ms":    0,
			"sys_notif":              1,
			"messaging_channel_mask": 118,
		},
		"android_id":               hex.EncodeToString(androidID[:8]),
		"request_id":               uuid.NewString(),
		"client_reported_user_ids": strconv.FormatInt(userID, 10),
	})
	if err != nil {
		return fmt.Errorf("failed to encode native push parameters: %w", err)
	}
	form := url.Values{
		"format":                   {"json"},
		"return_structure":         {"1"},
		"protocol_params":          {string(protocol)},
		"locale":                   {"en_US"},
		"client_country_code":      {"US"},
		"fb_api_req_friendly_name": {"registerPush"},
		"fb_api_caller_class":      {"FacebookPushServerRegisterJobImpl"},
	}
	var body bytes.Buffer
	compressed := gzip.NewWriter(&body)
	if _, err = compressed.Write([]byte(form.Encode())); err != nil {
		return fmt.Errorf("failed to compress native push request: %w", err)
	}
	if err = compressed.Close(); err != nil {
		return fmt.Errorf("failed to finish native push request: %w", err)
	}
	headers := http.Header{}
	headers.Set("Authorization", "OAuth "+session.AccessToken)
	headers.Set("User-Agent", useragent.MessengerLiteAndroidUserAgent)
	headers.Set("X-FB-Connection-Quality", "EXCELLENT")
	headers.Set("X-FB-Friendly-Name", "registerPush")
	headers.Set("X-ZERO-STATE", "unknown")
	headers.Set("X-FB-Family-Device-Id", session.FamilyDeviceID.String())
	headers.Set("Content-Encoding", "gzip")
	_, response, err := fb.client.http.MakeRequestOnceNoRedirect(ctx, "https://b-graph.facebook.com/me/register_push_tokens", http.MethodPost, headers, body.Bytes(), types.FORM)
	if err != nil {
		return fmt.Errorf("failed to register native push: %w", err)
	}
	var result struct {
		Success        bool `json:"success"`
		DisabledSource int  `json:"disabled_source"`
	}
	if err = json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to decode native push response: %w", err)
	} else if !result.Success || result.DisabledSource != 0 {
		return fmt.Errorf("native push registration rejected (disabled source %d)", result.DisabledSource)
	}
	return nil
}
