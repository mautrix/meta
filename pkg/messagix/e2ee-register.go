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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/libsignal/ecc"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/proto/waArmadilloICDC"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type ICDCFetchResponse struct {
	DeviceIdentities []string `json:"device_identities"`
	ICDCSeq          int      `json:"icdc_seq"`
	Status           int      `json:"status"`
}

func (ifr *ICDCFetchResponse) DeviceIdentityBytes() (deviceIdentityBytes [][32]byte, err error) {
	deviceIdentityBytes = make([][32]byte, len(ifr.DeviceIdentities))
	for i, deviceIdentity := range ifr.DeviceIdentities {
		var ident []byte
		ident, err = base64.RawURLEncoding.DecodeString(deviceIdentity)
		if err != nil {
			break
		} else if len(ident) != 32 {
			err = fmt.Errorf("device identity is not 32 bytes long")
			break
		}
		deviceIdentityBytes[i] = [32]byte(ident)
	}
	return
}

type ICDCRegisterResponse struct {
	ICDCSuccess bool   `json:"icdc_success"` // not always present?
	Product     string `json:"product"`      // msgr
	Status      int    `json:"status"`       // 200
	Type        string `json:"type"`         // "new"
	WADeviceID  int    `json:"wa_device_id"`
}

func (c *Client) doE2EERequest(ctx context.Context, endpoint string, body url.Values, into any) error {
	addr := c.GetEndpoint(endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, strings.NewReader(body.Encode()))
	if err != nil {
		return fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", useragent.UserAgent)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "cross-site")
	req.Header.Set("origin", c.GetEndpoint("base_url"))
	req.Header.Set("referer", c.GetEndpoint("messages")+"/")
	zerolog.Ctx(ctx).Trace().
		Str("url", addr).
		Any("body", body).
		Msg("ICDC request")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	logEvt := zerolog.Ctx(ctx).Trace().
		Str("url", addr).
		Int("status_code", resp.StatusCode)
	if json.Valid(respBody) {
		logEvt.RawJSON("response_body", respBody)
	} else {
		logEvt.Str("response_body", base64.StdEncoding.EncodeToString(respBody))
	}
	logEvt.Msg("ICDC response")
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	} else if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	} else if err = json.Unmarshal(respBody, into); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return nil
}

func (c *Client) fetchICDC(ctx context.Context, fbid int64, deviceUUID uuid.UUID) (*ICDCFetchResponse, error) {
	formBody := url.Values{
		"fbid":      {strconv.FormatInt(fbid, 10)},
		"fb_cat":    {c.configs.BrowserConfigTable.MessengerWebInitData.CryptoAuthToken.EncryptedSerializedCat},
		"app_id":    {strconv.FormatInt(c.configs.BrowserConfigTable.MessengerWebInitData.AppID, 10)},
		"device_id": {deviceUUID.String()},
	}
	var icdcResp ICDCFetchResponse
	err := c.doE2EERequest(ctx, "icdc_fetch", formBody, &icdcResp)
	return &icdcResp, err
}

func calculateIdentitiesHash(identities [][32]byte) []byte {
	identitiesSlice := sliceifyIdentities(identities)
	slices.SortFunc(identitiesSlice, bytes.Compare)
	ihash := sha256.Sum256(bytes.Join(identitiesSlice, nil))
	return ihash[:10]
}

func sliceifyIdentities(identities [][32]byte) [][]byte {
	sliceIdentities := make([][]byte, len(identities))
	for i, identity := range identities {
		sliceIdentities[i] = identity[:]
	}
	return sliceIdentities
}

func (c *Client) SetDevice(dev *store.Device) {
	if c != nil {
		c.device = dev
	}
}

func (c *Client) RegisterE2EE(ctx context.Context, fbid int64) error {
	if c == nil {
		return ErrClientIsNil
	} else if c.device == nil {
		return fmt.Errorf("cannot register for E2EE without a device")
	}
	if c.device.FacebookUUID == uuid.Nil {
		c.device.FacebookUUID = uuid.New()
	}
	icdcMeta, err := c.fetchICDC(ctx, fbid, c.device.FacebookUUID)
	if err != nil {
		return fmt.Errorf("failed to fetch ICDC metadata: %w", err)
	}
	deviceIdentities, err := icdcMeta.DeviceIdentityBytes()
	if err != nil {
		return fmt.Errorf("failed to decode device identities: %w", err)
	}
	ownIdentityIndex := slices.Index(deviceIdentities, *c.device.IdentityKey.Pub)
	if ownIdentityIndex == -1 {
		ownIdentityIndex = len(deviceIdentities)
		deviceIdentities = append(deviceIdentities, *c.device.IdentityKey.Pub)
		// TODO does this need to be incremented when reregistering?
		icdcMeta.ICDCSeq++
	}
	icdcTS := time.Now().Unix()
	unsignedList, err := proto.Marshal(&waArmadilloICDC.ICDCIdentityList{
		Seq:                proto.Int32(int32(icdcMeta.ICDCSeq)),
		Timestamp:          proto.Int64(icdcTS),
		Devices:            sliceifyIdentities(deviceIdentities),
		SigningDeviceIndex: proto.Int32(int32(ownIdentityIndex)),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal ICDC identity list: %w", err)
	}
	signature := ecc.CalculateSignature(ecc.NewDjbECPrivateKey(*c.device.IdentityKey.Priv), unsignedList)
	signedList, err := proto.Marshal(&waArmadilloICDC.SignedICDCIdentityList{
		Details:   unsignedList,
		Signature: signature[:],
	})
	if err != nil {
		return fmt.Errorf("failed to marshal signed ICDC identity list: %w", err)
	}
	formBody := url.Values{
		"fbid":       {strconv.FormatInt(fbid, 10)},
		"fb_cat":     {c.configs.BrowserConfigTable.MessengerWebInitData.CryptoAuthToken.EncryptedSerializedCat},
		"app_id":     {strconv.FormatInt(c.configs.BrowserConfigTable.MessengerWebInitData.AppID, 10)},
		"device_id":  {c.device.FacebookUUID.String()},
		"e_regid":    {base64.StdEncoding.EncodeToString(binary.BigEndian.AppendUint32(nil, c.device.RegistrationID))},
		"e_keytype":  {base64.StdEncoding.EncodeToString([]byte{ecc.DjbType})},
		"e_ident":    {base64.StdEncoding.EncodeToString(c.device.IdentityKey.Pub[:])},
		"e_skey_id":  {base64.StdEncoding.EncodeToString(binary.BigEndian.AppendUint32(nil, c.device.SignedPreKey.KeyID)[1:])},
		"e_skey_val": {base64.StdEncoding.EncodeToString(c.device.SignedPreKey.Pub[:])},
		"e_skey_sig": {base64.StdEncoding.EncodeToString(c.device.SignedPreKey.Signature[:])},
		"icdc_list":  {base64.StdEncoding.EncodeToString(signedList)},
		"icdc_ts":    {strconv.FormatInt(icdcTS, 10)},
		"icdc_seq":   {strconv.Itoa(icdcMeta.ICDCSeq)},
		"ihash":      {base64.StdEncoding.EncodeToString(calculateIdentitiesHash(deviceIdentities))},
	}
	var icdcResp ICDCRegisterResponse
	err = c.doE2EERequest(ctx, "icdc_register", formBody, &icdcResp)
	if err != nil {
		return err
	} else if icdcResp.Status != 200 {
		return fmt.Errorf("ICDC registration returned non-200 inner status %d", icdcResp.Status)
	}
	c.device.ID = &types.JID{
		User:   strconv.FormatInt(fbid, 10),
		Device: uint16(icdcResp.WADeviceID),
		Server: types.MessengerServer,
	}
	// This is a hack since currently whatsmeow requires it to be set
	c.device.Account = &waAdv.ADVSignedDeviceIdentity{
		Details:             make([]byte, 0),
		AccountSignatureKey: make([]byte, 32),
		AccountSignature:    make([]byte, 64),
		DeviceSignature:     make([]byte, 64),
	}
	zerolog.Ctx(ctx).Info().
		Stringer("jid", c.device.ID).
		Msg("ICDC registration successful")
	return err
}
