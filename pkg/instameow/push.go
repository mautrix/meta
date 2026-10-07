package instameow

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/pushcrypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func (c *Client) RegisterNativePushNotifications(ctx context.Context, token string, keys *pushcrypto.NativePushKeys) error {
	if c == nil {
		return ErrClientIsNil
	}
	session := c.GetInstagramNativeSession()
	if session == nil || session.Authorization == "" || session.UserID == "" || session.Device.DeviceID == "" || session.Device.PhoneID == "" {
		return errors.New("missing native Instagram session")
	} else if session.UserID != c.cookies.Get(cookies.IGCookieDSUserID) {
		return errors.New("native Instagram session belongs to a different account")
	} else if token == "" || keys == nil || keys.HPKEKeyID == "" {
		return errors.New("missing native push token or keys")
	}
	private, err := ecdh.P256().NewPrivateKey(keys.HPKEPrivateKey)
	if err != nil {
		return fmt.Errorf("invalid native push private key: %w", err)
	}
	form := url.Values{
		"device_token":         {token},
		"device_type":          {"android_fcm"},
		"device_sub_type":      {"0"},
		"is_main_push_channel": {"true"},
		"guid":                 {session.Device.DeviceID},
		"_uuid":                {session.Device.DeviceID},
		"family_device_id":     {session.Device.PhoneID},
		"android_id":           {strings.TrimPrefix(session.Device.AndroidDeviceID, "android-")},
		"users":                {session.UserID},
		"request_id":           {uuid.NewString()},
		"os_settings":          {`{"notificationEnabled":true}`},
		"hpke_ciphersuite":     {"1001000010000"},
		"hpke_keystore_id":     {keys.HPKEKeyID},
		"hpke_pubkey":          {base64.StdEncoding.EncodeToString(private.PublicKey().Bytes())},
	}
	headers := c.instagramAccountManagerHeaders()
	if session.Device.MachineID != "" {
		headers.Set("x-mid", session.Device.MachineID)
	}
	response, body, err := c.http.MakeRequest(ctx, instagramMobileAPIBase+"push/register/", http.MethodPost, headers, []byte(form.Encode()), types.FORM)
	if err != nil {
		c.checkResponseError(err)
		return fmt.Errorf("failed to register native push: %w", err)
	}
	var result struct {
		Status string `json:"status"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode native push response: %w", err)
	} else if result.Status != "ok" {
		return fmt.Errorf("native push registration rejected: %s", result.Status)
	}
	if userID := response.Header.Get("ig-set-ig-u-ds-user-id"); userID != "" && userID != session.UserID {
		return errors.New("native push response belongs to a different account")
	}
	if authorization := response.Header.Get("ig-set-authorization"); authorization != "" {
		session.Authorization = authorization
	}
	if rur := response.Header.Get("ig-set-ig-u-rur"); rur != "" {
		session.RUR = rur
	}
	if mid := response.Header.Get("ig-set-x-mid"); mid != "" {
		session.Device.MachineID = mid
	}
	c.SetInstagramNativeSession(session)
	return nil
}
