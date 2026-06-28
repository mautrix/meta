package httpclient

import (
	"cmp"
	"reflect"
	"strconv"

	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type Configs struct {
	client             Client
	BrowserConfigTable *types.SchedulerJSDefineConfig
	LSDToken           string
	CometReq           string
	VersionID          int64
	Jazoest            string
	WebSessionID       string
	RoutingNamespace   string
	Bitmap             *crypto.Bitmap
	CSRBitmap          *crypto.Bitmap
	ParentThreadKeys   []int64
}

func NewConfigs(client Client) *Configs {
	return &Configs{
		client:             client,
		BrowserConfigTable: &types.SchedulerJSDefineConfig{},
		Bitmap:             crypto.NewBitmap(),
		CSRBitmap:          crypto.NewBitmap(),
	}
}

func (c *Configs) SetClient(client Client) {
	c.client = client
}

func (c *Configs) Setup(authenticated bool) {
	c.WebSessionID = methods.GenerateWebsessionID(authenticated)
	c.LSDToken = c.BrowserConfigTable.LSD.Token
	c.Bitmap, c.CSRBitmap = c.LoadBitmaps()

	if !authenticated {
		if c.Bitmap.CompressedStr != "" {
			c.client.GetLogger().Trace().Any("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
			c.client.GetLogger().Trace().Any("value", c.CSRBitmap.CompressedStr).Msg("Loaded __csr bitmap")
		}
		c.client.GetLogger().Debug().Any("platform", c.client.GetPlatform()).Msg("Configs loaded, but not yet logged in.")
		return
	}

	if c.client.GetPlatform() == types.Instagram {
		currentUserAppID, _ := strconv.ParseInt(c.BrowserConfigTable.CurrentUserInitialData.AppID, 10, 64)
		c.BrowserConfigTable.MqttWebConfig.AppID = cmp.Or(
			c.BrowserConfigTable.MessengerWebInitData.AppID,
			currentUserAppID,
			936619743392459,
		)
	}
	c.client.GetLogger().Trace().Str("value", c.Bitmap.CompressedStr).Msg("Loaded __dyn bitmap")
	c.client.GetLogger().Trace().Str("value", c.CSRBitmap.CompressedStr).Msg("Loaded __csr bitmap")
	c.client.GetLogger().Trace().
		Int64("versionId", c.VersionID).
		Int64("appId", c.BrowserConfigTable.MessengerWebInitData.AppID).
		Msg("Loaded versionId & appId")
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
