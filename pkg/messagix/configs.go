package messagix

import (
	"context"
	"fmt"
	"reflect"

	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type Configs struct {
	client             *Client
	BrowserConfigTable *types.SchedulerJSDefineConfig
	graphqlConfigTable *graphql.GraphQLTable
	LSDToken           string
	CometReq           string
	VersionID          int64
	Jazoest            string
	WebSessionID       string
	RoutingNamespace   string
	Bitmap             *crypto.Bitmap
	CSRBitmap          *crypto.Bitmap
}

func (c *Configs) SetupConfigs(ctx context.Context, ls *table.LSTable) (*table.LSTable, error) {
	if c.client.socket != nil {
		c.client.socket.previouslyConnected = false
	}
	authenticated := c.client.IsAuthenticated()
	c.WebSessionID = methods.GenerateWebsessionID(authenticated)
	c.LSDToken = c.BrowserConfigTable.LSD.Token

	c.Bitmap, c.CSRBitmap = c.LoadBitmaps()

	if !authenticated {
		if c.Bitmap.CompressedStr != "" {
			c.client.Logger.Trace().Any("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
			c.client.Logger.Trace().Any("value", c.CSRBitmap.CompressedStr).Msg("Loaded __csr bitmap")
		}
		c.client.Logger.Debug().Any("platform", c.client.Platform).Msg("Configs loaded, but not yet logged in.")
		return ls, nil
	}

	if c.client.Platform == types.Instagram {
		c.client.socket.broker = "wss://edge-chat.instagram.com/chat?"
		c.BrowserConfigTable.MqttWebConfig.AppID = c.BrowserConfigTable.MessengerWebInitData.AppID
	} else {
		if c.BrowserConfigTable.MqttWebConfig.Endpoint == "" {
			return ls, fmt.Errorf("MQTT broker endpoint not found in page response (MqttWebConfig.Endpoint is empty)")
		}
		c.client.socket.broker = c.BrowserConfigTable.MqttWebConfig.Endpoint
	}
	c.client.syncManager.syncParams = &c.BrowserConfigTable.LSPlatformMessengerSyncParams
	if len(ls.LSExecuteFinallyBlockForSyncTransaction) == 0 {
		c.client.Logger.Warn().Msg("Syncing initial data via graphql")
		err := c.client.syncManager.UpdateDatabaseSyncParams(
			[]*socket.QueryMetadata{
				{DatabaseId: 1, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.MailBox},
				{DatabaseId: 2, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
				{DatabaseId: 95, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
			},
		)
		if err != nil {
			return ls, fmt.Errorf("failed to update sync params for databases: 1, 2, 95: %w", err)
		}

		ls, err = c.client.syncManager.SyncDataGraphQL(ctx, []int64{1, 2, 95})
		if err != nil {
			return ls, fmt.Errorf("failed to sync data via graphql for databases: 1, 2, 95: %w", err)
		}
	} else {
		if len(ls.LSUpsertSyncGroupThreadsRange) > 0 {
			err := c.client.syncManager.updateThreadRanges(ls.LSUpsertSyncGroupThreadsRange)
			if err != nil {
				return ls, fmt.Errorf("failed to update thread ranges from js module data: %w", err)
			}
		}
		err := c.client.syncManager.SyncTransactions(ls.LSExecuteFirstBlockForSyncTransaction)
		if err != nil {
			return ls, fmt.Errorf("failed to sync transactions from js module data with syncManager: %w", err)
		}
	}
	c.client.Logger.Trace().Any("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
	c.client.Logger.Trace().Any("value", c.CSRBitmap.CompressedStr).Msg("Loaded __csr bitmap")
	c.client.Logger.Trace().Any("versionId", c.VersionID).Any("appId", c.BrowserConfigTable.MessengerWebInitData.AppID).Msg("Loaded versionId & appId")
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
	csrBitmap := crypto.NewBitmap().Update(c.CSRBitmap.BMap).ToCompressedString()

	return bitmap, csrBitmap
}
