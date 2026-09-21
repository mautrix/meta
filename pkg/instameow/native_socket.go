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
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

func (c *Client) nativeSocketHeaders() http.Header {
	session := c.mobileSession.Load()
	authorization, _ := json.Marshal(map[string]string{"authorization": session.Authorization})
	return nativeStreamHeaders(session, "OAuth "+base64.RawURLEncoding.EncodeToString(authorization), strconv.FormatInt(c.GetOwnFBID(), 10), "lightspeed")
}

func nativeStreamHeaders(session *types.InstagramNativeSession, authorization, userID, group string) http.Header {
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
