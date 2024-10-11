package messagix

import (
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

func (c *Client) NewSyncManager() *SyncManager {
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

func (sm *SyncManager) syncSocketData(db int64, cb func()) {
	defer cb()
	database, ok := sm.store[db]
	if !ok {
		sm.client.Logger.Error().Int64("database_id", db).Msg("Could not find sync store for database")
		return
	}

	_, err := sm.SyncSocketData(db, database)
	if err != nil {
		sm.client.Logger.Err(err).Int64("database_id", db).Msg("Failed to sync database through socket")
	} else {
		sm.client.Logger.Debug().Any("database_id", db).Any("database", database).Msg("Synced database")
	}
}

func (sm *SyncManager) EnsureSyncedSocket(databases []int64) error {
	var wg sync.WaitGroup
	wg.Add(len(databases))
	for _, db := range databases {
		go sm.syncSocketData(db, wg.Done)
	}
	wg.Wait()

	return nil
}

func (sm *SyncManager) SyncSocketData(databaseId int64, db *socket.QueryMetadata) (*table.LSTable, error) {
	var t int
	payload := &socket.DatabaseQuery{
		Database: databaseId,
		Version:  json.Number(strconv.FormatInt(sm.client.configs.VersionId, 10)),
		EpochId:  methods.GenerateEpochId(),
	}

	var prevCursor string
	if db.LastAppliedCursor != nil {
		prevCursor = *db.LastAppliedCursor
	}
	if db.SendSyncParams {
		t = 1
		payload.SyncParams = sm.getSyncParams(databaseId, db.SyncChannel)
	} else {
		t = 2
		payload.LastAppliedCursor = db.LastAppliedCursor
	}

	jsonPayload, err := json.Marshal(&payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DatabaseQuery struct into json bytes (databaseId=%d): %w", databaseId, err)
	}

	sm.client.Logger.Trace().
		RawJSON("payload", jsonPayload).
		Int64("database_id", databaseId).
		Msg("Syncing database via socket")
	resp, err := sm.client.socket.makeLSRequest(jsonPayload, t)
	if err != nil {
		return nil, fmt.Errorf("failed to make lightspeed socket request with DatabaseQuery byte payload (databaseId=%d): %w", databaseId, err)
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
			Any("database_id", databaseId).
			Any("payload", string(jsonPayload)).
			Any("response", resp.Data).
			Any("table", resp.Table).
			Msg("No transactions found")
		return resp.Table, nil
	}
	block := resp.Table.LSExecuteFirstBlockForSyncTransaction[0]
	nextCursor, currentCursor := block.NextCursor, block.CurrentCursor
	sm.client.Logger.Debug().
		Any("full_block", block).
		Any("block_response", block).
		Any("database_id", payload.Database).
		Any("payload", string(jsonPayload)).
		Msg("Synced database")
	if nextCursor == currentCursor || nextCursor == prevCursor || nextCursor == "dummy_cursor" || nextCursor == "" || !shouldRecurseDatabase[databaseId] {
		return resp.Table, nil
	}

	// Update the last applied cursor to the next cursor and recursively fetch again
	db.LastAppliedCursor = &nextCursor
	db.SendSyncParams = block.SendSyncParams
	db.SyncChannel = socket.SyncChannel(block.SyncChannel)
	err = sm.updateSyncGroupCursors(resp.Table) // Also sync the transaction with the store map because the db param is just a copy of the map entry
	if err != nil {
		return nil, err
	}

	return sm.SyncSocketData(databaseId, db)
}

func (sm *SyncManager) SyncDataGraphQL(dbs []int64) (*table.LSTable, error) {
	var tableData *table.LSTable
	for _, db := range dbs {
		database, ok := sm.store[db]
		if !ok {
			return nil, fmt.Errorf("could not find sync store for database: %d", db)
		}

		variables := &graphql.LSPlatformGraphQLLightspeedVariables{
			Database:          int(db),
			LastAppliedCursor: database.LastAppliedCursor,
			Version:           sm.client.configs.VersionId,
			EpochID:           0,
		}
		if database.SendSyncParams {
			variables.SyncParams = sm.getSyncParams(db, database.SyncChannel)
		}

		lsTable, err := sm.client.makeLSRequest(variables, 1)
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
		database, ok := sm.store[transaction.DatabaseId]
		if !ok {
			return fmt.Errorf("failed to update database %d by block transaction", transaction.DatabaseId)
		}

		database.LastAppliedCursor = &transaction.NextCursor
		database.SendSyncParams = transaction.SendSyncParams
		database.SyncChannel = socket.SyncChannel(transaction.SyncChannel)
		sm.client.Logger.Info().
			Any("new_cursor", database.LastAppliedCursor).
			Any("syncChannel", database.SyncChannel).
			Any("sendSyncParams", database.SendSyncParams).
			Any("database_id", transaction.DatabaseId).
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

func (sm *SyncManager) updateThreadRanges(ranges []*table.LSUpsertSyncGroupThreadsRange) error {
	var err error
	for _, syncGroupData := range ranges {
		if !syncGroupData.HasMoreBefore {
			continue
		}
		syncGroup := syncGroupData.SyncGroup
		keyStore, ok := sm.keyStore[syncGroup]
		if !ok {
			err = fmt.Errorf("could not find keyStore by databaseId %d", syncGroup)
			sm.client.Logger.Err(err).Any("syncGroupData", syncGroupData).Msg("failed to update thread ranges")
			continue
		}
		keyStore.HasMoreBefore = syncGroupData.HasMoreBefore
		keyStore.MinLastActivityTimestampMs = syncGroupData.MinLastActivityTimestampMs
		keyStore.MinThreadKey = syncGroupData.MinThreadKey
		keyStore.ParentThreadKey = syncGroupData.ParentThreadKey

		sm.client.Logger.Debug().Any("keyStore", keyStore).Any("databaseId", syncGroup).Msg("Updated thread ranges.")
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
