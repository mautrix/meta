package instameow

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

func (c *Client) nativeSocketHeaders() http.Header {
	authorization, _ := json.Marshal(map[string]string{"authorization": c.mobileSession.Authorization})
	return c.nativeStreamHeaders("OAuth "+base64.RawURLEncoding.EncodeToString(authorization), strconv.FormatInt(c.GetOwnFBID(), 10), "lightspeed")
}

func (c *Client) nativeStreamHeaders(authorization, userID, group string) http.Header {
	session := c.mobileSession
	return http.Header{
		"Authorization":          {authorization},
		"User-Agent":             {instagramMobileUserAgent},
		"X-Dgw-Authtype":         {"6:0"},
		"X-Dgw-Uuid":             {userID},
		"X-Dgw-Regionhint":       {session.DirectRegionHint},
		"X-Dgw-Appid":            {useragent.IGAndroidAppID},
		"X-Dgw-Appversion":       {instagramMobileAppVersion},
		"X-Dgw-Deviceid":         {session.Device.DeviceID},
		"X-Dgw-Fdid":             {session.Device.PhoneID},
		"X-Dgw-Version":          {"9"},
		"X-Dgw-App-Stream-Group": {group},
	}
}

func (c *Client) makeNativeStreamInitPayload(retryCount int, syncParameters, cursor []byte) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{
		"database": "223", "epoch_id": methods.GenerateEpochID(), "format": "flatbuffer",
		"last_applied_cursor": string(cursor), "sync_params": string(syncParameters), "version": "-2",
	})
	if err != nil {
		return nil, err
	}
	userID := c.GetOwnFBID()
	if userID == 0 {
		return nil, fmt.Errorf("native messaging user ID is missing")
	}
	return marshalNativeStreamRequest(uint16(retryCount%65535+1), uint64(userID), payload), nil
}

func marshalNativeStreamRequest(requestID uint16, userID uint64, payload []byte) []byte {
	b := make([]byte, 56, 61+len(payload))
	binary.LittleEndian.PutUint32(b, 24)
	for index, value := range []uint16{16, 32, 4, 6, 8, 16, 24, 0} {
		binary.LittleEndian.PutUint16(b[4+index*2:], value)
	}
	binary.LittleEndian.PutUint32(b[24:], 20)
	binary.LittleEndian.PutUint16(b[28:], requestID)
	binary.LittleEndian.PutUint16(b[30:], 2)
	binary.LittleEndian.PutUint32(b[32:], 24)
	binary.LittleEndian.PutUint64(b[40:], userID)
	binary.LittleEndian.PutUint16(b[48:], 3)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	b = append(b, payload...)
	return append(b, 0)
}

func unmarshalNativeStreamResponse(b []byte) ([]byte, error) {
	invalid := fmt.Errorf("invalid native messaging response")
	if len(b) < 4 {
		return nil, invalid
	}
	root := int64(binary.LittleEndian.Uint32(b))
	if root < 4 || root+4 > int64(len(b)) {
		return nil, invalid
	}
	vtable := root - int64(int32(binary.LittleEndian.Uint32(b[root:])))
	if vtable < 0 || vtable+8 > int64(len(b)) || binary.LittleEndian.Uint16(b[vtable:]) < 8 {
		return nil, invalid
	}
	field := root + int64(binary.LittleEndian.Uint16(b[vtable+6:]))
	if field == root || field+4 > int64(len(b)) {
		return nil, invalid
	}
	vector := field + int64(binary.LittleEndian.Uint32(b[field:]))
	if vector+4 > int64(len(b)) {
		return nil, invalid
	}
	end := vector + 4 + int64(binary.LittleEndian.Uint32(b[vector:]))
	if end > int64(len(b)) {
		return nil, invalid
	}
	return b[vector+4 : end], nil
}
