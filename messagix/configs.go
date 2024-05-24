package messagix

import (
	"fmt"
	"reflect"

	"go.mau.fi/mautrix-meta/messagix/crypto"
	"go.mau.fi/mautrix-meta/messagix/graphql"
	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
)

type Configs struct {
	client             *Client
	browserConfigTable *types.SchedulerJSDefineConfig
	graphqlConfigTable *graphql.GraphQLTable
	LsdToken           string
	CometReq           string
	VersionId          int64
	Jazoest            string
	Eqmc 			   *types.Eqmc
	WebSessionId       string
	Bitmap             *crypto.Bitmap
	CsrBitmap          *crypto.Bitmap
}

func (c *Configs) SetupConfigs(ls *table.LSTable) (*table.LSTable, error) {
	if c.client.socket != nil {
		c.client.socket.previouslyConnected = false
	}
	authenticated := c.client.IsAuthenticated()
	c.WebSessionId = methods.GenerateWebsessionID(authenticated)
	c.LsdToken = c.browserConfigTable.LSD.Token

	c.Bitmap, c.CsrBitmap = c.LoadBitmaps()

	if !authenticated {
		if c.Bitmap.CompressedStr != "" {
			c.client.Logger.Trace().Any("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
			c.client.Logger.Trace().Any("value", c.CsrBitmap.CompressedStr).Msg("Loaded __csr bitmap")
		}
		c.client.Logger.Debug().Any("platform", c.client.platform).Msg("Configs loaded, but not yet logged in.")
		return ls, nil
	}

	if c.client.platform == types.Instagram {
		c.client.socket.broker = "wss://edge-chat.instagram.com/chat?"
		c.browserConfigTable.MqttWebConfig.AppID = c.browserConfigTable.MessengerWebInitData.AppID
	} else {
		c.client.socket.broker = c.browserConfigTable.MqttWebConfig.Endpoint
	}
	c.client.SyncManager.syncParams = &c.browserConfigTable.LSPlatformMessengerSyncParams
	if len(ls.LSExecuteFinallyBlockForSyncTransaction) == 0 {
		c.client.Logger.Warn().Msg("Syncing initial data via graphql")
		err := c.client.SyncManager.UpdateDatabaseSyncParams(
			[]*socket.QueryMetadata{
				{DatabaseId: 1, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.MailBox},
				{DatabaseId: 2, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
				{DatabaseId: 95, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
			},
		)
		if err != nil {
			return ls, fmt.Errorf("failed to update sync params for databases: 1, 2, 95: %w", err)
		}

		ls, err = c.client.SyncManager.SyncDataGraphQL([]int64{1, 2, 95})
		if err != nil {
			return ls, fmt.Errorf("failed to sync data via graphql for databases: 1, 2, 95: %w", err)
		}
	} else {
		if len(ls.LSUpsertSyncGroupThreadsRange) > 0 {
			err := c.client.SyncManager.updateThreadRanges(ls.LSUpsertSyncGroupThreadsRange)
			if err != nil {
				return ls, fmt.Errorf("failed to update thread ranges from js module data: %w", err)
			}
		}
		err := c.client.SyncManager.SyncTransactions(ls.LSExecuteFirstBlockForSyncTransaction)
		if err != nil {
			return ls, fmt.Errorf("failed to sync transactions from js module data with syncManager: %w", err)
		}
	}
	c.client.Logger.Trace().Any("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
	c.client.Logger.Trace().Any("value", c.CsrBitmap.CompressedStr).Msg("Loaded __csr bitmap")
	c.client.Logger.Trace().Any("versionId", c.VersionId).Any("appId", c.browserConfigTable.MessengerWebInitData.AppID).Msg("Loaded versionId & appId")
	c.client.Logger.Debug().Any("broker", c.client.socket.broker).Msg("Configs successfully setup!")

	return ls, nil
}

func (c *Configs) ParseFormInputs(inputs []InputTag, reflectedMs reflect.Value) {
	for _, input := range inputs {
		attr := input.Attributes
		key := attr["name"]
		val := attr["value"]

		field := reflectedMs.FieldByNameFunc(func(s string) bool {
			field, _ := reflectedMs.Type().FieldByName(s)
			return field.Tag.Get("name") == key
		})

		if field.IsValid() && field.CanSet() {
			field.SetString(val)
		}
	}
}

// (bitmap, csrBitmap)
func (c *Configs) LoadBitmaps() (*crypto.Bitmap, *crypto.Bitmap) {
	bitmap := crypto.NewBitmap().Update(c.Bitmap.BMap).ToCompressedString()
	csrBitmap := crypto.NewBitmap().Update(c.CsrBitmap.BMap).ToCompressedString()

	return bitmap, csrBitmap
}
