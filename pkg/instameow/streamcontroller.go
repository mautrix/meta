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
	"fmt"
	"regexp"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/instameow/thrift"
	"go.mau.fi/mautrix-meta/pkg/instameow/thrift/requeststream"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
)

func (c *Client) connectStreamController(ctx context.Context) {
	defer c.streamControllerStopped.Set()
	sequentialFailures := 0
	sock := c.streamControllerSocket.Load()
	if sock == nil {
		return
	}
	ctx = sock.Log.WithContext(ctx)
	for {
		err := sock.Connect(ctx)
		wasConnected := c.streamControllerConnected.Swap(false)
		if err == nil {
			sock.Log.Debug().Msg("Socket closed cleanly")
			return
		} else if ctx.Err() != nil {
			sock.Log.Debug().Err(err).Msg("Context canceled, stopping socket reconnect attempts")
			return
		}
		if !wasConnected {
			sequentialFailures++
			sock.Log.Warn().Err(err).
				Int("sequential_failures", sequentialFailures).
				Msg("Connection failed, reconnecting...")
			select {
			case <-ctx.Done():
				sock.Log.Debug().Msg("Context canceled, stopping socket reconnect attempts")
				return
			case <-time.After(min(time.Duration(1<<sequentialFailures)*time.Second, MaxConnectionRetryInterval)):
				continue
			}
		} else {
			sock.Log.Debug().Err(err).Msg("Socket disconnected, reconnecting immediately")
			sequentialFailures = 0
		}
	}
}

type typingSubscribeParameters struct {
	Method      string `json:"x-dgw-app-XRSS-method"`
	DocID       string `json:"x-dgw-app-XRSS-doc_id"`
	RoutingHint string `json:"x-dgw-app-XRSS-routing_hint"`
	Body        string `json:"x-dgw-app-xrs-body"`
	AcceptAck   string `json:"x-dgw-app-XRS-Accept-Ack"`
	HTTPReferer string `json:"x-dgw-app-XRSS-http_referer"`
}

type typingSubscribePayload struct {
	InputData struct {
		UserID string `json:"user_id"`
	} `json:"input_data"`
	Options struct {
		UseOSSResponseFormat        bool `json:"useOSSResponseFormat"`
		ClientHasODSUsecaseCounters bool `json:"client_has_ods_usecase_counters"`
	} `json:"%options"`
}

func (c *Client) getStreamControllerSocketOptions() dgw.SocketOptions {
	return dgw.SocketOptions{
		GetCookies:     c.cookies.String,
		Origin:         c.GetEndpoint("base_url"),
		WSURL:          c.GetEndpoint("dgw_streamcontroller"),
		DialOpts:       *c.http.GetWebsocketDialer(),
		Log:            c.log.With().Str("socket", "streamcontroller").Logger(),
		AppStreamGroup: "group1",
		AppID:          c.configs.BrowserConfigTable.DGWWebConfig.AppID,
		UserID:         c.configs.BrowserConfigTable.PolarisViewer.ID,
		DeviceID:       c.configs.BrowserConfigTable.IGDMqttWebDeviceID.ClientID,
		OnConnect: func(ctx context.Context) error {
			// Note: the official web app subscribes to multiple streams like presence and other realtime events,
			// but we only support typing for now
			params, err := json.Marshal(&typingSubscribeParameters{
				Method:      "FBGQLS:XDT_DIRECT_REALTIME_EVENT",
				DocID:       graphql.IGDTypingIndicatorClientSubscription,
				RoutingHint: "IGDTypingIndicatorClientSubscription",
				Body:        "true",
				AcceptAck:   "RSAck",
				HTTPReferer: "https://www.instagram.com/direct/",
			})
			if err != nil {
				return fmt.Errorf("failed to marshal typing subscribe parameters: %w", err)
			}
			stream, err := c.streamControllerSocket.Load().EstablishStream(ctx, dgw.StreamInit{
				Parameters:   params,
				FrameHandler: c.handleStreamControllerFrame,
			})
			if err != nil {
				c.log.Err(err).Msg("Failed to establish typing stream")
				return err
			}
			var tsp typingSubscribePayload
			tsp.InputData.UserID = c.configs.BrowserConfigTable.PolarisViewer.ID
			tsp.Options.UseOSSResponseFormat = true
			tsp.Options.ClientHasODSUsecaseCounters = true
			tspMarshaled, err := json.Marshal(tsp)
			if err != nil {
				return fmt.Errorf("failed to marshal typing subscribe payload: %w", err)
			}
			postEstablishPayload, err := thrift.Marshal(&requeststream.Payload{
				RequestBody: &requeststream.RequestStreamBody{
					Body: tspMarshaled,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to marshal post-establish payload: %w", err)
			}
			return stream.SendData(ctx, postEstablishPayload)
		},
	}
}

var typingPathRegex = regexp.MustCompile(`^/direct_v2/threads/(\d+)/activity_indicator_id/.+$`)

func (c *Client) handleStreamControllerFrame(ctx context.Context, frame []byte) error {
	var payload requeststream.Payload
	err := thrift.Unmarshal(frame, &payload)
	if err != nil {
		return fmt.Errorf("failed to unmarshal streamcontroller frame: %w", err)
	}
	zerolog.Ctx(ctx).Trace().Any("payload", &payload).Msg("Received streamcontroller frame")
	if payload.Response == nil {
		return nil
	}
	for _, delta := range payload.Response.Delta {
		if delta.GetFlowStatus() == requeststream.FlowStatus_Started {
			zerolog.Ctx(ctx).Debug().Msg("Successfully connected to stream controller for typing notifications")
			c.streamControllerConnected.Store(true)
		}
		if delta.Data == nil {
			continue
		}
		unexpectedData := false
		var evt slidetypes.StreamControllerPayload
		if err = json.Unmarshal(delta.Data.Bytes, &evt); err != nil {
			zerolog.Ctx(ctx).Err(err).
				Bytes("data", delta.Data.Bytes).
				Msg("Failed to unmarshal stream controller payload")
		} else if evt.Data.XDTDirectRealtimeEvent.Event != "patch" {
			unexpectedData = true
		} else {
			for _, p := range evt.Data.XDTDirectRealtimeEvent.Data {
				var realEvt slidetypes.TypingNotification
				if p.Op != "add" {
					unexpectedData = true
				} else if match := typingPathRegex.FindStringSubmatch(p.Path); len(match) != 2 {
					unexpectedData = true
				} else if err = json.Unmarshal([]byte(p.Value), &realEvt); err != nil {
					zerolog.Ctx(ctx).Err(err).
						Str("path", p.Path).
						Str("value", p.Value).
						Msg("Failed to unmarshal typing notification event")
				} else {
					realEvt.ThreadID = match[1]
					err = c.eventHandler(ctx, &realEvt)
					if err != nil {
						return fmt.Errorf("failed to handle typing notification event: %w", err)
					}
				}
			}
		}
		if unexpectedData {
			zerolog.Ctx(ctx).Err(err).
				RawJSON("data", delta.Data.Bytes).
				Msg("Unexpected data in stream controller payload")
		}
	}
	return nil
}
