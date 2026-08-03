package messagix

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/lightspeed"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type TransientDisconnectEvent struct {
	Err                error
	ConnectionAttempts int
}

type PermanentErrorEvent struct {
	Err error
}

type ReconnectedEvent struct{}
type ConnectedEvent struct{}

var (
	minimalFBSync = []int64{1, 2, 95, 104}
	// Business Inbox thread groups 127 and 205 are hydrated by the embedded
	// Page snapshot and continued with task 313. Asking the socket to sync them
	// as databases stalls because Meta only accepts its browser-owned cursors.
	businessSuiteSync = []int64{2, 26}

	shouldRecurseDatabase = map[int64]bool{
		1:   true,
		2:   true,
		95:  true,
		104: true,
		26:  true,
		127: true,
		205: true,
	}
)

func initialSyncDatabases(platform types.Platform) []int64 {
	if platform == types.BusinessSuite {
		return businessSuiteSync
	}
	return minimalFBSync
}

type SocketLSRequestPayload struct {
	AppID     string `json:"app_id"`
	Payload   string `json:"payload"`
	RequestID int    `json:"request_id"`
	Type      int    `json:"type"`
}

func (c *Client) onSocketConnect(ctx context.Context) error {
	c.canSendMessages.Set()

	reconnect := c.socketWasSynced.Load()
	initialSync := initialSyncDatabases(c.Platform)
	err := c.syncManager.ensureSyncedSocket(ctx, initialSync)
	if err != nil {
		return fmt.Errorf("failed to ensure initial databases are synced: %w", err)
	}

	if !reconnect {
		err = c.sendInitialThreadFetch(ctx)
		if err != nil {
			return err
		}
	}

	if reconnect {
		c.HandleEvent(ctx, &ReconnectedEvent{})
	} else {
		c.HandleEvent(ctx, &ConnectedEvent{})
	}
	c.socketWasSynced.Store(true)
	c.socketWasConnected.Store(true)

	return nil
}

func (c *Client) sendInitialThreadFetch(ctx context.Context) error {
	tskm := c.newTaskManager()
	if c.Platform == types.BusinessSuite {
		// The embedded Business Suite snapshot is unified and its group-127 cursor
		// may have advanced through Instagram rows. Start the Messenger-only fetch
		// without that mixed cursor so Page threads are not skipped.
		for _, task := range businessSuiteInitialThreadTasks("") {
			tskm.AddNewTask(task)
		}
		payload, err := tskm.FinalizePayload()
		if err != nil {
			return fmt.Errorf("failed to finalize Business Inbox sync tasks: %w", err)
		}
		response, err := c.makeLSRequest(ctx, payload, 3)
		if err != nil {
			return fmt.Errorf("failed to send Business Inbox sync tasks: %w", err)
		}
		return c.dispatchRequestedTable(ctx, response)
	}
	ptks := c.configs.ParentThreadKeys
	if len(ptks) == 0 {
		zerolog.Ctx(ctx).Warn().Msg("Parent thread keys are not known")
		ptks = []int64{-1}
	} else if !slices.Contains(ptks, -1) {
		zerolog.Ctx(ctx).Warn().Ints64("ptks", ptks).Msg("Parent thread keys don't contain -1")
		ptks = append(ptks, -1)
	}
	for _, tk := range ptks {
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            tk,
			ReferenceThreadKey:         0,
			ReferenceActivityTimestamp: 9999999999999,
			AdditionalPagesToFetch:     0,
			Cursor:                     c.syncManager.GetCursor(1),
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

	syncGroupKeyStore1 := c.syncManager.getSyncGroupKeyStore(1)
	if syncGroupKeyStore1 != nil && (syncGroupKeyStore1.ParentThreadKey != -1 || syncGroupKeyStore1.MinThreadKey != 0 || syncGroupKeyStore1.MinLastActivityTimestampMs != 9999999999999) {
		// TODO determine if this is needed at all, the ptks thing above might cover it
		zerolog.Ctx(ctx).Debug().Msg("Adding fetch threads task from sync group key store")
		//  syncGroupKeyStore95 := c.syncManager.getSyncGroupKeyStore(95)
		tskm.AddNewTask(&socket.FetchThreadsTask{
			IsAfter:                    0,
			ParentThreadKey:            syncGroupKeyStore1.ParentThreadKey,
			ReferenceThreadKey:         syncGroupKeyStore1.MinThreadKey,
			ReferenceActivityTimestamp: syncGroupKeyStore1.MinLastActivityTimestampMs,
			AdditionalPagesToFetch:     0,
			Cursor:                     c.syncManager.GetCursor(1),
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
		return fmt.Errorf("failed to finalize sync tasks: %w", err)
	}

	c.Logger.Trace().Any("data", string(payload)).Msg("Sync groups tasks")
	_, err = c.makeLSRequest(ctx, payload, 3)
	if err != nil {
		return fmt.Errorf("failed to send sync tasks: %w", err)
	}
	return nil
}

func businessSuiteInitialThreadTasks(messengerCursor string) []socket.Task {
	const (
		messengerSyncGroup = 205
		messengerFilter    = 24
	)
	makeTask := func(syncGroup, secondaryFilter int, cursor string) socket.Task {
		return &socket.FetchBusinessInboxThreadsTask{
			Cursor:                     cursor,
			Filter:                     0,
			FilterValue:                "",
			IsAfter:                    0,
			ParentThreadKey:            0,
			ReferenceActivityTimestamp: 9999999999999,
			ReferenceThreadKey:         0,
			SecondaryFilter:            secondaryFilter,
			SyncGroup:                  syncGroup,
		}
	}
	return []socket.Task{
		makeTask(messengerSyncGroup, messengerFilter, messengerCursor),
	}
}

func (c *Client) dispatchRequestedTable(ctx context.Context, response *PublishResponseData) error {
	if response == nil {
		return nil
	}
	tbl, err := response.Parse(ctx)
	if err != nil {
		return fmt.Errorf("failed to parse requested Business Inbox threads: %w", err)
	}
	c.PostHandlePublishResponse(tbl)
	c.HandleEvent(ctx, tbl)
	return nil
}

func (c *Client) handleFrame(ctx context.Context, frame []byte) error {
	var prd PublishResponseData
	err := json.Unmarshal(frame, &prd)
	if err != nil {
		return fmt.Errorf("failed to unmarshal publish response data: %w", err)
	}
	ch, ok := c.socketSyncWaiters.Pop(prd.RequestID)
	if ok {
		go func() {
			ch <- &prd
		}()
	} else if tbl, err := prd.Parse(ctx); err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to parse table")
	} else {
		c.HandleEvent(ctx, tbl)
	}
	return nil
}

func (c *Client) PostHandlePublishResponse(tbl *table.LSTable) {
	if c == nil {
		return
	}
	syncGroupsNeedUpdate := methods.NeedUpdateSyncGroups(tbl)
	if syncGroupsNeedUpdate {
		c.Logger.Debug().
			Any("LSExecuteFirstBlockForSyncTransaction", tbl.LSExecuteFirstBlockForSyncTransaction).
			Any("LSUpsertSyncGroupThreadsRange", tbl.LSUpsertSyncGroupThreadsRange).
			Msg("Updating sync groups")
		err := c.syncManager.updateSyncGroupCursors(tbl)
		if err != nil {
			c.Logger.Err(err).Msg("Failed to sync transactions from publish response event")
		}
	}
}

type PublishResponseData struct {
	RequestID int64    `json:"request_id,omitempty"`
	Payload   string   `json:"payload,omitempty"`
	Sp        []string `json:"sp,omitempty"` // dependencies
	Target    int      `json:"target,omitempty"`
}

func (pb *PublishResponseData) Parse(ctx context.Context) (*table.LSTable, error) {
	var lsData *lightspeed.LightSpeedData
	err := json.Unmarshal([]byte(pb.Payload), &lsData)
	if err != nil {
		return nil, err
	}

	tbl := &table.LSTable{}
	dependencies := table.SPToDepMap(pb.Sp)
	decoder := lightspeed.NewLightSpeedDecoder(dependencies, tbl)
	decoder.Decode(lsData.Steps)
	return tbl, nil
}

func (c *Client) encodeLSRequest(payload []byte, t int) (json.RawMessage, int64, error) {
	packetID := uint16(c.packetsSent.Add(1))
	if packetID == 0 {
		packetID = uint16(c.packetsSent.Add(1))
	}
	lsPayload := &SocketLSRequestPayload{
		AppID:     c.configs.BrowserConfigTable.CurrentUserInitialData.AppID,
		Payload:   string(payload),
		RequestID: int(packetID),
		Type:      t,
	}

	jsonPayload, err := json.Marshal(lsPayload)
	return jsonPayload, int64(packetID), err
}

func (c *Client) makeLSRequest(ctx context.Context, payload []byte, t int) (*PublishResponseData, error) {
	jsonPayload, _, err := c.encodeLSRequest(payload, t)
	if err != nil {
		return nil, err
	}
	if c.Platform == types.BusinessSuite {
		var request SocketLSRequestPayload
		if err = json.Unmarshal(jsonPayload, &request); err != nil {
			return nil, err
		}
		if t == 4 {
			err = c.businessSocket.publish(ctx, "/ls_req", jsonPayload, int64(request.RequestID))
			return nil, err
		}
		return c.businessSocket.request(ctx, jsonPayload, int64(request.RequestID))
	}

	resp, err := c.socket.DoOneOffStream(ctx, jsonPayload, t == 4)
	if err != nil {
		return nil, err
	} else if t == 4 {
		return nil, nil
	}
	var prd PublishResponseData
	err = json.Unmarshal(resp, &prd)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response payload: %w", err)
	}
	return &prd, nil
}
