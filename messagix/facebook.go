package messagix

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/go-querystring/query"

	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/crypto"
	"go.mau.fi/mautrix-meta/messagix/types"
)

type FacebookMethods struct {
	client *Client
}

func (fb *FacebookMethods) Login(identifier, password string) (*cookies.Cookies, error) {
	moduleLoader := fb.client.loadLoginPage()
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

	needsCookieConsent := len(fb.client.configs.browserConfigTable.InitialCookieConsent.InitialConsent) == 0
	if needsCookieConsent {
		err := fb.client.sendCookieConsent(moduleLoader.JSDatr)
		if err != nil {
			return nil, err
		}
	}

	testDataSimulator := crypto.NewABTestData()
	data := testDataSimulator.GenerateAbTestData([]string{identifier, password})

	encryptedPW, err := crypto.EncryptPassword(int(types.Facebook), crypto.FacebookPubKeyId, crypto.FacebookPubKey, password)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password for facebook: %v", err)
	}

	loginForm.Email = identifier
	loginForm.EncPass = encryptedPW
	loginForm.AbTestData = data
	loginForm.Lgndim = "eyJ3IjoyMjc1LCJoIjoxMjgwLCJhdyI6MjI3NiwiYWgiOjEyMzIsImMiOjI0fQ==" // irrelevant
	loginForm.Lgnjs = strconv.Itoa(fb.client.configs.browserConfigTable.SiteData.SpinT)
	loginForm.Timezone = "-120"

	form, err := query.Values(&loginForm)
	if err != nil {
		return nil, err
	}

	loginUrl := fb.client.getEndpoint("base_url") + loginPath
	loginResp, loginBody, err := fb.client.sendLoginRequest(form, loginUrl)
	if err != nil {
		return nil, err
	}

	loginResult := fb.client.processLogin(loginResp, loginBody)
	if loginResult != nil {
		return nil, loginResult
	}

	return fb.client.cookies, nil
}

func (fb *FacebookMethods) RegisterPushNotifications(endpoint string) error {
	c := fb.client
	jsonKeys, err := json.Marshal(c.cookies.PushKeys.Public)
	if err != nil {
		c.Logger.Err(err).Msg("failed to encode push keys to json")
		return err
	}

	payload := c.NewHttpQuery()
	payload.AppID = "1443096165982425"
	payload.PushEndpoint = endpoint
	payload.SubscriptionKeys = string(jsonKeys)

	form, err := query.Values(payload)
	if err != nil {
		return err
	}

	payloadBytes := []byte(form.Encode())

	headers := c.buildHeaders(true)
	headers.Set("Referer", c.getEndpoint("host"))
	headers.Set("Sec-fetch-site", "same-origin")

	url := c.getEndpoint("web_push")

	resp, body, err := c.MakeRequest(url, "POST", headers, payloadBytes, types.FORM)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	bodyStr := string(body)
	jsonStr := strings.TrimPrefix(bodyStr, "for (;;);")
	jsonBytes := []byte(jsonStr)

	var r pushNotificationsResponse
	err = json.Unmarshal(jsonBytes, &r)
	if err != nil {
		c.Logger.Err(err).Str("body", bodyStr).Msg("failed to unmarshal response")
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
