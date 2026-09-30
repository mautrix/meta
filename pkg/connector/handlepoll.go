package connector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var _ bridgev2.PollHandlingNetworkAPI = (*MetaClient)(nil)

func (m *MetaClient) HandleMatrixPollStart(ctx context.Context, msg *bridgev2.MatrixPollStart) (*bridgev2.MatrixMessageResponse, error) {
	return nil, errors.New("creating polls from Matrix is not currently supported")
}

func (m *MetaClient) HandleMatrixPollVote(ctx context.Context, msg *bridgev2.MatrixPollVote) (*bridgev2.MatrixMessageResponse, error) {
	if m.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
		return nil, ErrNotConnected
	}
	pollMeta, ok := msg.VoteTo.Metadata.(*metaid.MessageMetadata)
	if !ok || pollMeta.PollID == 0 {
		return nil, fmt.Errorf("target message is not a recognized poll")
	}
	selections := make([]int64, 0, len(msg.Content.Response.Answers))
	for _, answer := range msg.Content.Response.Answers {
		if optionID, err := strconv.ParseInt(answer, 10, 64); err == nil {
			selections = append(selections, optionID)
		}
	}
	_, err := m.Client.ExecuteTasks(ctx, &socket.UpdatePollTask{
		ThreadKey:       metaid.ParseFBPortalID(msg.Portal.ID),
		PollID:          pollMeta.PollID,
		SelectedOptions: selections,
		SyncGroup:       1,
	})
	if err != nil {
		return nil, err
	}
	// Update the cached poll data so that our own vote isn't bridged back.
	m.pollsLock.Lock()
	if state := m.polls[pollMeta.PollID]; state != nil && state.data != nil {
		state.data = state.data.WithVote(metaid.ParseUserLoginID(m.UserLogin.ID), selections)
	}
	m.pollsLock.Unlock()
	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        metaid.MakeFBMessageID(fmt.Sprintf("pollvote-out-%d-%s", pollMeta.PollID, msg.Event.ID)),
			SenderID:  networkid.UserID(m.UserLogin.ID),
			Timestamp: time.Now(),
		},
	}, nil
}
