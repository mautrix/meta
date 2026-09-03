// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Killian Lelong
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

package instameow

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const (
	instagramMobileAPIBase = "https://i.instagram.com/api/v1/"

	// These values match the current first-party Android profile used by the
	// official Instagram APK. They must be updated together.
	instagramMobileAppVersion  = "440.0.0.19.86"
	instagramMobileVersionCode = "384608963"
	instagramMobileUserAgent   = "Instagram " + instagramMobileAppVersion +
		" Android (34/14; 480dpi; 1344x2992; Google/google; Pixel 8 Pro; husky; husky; en_US; " +
		instagramMobileVersionCode + ")"
)

type mobileLoginState struct {
	PhoneID           string
	DeviceID          string
	AdvertisingID     string
	AndroidDeviceID   string
	CSRFToken         string
	MachineID         string
	PasswordKeyID     int
	PasswordPublicKey string
	USDIDKey          *ecdsa.PrivateKey
	USDIDHeader       string
	USDID             string
	USDIDKeyID        string
	USDIDPrivateKey   string
	USDIDRegistered   bool
}

type instagramMobileSession struct {
	Authorization    string
	UserID           string
	Username         string
	RUR              string
	SHBID            string
	SHBTS            string
	DirectRegionHint string
	WWWClaim         string
	Device           types.InstagramLoginDevice
}

type instagramMobileLoginResponse struct {
	LoggedInUser *instagramMobileUser `json:"logged_in_user,omitempty"`
}

type instagramMobileUser struct {
	PK       json.RawMessage `json:"pk,omitempty"`
	Username string          `json:"username,omitempty"`
}

func newMobileLoginDevice() types.InstagramLoginDevice {
	deviceID := uuid.NewString()
	deviceHash := sha256.Sum256([]byte(deviceID))
	return types.InstagramLoginDevice{
		PhoneID:         uuid.NewString(),
		DeviceID:        deviceID,
		AdvertisingID:   uuid.NewString(),
		AndroidDeviceID: "android-" + hex.EncodeToString(deviceHash[:8]),
	}
}

func initializeUSDID(device *types.InstagramLoginDevice) (*ecdsa.PrivateKey, error) {
	if device.USDID != "" && device.USDIDKeyID != "" && device.USDIDPrivateKey != "" {
		der, err := base64.StdEncoding.DecodeString(device.USDIDPrivateKey)
		if err == nil {
			key, parseErr := x509.ParseECPrivateKey(der)
			if parseErr == nil && key.Curve == elliptic.P256() {
				return key, nil
			}
		}
		return nil, errors.New("stored Instagram USDID signing key is invalid")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate Instagram USDID signing key: %w", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal Instagram USDID signing key: %w", err)
	}
	kid := make([]byte, 32)
	if _, err = rand.Read(kid); err != nil {
		return nil, fmt.Errorf("generate Instagram USDID key ID: %w", err)
	}
	device.USDID = uuid.NewString()
	device.USDIDKeyID = base64.RawURLEncoding.EncodeToString(kid)
	device.USDIDPrivateKey = base64.StdEncoding.EncodeToString(der)
	device.USDIDRegistered = false
	return key, nil
}

func signUSDID(key *ecdsa.PrivateKey, value string) (string, error) {
	hash := sha256.Sum256([]byte(value))
	signature, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	return base64.RawURLEncoding.EncodeToString(signature), err
}

func makeUSDIDHeader(device types.InstagramLoginDevice, key *ecdsa.PrivateKey) (string, error) {
	signed := device.USDID + "." + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	signature, err := signUSDID(key, signed)
	return signed + "." + signature, err
}

func validMobileLoginDevice(device *types.InstagramLoginDevice) bool {
	if device == nil {
		return false
	}
	for _, value := range []string{device.PhoneID, device.DeviceID, device.AdvertisingID} {
		if _, err := uuid.Parse(value); err != nil {
			return false
		}
	}
	const androidPrefix = "android-"
	if !strings.HasPrefix(device.AndroidDeviceID, androidPrefix) {
		return false
	}
	androidID, err := hex.DecodeString(strings.TrimPrefix(device.AndroidDeviceID, androidPrefix))
	return err == nil && len(androidID) == 8
}

func (state *mobileLoginState) device() types.InstagramLoginDevice {
	return types.InstagramLoginDevice{
		PhoneID:         state.PhoneID,
		DeviceID:        state.DeviceID,
		AdvertisingID:   state.AdvertisingID,
		AndroidDeviceID: state.AndroidDeviceID,
		MachineID:       state.MachineID,
		USDID:           state.USDID,
		USDIDKeyID:      state.USDIDKeyID,
		USDIDPrivateKey: state.USDIDPrivateKey,
		USDIDRegistered: state.USDIDRegistered,
	}
}

func (c *Client) persistMobileLoginDevice(ctx context.Context, state *mobileLoginState) error {
	device := state.device()
	if c.mobileLoginDevice != nil && *c.mobileLoginDevice == device {
		return nil
	}
	if c.saveMobileLoginDevice != nil {
		if err := c.saveMobileLoginDevice(ctx, device); err != nil {
			return err
		}
	}
	c.mobileLoginDevice = &device
	return nil
}

func (c *Client) newMobileLoginState(ctx context.Context) (*mobileLoginState, error) {
	device := newMobileLoginDevice()
	if validMobileLoginDevice(c.mobileLoginDevice) {
		device = *c.mobileLoginDevice
	}
	usdidKey, err := initializeUSDID(&device)
	if err != nil {
		return nil, err
	}
	usdidHeader, err := makeUSDIDHeader(device, usdidKey)
	if err != nil {
		return nil, fmt.Errorf("sign Instagram USDID header: %w", err)
	}
	csrfBytes := make([]byte, 32)
	if _, err = rand.Read(csrfBytes); err != nil {
		return nil, fmt.Errorf("failed to generate Instagram app CSRF token: %w", err)
	}
	state := &mobileLoginState{
		PhoneID:         device.PhoneID,
		DeviceID:        device.DeviceID,
		AdvertisingID:   device.AdvertisingID,
		AndroidDeviceID: device.AndroidDeviceID,
		CSRFToken:       hex.EncodeToString(csrfBytes),
		MachineID:       device.MachineID,
		USDIDKey:        usdidKey,
		USDIDHeader:     usdidHeader,
		USDID:           device.USDID,
		USDIDKeyID:      device.USDIDKeyID,
		USDIDPrivateKey: device.USDIDPrivateKey,
		USDIDRegistered: device.USDIDRegistered,
	}
	if err := c.persistMobileLoginDevice(ctx, state); err != nil {
		return nil, fmt.Errorf("failed to persist Instagram app installation identity: %w", err)
	}
	return state, nil
}

func (c *Client) prepareMobilePasswordLogin(ctx context.Context) (*mobileLoginState, error) {
	if c.mobileLogin == nil {
		var err error
		c.mobileLogin, err = c.newMobileLoginState(ctx)
		if err != nil {
			return nil, err
		}
	}
	state := c.mobileLogin
	if state.PasswordKeyID != 0 && state.PasswordPublicKey != "" {
		return state, nil
	}

	keyRequest := struct {
		ID                    string `json:"id"`
		ServerConfigRetrieval string `json:"server_config_retrieval"`
	}{
		ID:                    state.DeviceID,
		ServerConfigRetrieval: "1",
	}
	response, _, requestErr := c.makeMobileSignedRequest(ctx, state, "qe/sync/", keyRequest)
	if response != nil {
		if err := c.updateMobileLoginResponseState(ctx, state, response); err != nil {
			return nil, err
		}
	}
	if requestErr != nil {
		return nil, requestErr
	}
	keyID, err := strconv.ParseUint(
		response.Header.Get("ig-set-password-encryption-key-id"),
		10,
		8,
	)
	if err != nil {
		return nil, fmt.Errorf("instagram app returned an invalid password key ID: %w", err)
	}
	publicKey := response.Header.Get("ig-set-password-encryption-pub-key")
	if publicKey == "" {
		return nil, errors.New("instagram app did not provide a password public key")
	}
	state.PasswordKeyID = int(keyID)
	state.PasswordPublicKey = publicKey
	return state, nil
}

func (c *Client) makeMobileSignedRequest(
	ctx context.Context,
	state *mobileLoginState,
	endpoint string,
	payload any,
) (*http.Response, []byte, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal Instagram app request: %w", err)
	}
	body := []byte("signed_body=SIGNATURE." + url.QueryEscape(string(payloadJSON)))
	return c.http.MakeRequest(
		ctx,
		instagramMobileAPIBase+endpoint,
		http.MethodPost,
		c.mobileLoginHeaders(state),
		body,
		types.FORM,
	)
}

func (c *Client) mobileLoginHeaders(state *mobileLoginState) http.Header {
	_, timezoneOffset := time.Now().Zone()
	headers := make(http.Header)
	headers.Set("accept", "*/*")
	headers.Set("accept-language", "en-US")
	headers.Set("user-agent", instagramMobileUserAgent)
	headers.Set("x-ig-app-locale", "en_US")
	headers.Set("x-ig-device-locale", "en_US")
	headers.Set("x-ig-mapped-locale", "en_US")
	headers.Set("x-pigeon-session-id", "UFS-"+uuid.NewString()+"-1")
	headers.Set(
		"x-pigeon-rawclienttime",
		strconv.FormatFloat(float64(time.Now().UnixMilli())/1000, 'f', 3, 64),
	)
	headers.Set("x-ig-app-startup-country", "US")
	headers.Set("x-bloks-version-id", bloks.BloksVersionInstagramAndroid)
	headers.Set("x-ig-www-claim", "0")
	headers.Set("x-bloks-is-layout-rtl", "false")
	headers.Set("x-bloks-is-panorama-enabled", "true")
	headers.Set("x-ig-device-id", state.DeviceID)
	headers.Set("x-ig-family-device-id", state.PhoneID)
	headers.Set("x-ig-android-id", state.AndroidDeviceID)
	headers.Set("x-ig-timezone-offset", strconv.Itoa(timezoneOffset))
	headers.Set("x-ig-connection-type", "WIFI")
	headers.Set("x-ig-capabilities", "3brTv10=")
	headers.Set("x-ig-app-id", useragent.IGAndroidAppID)
	headers.Set("x-fb-http-engine", "Tigon/MNS/TCP")
	headers.Set("x-tigon-is-retry", "False")
	headers.Set("x-fb-client-ip", "True")
	headers.Set("x-fb-server-cluster", "True")
	headers.Set("ig-intended-user-id", "0")
	if state.MachineID != "" {
		headers.Set("x-mid", state.MachineID)
	}
	if state.USDIDHeader != "" {
		headers.Set("x-meta-usdid", state.USDIDHeader)
	}
	if cookieHeader := c.cookies.String(); cookieHeader != "" {
		headers.Set("cookie", cookieHeader)
	}
	return headers
}

func (c *Client) makeMobileLoginRequest(ctx context.Context, state *mobileLoginState, requestURL string, headers http.Header, body []byte) ([]byte, error) {
	response, responseBody, err := c.http.MakeRequest(ctx, requestURL, http.MethodPost, headers, body, types.FORM)
	if response != nil {
		err = errors.Join(err, c.updateMobileLoginResponseState(ctx, state, response))
	}
	return responseBody, err
}

func (c *Client) updateMobileLoginResponseState(
	ctx context.Context,
	state *mobileLoginState,
	response *http.Response,
) error {
	c.cookies.UpdateFromResponse(response)
	if machineID := response.Header.Get("ig-set-x-mid"); machineID != "" {
		state.MachineID = machineID
		c.cookies.Set(cookies.IGCookieMachineID, machineID)
	}
	c.updateMobileSessionHeaders(response, state)
	if err := c.persistMobileLoginDevice(ctx, state); err != nil {
		return fmt.Errorf("failed to persist Instagram app installation identity: %w", err)
	}
	return nil
}

func firstMobileResponseHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func (c *Client) updateMobileSessionHeaders(response *http.Response, state *mobileLoginState) {
	if response == nil || state == nil {
		return
	}
	authorization := response.Header.Get("ig-set-authorization")
	if authorization == "" && c.mobileSession == nil {
		return
	}
	session := instagramMobileSession{}
	if c.mobileSession != nil {
		session = *c.mobileSession
	}
	if authorization != "" {
		session.Authorization = authorization
	}
	if value := response.Header.Get("ig-set-ig-u-rur"); value != "" {
		session.RUR = value
		c.cookies.Set(cookies.IGCookieRUR, value)
	}
	if value := response.Header.Get("ig-set-ig-u-shbid"); value != "" {
		session.SHBID = value
		c.cookies.Set(cookies.IGCookieSHBID, value)
	}
	if value := response.Header.Get("ig-set-ig-u-shbts"); value != "" {
		session.SHBTS = value
		c.cookies.Set(cookies.IGCookieSHBTS, value)
	}
	if value := firstMobileResponseHeader(
		response.Header,
		"ig-set-ig-u-ig-direct-region-hint",
		"ig-set-ig-u-direct-region-hint",
	); value != "" {
		session.DirectRegionHint = value
	}
	if value := firstMobileResponseHeader(
		response.Header,
		"x-ig-set-www-claim",
		"ig-set-www-claim",
	); value != "" {
		session.WWWClaim = value
	}
	if value := response.Header.Get("ig-set-ig-u-ds-user-id"); value != "" {
		session.UserID = value
	}
	session.Device = state.device()
	c.mobileSession = &session
}

func (c *Client) applyMobileAuthorization(
	response *http.Response,
	loginResponse *instagramMobileLoginResponse,
	state *mobileLoginState,
) error {
	if c.mobileSession == nil {
		c.mobileSession = &instagramMobileSession{Device: state.device()}
	}
	authorization := response.Header.Get("ig-set-authorization")
	if authorization != "" {
		lastColon := strings.LastIndex(authorization, ":")
		if lastColon < 0 || lastColon == len(authorization)-1 {
			return errors.New("instagram app returned an invalid authorization header")
		}
		encoded := authorization[lastColon+1:]
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		}
		if err != nil {
			return fmt.Errorf("instagram app returned an invalid authorization payload: %w", err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(decoded)))
		decoder.UseNumber()
		var authData map[string]any
		if err = decoder.Decode(&authData); err != nil {
			return fmt.Errorf("instagram app returned an invalid authorization payload: %w", err)
		}
		if sessionID := mobileAuthorizationValue(authData["sessionid"]); sessionID != "" {
			c.cookies.Set(cookies.IGCookieSessionID, sessionID)
		}
		if userID := mobileAuthorizationValue(authData["ds_user_id"]); userID != "" {
			c.cookies.Set(cookies.IGCookieDSUserID, userID)
			c.mobileSession.UserID = userID
		}
		for _, route := range []struct {
			authorizationKey string
			responseHeader   string
			cookieName       cookies.MetaCookieName
		}{
			{"rur", "ig-set-ig-u-rur", cookies.IGCookieRUR},
			{"shbid", "ig-set-ig-u-shbid", cookies.IGCookieSHBID},
			{"shbts", "ig-set-ig-u-shbts", cookies.IGCookieSHBTS},
		} {
			if response.Header.Get(route.responseHeader) == "" {
				if value := mobileAuthorizationValue(authData[route.authorizationKey]); value != "" {
					c.cookies.Set(route.cookieName, value)
				}
			}
		}
	}
	if c.cookies.Get(cookies.IGCookieDSUserID) == "" && loginResponse.LoggedInUser != nil {
		if userID := rawJSONScalar(loginResponse.LoggedInUser.PK); userID != "" {
			c.cookies.Set(cookies.IGCookieDSUserID, userID)
			c.mobileSession.UserID = userID
		}
	}
	if loginResponse.LoggedInUser != nil {
		c.mobileSession.Username = loginResponse.LoggedInUser.Username
	}
	if c.cookies.Get(cookies.IGCookieCSRFToken) == "" {
		c.cookies.Set(cookies.IGCookieCSRFToken, state.CSRFToken)
	}
	if c.cookies.Get(cookies.IGCookieMachineID) == "" {
		c.cookies.Set(cookies.IGCookieMachineID, state.MachineID)
	}
	if c.cookies.Get(cookies.IGCookieDeviceID) == "" {
		c.cookies.Set(cookies.IGCookieDeviceID, strings.ToUpper(state.DeviceID))
	}
	if c.cookies.Get(cookies.IGCookieSessionID) == "" {
		return errors.New("instagram app login response did not include a session")
	}
	return nil
}

func mobileAuthorizationValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func rawJSONScalar(value json.RawMessage) string {
	if len(value) == 0 {
		return ""
	}
	var stringValue string
	if json.Unmarshal(value, &stringValue) == nil {
		return stringValue
	}
	var numberValue json.Number
	if json.Unmarshal(value, &numberValue) == nil {
		return numberValue.String()
	}
	return ""
}
