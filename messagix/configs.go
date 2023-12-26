package messagix

import (
	"fmt"
	"log"
	"reflect"

	"github.com/0xzer/messagix/crypto"
	"github.com/0xzer/messagix/methods"
	"github.com/0xzer/messagix/socket"
	"github.com/0xzer/messagix/table"
	"github.com/0xzer/messagix/types"
)

type Configs struct {
	client *Client
	needSync bool
	browserConfigTable *types.SchedulerJSDefineConfig
	accountConfigTable *table.LSTable
	LsdToken string
	CometReq string
	VersionId int64
	Jazoest string
	WebSessionId string
	Bitmap *crypto.Bitmap
	CsrBitmap *crypto.Bitmap
}

func (c *Configs) SetupConfigs() error {
	authenticated := c.client.IsAuthenticated()
	c.WebSessionId = methods.GenerateWebsessionID(authenticated)
	c.LsdToken = c.browserConfigTable.LSD.Token

	c.Bitmap, c.CsrBitmap = c.LoadBitmaps()

	if authenticated {
		if c.client.platform == types.Instagram {
			c.client.socket.broker = "wss://edge-chat.instagram.com/chat?"
			c.browserConfigTable.MqttWebConfig.AppID = c.browserConfigTable.MessengerWebInitData.AppID
		} else {
			c.client.socket.broker = c.browserConfigTable.MqttWebConfig.Endpoint
		}
		c.client.SyncManager.syncParams = &c.browserConfigTable.LSPlatformMessengerSyncParams
		if c.needSync {
			err := c.client.SyncManager.UpdateDatabaseSyncParams(
				[]*socket.QueryMetadata{
					{DatabaseId: 1, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.MailBox},
					{DatabaseId: 2, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
					{DatabaseId: 95, SendSyncParams: true, LastAppliedCursor: nil, SyncChannel: socket.Contact},
				},
			)
			if err != nil {
				return fmt.Errorf("failed to update sync params for databases: 1, 2, 95")
			}

			lsData, err := c.client.SyncManager.SyncDataGraphQL([]int64{1,2,95})
			if err != nil {
				return fmt.Errorf("failed to sync data via graphql for databases: 1, 2, 95")
			}

			//c.client.Logger.Info().Any("lsData", lsData).Msg("Synced data through graphql query")
			c.accountConfigTable = lsData
		} else {
			err := c.client.SyncManager.SyncTransactions(c.accountConfigTable.LSExecuteFirstBlockForSyncTransaction)
			if err != nil {
				return fmt.Errorf("failed to sync transactions from js module data with syncManager: %e", err)
			}
			log.Println("hi")
		}
		c.client.Logger.Info().Any("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
		c.client.Logger.Info().Any("value", c.CsrBitmap.CompressedStr).Msg("Loaded __csr bitmap")
		c.client.Logger.Info().Any("versionId", c.VersionId).Any("appId", c.browserConfigTable.MessengerWebInitData.AppID).Msg("Loaded versionId & appId")
		c.client.Logger.Info().Any("broker", c.client.socket.broker).Msg("Configs successfully setup!")
	} else {
		if c.Bitmap.CompressedStr != "" {
			c.client.Logger.Info().Any("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
			c.client.Logger.Info().Any("value", c.CsrBitmap.CompressedStr).Msg("Loaded __csr bitmap")
		}
		c.client.Logger.Info().Any("platform", c.client.platform).Msg("Configs loaded, but not yet logged in.")
	}
	
	return nil
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