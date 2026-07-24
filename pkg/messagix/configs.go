package messagix

import (
	"cmp"
	"context"
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func (c *Client) updateSocketIDs() {
	c.socket.AppID = c.configs.BrowserConfigTable.DGWWebConfig.AppID
	c.socket.UserID = cmp.Or(c.configs.BrowserConfigTable.PolarisViewer.ID, c.configs.BrowserConfigTable.CurrentUserInitialData.UserID)
	c.socket.DeviceID = cmp.Or(c.configs.BrowserConfigTable.IGDMqttWebDeviceID.ClientID, c.configs.BrowserConfigTable.MqttWebDeviceID.ClientID)
}

func (c *Client) setupConfigs(ctx context.Context, ls *table.LSTable) (*table.LSTable, error) {
	c.socketWasSynced.Store(false)
	authenticated := c.IsAuthenticated()
	c.configs.Setup(authenticated)
	if !authenticated {
		return ls, nil
	}

	c.updateSocketIDs()
	c.syncManager.syncParams = &c.configs.BrowserConfigTable.LSPlatformMessengerSyncParams
	if len(ls.LSExecuteFinallyBlockForSyncTransaction) == 0 {
		c.Logger.Warn().Msg("Syncing initial data via graphql")
		err := c.syncManager.UpdateDatabaseSyncParams(
			[]*socket.QueryMetadata{
				{DatabaseID: 1, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.MailBox},
				{DatabaseID: 2, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
				{DatabaseID: 95, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
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
	c.Logger.Debug().Msg("Configs successfully setup!")

	return ls, nil
}
