package messagix

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/0xzer/messagix/cookies"
	"github.com/0xzer/messagix/crypto"
	"github.com/0xzer/messagix/types"
	"github.com/google/go-querystring/query"
)

type FacebookMethods struct {
	client *Client
}

func (fb *FacebookMethods) Login(identifier, password string) (cookies.Cookies, error) {
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
		return nil, fmt.Errorf("failed to encrypt password for facebook: %e", err)
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
	loginResp, loginBody, err := fb.client.Account.sendLoginRequest(form, loginUrl)
	if err != nil {
		return nil, err
	}

	loginResult := fb.client.Account.processLogin(loginResp, loginBody)
	if loginResult != nil {
		return nil, loginResult
	}

	return fb.client.cookies, nil
}