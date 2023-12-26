package messagix

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"github.com/0xzer/messagix/cookies"
	"github.com/0xzer/messagix/socket"
	"github.com/0xzer/messagix/table"
	"github.com/0xzer/messagix/types"
	"github.com/google/uuid"
)

type Account struct {
	client *Client
}

func (a *Account) processLogin(resp *http.Response, respBody []byte) error {
	statusCode := resp.StatusCode
	var err error
	switch a.client.platform {
	case types.Facebook:
		if hasUserCookie := a.client.findCookie(resp.Cookies(), "c_user"); hasUserCookie == nil {
			err = fmt.Errorf("failed to login to facebook")
		}
	case types.Instagram:
		var loginResp *types.InstagramLoginResponse
		err = json.Unmarshal(respBody, &loginResp)
		if err != nil {
			return fmt.Errorf("failed to unmarshal instagram login response to *types.InstagramLoginResponse (statusCode=%d): %e", statusCode, err)
		}
		if loginResp.Status == "fail" {
			err = fmt.Errorf("failed to process login request (message=%s, statusText=%s, statusCode=%d)", loginResp.Message, loginResp.Status, statusCode)
		} else if !loginResp.Authenticated {
			err = fmt.Errorf("failed to login, invalid password (userExists=%t, statusText=%s, statusCode=%d)", loginResp.User, loginResp.Status, statusCode)
		} else {
			a.client.cookies.(*cookies.InstagramCookies).IgWWWClaim = resp.Header.Get("x-ig-set-www-claim")
		}
	}
	
	if err == nil {
		cookies.UpdateFromResponse(a.client.cookies, resp.Header)
	}

	return err
}

func (a *Account) GetContacts(limit int64) ([]table.LSVerifyContactRowExists, error) {
	tskm := a.client.NewTaskManager()
	tskm.AddNewTask(&socket.GetContactsTask{Limit: limit})

	payload, err := tskm.FinalizePayload()
	if err != nil {
		log.Fatal(err)
	}

	packetId, err := a.client.socket.makeLSRequest(payload, 3)
	if err != nil {
		log.Fatal(err)
	}

	resp := a.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("failed to receive response from socket while trying to fetch contacts. packetId: %d", packetId)
	}

	return resp.Table.LSVerifyContactRowExists, nil
}

func (a *Account) GetContactsFull(contactIds []int64) ([]table.LSDeleteThenInsertContact, error) {
	tskm := a.client.NewTaskManager()
	for _, id := range contactIds {
		tskm.AddNewTask(&socket.GetContactsFullTask{
			ContactId: id,
		})
	}

	payload, err := tskm.FinalizePayload()
	if err != nil {
		log.Fatal(err)
	}

	packetId, err := a.client.socket.makeLSRequest(payload, 3)
	if err != nil {
		log.Fatal(err)
	}

	resp := a.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("failed to receive response from socket while trying to fetch full contact information. packetId: %d", packetId)
	}

	return resp.Table.LSDeleteThenInsertContact, nil
}

func (a *Account) ReportAppState(state table.AppState) error {
	tskm := a.client.NewTaskManager()
	tskm.AddNewTask(&socket.ReportAppStateTask{AppState: state, RequestId: uuid.NewString()})

	payload, err := tskm.FinalizePayload()
	if err != nil {
		log.Fatal(err)
	}

	packetId, err := a.client.socket.makeLSRequest(payload, 3)
	if err != nil {
		log.Fatal(err)
	}


	resp := a.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return fmt.Errorf("failed to receive response from socket while trying to report app state. packetId: %d", packetId)
	}

	return nil
}