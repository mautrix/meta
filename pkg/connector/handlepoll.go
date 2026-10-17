package connector

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/simplevent"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

var _ bridgev2.PollHandlingNetworkAPI = (*MetaClient)(nil)

var errPollUnsupportedChat = bridgev2.WrapErrorInStatus(bridgev2.ErrPollsNotSupported).
	WithMessage("Polls are only supported in unencrypted Messenger groups")

type pendingPollSend struct {
	threadID   int64
	pollID     int64
	question   string
	options    []string
	previous   []int64
	selections []int64
	result     chan *table.WrappedMessage
	claimed    atomic.Bool
}

func (m *MetaClient) handlePendingPoll(msg *table.WrappedMessage) bool {
	pending := m.pendingPoll.Load()
	if pending == nil || msg.IsUpsert || msg.SenderId != metaid.ParseUserLoginID(m.UserLogin.ID) ||
		msg.ThreadKey != pending.threadID || msg.MessageId == "" {
		return false
	}
	if pending.pollID == 0 {
		if msg.PollEventType != table.PollEventCreate || msg.PollQuestion != pending.question || len(msg.PollOptions) != len(pending.options) {
			return false
		}
		for i, option := range msg.PollOptions {
			if option.Text != pending.options[i] {
				return false
			}
		}
	} else {
		if msg.PollEventType != table.PollEventVote || msg.PollID != pending.pollID || len(msg.PollVotes)+len(msg.PollRemovals) == 0 {
			return false
		}
		selection, err := msgconv.ApplyPollVote(msg, pending.previous)
		if err != nil || !slices.Equal(selection, pending.selections) {
			return false
		}
	}
	if !pending.claimed.CompareAndSwap(false, true) {
		return false
	}
	pending.result <- msg
	return true
}

func (m *MetaClient) sendPollTask(ctx context.Context, pending *pendingPollSend, task socket.Task) (*table.WrappedMessage, error) {
	if err := m.Client.WaitUntilCanSendMessages(ctx, 15*time.Second); err != nil {
		return nil, err
	}
	pending.result = make(chan *table.WrappedMessage, 1)
	m.pendingPoll.Store(pending)
	defer m.pendingPoll.CompareAndSwap(pending, nil)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := m.Client.ExecuteTasks(ctx, task)
	select {
	case result := <-pending.result:
		return result, nil
	default:
	}
	if err == nil && resp != nil {
		switch {
		case len(resp.LSHandleFailedTask) > 0:
			err = fmt.Errorf("%w: %s", ErrServerRejectedMessage, resp.LSHandleFailedTask[0].Message)
		case len(resp.LSMarkOptimisticMessageFailed) > 0:
			err = fmt.Errorf("%w: %s", ErrServerRejectedMessage, resp.LSMarkOptimisticMessageFailed[0].Message)
		case len(resp.LSIssueNewError) > 0:
			err = fmt.Errorf("%w: %s", ErrServerRejectedMessage, resp.LSIssueNewError[0].ErrorMessage)
		default:
			for _, completion := range resp.LSMailboxTaskCompletionApiOnTaskCompletion {
				if !completion.Success {
					err = ErrServerRejectedMessage
					break
				}
			}
		}
	}
	if err == nil {
		select {
		case result := <-pending.result:
			return result, nil
		case <-ctx.Done():
			err = fmt.Errorf("poll was acknowledged, but its message was not received: %w", ctx.Err())
		}
	}
	if !pending.claimed.CompareAndSwap(false, true) {
		return <-pending.result, nil
	}
	return nil, err
}

func (m *MetaClient) checkPollConnection(ctx context.Context, portal *bridgev2.Portal) error {
	if m.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	} else if !m.GetCapabilities(ctx, portal).Poll.Partial() {
		return errPollUnsupportedChat
	} else if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
		return ErrNotConnected
	}
	return nil
}

func pollMessageResponse(msg *table.WrappedMessage, metadata *metaid.MessageMetadata) *bridgev2.MatrixMessageResponse {
	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        metaid.MakeFBMessageID(msg.MessageId),
			SenderID:  metaid.MakeUserID(msg.SenderId),
			Timestamp: time.UnixMilli(msg.TimestampMs),
			Metadata:  metadata,
		},
		StreamOrder: msg.TimestampMs,
	}
}

func (m *MetaClient) HandleMatrixPollStart(ctx context.Context, msg *bridgev2.MatrixPollStart) (*bridgev2.MatrixMessageResponse, error) {
	if err := m.checkPollConnection(ctx, msg.Portal); err != nil {
		return nil, err
	}
	task, err := m.Main.MsgConv.PollStartToMeta(msg)
	if err != nil {
		return nil, err
	}
	m.pollSendLock.Lock()
	defer m.pollSendLock.Unlock()
	result, err := m.sendPollTask(ctx, &pendingPollSend{
		threadID: task.ThreadKey,
		question: task.QuestionText,
		options:  task.Options,
	}, task)
	if err != nil {
		return nil, err
	}
	aliases := make(map[string]int64, len(result.PollOptions))
	for i, option := range result.PollOptions {
		aliases[msg.Content.PollStart.Answers[i].ID] = option.OptionID
	}
	response := pollMessageResponse(result, &metaid.MessageMetadata{
		PollID:        result.PollID,
		PollQuestion:  result.PollQuestion,
		PollAnswers:   result.PollOptions,
		PollOptions:   aliases,
		PollFetched:   true,
		PollUpdatedMS: result.TimestampMs,
	})
	if msg.Content.PollStart.MaxSelections < len(result.PollOptions) {
		response.PostSave = func(ctx context.Context, saved *database.Message) {
			go m.UserLogin.QueueRemoteEvent(&simplevent.Message[struct{}]{
				EventMeta: simplevent.EventMeta{
					Type:      bridgev2.RemoteEventMessageUpsert,
					PortalKey: msg.Portal.PortalKey,
				},
				ID: saved.ID,
				ConvertMessageFunc: func(context.Context, *bridgev2.Portal, bridgev2.MatrixAPI, struct{}) (*bridgev2.ConvertedMessage, error) {
					return nil, bridgev2.ErrIgnoringRemoteEvent
				},
				HandleExistingFunc: func(
					ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI,
					existing []*database.Message, _ struct{},
				) (bridgev2.UpsertResult, error) {
					parent := pollRoot(existing)
					if parent == nil {
						return bridgev2.UpsertResult{}, nil
					}
					part := m.Main.MsgConv.ConvertPollStart(parent.Metadata.(*metaid.MessageMetadata)).Parts[0].ToEditPart(parent)
					edit := &bridgev2.ConvertedEdit{ModifiedParts: []*bridgev2.ConvertedEditPart{part}}
					return bridgev2.UpsertResult{SubEvents: []bridgev2.RemoteEvent{makePollEdit(portal.PortalKey, parent, edit)}}, nil
				},
			})
		}
	}
	return response, nil
}

func (m *MetaClient) HandleMatrixPollVote(ctx context.Context, msg *bridgev2.MatrixPollVote) (*bridgev2.MatrixMessageResponse, error) {
	if err := m.checkPollConnection(ctx, msg.Portal); err != nil {
		return nil, err
	}
	m.pollSendLock.Lock()
	defer m.pollSendLock.Unlock()
	task, err := m.Main.MsgConv.PollVoteToMeta(msg)
	if err != nil {
		return nil, err
	}
	meta := msg.VoteTo.Metadata.(*metaid.MessageMetadata)
	actorID := metaid.ParseUserLoginID(m.UserLogin.ID)
	previous := meta.PollVotes[actorID].OptionIDs
	if slices.Equal(previous, task.SelectedOptions) {
		return &bridgev2.MatrixMessageResponse{}, nil
	}
	result, err := m.sendPollTask(ctx, &pendingPollSend{
		threadID:   task.ThreadKey,
		pollID:     task.PollID,
		previous:   previous,
		selections: task.SelectedOptions,
	}, task)
	if err != nil {
		return nil, err
	}
	response := pollMessageResponse(result, &metaid.MessageMetadata{})
	response.PostSave = func(ctx context.Context, _ *database.Message) {
		updated := *meta
		updated.PollVotes = maps.Clone(meta.PollVotes)
		if updated.PollVotes == nil {
			updated.PollVotes = make(map[int64]metaid.PollVote)
		}
		updated.PollVotes[actorID] = metaid.PollVote{OptionIDs: task.SelectedOptions, TimestampMS: result.TimestampMs}
		msg.VoteTo.Metadata = &updated
		if err := m.Main.Bridge.DB.Message.Update(ctx, msg.VoteTo); err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save poll selection")
		}
	}
	return response, nil
}
