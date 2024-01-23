package messagix

import (
	"encoding/json"
	"slices"
	"strconv"

	"go.mau.fi/mautrix-meta/messagix/packets"
)

type ConnectPostMessage struct {
	IsBase64Publish bool        `json:"isBase64Publish"`
	MessageID       int64       `json:"messageId"`
	Payload         string      `json:"payload"`
	QoS             packets.QoS `json:"qos"`
	Topic           string      `json:"topic"`
}

type Connect struct {
	AccountId          string               `json:"u"`            // account id
	SessionId          int64                `json:"s"`            // randomly generated sessionid
	ClientCapabilities int                  `json:"cp"`           // mqttconfig clientCapabilities (3)
	Capabilities       int                  `json:"ecp"`          // mqttconfig capabilities (10)
	ChatOn             bool                 `json:"chat_on"`      // mqttconfig chatVisibility (true) - not 100% sure
	Fg                 bool                 `json:"fg"`           // idk what this is
	Cid                string               `json:"d"`            // cid from html content
	ConnectionType     string               `json:"ct"`           // connection type? facebook=websocket , insta=cookie_auth
	MqttSid            string               `json:"mqtt_sid"`     // ""
	AppId              int64                `json:"aid"`          // mqttconfig appID (219994525426954)
	SubscribedTopics   []string             `json:"st"`           // mqttconfig subscribedTopics ([])
	PostMessage        []ConnectPostMessage `json:"pm"`           // only seen empty array
	Dc                 string               `json:"dc"`           // only seem empty string
	NoAutoFg           bool                 `json:"no_auto_fg"`   // only seen true
	Gas                any                  `json:"gas"`          // only seen null
	Pack               []any                `json:"pack"`         // only seen empty arr
	HostNameOverride   string               `json:"php_override"` // mqttconfig hostNameOverride
	P                  any                  `json:"p"`            // only seen null
	UserAgent          string               `json:"a"`            // user agent
	Aids               any                  `json:"aids"`         // only seen null
}

func (s *Socket) newConnectJSON() (string, error) {
	payload := &Connect{
		AccountId:          s.client.configs.browserConfigTable.CurrentUserInitialData.AccountID,
		SessionId:          s.client.socket.sessionId,
		ClientCapabilities: s.client.configs.browserConfigTable.MqttWebConfig.ClientCapabilities,
		Capabilities:       s.client.configs.browserConfigTable.MqttWebConfig.Capabilities,
		ChatOn:             s.client.configs.browserConfigTable.MqttWebConfig.ChatVisibility,
		Fg:                 false,
		ConnectionType:     s.client.socket.getConnectionType(),
		MqttSid:            "",
		AppId:              s.client.configs.browserConfigTable.MqttWebConfig.AppID,
		SubscribedTopics:   s.client.configs.browserConfigTable.MqttWebConfig.SubscribedTopics,
		PostMessage:        make([]ConnectPostMessage, 0),
		Dc:                 "",
		NoAutoFg:           true,
		Gas:                nil,
		Pack:               make([]any, 0),
		HostNameOverride:   s.client.configs.browserConfigTable.MqttWebConfig.HostNameOverride,
		P:                  nil,
		UserAgent:          UserAgent,
		Aids:               nil,
		Cid:                s.client.configs.browserConfigTable.MqttWebDeviceID.ClientID,
	}
	if s.previouslyConnected {
		appSettingPublishJSON, err := s.newAppSettingsPublishJSON(s.client.configs.VersionId)
		if err != nil {
			return "", err
		}
		payload.PostMessage = append(payload.PostMessage, ConnectPostMessage{
			IsBase64Publish: false,
			MessageID:       65536,
			Payload:         appSettingPublishJSON,
			QoS:             packets.QOS_LEVEL_1,
			Topic:           "/ls_app_settings",
		})
		if !slices.Contains(payload.SubscribedTopics, "/ls_foreground_state") {
			payload.SubscribedTopics = append(payload.SubscribedTopics, "/ls_foreground_state")
		}
		if !slices.Contains(payload.SubscribedTopics, "/ls_resp") {
			payload.SubscribedTopics = append(payload.SubscribedTopics, "/ls_resp")
		}
	}

	jsonData, err := json.Marshal(payload)
	return string(jsonData), err
}

type AppSettingsPublish struct {
	LsFdid        string `json:"ls_fdid"`
	SchemaVersion string `json:"ls_sv"`
}

func (s *Socket) newAppSettingsPublishJSON(versionId int64) (string, error) {
	payload := &AppSettingsPublish{
		LsFdid:        "",
		SchemaVersion: strconv.Itoa(int(versionId)),
	}

	jsonData, err := json.Marshal(payload)
	return string(jsonData), err
}
