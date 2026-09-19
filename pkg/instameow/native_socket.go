package instameow

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	flatbuffers "github.com/google/flatbuffers/go"

	lightspeed "go.mau.fi/mautrix-meta/pkg/instameow/flatbuffer"
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
	return marshalNativeStreamRequest(uint16(retryCount%65535+1), userID, payload), nil
}

func marshalNativeStreamRequest(requestID uint16, userID int64, payload []byte) []byte {
	b := flatbuffers.NewBuilder(len(payload) + 64)
	body := b.CreateByteString(payload)
	lightspeed.RequestStart(b)
	lightspeed.RequestAddRequestId(b, requestID)
	lightspeed.RequestAddRequestType(b, 2)
	lightspeed.RequestAddPayload(b, body)
	lightspeed.RequestAddUserId(b, userID)
	lightspeed.RequestAddTransport(b, 3)
	b.Finish(lightspeed.RequestEnd(b))
	return b.FinishedBytes()
}

func unmarshalNativeStreamResponse(b []byte) (payload []byte, err error) {
	b = b[:len(b):len(b)]
	invalid := fmt.Errorf("invalid native messaging response")
	defer func() {
		if recover() != nil {
			payload, err = nil, invalid
		}
	}()
	response := lightspeed.GetRootAsResponse(b, 0)
	table := response.Table()
	vtable := int64(table.Pos) - int64(table.GetSOffsetT(table.Pos))
	if vtable < 0 || vtable+2 > int64(len(b)) {
		return nil, invalid
	}
	offset := table.Offset(6)
	if offset == 0 {
		return nil, nil
	}
	field := uint64(table.Pos) + uint64(offset)
	relative := uint64(flatbuffers.GetUOffsetT(b[field:]))
	if relative == 0 || field+relative+4 > uint64(len(b)) {
		return nil, invalid
	}
	return response.PayloadBytes(), nil
}
