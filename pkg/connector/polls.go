package connector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type pollState struct {
	threadKey int64
	question  string
	portalKey networkid.PortalKey
	// data is the poll's latest full data, or nil until it has arrived.
	data *table.PollData
}

// handlePollUpdates queries a poll's full data whenever a poll attachment or
// vote change is seen, and bridges the full data when it arrives: the poll
// itself once (poll attachments only have the first 3 options), and a vote
// event for every voter whose selection changed since the previous data.
func (m *MetaClient) handlePollUpdates(params threadMaps, tbl *table.LSTable, innerQueue *[]bridgev2.RemoteEvent) {
	attachments := tbl.WrapPolls()
	changed := tbl.ChangedPollIDs()
	fullData := tbl.WrapPollData()
	if len(attachments) == 0 && len(changed) == 0 && len(fullData) == 0 {
		return
	}

	m.pollsLock.Lock()
	defer m.pollsLock.Unlock()
	toQuery := make(map[int64]int64)
	collectPortalEvents(params, attachments, func(tk handlerParams, att *table.PollAttachment) bridgev2.RemoteEvent {
		if m.polls[att.PollID] == nil {
			m.polls[att.PollID] = &pollState{threadKey: att.ThreadKey, question: att.Question, portalKey: tk.Portal}
		}
		toQuery[att.PollID] = att.ThreadKey
		return nil
	}, innerQueue)
	for _, pollID := range changed {
		// The question and portal are only known from a poll attachment.
		if state := m.polls[pollID]; state != nil {
			toQuery[pollID] = state.threadKey
		}
	}
	for pollID, threadKey := range toQuery {
		go m.queryPollData(params.ctx, pollID, threadKey)
	}
	for _, data := range fullData {
		if state := m.polls[data.PollID]; state != nil {
			*innerQueue = append(*innerQueue, m.pollDataEvents(params.ctx, state, data)...)
		}
	}
}

func (m *MetaClient) queryPollData(ctx context.Context, pollID, threadKey int64) {
	log := zerolog.Ctx(ctx).With().Int64("poll_id", pollID).Logger()
	ctx = log.WithContext(context.Background())
	client := m.Client
	if client == nil || !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
		log.Warn().Msg("Not querying poll data: not connected")
	} else if _, err := client.ExecuteTasks(ctx, &socket.PollPointQueryTask{
		ThreadKey: threadKey,
		PollID:    pollID,
		SyncGroup: 1,
	}); err != nil {
		log.Err(err).Msg("Failed to query poll data")
	} else {
		log.Debug().Msg("Queried poll data")
	}
}

// pollDataEvents must be called with pollsLock held.
func (m *MetaClient) pollDataEvents(ctx context.Context, state *pollState, data *table.PollData) []bridgev2.RemoteEvent {
	log := zerolog.Ctx(ctx).With().Int64("poll_id", data.PollID).Logger()
	logContext := func(c zerolog.Context) zerolog.Context {
		return c.Int64("poll_id", data.PollID)
	}
	startID := metaid.MakeFBMessageID(fmt.Sprintf("pollstart-%d", data.PollID))
	var events []bridgev2.RemoteEvent
	if state.data == nil {
		existingStart, err := m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, m.UserLogin.ID, startID)
		if err != nil {
			log.Err(err).Msg("Failed to check for existing poll-start message")
			return nil
		} else if existingStart == nil {
			question := state.question
			// Sent by the bridge bot, since the poll creator isn't known.
			events = append(events, &simplevent.Message[*table.PollData]{
				EventMeta: simplevent.EventMeta{
					Type:       bridgev2.RemoteEventMessage,
					LogContext: logContext,
					PortalKey:  state.portalKey,
					Timestamp:  time.UnixMilli(data.CreatedAtMs),
				},
				ID:   startID,
				Data: data,
				ConvertMessageFunc: func(_ context.Context, _ *bridgev2.Portal, _ bridgev2.MatrixAPI, data *table.PollData) (*bridgev2.ConvertedMessage, error) {
					return convertPollStart(question, data), nil
				},
			})
		}
	}
	now := time.Now()
	for _, vote := range table.DiffPollVotes(state.data, data) {
		events = append(events, &simplevent.Message[table.PollVote]{
			EventMeta: simplevent.EventMeta{
				Type:       bridgev2.RemoteEventMessage,
				LogContext: logContext,
				PortalKey:  state.portalKey,
				Sender:     m.makeEventSender(vote.ContactID),
				Timestamp:  now,
			},
			ID:   metaid.MakeFBMessageID(fmt.Sprintf("pollvote-%d-%d-%d", data.PollID, vote.ContactID, now.UnixMilli())),
			Data: vote,
			ConvertMessageFunc: func(ctx context.Context, _ *bridgev2.Portal, _ bridgev2.MatrixAPI, vote table.PollVote) (*bridgev2.ConvertedMessage, error) {
				return m.convertPollVote(ctx, startID, vote)
			},
		})
	}
	state.data = data
	log.Debug().Int("events", len(events)).Msg("Received full poll data")
	return events
}

// convertPollStart builds an MSC3381 poll start like mautrix-whatsapp does.
func convertPollStart(question string, data *table.PollData) *bridgev2.ConvertedMessage {
	answers := make([]map[string]any, len(data.Options))
	optionsListText := make([]string, len(data.Options))
	optionsListHTML := make([]string, len(data.Options))
	for i, opt := range data.Options {
		answers[i] = map[string]any{
			"id":                      strconv.FormatInt(opt.OptionID, 10),
			"org.matrix.msc1767.text": opt.Text,
		}
		optionsListText[i] = fmt.Sprintf("%d. %s", i+1, opt.Text)
		optionsListHTML[i] = fmt.Sprintf("<li>%s</li>", event.TextToHTML(opt.Text))
	}
	body := fmt.Sprintf("%s\n\n%s\n\n(This message is a poll. Please open Messenger to vote.)", question, strings.Join(optionsListText, "\n"))
	formattedBody := fmt.Sprintf("<p>%s</p><ol>%s</ol><p>(This message is a poll. Please open Messenger to vote.)</p>", event.TextToHTML(question), strings.Join(optionsListHTML, ""))
	return &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{{
			Type: event.EventUnstablePollStart,
			Content: &event.MessageEventContent{
				MsgType:       event.MsgText,
				Body:          body,
				Format:        event.FormatHTML,
				FormattedBody: formattedBody,
			},
			Extra: map[string]any{
				"org.matrix.msc1767.message": []map[string]any{
					{"mimetype": "text/html", "body": formattedBody},
					{"mimetype": "text/plain", "body": body},
				},
				"org.matrix.msc3381.poll.start": map[string]any{
					"kind":           "org.matrix.msc3381.poll.disclosed",
					"max_selections": len(answers), // Meta polls allow any number of selections
					"question": map[string]any{
						"org.matrix.msc1767.text": question,
					},
					"answers": answers,
				},
			},
			DBMetadata: &metaid.MessageMetadata{PollID: data.PollID},
		}},
	}
}

// convertPollVote builds an MSC3381 poll response with the voter's full
// selection. An empty selection retracts the vote.
func (m *MetaClient) convertPollVote(ctx context.Context, startID networkid.MessageID, vote table.PollVote) (*bridgev2.ConvertedMessage, error) {
	target, err := m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, m.UserLogin.ID, startID)
	if err != nil {
		return nil, fmt.Errorf("failed to find poll-start message to relate vote to: %w", err)
	} else if target == nil {
		return nil, fmt.Errorf("%w: poll-start message not found", bridgev2.ErrIgnoringRemoteEvent)
	}
	selections := make([]string, len(vote.OptionIDs))
	for i, id := range vote.OptionIDs {
		selections[i] = strconv.FormatInt(id, 10)
	}
	return &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{{
			Type: event.EventUnstablePollResponse,
			Content: &event.MessageEventContent{
				RelatesTo: &event.RelatesTo{Type: event.RelReference, EventID: target.MXID},
			},
			Extra: map[string]any{
				"org.matrix.msc3381.poll.response": map[string]any{"answers": selections},
			},
		}},
	}, nil
}
