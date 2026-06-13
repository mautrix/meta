package messagix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"go.mau.fi/util/exhttp"

	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/packets"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const dgwLightSpeedEndpoint = "wss://gateway.facebook.com/ws/lightspeed"

type DGWLightSpeedSocket struct {
	client          *Client
	conn            *websocket.Conn
	responseHandler *ResponseHandler
	mu              sync.Mutex
	packetsSent     uint16
	streamsSent     uint16

	previouslyConnected bool
	lastReceived        atomic.Int64
}

func (c *Client) newDGWLightSpeedSocket() *DGWLightSpeedSocket {
	return &DGWLightSpeedSocket{
		client: c,
		responseHandler: &ResponseHandler{
			client:          c,
			requestChannels: make(map[uint16]chan any),
			packetChannels:  make(map[uint16]chan any),
		},
	}
}

func (s *DGWLightSpeedSocket) CanConnect() error {
	if s.conn != nil {
		return fmt.Errorf("DGW Lightspeed: %w", socket.ErrSocketAlreadyOpen)
	} else if !s.client.IsAuthenticated() {
		return fmt.Errorf("DGW Lightspeed: %w", socket.ErrNotAuthenticated)
	}
	return nil
}

func (s *DGWLightSpeedSocket) Connect(ctx context.Context) error {
	if err := s.CanConnect(); err != nil {
		return err
	}

	opts := &websocket.DialOptions{
		// c.http can't be used directly: coder/websocket rejects clients with a Timeout set.
		HTTPClient: &http.Client{Transport: s.client.http.Transport},
		HTTPHeader: s.getConnHeaders(),
	}
	socketURL := s.getConnURL()

	s.client.Logger.Debug().Str("url", socketURL).Msg("Dialing DGW Lightspeed socket")
	conn, resp, err := websocket.Dial(ctx, socketURL, opts)
	if err != nil {
		statusCode := 999
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return fmt.Errorf("DGW Lightspeed: %w: %w (status code %d)", socket.ErrDial, err, statusCode)
	}
	conn.SetReadLimit(-1)
	s.conn = conn
	s.lastReceived.Store(time.Now().UnixNano())

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	readDone := make(chan error, 1)
	go func() {
		readDone <- s.readLoop(runCtx, conn)
		cancel()
	}()

	if err = s.handleReady(runCtx); err != nil {
		_ = conn.CloseNow()
		cancel()
		readErr := <-readDone
		if readErr != nil && !errors.Is(readErr, context.Canceled) {
			s.client.Logger.Debug().Err(readErr).Msg("DGW Lightspeed read loop stopped after ready failure")
		}
		return fmt.Errorf("DGW Lightspeed ready failed: %w", err)
	}

	err = <-readDone
	if err != nil {
		return fmt.Errorf("DGW Lightspeed: %w: %w", socket.ErrInReadLoop, err)
	}
	return nil
}

func (s *DGWLightSpeedSocket) Disconnect() {
	if s != nil && s.conn != nil {
		_ = s.conn.Close(websocket.StatusNormalClosure, "")
	}
}

func (s *DGWLightSpeedSocket) readLoop(ctx context.Context, conn *websocket.Conn) error {
	defer func() {
		s.conn = nil
		s.responseHandler.CancelAllRequests()
	}()

	done := make(chan struct{})
	defer close(done)

	go s.pingLoop(ctx, conn, done)
	go s.pongTimeoutLoop(conn, done)

	s.client.Logger.Debug().Msg("DGW Lightspeed connection established, starting read loop")
	for {
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				s.client.Logger.Info().Int("code", int(ce.Code)).Str("text", ce.Reason).Msg("DGW Lightspeed websocket closed by server")
				return fmt.Errorf("closed by server: %d %s", ce.Code, ce.Reason)
			}
			s.client.Logger.Err(err).Msg("Error reading message from DGW Lightspeed socket")
			return fmt.Errorf("failed to read message: %w", err)
		}

		s.lastReceived.Store(time.Now().UnixNano())
		switch messageType {
		case websocket.MessageText:
			s.client.Logger.Warn().Bytes("bytes", payload).Msg("Unexpected text message in DGW Lightspeed websocket")
		case websocket.MessageBinary:
			s.handleBinaryMessage(ctx, payload)
		}
	}
}

func (s *DGWLightSpeedSocket) pingLoop(ctx context.Context, conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.sendFrames(&dgw.PingFrame{}); err != nil {
				s.client.Logger.Err(err).Msg("Error sending DGW Lightspeed ping")
				_ = conn.CloseNow()
				return
			}
		case <-done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *DGWLightSpeedSocket) pongTimeoutLoop(conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			lastReceived := time.Unix(0, s.lastReceived.Load())
			if time.Since(lastReceived) > PongTimeout {
				s.client.Logger.Error().Msg("DGW Lightspeed pong timeout")
				_ = conn.CloseNow()
				return
			}
		case <-done:
			return
		}
	}
}

func (s *DGWLightSpeedSocket) handleBinaryMessage(ctx context.Context, data []byte) {
	for len(data) > 0 {
		frame := dgw.CheckFrameType(data)
		rest, err := frame.Unmarshal(data)
		if err != nil {
			s.client.Logger.Warn().Err(err).Bytes("bytes", data).Msg("Failed to unmarshal DGW Lightspeed frame, dropping websocket message")
			return
		}
		s.handleFrame(ctx, frame)
		data = rest
	}
}

func (s *DGWLightSpeedSocket) handleFrame(ctx context.Context, frame dgw.Frame) {
	switch f := frame.(type) {
	case *dgw.UnsupportedFrame:
		if f.MayHaveLostData() {
			s.client.Logger.Warn().Bytes("bytes", f.Raw).Msg("Encountered unknown DGW Lightspeed frame, dropping with potential data loss")
		}
	case *dgw.PingFrame:
		if err := s.sendFrames(&dgw.PongFrame{}); err != nil {
			s.client.Logger.Err(err).Msg("Failed to send DGW Lightspeed pong")
		}
	case *dgw.PongFrame:
		s.client.Logger.Trace().Msg("Got DGW Lightspeed pong")
	case *dgw.OpenFrame:
		if f.Parameters.StatusCode != 0 && f.Parameters.StatusCode != http.StatusOK {
			s.client.Logger.Warn().Any("frame", f).Msg("DGW Lightspeed stream open returned non-OK status")
		}
	case *dgw.AckFrame:
		s.client.Logger.Trace().Uint16("stream_id", uint16(f.StreamID)).Uint16("ack_id", f.AckID).Msg("Got DGW Lightspeed ack")
	case *dgw.DataFrame:
		if f.RequiresAck {
			if err := s.sendFrames(&dgw.AckFrame{StreamID: f.StreamID, AckID: f.AckID}); err != nil {
				s.client.Logger.Err(err).Uint16("stream_id", uint16(f.StreamID)).Uint16("ack_id", f.AckID).Msg("Failed to ack DGW Lightspeed data frame")
			}
		}
		s.handleDataFrame(ctx, f)
	}
}

func (s *DGWLightSpeedSocket) handleDataFrame(ctx context.Context, frame *dgw.DataFrame) {
	jsonPayload, err := extractDGWJSON(frame.Payload)
	if err != nil {
		s.client.Logger.Trace().Err(err).Bytes("payload", frame.Payload).Msg("Ignoring DGW Lightspeed data frame without JSON payload")
		return
	}

	var data PublishResponseData
	if err = json.Unmarshal(jsonPayload, &data); err != nil {
		s.client.Logger.Warn().Err(err).Bytes("json", jsonPayload).Msg("Failed to parse DGW Lightspeed publish response JSON")
		return
	}
	if data.Payload == "" {
		s.client.Logger.Trace().Bytes("json", jsonPayload).Msg("Ignoring DGW Lightspeed data frame without Lightspeed payload")
		return
	}
	resp := &Event_PublishResponse{
		Topic:             string(LS_RESP),
		Data:              data,
		MessageIdentifier: uint16(frame.StreamID),
		QoS:               packets.QOS_LEVEL_0,
	}
	s.handlePublishResponseEvent(ctx, resp)
}

func extractDGWJSON(payload []byte) ([]byte, error) {
	msgStart := bytes.IndexByte(payload, '{')
	msgEnd := bytes.LastIndexByte(payload, '}')
	if msgStart < 0 || msgEnd < msgStart {
		return nil, fmt.Errorf("JSON object not found")
	}
	return payload[msgStart : msgEnd+1], nil
}

func (s *DGWLightSpeedSocket) handlePublishResponseEvent(ctx context.Context, resp *Event_PublishResponse) {
	requestID := uint16(resp.Data.RequestID)
	hasRequest := s.responseHandler.hasPacket(requestID)
	switch resp.Topic {
	case string(LS_RESP):
		resp.Finish()
		if hasRequest {
			if ok := s.responseHandler.updateRequestChannel(requestID, resp); !ok {
				s.client.Logger.Warn().Int64("request_id", resp.Data.RequestID).Msg("Dropped DGW Lightspeed response to request")
			}
		} else if resp.Data.RequestID == 0 {
			s.client.HandleEvent(ctx, resp)
		} else {
			s.client.Logger.Debug().Int64("request_id", resp.Data.RequestID).Msg("Got unexpected DGW Lightspeed publish response")
		}
	default:
		s.client.Logger.Info().Any("topic", resp.Topic).Any("data", resp.Data).Msg("Got unknown DGW Lightspeed publish response topic")
	}
}

func (c *Client) makeRealtimeLSRequest(ctx context.Context, payload []byte, t int) (*Event_PublishResponse, error) {
	if c.useDGWLightSpeedRealtime() {
		return c.dgwLightSpeed.makeLSRequest(ctx, payload, t)
	}
	return c.socket.makeLSRequest(ctx, payload, t)
}

func (s *DGWLightSpeedSocket) makeLSRequest(ctx context.Context, payload []byte, t int) (*Event_PublishResponse, error) {
	packetID := s.SafePacketID()
	streamID := s.SafeStreamID()
	lsPayload := &SocketLSRequestPayload{
		AppId:     s.client.messengerAppID(),
		Payload:   string(payload),
		RequestId: int(packetID),
		Type:      t,
	}

	jsonPayload, err := json.Marshal(lsPayload)
	if err != nil {
		return nil, err
	}

	if t != 4 {
		s.responseHandler.addRequestChannel(packetID)
	}
	err = s.sendFrames(
		&dgw.OpenFrame{StreamID: streamID},
		&dgw.DataFrame{
			StreamID:    streamID,
			Payload:     append([]byte{0x80}, jsonPayload...),
			RequiresAck: true,
			AckID:       0,
		},
	)
	if err != nil {
		if t != 4 {
			s.responseHandler.deleteDetails(packetID, RequestChannel)
		}
		return nil, err
	}

	if t == 4 {
		return nil, nil
	}
	return s.responseHandler.waitForPubResponseDetails(ctx, packetID)
}

func (s *DGWLightSpeedSocket) sendFrames(frames ...dgw.Frame) error {
	data, err := marshalDGWFrames(frames...)
	if err != nil {
		return err
	}
	return s.sendData(data)
}

func marshalDGWFrames(frames ...dgw.Frame) ([]byte, error) {
	var data []byte
	for _, frame := range frames {
		frameBytes, err := frame.Marshal()
		if err != nil {
			return nil, err
		}
		data = append(data, frameBytes...)
	}
	return data, nil
}

func (s *DGWLightSpeedSocket) sendData(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn := s.conn
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), WriteTimeout)
	defer cancel()
	err := conn.Write(ctx, websocket.MessageBinary, data)
	if exhttp.IsNetworkError(err) {
		closeErr := conn.CloseNow()
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			s.client.Logger.Debug().Err(closeErr).Msg("Error closing DGW Lightspeed connection after network error")
			return errors.Join(err, closeErr)
		}
		return err
	} else if err != nil {
		return fmt.Errorf("failed to write to DGW Lightspeed websocket: %w", err)
	}
	return nil
}

func (s *DGWLightSpeedSocket) SafePacketID() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.packetsSent++
	if s.packetsSent == 0 {
		s.packetsSent = 1
	}
	return s.packetsSent
}

func (s *DGWLightSpeedSocket) SafeStreamID() dgw.StreamID {
	s.mu.Lock()
	defer s.mu.Unlock()

	streamID := s.streamsSent
	s.streamsSent++
	return dgw.StreamID(streamID)
}

func (s *DGWLightSpeedSocket) handleReady(ctx context.Context) error {
	if s.previouslyConnected {
		s.client.canSendMessages.Set()
		err := s.client.syncManager.EnsureSyncedSocket(ctx, minimalFBReconnectSync)
		if err != nil {
			return fmt.Errorf("failed to sync after DGW Lightspeed reconnect: %w", err)
		}
		s.client.HandleEvent(ctx, &Event_Reconnected{})
		return nil
	}

	s.client.canSendMessages.Set()
	if err := s.sendInitialSyncTasks(ctx); err != nil {
		return err
	}
	if _, err := s.client.ExecuteTasks(ctx, &socket.ReportAppStateTask{AppState: table.FOREGROUND, RequestID: uuid.NewString()}); err != nil {
		return fmt.Errorf("failed to report app state: %w", err)
	}
	if err := s.client.syncManager.EnsureSyncedSocket(ctx, minimalFBInitialSync); err != nil {
		return fmt.Errorf("failed to ensure initial databases are synced through DGW Lightspeed: %w", err)
	}

	s.client.HandleEvent(ctx, (&Event_Ready{
		client:         s.client,
		ConnectionCode: CONNECTION_ACCEPTED,
	}).Finish())
	s.previouslyConnected = true

	return nil
}

func (s *DGWLightSpeedSocket) sendInitialSyncTasks(ctx context.Context) error {
	tskm := s.client.newTaskManager()
	ptks := s.client.configs.ParentThreadKeys
	if len(ptks) == 0 {
		s.client.Logger.Warn().Msg("Parent thread keys are not known")
		ptks = []int64{-1}
	} else if !slicesContainsInt64(ptks, -1) {
		s.client.Logger.Warn().Ints64("ptks", ptks).Msg("Parent thread keys don't contain -1")
		ptks = append(ptks, -1)
	}
	for _, tk := range ptks {
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            tk,
			ReferenceThreadKey:         0,
			ReferenceActivityTimestamp: 9999999999999,
			AdditionalPagesToFetch:     0,
			Cursor:                     s.client.syncManager.GetCursor(1),
			SyncGroup:                  1,
		})
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            tk,
			ReferenceThreadKey:         0,
			ReferenceActivityTimestamp: 9999999999999,
			AdditionalPagesToFetch:     0,
			SyncGroup:                  95,
		})
	}

	syncGroupKeyStore1 := s.client.syncManager.getSyncGroupKeyStore(1)
	if syncGroupKeyStore1 != nil {
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey:         syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch:     0,
			Cursor:                     s.client.syncManager.GetCursor(1),
			SyncGroup:                  1,
		})
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey:         syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch:     0,
			SyncGroup:                  95,
		})
	}

	payload, err := tskm.FinalizePayload()
	if err != nil {
		return fmt.Errorf("failed to finalize DGW Lightspeed sync tasks: %w", err)
	}

	s.client.Logger.Trace().Any("data", string(payload)).Msg("DGW Lightspeed sync groups tasks")
	if _, err = s.makeLSRequest(ctx, payload, 3); err != nil {
		return fmt.Errorf("failed to send DGW Lightspeed sync tasks: %w", err)
	}
	return nil
}

func slicesContainsInt64(values []int64, needle int64) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func (s *DGWLightSpeedSocket) getConnHeaders() http.Header {
	h := http.Header{}
	h.Set("cookie", s.client.cookies.String())
	h.Set("user-agent", useragent.UserAgent)
	h.Set("origin", s.client.GetEndpoint("base_url"))
	h.Set("cache-control", "no-cache")
	h.Set("pragma", "no-cache")
	return h
}

func (s *DGWLightSpeedSocket) getConnURL() string {
	query := &url.Values{}
	query.Add("x-dgw-appid", s.client.messengerAppID())
	query.Add("x-dgw-appversion", "0")
	query.Add("x-dgw-authtype", "1:0")
	query.Add("x-dgw-version", "5")
	query.Add("x-dgw-uuid", s.client.messengerDGWUUID())
	query.Add("x-dgw-tier", "prod")
	query.Add("x-dgw-loggingid", uuid.NewString())
	if region := s.client.configs.BrowserConfigTable.MessengerWebRegion.Region; region != "" {
		query.Add("x-dgw-regionhint", strings.ToUpper(region))
	}
	query.Add("x-dgw-deviceid", uuid.NewString())
	return dgwLightSpeedEndpoint + "?" + query.Encode()
}

func (c *Client) messengerAppID() string {
	if appID := nonZeroString(c.configs.BrowserConfigTable.CurrentUserInitialData.AppID); appID != "" {
		return appID
	}
	if appID := c.configs.BrowserConfigTable.MessengerWebInitData.AppID; appID != 0 {
		return strconv.FormatInt(appID, 10)
	}
	if appID := c.configs.BrowserConfigTable.MqttWebConfig.AppID; appID != 0 {
		return strconv.FormatInt(appID, 10)
	}
	return "2220391788200892"
}

func (c *Client) messengerDGWUUID() string {
	if accountID := nonZeroString(c.configs.BrowserConfigTable.CurrentUserInitialData.AccountID); accountID != "" {
		return accountID
	}
	if userID := nonZeroString(c.configs.BrowserConfigTable.CurrentUserInitialData.UserID); userID != "" {
		return userID
	}
	if userID := c.configs.BrowserConfigTable.MessengerWebInitData.UserID.String(); userID != "" {
		return userID
	}
	return strconv.FormatInt(c.cookies.GetUserID(), 10)
}

func nonZeroString(value string) string {
	if value == "" || value == "0" {
		return ""
	}
	return value
}
