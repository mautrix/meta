package messagix

import (
	"fmt"
	"log"
	"strconv"

	"github.com/0xzer/messagix/socket"
	"github.com/0xzer/messagix/table"
)

type Messages struct {
	client *Client
}

func (m *Messages) SendReaction(threadId int64, messageId string, reaction string) (*table.LSTable, error) {
	accId, err := strconv.Atoi(m.client.configs.browserConfigTable.CurrentUserInitialData.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert AccountId to int64: %e", err)
	}
	tskm := m.client.NewTaskManager()
	tskm.AddNewTask(&socket.SendReactionTask{
		ThreadKey: threadId,
		MessageID: messageId,
		ActorID: int64(accId),
		Reaction: reaction,
		SendAttribution: table.MESSENGER_INBOX_IN_THREAD,
	})

	payload, err := tskm.FinalizePayload()
	if err != nil {
		log.Fatal(err)
	}
	
	packetId, err := m.client.socket.makeLSRequest(payload, 3)
	if err != nil {
		log.Fatal(err)
	}


	resp := m.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("failed to receive response from socket after sending reaction. packetId: %d", packetId)
	}
	resp.Finish()

	return &resp.Table, nil
}

func (m *Messages) DeleteMessage(messageId string, deleteForMeOnly bool) (*table.LSTable, error) {
	var taskData socket.Task
	if deleteForMeOnly {
		taskData = &socket.DeleteMessageMeOnlyTask{MessageId: messageId,}
	} else {
		taskData = &socket.DeleteMessageTask{MessageId: messageId}
	}
	tskm := m.client.NewTaskManager()
	tskm.AddNewTask(taskData)

	payload, err := tskm.FinalizePayload()
	if err != nil {
		log.Fatal(err)
	}
	
	packetId, err := m.client.socket.makeLSRequest(payload, 3)
	if err != nil {
		log.Fatal(err)
	}

	resp := m.client.socket.responseHandler.waitForPubResponseDetails(packetId)
	if resp == nil {
		return nil, fmt.Errorf("failed to receive response from socket after deleting message. packetId: %d", packetId)
	}
	resp.Finish()
	
	return &resp.Table, nil
}