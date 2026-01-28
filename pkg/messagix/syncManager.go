package messagix

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type SyncManager struct {
	client *Client
	// for syncing / cursors
	store map[int64]*socket.QueryMetadata
	// for thread/message fetching
	keyStore   map[int64]*socket.KeyStoreData
	syncParams *types.LSPlatformMessengerSyncParams
}

func (c *Client) newSyncManager() *SyncManager {
	return &SyncManager{
		client: c,
		store: map[int64]*socket.QueryMetadata{
			1: {SendSyncParams: false, SyncChannel: socket.MailBox},
			2: {SendSyncParams: true, SyncChannel: socket.Contact}, // FB/IG sync params (previously null?): {"locale": "en_US"}
			//2:   {SendSyncParams: false, SyncChannel: socket.Contact},
			5:   {SendSyncParams: true}, // FB sync params: {"locale": "en_US"} TODO may be removed
			6:   {SendSyncParams: true}, // IG sync params: {"locale": "en_US"}
			7:   {SendSyncParams: true}, // IG sync params: {"mnet_rank_types": [44]}
			16:  {SendSyncParams: true}, // FB/IG sync params: {"locale": "en_US"}
			26:  {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			28:  {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			89:  {SendSyncParams: true}, // FB/IG sync params: {"locale": "en_US"}
			95:  {SendSyncParams: false, SyncChannel: socket.Contact},
			104: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			120: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			140: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			141: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			142: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			143: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			145: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			196: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
			197: {SendSyncParams: true}, // FB/IG sync params: {"locale": "en_US"}
			198: {SendSyncParams: true}, // FB/IG sync params: {"locale": "en_US"}
			202: {SendSyncParams: true}, // FB sync params: {"locale": "en_US"}
		},
		keyStore: map[int64]*socket.KeyStoreData{
			1:  {MinThreadKey: 0, ParentThreadKey: -1, MinLastActivityTimestampMs: 9999999999999, HasMoreBefore: false},
			95: {MinThreadKey: 0, ParentThreadKey: -1, MinLastActivityTimestampMs: 9999999999999, HasMoreBefore: false},
		},
		syncParams: &types.LSPlatformMessengerSyncParams{},
	}
}

func (sm *SyncManager) syncSocketData(ctx context.Context, db int64, cb func()) {
	defer cb()
	database, ok := sm.store[db]
	if !ok {
		sm.client.Logger.Error().Int64("database_id", db).Msg("Could not find sync store for database")
		return
	}

	err := sm.SyncSocketData(ctx, db, database)
	if err != nil {
		sm.client.Logger.Err(err).Int64("database_id", db).Msg("Failed to sync database through socket")
	} else {
		sm.client.Logger.Debug().Any("database_id", db).Any("database", database).Msg("Synced database")
	}
}

func (sm *SyncManager) EnsureSyncedSocket(ctx context.Context, databases []int64) error {
	var wg sync.WaitGroup
	wg.Add(len(databases))
	for _, db := range databases {
		go sm.syncSocketData(ctx, db, wg.Done)
	}
	wg.Wait()

	return nil
}

func (sm *SyncManager) SyncSocketData(ctx context.Context, databaseID int64, db *socket.QueryMetadata) error {
	var t int
	payload := &socket.DatabaseQuery{
		Database: databaseID,
		Version:  json.Number(strconv.FormatInt(sm.client.configs.VersionID, 10)),
		EpochId:  methods.GenerateEpochID(),
	}

	var prevCursor string
	if db.LastAppliedCursor != nil {
		prevCursor = *db.LastAppliedCursor
	}
	if db.SendSyncParams {
		t = 1
		payload.SyncParams = sm.getSyncParams(databaseID, db.SyncChannel)
	} else {
		t = 2
		payload.LastAppliedCursor = db.LastAppliedCursor
	}

	jsonPayload, err := json.Marshal(&payload)
	if err != nil {
		return fmt.Errorf("failed to marshal DatabaseQuery struct into json bytes (databaseID=%d): %w", databaseID, err)
	}

	sm.client.Logger.Trace().
		RawJSON("payload", jsonPayload).
		Int64("database_id", databaseID).
		Msg("Syncing database via socket")
	resp, err := sm.client.socket.makeLSRequest(ctx, jsonPayload, t)
	if err != nil {
		return fmt.Errorf("failed to make lightspeed socket request with DatabaseQuery byte payload (databaseID=%d): %w", databaseID, err)
	}

	resp.Finish()

	if len(resp.Table.LSHandleSyncFailure) > 0 {
		// TODO handle these somehow?
		sm.client.Logger.Warn().
			Any("sync_failures", resp.Table.LSHandleSyncFailure).
			Msg("Sync failures found")
	}
	if len(resp.Table.LSExecuteFirstBlockForSyncTransaction) == 0 {
		sm.client.Logger.Warn().
			Any("database_id", databaseID).
			Any("payload", string(jsonPayload)).
			Any("response", resp.Data).
			Any("table", resp.Table).
			Msg("No transactions found")
		return nil
	}
	block := resp.Table.LSExecuteFirstBlockForSyncTransaction[0]
	nextCursor, currentCursor := block.NextCursor, block.CurrentCursor
	sm.client.Logger.Debug().
		Any("full_block", block).
		Any("block_response", block).
		Any("database_id", payload.Database).
		Any("payload", string(jsonPayload)).
		Strs("response_table_fields", resp.Table.NonNilFields()).
		Msg("Synced database")
	// TODO remove this after confirming there's nothing useful
	sm.client.HandleEvent(ctx, resp)
	if nextCursor == currentCursor || nextCursor == prevCursor || nextCursor == "dummy_cursor" || nextCursor == "" || !shouldRecurseDatabase[databaseID] {
		return nil
	}

	// Update the last applied cursor to the next cursor and recursively fetch again
	db.LastAppliedCursor = &nextCursor
	db.SendSyncParams = block.SendSyncParams
	db.SyncChannel = socket.SyncChannel(block.SyncChannel)
	err = sm.updateSyncGroupCursors(resp.Table) // Also sync the transaction with the store map because the db param is just a copy of the map entry
	if err != nil {
		return err
	}

	return sm.SyncSocketData(ctx, databaseID, db)
}

func (sm *SyncManager) SyncDataGraphQL(ctx context.Context, dbs []int64) (*table.LSTable, error) {
	var tableData *table.LSTable
	for _, db := range dbs {
		database, ok := sm.store[db]
		if !ok {
			return nil, fmt.Errorf("could not find sync store for database: %d", db)
		}

		variables := &graphql.LSPlatformGraphQLLightspeedVariables{
			Database:          int(db),
			LastAppliedCursor: database.LastAppliedCursor,
			Version:           sm.client.configs.VersionID,
			EpochID:           0,
		}
		if database.SendSyncParams {
			variables.SyncParams = sm.getSyncParams(db, database.SyncChannel)
		}

		lsTable, err := sm.client.makeLSRequest(ctx, variables, 1)
		if err != nil {
			return nil, err
		}

		if db == 1 {
			tableData = lsTable
		}
		err = sm.updateSyncGroupCursors(lsTable)
		if err != nil {
			return nil, err
		}
	}

	return tableData, nil
}

func (sm *SyncManager) SyncTransactions(transactions []*table.LSExecuteFirstBlockForSyncTransaction) error {
	for _, transaction := range transactions {
		database, ok := sm.store[transaction.DatabaseID]
		if !ok {
			return fmt.Errorf("failed to update database %d by block transaction", transaction.DatabaseID)
		}

		database.LastAppliedCursor = &transaction.NextCursor
		database.SendSyncParams = transaction.SendSyncParams
		database.SyncChannel = socket.SyncChannel(transaction.SyncChannel)
		sm.client.Logger.Debug().
			Any("new_cursor", database.LastAppliedCursor).
			Any("sync_channel", database.SyncChannel).
			Any("send_sync_params", database.SendSyncParams).
			Any("database_id", transaction.DatabaseID).
			Msg("Updated database by transaction...")
	}

	return nil
}

func (sm *SyncManager) UpdateDatabaseSyncParams(dbs []*socket.QueryMetadata) error {
	for _, db := range dbs {
		database, ok := sm.store[db.DatabaseId]
		if !ok {
			return fmt.Errorf("failed to update sync params for database: %d", db.DatabaseId)
		}
		database.SendSyncParams = db.SendSyncParams
		database.SyncChannel = db.SyncChannel
	}
	return nil
}

var dbID7Params = `{"mnet_rank_types":[44]}`

func (sm *SyncManager) getSyncParams(dbID int64, ch socket.SyncChannel) *string {
	if dbID == 7 {
		return &dbID7Params
	}
	switch ch {
	case socket.MailBox:
		return &sm.syncParams.Mailbox
	case socket.Contact:
		return &sm.syncParams.Contact
	default:
		return &sm.syncParams.E2Ee
	}
}

func (sm *SyncManager) GetCursor(db int64) string {
	database, ok := sm.store[db]
	if !ok || database.LastAppliedCursor == nil {
		return ""
	}
	return *database.LastAppliedCursor
}

func (c *Client) GetCursor(db int64) string {
	if c == nil || c.syncManager == nil {
		return ""
	}
	return c.syncManager.GetCursor(db)
}

func (sm *SyncManager) updateThreadRanges(ranges []*table.LSUpsertSyncGroupThreadsRange) error {
	var err error
	for _, syncGroupData := range ranges {
		if !syncGroupData.HasMoreBefore {
			continue
		}
		syncGroup := syncGroupData.SyncGroup
		keyStore, ok := sm.keyStore[syncGroup]
		if !ok {
			err = fmt.Errorf("could not find keyStore by database ID %d", syncGroup)
			sm.client.Logger.Error().
				Any("sync_group_data", syncGroupData).
				Int64("database_id", syncGroup).
				Msg("Could not find key store by database ID")
			continue
		}
		keyStore.HasMoreBefore = syncGroupData.HasMoreBefore
		keyStore.MinLastActivityTimestampMs = syncGroupData.MinLastActivityTimestampMS
		keyStore.MinThreadKey = syncGroupData.MinThreadKey
		keyStore.ParentThreadKey = syncGroupData.ParentThreadKey

		sm.client.Logger.Debug().
			Any("key_store", keyStore).
			Any("database_id", syncGroup).
			Msg("Updated thread ranges")
	}
	return err
}

func (sm *SyncManager) getSyncGroupKeyStore(db int64) *socket.KeyStoreData {
	keyStore, ok := sm.keyStore[db]
	if !ok {
		sm.client.Logger.Warn().Any("databaseId", db).Msg("could not get sync group keystore by databaseId")
	}

	return keyStore
}

func (c *Client) GetSyncGroupKeyStore(syncGroup int64) *socket.KeyStoreData {
	if c == nil || c.syncManager == nil {
		return nil
	}
	return c.syncManager.getSyncGroupKeyStore(syncGroup)
}

/*
these 3 return the same stuff
updateThreadsRangesV2, upsertInboxThreadsRange, upsertSyncGroupThreadsRange
*/
func (sm *SyncManager) updateSyncGroupCursors(table *table.LSTable) error {
	var err error
	if len(table.LSUpsertSyncGroupThreadsRange) > 0 {
		err = sm.updateThreadRanges(table.LSUpsertSyncGroupThreadsRange)
	}

	if len(table.LSExecuteFirstBlockForSyncTransaction) > 0 {
		err = sm.SyncTransactions(table.LSExecuteFirstBlockForSyncTransaction)
	}

	return err
}
