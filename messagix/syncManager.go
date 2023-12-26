package messagix

import (
	"encoding/json"
	"fmt"
	"log"
	"github.com/0xzer/messagix/graphql"
	"github.com/0xzer/messagix/methods"
	"github.com/0xzer/messagix/socket"
	"github.com/0xzer/messagix/table"
	"github.com/0xzer/messagix/types"
)

type SyncManager struct {
	client *Client
	// for syncing / cursors
	store map[int64]*socket.QueryMetadata
	// for thread/message fetching
	keyStore map[int64]*socket.KeyStoreData
	syncParams *types.LSPlatformMessengerSyncParams
}

func (c *Client) NewSyncManager() *SyncManager {
	return &SyncManager{
		client: c,
		store: map[int64]*socket.QueryMetadata{
			1: { SendSyncParams: false, SyncChannel: socket.MailBox },
			2: { SendSyncParams: false, SyncChannel: socket.Contact },
			5: { SendSyncParams: true },
			16: { SendSyncParams: true },
			26: { SendSyncParams: true },
			28: { SendSyncParams: true },
			95: { SendSyncParams: false, SyncChannel: socket.Contact },
			104: { SendSyncParams: true },
			140: { SendSyncParams: true },
			141: { SendSyncParams: true },
			142: { SendSyncParams: true },
			143: { SendSyncParams: true },
			196: { SendSyncParams: true },
			198: { SendSyncParams: true },
		},
		keyStore: map[int64]*socket.KeyStoreData{
			1: { MinThreadKey: 0, ParentThreadKey: -1, MinLastActivityTimestampMs: 9999999999999, HasMoreBefore: false },
			95: { MinThreadKey: 0, ParentThreadKey: -1, MinLastActivityTimestampMs: 9999999999999, HasMoreBefore: false },
		},
		syncParams: &types.LSPlatformMessengerSyncParams{},
	}
}

func (sm *SyncManager) EnsureSyncedSocket(databases []int64) error {
	for _, db := range databases {
		database, ok := sm.store[db]
		if !ok {
			return fmt.Errorf("could not find sync store for database: %d", db)
		}

		_, err := sm.SyncSocketData(db, database)
		if err != nil {
			return fmt.Errorf("failed to ensure database is synced through socket: (databaseId=%d)", db)
		}
		sm.client.Logger.Debug().Any("database_id", db).Any("database", database).Msg("Synced database")
	}

	return nil
}

func (sm *SyncManager) SyncSocketData(databaseId int64, db *socket.QueryMetadata) (*table.LSTable, error) {
	var t int
	payload := &socket.DatabaseQuery{
		Database: databaseId,
		Version: sm.client.configs.VersionId,
		EpochId: methods.GenerateEpochId(),
	}

	if db.SendSyncParams {
		t = 1
		payload.SyncParams = sm.getSyncParams(db.SyncChannel)
	} else {
		t = 2
		payload.LastAppliedCursor = db.LastAppliedCursor
	}

	jsonPayload, err := json.Marshal(&payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DatabaseQuery struct into json bytes (databaseId=%d): %e", databaseId, err)
	}

	packetId, err := sm.client.socket.makeLSRequest(jsonPayload, t)
	if err != nil {
		return nil, fmt.Errorf("failed to make lightspeed socket request with DatabaseQuery byte payload (databaseId=%d): %e", databaseId, err)
	}

	resp := sm.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("timed out while waiting for sync response from socket (databaseId=%d)", databaseId)
	}
	resp.Finish()

	block := resp.Table.LSExecuteFirstBlockForSyncTransaction[0]
	nextCursor, currentCursor := block.NextCursor, block.CurrentCursor
	sm.client.Logger.Debug().Any("full_block", block).Any("block_response", block).Any("database_id", payload.Database).Any("payload", string(jsonPayload)).Msg("Synced database")
	if nextCursor == currentCursor {
		return &resp.Table, nil
	}

	// Update the last applied cursor to the next cursor and recursively fetch again
	db.LastAppliedCursor = nextCursor
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
			Database: int(db),
			LastAppliedCursor: database.LastAppliedCursor,
			Version: sm.client.configs.VersionId,
			EpochID: 0,
		}
		if database.SendSyncParams {
			variables.SyncParams = sm.getSyncParams(database.SyncChannel)
		}

		lsTable, err := sm.client.makeLSRequest(variables, 1)
		if err != nil {
			return nil, err
		}

		if db == 1 {
			tableData = lsTable
		}
		err = sm.updateSyncGroupCursors(*lsTable)
		if err != nil {
			return nil, err
		}
		// sm.SyncTransactions(lsTable.LSExecuteFirstBlockForSyncTransaction)
	}

	return tableData, nil
}

func (sm *SyncManager) SyncTransactions(transactions []table.LSExecuteFirstBlockForSyncTransaction) error {
	for _, transaction := range transactions {
		database, ok := sm.store[transaction.DatabaseId]
		if !ok {
			return fmt.Errorf("failed to update database %d by block transaction", transaction.DatabaseId)
		}
	
		database.LastAppliedCursor = transaction.NextCursor
		database.SendSyncParams = transaction.SendSyncParams
		database.SyncChannel = socket.SyncChannel(transaction.SyncChannel)
		sm.client.Logger.Info().Any("new_cursor", database.LastAppliedCursor).Any("syncChannel", database.SyncChannel).Any("sendSyncParams", database.SendSyncParams).Any("database_id", transaction.DatabaseId).Msg("Updated database by transaction...")
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

func (sm *SyncManager) getSyncParams(ch socket.SyncChannel) interface{} {
	switch ch {
	case socket.MailBox:
		return sm.syncParams.Mailbox
	case socket.Contact:
		return sm.syncParams.Contact
	default:
		log.Fatalf("Unknown syncChannel: %d", ch)
		return nil
	}
}

func (sm *SyncManager) GetCursor(db int64) string {
	database, ok := sm.store[db]
	if !ok {
		return ""
	}
	return database.LastAppliedCursor.(string)
}

func (sm *SyncManager) updateThreadRanges(ranges []table.LSUpsertSyncGroupThreadsRange) error {
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

		sm.client.Logger.Info().Any("keyStore", keyStore).Any("databaseId", syncGroup).Msg("Updated thread ranges.")
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
func (sm *SyncManager) updateSyncGroupCursors(table table.LSTable) error {
	var err error
	if len(table.LSUpsertSyncGroupThreadsRange) > 0 {
		err = sm.updateThreadRanges(table.LSUpsertSyncGroupThreadsRange)
	}

	if len(table.LSExecuteFirstBlockForSyncTransaction) > 0 {
		err = sm.SyncTransactions(table.LSExecuteFirstBlockForSyncTransaction)
	}

	return err
}
/*
func (db *DatabaseManager) AddInitQueries() {
	queries := []socket.DatabaseQuery{
		{Database: 1},
		{Database: 2},
		{Database: 95},
		{Database: 16},
		{Database: 26},
		{Database: 28},
		{Database: 95},
		{Database: 104},
		{Database: 140},
		{Database: 141},
		{Database: 142},
		{Database: 143},
		{Database: 196},
		{Database: 198},
		
	}
}
*/