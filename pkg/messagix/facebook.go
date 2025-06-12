package messagix

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/google/go-querystring/query"
	"go.mau.fi/util/jsonbytes"
	"golang.org/x/net/context"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/crypto"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type FacebookMethods struct {
	client *Client
}

func (fb *FacebookMethods) Login(ctx context.Context, identifier, password string) (*cookies.Cookies, error) {
	moduleLoader := fb.client.loadLoginPage(ctx)
	loginFormTags := moduleLoader.FormTags[0]
	loginPath, ok := loginFormTags.Attributes["action"]
	if !ok {
		return nil, fmt.Errorf("failed to resolve login path / action from html form tags for facebook login")
	}

	loginInputs := append(loginFormTags.Inputs, moduleLoader.LoginInputs...)
	loginForm := types.LoginForm{}
	v := reflect.ValueOf(&loginForm).Elem()
	fb.client.configs.ParseFormInputs(loginInputs, v)

	fb.client.configs.Jazoest = loginForm.Jazoest

	needsCookieConsent := len(fb.client.configs.BrowserConfigTable.InitialCookieConsent.InitialConsent) == 0
	if needsCookieConsent {
		err := fb.client.sendCookieConsent(ctx, moduleLoader.JSDatr)
		if err != nil {
			return nil, err
		}
	}

	testDataSimulator := crypto.NewABTestData()
	data := testDataSimulator.GenerateAbTestData([]string{identifier, password})

	encryptedPW, err := crypto.EncryptPassword(int(types.Facebook), crypto.FacebookPubKeyId, crypto.FacebookPubKey, password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password for facebook: %w", err)
	}

	loginForm.Email = identifier
	loginForm.EncPass = encryptedPW
	loginForm.AbTestData = data
	loginForm.Lgndim = "eyJ3IjoyMjc1LCJoIjoxMjgwLCJhdyI6MjI3NiwiYWgiOjEyMzIsImMiOjI0fQ==" // irrelevant
	loginForm.Lgnjs = strconv.Itoa(fb.client.configs.BrowserConfigTable.SiteData.SpinT)
	loginForm.Timezone = "-120"

	form, err := query.Values(&loginForm)
	if err != nil {
		return nil, err
	}

	loginUrl := fb.client.getEndpoint("base_url") + loginPath
	loginResp, loginBody, err := fb.client.sendLoginRequest(ctx, form, loginUrl)
	if err != nil {
		return nil, err
	}

	loginResult := fb.client.processLogin(loginResp, loginBody)
	if loginResult != nil {
		return nil, loginResult
	}

	return fb.client.cookies, nil
}

type PushKeys struct {
	P256DH jsonbytes.UnpaddedURLBytes `json:"p256dh"`
	Auth   jsonbytes.UnpaddedURLBytes `json:"auth"`
}

func (fb *FacebookMethods) RegisterPushNotifications(ctx context.Context, endpoint string, keys PushKeys) error {
	c := fb.client
	jsonKeys, err := json.Marshal(&keys)
	if err != nil {
		c.Logger.Err(err).Msg("failed to encode push keys to json")
		return err
	}

	payload := c.newHTTPQuery()
	payload.AppID = "1443096165982425"
	payload.PushEndpoint = endpoint
	payload.SubscriptionKeys = string(jsonKeys)

	form, err := query.Values(payload)
	if err != nil {
		return err
	}

	payloadBytes := []byte(form.Encode())

	headers := c.buildHeaders(true, false)
	headers.Set("Referer", c.getEndpoint("base_url"))
	headers.Set("Sec-fetch-site", "same-origin")
	headers.Set("Accept", "*/*")

	url := c.getEndpoint("web_push")

	resp, body, err := c.MakeRequest(ctx, url, "POST", headers, payloadBytes, types.FORM)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	body = bytes.TrimPrefix(body, antiJSPrefix)

	var r pushNotificationsResponse
	err = json.Unmarshal(body, &r)
	if err != nil {
		c.Logger.Err(err).Bytes("body", body).Msg("failed to unmarshal response")
		return err
	}

	if !r.Payload.Success {
		return errors.New("failed to register for push notifications")
	}

	return nil
}

type pushNotificationsResponse struct {
	Ar        int     `json:"__ar"`
	Payload   payload `json:"payload"`
	DtsgToken string  `json:"dtsgToken"`
	Lid       string  `json:"lid"`
}

type payload struct {
	Success bool `json:"success"`
}
