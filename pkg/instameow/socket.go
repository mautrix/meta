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

	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/exstrings"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/mautrix-meta/pkg/instameow/mdCoreSync"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

var MaxConnectionRetryInterval = 60 * time.Second

func (c *Client) makeNewSocket() {
	old := c.socket.Swap(dgw.NewSocket(c.getSocketOptions()))
	if old != nil {
		old.Disconnect()
	}
	var newSC *dgw.Socket
	if c.enableTyping {
		newSC = dgw.NewSocket(c.getStreamControllerSocketOptions())
	}
	oldSC := c.streamControllerSocket.Swap(newSC)
	if oldSC != nil {
		oldSC.Disconnect()
	}
	c.disconnectMQTTBypass()
}

func (c *Client) Connect(ctx context.Context) {
	sock := c.socket.Load()
	if sock == nil {
		c.log.Error().Msg("Connect() called without initializing socket")
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.connectionCtx.Store(&ctx)
	if oldCancel := c.cancelSocket.Swap(&cancel); oldCancel != nil {
		(*oldCancel)()
		if !c.socketStopped.IsSet() {
			c.log.Warn().Msg("Socket was previously connected, waiting for it to stop")
			err := c.socketStopped.Wait(ctx)
			if err != nil {
				return
			}
			err = c.streamControllerStopped.Wait(ctx)
			if err != nil {
				return
			}
		}
	}

	c.connected.Clear()
	c.socketStopped.Clear()
	defer c.socketStopped.Set()
	c.streamControllerStopped.Clear()
	go c.connectStreamController(ctx)
	c.socketRetries = 0
	sequentialFailures := 0
	ctx = sock.Log.WithContext(ctx)
	for {
		err := sock.Connect(ctx)
		wasConnected := c.connected.IsSet()
		c.connected.Clear()
		if err == nil {
			sock.Log.Debug().Msg("Socket closed cleanly")
			return
		} else if ctx.Err() != nil {
			sock.Log.Debug().Err(err).Msg("Context canceled, stopping socket reconnect attempts")
			return
		} else if errors.Is(err, errResnapshotRequired) {
			sock.Log.Debug().Msg("Socket closed due to resnapshot requirement, dispatching event")
			_ = c.eventHandler(ctx, &slidetypes.ResnapshotRequired{})
			return
		}
		if dispatchErr := c.eventHandler(ctx, &slidetypes.Disconnected{Error: err}); dispatchErr != nil {
			sock.Log.Err(dispatchErr).Msg("Failed to dispatch disconnected event, not reconnecting")
			return
		}
		c.socketRetries++
		if !wasConnected {
			sequentialFailures++
			sock.Log.Warn().Err(err).
				Int("connection_num", c.socketRetries).
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
			sock.Log.Debug().Err(err).
				Int("connection_num", c.socketRetries).
				Msg("Socket disconnected, reconnecting immediately")
			sequentialFailures = 0
		}
	}
}

func (c *Client) ForceReconnect() {
	if c == nil {
		return
	}
	sock := c.socket.Load()
	if sock != nil {
		sock.ForceReconnect()
	}
}

func (c *Client) getSocketOptions() dgw.SocketOptions {
	return dgw.SocketOptions{
		GetCookies: c.cookies.String,
		Origin:     c.GetEndpoint("base_url"),
		WSURL:      "wss://gateway.instagram.com/ws/lightspeed",
		DialOpts:   *c.http.GetWebsocketDialer(),
		Log:        c.log.With().Str("socket", "dgw").Logger(),
		Facebook:   false,
		AppID:      c.configs.BrowserConfigTable.DGWWebConfig.AppID,
		UserID:     c.configs.BrowserConfigTable.PolarisViewer.ID,
		DeviceID:   c.configs.BrowserConfigTable.IGDMqttWebDeviceID.ClientID,
		OnConnect: func(ctx context.Context) error {
			_, err := c.socket.Load().EstablishStream(ctx, dgw.StreamInit{
				InitPayload:  exerrors.Must(c.makeStreamInitPayload(c.socketRetries)),
				FrameHandler: c.handleDataFrame,
			})
			if err != nil {
				c.log.Err(err).Msg("Failed to establish main stream")
			}
			return err
		},
	}
}

type connectPayload struct {
	AppID     string `json:"app_id"`
	DeviceID  string `json:"device_id"`
	Payload   string `json:"payload"`
	RequestID int    `json:"request_id"`
	Type      int    `json:"type"`
}

type syncParams struct {
	UserAgent                string             `json:"user_agent"`
	SnapshotAtMS             jsontime.UnixMilli `json:"snapshot_at_ms"`
	PrevalidatedGraphQLDocID string             `json:"prevalidated_graphql_doc_id"`
}

type seqIDCursor struct {
	SeqID int64 `json:"seq_id"`
}

func (c *Client) makeStreamInitPayload(retryCount int) (json.RawMessage, error) {
	marshaledSyncParams, err := json.Marshal(&syncParams{
		UserAgent:                useragent.IGDUserAgent,
		SnapshotAtMS:             jsontime.UM(c.seqIDTS),
		PrevalidatedGraphQLDocID: graphql.IGDSlideDeltaProcessorQueryInstagramRelayOperation,
	})
	if err != nil {
		return nil, err
	}
	marshaledCursor, err := json.Marshal(seqIDCursor{
		SeqID: c.seqID,
	})
	if err != nil {
		return nil, err
	}
	marshaledDatabaseQuery, err := json.Marshal(&socket.DatabaseQuery{
		Database:          223,
		LastAppliedCursor: ptr.Ptr(string(marshaledCursor)),
		SyncParams:        ptr.Ptr(string(marshaledSyncParams)),
		EpochId:           0,
		Version:           "-3",
		FailureCount:      retryCount,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(&connectPayload{
		AppID:     c.configs.BrowserConfigTable.DGWWebConfig.AppID,
		DeviceID:  c.socket.Load().DeviceID,
		Payload:   string(marshaledDatabaseQuery),
		RequestID: 4 + retryCount,
		Type:      2,
	})
}

type IGFrame struct {
	RequestID *int   `json:"request_id"`
	Payload   []byte `json:"payload"`
}

func (c *Client) handleDataFrame(ctx context.Context, frame []byte) error {
	var igFrame IGFrame
	err := json.Unmarshal(frame, &igFrame)
	if err != nil {
		return fmt.Errorf("failed to unmarshal outermost JSON layer: %w", err)
	}
	var lsResponse mdCoreSync.LSResponse
	err = proto.Unmarshal(igFrame.Payload, &lsResponse)
	if err != nil {
		return fmt.Errorf("failed to unmarshal protobuf layer: %w", err)
	}
	for _, op := range lsResponse.Operations {
		err = c.handleOperation(ctx, op)
		if err != nil {
			return err
		}
	}
	return nil
}

var errResnapshotRequired = errors.New("resnapshot required")

func (c *Client) handleOperation(ctx context.Context, rawOp *mdCoreSync.Operation) error {
	switch op := rawOp.Operation.(type) {
	case *mdCoreSync.Operation_Subscribe:
		c.connected.Set()
		c.log.Info().Any("subscribe_payload", op.Subscribe).Msg("Successfully subscribed to iris socket")
		err := c.eventHandler(ctx, &slidetypes.Connected{
			LatestSeqID:     op.Subscribe.GetLatestSeqID(),
			SubscribedSeqID: op.Subscribe.GetSubscribedSeqID(),
		})
		if err != nil {
			return fmt.Errorf("failed to handle connected event: %w", err)
		}
		return nil
	case *mdCoreSync.Operation_Resnapshot:
		c.log.Info().Any("resnapshot_payload", op.Resnapshot).Msg("Received resnapshot operation")
		return fmt.Errorf("%w (%d): %s", errResnapshotRequired, op.Resnapshot.GetErrorType(), op.Resnapshot.GetErrorMessage())
	case *mdCoreSync.Operation_Delta:
		var deltaWrappers []*slidetypes.DeltaWrapper
		err := json.Unmarshal(exstrings.UnsafeBytes(op.Delta.GetDelta()), &deltaWrappers)
		if err != nil {
			return fmt.Errorf("failed to unmarshal delta: %w", err)
		}
		var saveSeqID bool
		for _, deltas := range deltaWrappers {
			for _, delta := range deltas.Data.SlideDeltaProcessor {
				err = c.eventHandler(ctx, delta)
				if err != nil {
					return fmt.Errorf("failed to handle delta: %w", err)
				}
				if c.seqID < delta.UQSeqID {
					c.seqID = delta.UQSeqID
					c.seqIDTS = time.Now()
					saveSeqID = true
				}
			}
		}
		if saveSeqID {
			err = c.eventHandler(ctx, &slidetypes.SeqIDUpdate{
				SeqID:     c.seqID,
				Timestamp: c.seqIDTS,
			})
			if err != nil {
				return fmt.Errorf("failed to save sequence ID: %w", err)
			}
		}
		return nil
	case *mdCoreSync.Operation_Noop:
		return nil
	case *mdCoreSync.Operation_Failure:
		// This will reconnect the websocket to reset the stream
		return fmt.Errorf("received sync failure for %d: %s", op.Failure.GetGroupID(), op.Failure.GetErrorMessage())
	default:
		return fmt.Errorf("unrecognized operation type: %T", rawOp.Operation)
	}
}

func (c *Client) Disconnect() {
	if sock := c.socket.Load(); sock != nil {
		sock.Disconnect()
	}
	if scSock := c.streamControllerSocket.Load(); scSock != nil {
		scSock.Disconnect()
	}
	c.disconnectMQTTBypass()
	c.connectionCtx.Store(nil)
	cancel := c.cancelSocket.Swap(nil)
	if cancel != nil {
		(*cancel)()
	}
	timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
	err := c.socketStopped.Wait(timeoutCtx)
	if err != nil {
		c.log.Warn().Err(err).Msg("Connection loop didn't exit in time")
	} else if err = c.streamControllerStopped.Wait(timeoutCtx); err != nil {
		c.log.Warn().Err(err).Msg("Stream controller socket didn't exit in time")
	}
	cancelTimeout()
}
