package messagix

import (
	"context"
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func (c *Client) setupConfigs(ctx context.Context, ls *table.LSTable) (*table.LSTable, error) {
	if c.socket != nil {
		c.socket.previouslyConnected = false
	}
	authenticated := c.IsAuthenticated()
	c.configs.Setup(authenticated)
	if !authenticated {
		return ls, nil
	}

	if c.Platform == types.Instagram {
		c.socket.broker = "wss://edge-chat.instagram.com/chat?"
	} else {
		if c.configs.BrowserConfigTable.MqttWebConfig.Endpoint == "" {
			return ls, fmt.Errorf("MQTT broker endpoint not found in page response (MqttWebConfig.Endpoint is empty)")
		}
		c.socket.broker = c.configs.BrowserConfigTable.MqttWebConfig.Endpoint
	}
	c.syncManager.syncParams = &c.configs.BrowserConfigTable.LSPlatformMessengerSyncParams
	if len(ls.LSExecuteFinallyBlockForSyncTransaction) == 0 {
		c.Logger.Warn().Msg("Syncing initial data via graphql")
		err := c.syncManager.UpdateDatabaseSyncParams(
			[]*socket.QueryMetadata{
				{DatabaseId: 1, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.MailBox},
				{DatabaseId: 2, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
				{DatabaseId: 95, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
			},
		)
		if err != nil {
			return ls, fmt.Errorf("failed to update sync params for databases: 1, 2, 95: %w", err)
		}

		ls, err = c.syncManager.SyncDataGraphQL(ctx, []int64{1, 2, 95})
		if err != nil {
			return ls, fmt.Errorf("failed to sync data via graphql for databases: 1, 2, 95: %w", err)
		} else if ls == nil {
			return ls, fmt.Errorf("sync data via graphql returned nil LSTable")
		}
	} else {
		if len(ls.LSUpsertSyncGroupThreadsRange) > 0 {
			err := c.syncManager.updateThreadRanges(ls.LSUpsertSyncGroupThreadsRange)
			if err != nil {
				return ls, fmt.Errorf("failed to update thread ranges from js module data: %w", err)
			}
		}
		err := c.syncManager.SyncTransactions(ls.LSExecuteFirstBlockForSyncTransaction)
		if err != nil {
			return ls, fmt.Errorf("failed to sync transactions from js module data with syncManager: %w", err)
		}
	}
	var ptks []int64
	for _, ptk := range ls.LSThreadsRangesQuery {
		c.Logger.Trace().Any("data", ptk).Msg("Found parent thread key")
		ptks = append(ptks, ptk.ParentThreadKey)
	}
	c.configs.ParentThreadKeys = ptks
	c.Logger.Debug().Str("broker", c.socket.broker).Msg("Configs successfully setup!")

	return ls, nil
}
