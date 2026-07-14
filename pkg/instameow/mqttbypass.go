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

package instameow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/instameow/thrift"
	"go.mau.fi/mautrix-meta/pkg/instameow/thrift/mqttbypass"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
)

type indicateActivity struct {
	Action         string `json:"action"`
	ActivityStatus int    `json:"activity_status"`
	ClientContext  string `json:"client_context"`
	ThreadID       string `json:"thread_id"`
}

func (c *Client) SetTyping(ctx context.Context, threadID string, typing bool) error {
	if !c.enableTyping {
		return nil
	}
	req := &indicateActivity{
		Action:         "indicate_activity",
		ActivityStatus: 1,
		ThreadID:       threadID,
	}
	if !typing {
		req.ActivityStatus = 0
	}
	marshaledReq, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal indicate activity request: %w", err)
	}
	payload, err := thrift.Marshal(&mqttbypass.RequestPayload{
		PublishRequest: &mqttbypass.PublishRequest{
			Payload:   marshaledReq,
			MqttTopic: "/ig_send_message",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal mqttbypass request: %w", err)
	}

	cc := c.connectionCtx.Load()
	if cc == nil {
		return fmt.Errorf("connection context is not set")
	}
	if !c.mqttBypassConnected.IsSet() {
		go c.connectMQTTBypassSocket(*cc)
	}
	if err = c.mqttBypassConnected.WaitTimeoutCtx(ctx, 5*time.Second); err != nil {
		return fmt.Errorf("failed to wait for mqttbypass connection: %w", err)
	} else if stream := c.mqttBypassStream.Load(); stream == nil {
		return fmt.Errorf("mqttbypass stream is not established")
	} else if err = stream.SendDataNoAck(ctx, payload); err != nil {
		return fmt.Errorf("failed to send mqttbypass request: %w", err)
	}
	return nil
}

func (c *Client) getMQTTBypassSocketOptions() dgw.SocketOptions {
	return dgw.SocketOptions{
		GetCookies: c.cookies.String,
		Origin:     c.GetEndpoint("base_url"),
		WSURL:      c.GetEndpoint("dgw_mqttbypass"),
		DialOpts:   *c.http.GetWebsocketDialer(),
		Log:        c.log.With().Str("socket", "mqttbypass").Logger(),
		Facebook:   false,
		LoggingID:  true,
		AppID:      c.configs.BrowserConfigTable.DGWWebConfig.AppID,
		UserID:     c.configs.BrowserConfigTable.PolarisViewer.ID,
		DeviceID:   c.configs.BrowserConfigTable.IGDMqttWebDeviceID.ClientID,
		OnConnect: func(ctx context.Context) error {
			payload, err := thrift.Marshal(&mqttbypass.RequestPayload{
				ConnectRequest: &mqttbypass.ConnectRequest{
					DeviceId:     c.configs.BrowserConfigTable.IGDMqttWebDeviceID.ClientID,
					Subscription: &mqttbypass.SubscribeRequest{Topics: []string{}},
				},
			})
			if err != nil {
				return err
			}
			stream, err := c.mqttBypassSocket.EstablishStream(ctx, dgw.StreamInit{
				InitPayload:  payload,
				FrameHandler: c.handleMQTTBypassFrame,
			})
			if err != nil {
				c.log.Err(err).Msg("Failed to establish main stream")
			} else {
				c.mqttBypassStream.Store(stream)
				c.mqttBypassConnected.Set()
			}
			return err
		},
	}
}

func (c *Client) connectMQTTBypassSocket(ctx context.Context) {
	c.mqttBypassConnectLock.Lock()
	sock := c.mqttBypassSocket
	if sock == nil {
		sock = dgw.NewSocket(c.getMQTTBypassSocketOptions())
		c.mqttBypassSocket = sock
		c.mqttBypassConnected.Clear()
	}
	c.mqttBypassConnectLock.Unlock()

	err := sock.Connect(ctx)
	if errors.Is(err, dgw.ErrSocketAlreadyOpen) {
		return
	}
	if err != nil {
		zerolog.Ctx(ctx).Debug().Err(err).Msg("MQTT bypass socket connection lost")
	}

	c.mqttBypassConnectLock.Lock()
	if c.mqttBypassSocket == nil || c.mqttBypassSocket == sock {
		c.mqttBypassStream.Store(nil)
		c.mqttBypassConnected.Clear()
	}
	c.mqttBypassConnectLock.Unlock()
}

func (c *Client) disconnectMQTTBypass() {
	c.mqttBypassConnectLock.Lock()
	defer c.mqttBypassConnectLock.Unlock()
	if c.mqttBypassSocket != nil {
		c.mqttBypassSocket.Disconnect()
		c.mqttBypassSocket = nil
	}
}

func (c *Client) handleMQTTBypassFrame(ctx context.Context, frame []byte) error {
	logEvt := zerolog.Ctx(ctx).Trace()
	if !logEvt.Enabled() {
		logEvt.Discard().Send()
		return nil
	}
	var resp mqttbypass.ResponsePayload
	err := thrift.Unmarshal(frame, &resp)
	if err != nil {
		logEvt.Discard().Send()
		return fmt.Errorf("failed to unmarshal mqttbypass frame: %w", err)
	}
	logEvt.Any("payload", &resp).Msg("Received mqttbypass frame")
	return nil
}
