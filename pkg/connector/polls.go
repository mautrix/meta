package connector

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const pollVotePartPrefix = "pollvote:"

type pollQuery struct {
	portal      networkid.PortalKey
	target      networkid.MessageID
	done        chan struct{}
	running     atomic.Bool
	importVotes bool
}

type pollEditEvent struct {
	*simplevent.Message[*database.Message]
}

func (evt *pollEditEvent) GetTargetDBMessage() []*database.Message {
	return []*database.Message{evt.Data}
}

func pollRoot(parts []*database.Message) *database.Message {
	for _, part := range parts {
		if part.PartID == "" && part.Metadata.(*metaid.MessageMetadata).PollID != 0 {
			return part
		}
	}
	return nil
}

func makePollEdit(portal networkid.PortalKey, parent *database.Message, edit *bridgev2.ConvertedEdit) *pollEditEvent {
	return &pollEditEvent{&simplevent.Message[*database.Message]{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventEdit,
			PortalKey: portal,
			Sender:    bridgev2.EventSender{ForceEditOrigSender: true},
		},
		Data:          parent,
		TargetMessage: parent.ID,
		ConvertEditFunc: func(context.Context, *bridgev2.Portal, bridgev2.MatrixAPI, []*database.Message, *database.Message) (*bridgev2.ConvertedEdit, error) {
			return edit, nil
		},
	}}
}

func (m *MetaClient) retryPendingPolls(ctx context.Context) {
	m.pollQueries.Range(func(key, value any) bool {
		query := value.(*pollQuery)
		parent, err := m.Main.Bridge.DB.Message.GetPartByID(ctx, query.portal.Receiver, query.target, "")
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Int64("poll_id", key.(int64)).Msg("Failed to find poll for retry")
			return true
		} else if parent == nil {
			m.removePollQuery(key.(int64), query)
			return true
		}
		m.requestPoll(ctx, query.portal, key.(int64), query.target)
		return true
	})
}

func (m *MetaClient) removePollQuery(pollID int64, query *pollQuery) {
	if m.pollQueries.CompareAndDelete(pollID, query) {
		close(query.done)
	}
}

func (m *MetaClient) requestPoll(ctx context.Context, portal networkid.PortalKey, pollID int64, target networkid.MessageID) {
	query := &pollQuery{portal: portal, target: target, done: make(chan struct{})}
	previous, loaded := m.pollQueries.LoadOrStore(pollID, query)
	if loaded {
		query = previous.(*pollQuery)
	}
	if !query.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		ctx = context.WithoutCancel(ctx)
		defer query.running.Store(false)
		for attempt := range 3 {
			select {
			case <-query.done:
				return
			default:
			}
			if attempt > 0 {
				select {
				case <-query.done:
					return
				case <-time.After(time.Second << (attempt - 1)):
				}
			}
			client := m.Client
			if client == nil {
				return
			}
			requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_, err := client.ExecuteTasks(requestCtx, &socket.PollPointQueryTask{
				ThreadKey: metaid.ParseFBPortalID(portal.ID),
				PollID:    pollID,
				SyncGroup: 1,
			})
			if err == nil {
				select {
				case <-query.done:
					cancel()
					return
				case <-requestCtx.Done():
					err = requestCtx.Err()
				}
			}
			cancel()
			zerolog.Ctx(ctx).Err(err).Int64("poll_id", pollID).Int("attempt", attempt+1).Msg("Failed to fetch poll details")
		}
	}()
}

func (evt *FBMessageEvent) fetchPollForVote(ctx context.Context, portal *bridgev2.Portal) bool {
	targetID, err := evt.m.Main.DB.GetPollMessageID(ctx, portal.PortalKey, evt.PollID)
	if err != nil || targetID == "" {
		return false
	}
	target, err := evt.m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, portal.Receiver, targetID)
	if err != nil || target == nil {
		return false
	}
	fetched := target.Metadata.(*metaid.MessageMetadata).PollFetched
	_, pending := evt.m.pollQueries.Load(evt.PollID)
	if !fetched || pending {
		evt.m.requestPoll(ctx, portal.PortalKey, evt.PollID, targetID)
	}
	return !fetched
}

func (m *MetaClient) wrapPollSnapshot(data *table.PollData) bridgev2.RemoteEvent {
	value, ok := m.pollQueries.Load(data.PollID)
	if !ok {
		return nil
	}
	query := value.(*pollQuery)
	var expected []networkid.PartID
	var expectedOptions []table.PollOption
	return &simplevent.Message[*table.PollData]{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventMessageUpsert,
			PortalKey: query.portal,
			PostHandleFunc: func(ctx context.Context, portal *bridgev2.Portal) {
				parts, err := m.Main.Bridge.DB.Message.GetAllPartsByID(ctx, portal.Receiver, query.target)
				parent := pollRoot(parts)
				if err == nil && parent == nil {
					m.removePollQuery(data.PollID, query)
					return
				} else if err != nil || !parent.Metadata.(*metaid.MessageMetadata).PollFetched {
					zerolog.Ctx(ctx).Err(err).Int64("poll_id", data.PollID).Msg("Poll details were not saved")
					return
				}
				if !slices.Equal(parent.Metadata.(*metaid.MessageMetadata).PollAnswers, expectedOptions) {
					return
				}
				for _, partID := range expected {
					if !slices.ContainsFunc(parts, func(part *database.Message) bool { return part.PartID == partID }) {
						return
					}
				}
				m.removePollQuery(data.PollID, query)
			},
		},
		ID:   query.target,
		Data: data,
		ConvertMessageFunc: func(context.Context, *bridgev2.Portal, bridgev2.MatrixAPI, *table.PollData) (*bridgev2.ConvertedMessage, error) {
			return nil, bridgev2.ErrIgnoringRemoteEvent
		},
		HandleExistingFunc: func(
			ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI,
			parts []*database.Message, data *table.PollData,
		) (bridgev2.UpsertResult, error) {
			parent := pollRoot(parts)
			if parent == nil {
				return bridgev2.UpsertResult{}, nil
			}
			query.importVotes = query.importVotes || !parent.Metadata.(*metaid.MessageMetadata).PollFetched
			edit, err := m.Main.MsgConv.ConvertPollSnapshot(parent, data)
			if err != nil && !errors.Is(err, bridgev2.ErrIgnoringRemoteEvent) {
				return bridgev2.UpsertResult{}, err
			}
			result := bridgev2.UpsertResult{}
			meta := parent.Metadata.(*metaid.MessageMetadata)
			if edit != nil {
				result.SubEvents = append(result.SubEvents, makePollEdit(portal.PortalKey, parent, edit))
				meta = edit.ModifiedParts[0].Part.Metadata.(*metaid.MessageMetadata)
			}
			expectedOptions = meta.PollAnswers
			if !query.importVotes {
				return result, nil
			}
			for actor, vote := range meta.PollVotes {
				partID := networkid.PartID(pollVotePartPrefix + strconv.FormatInt(actor, 10))
				expected = append(expected, partID)
				result.SubEvents = append(result.SubEvents, &simplevent.Message[struct{}]{
					EventMeta: simplevent.EventMeta{
						Type:        bridgev2.RemoteEventMessageUpsert,
						PortalKey:   portal.PortalKey,
						Sender:      m.makeEventSender(actor),
						Timestamp:   parent.Timestamp,
						StreamOrder: parent.Timestamp.UnixMilli(),
					},
					ID: parent.ID,
					HandleExistingFunc: func(
						ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI,
						parts []*database.Message, _ struct{},
					) (bridgev2.UpsertResult, error) {
						missing := !slices.ContainsFunc(parts, func(part *database.Message) bool { return part.PartID == partID })
						return bridgev2.UpsertResult{ContinueMessageHandling: pollRoot(parts) != nil && missing}, nil
					},
					ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, _ struct{}) (*bridgev2.ConvertedMessage, error) {
						root, err := m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, portal.Receiver, parent.ID)
						if err != nil {
							return nil, err
						} else if root == nil || !root.Metadata.(*metaid.MessageMetadata).PollFetched {
							return nil, bridgev2.ErrIgnoringRemoteEvent
						}
						return m.Main.MsgConv.ConvertPollResponse(root, meta, vote.OptionIDs, partID), nil
					},
				})
			}
			return result, nil
		},
	}
}
