package msgconv

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (mc *MessageConverter) ConvertPollStart(meta *metaid.MessageMetadata) *bridgev2.ConvertedMessage {
	answers := make([]event.PollOption, len(meta.PollAnswers))
	lines := make([]string, len(meta.PollAnswers))
	htmlLines := make([]string, len(meta.PollAnswers))
	for i, opt := range meta.PollAnswers {
		answerID := strconv.FormatInt(opt.OptionID, 10)
		for alias, remoteID := range meta.PollOptions {
			if remoteID == opt.OptionID {
				answerID = alias
				break
			}
		}
		answers[i] = event.PollOption{ID: answerID, MSC1767Message: event.MSC1767Message{Text: opt.Text}}
		lines[i] = fmt.Sprintf("%d. %s", i+1, opt.Text)
		htmlLines[i] = "<li>" + event.TextToHTML(opt.Text) + "</li>"
	}
	body := meta.PollQuestion + "\n\n" + strings.Join(lines, "\n")
	formatted := "<p>" + event.TextToHTML(meta.PollQuestion) + "</p><ol>" + strings.Join(htmlLines, "") + "</ol>"
	return &bridgev2.ConvertedMessage{Parts: []*bridgev2.ConvertedMessagePart{{
		ID:   metaid.MakeMessagePartID(0),
		Type: event.EventUnstablePollStart,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgText,
			Body:          body,
			Format:        event.FormatHTML,
			FormattedBody: formatted,
		},
		Extra: map[string]any{
			"org.matrix.msc1767.message": []event.ExtensibleText{
				{MimeType: "text/plain", Body: body},
				{MimeType: "text/html", Body: formatted},
			},
			"org.matrix.msc3381.poll.start": event.PollStart{
				Kind:          "org.matrix.msc3381.poll.disclosed",
				MaxSelections: len(answers),
				Question:      event.MSC1767Message{Text: meta.PollQuestion},
				Answers:       answers,
			},
		},
		DBMetadata: meta,
	}}}
}

func (mc *MessageConverter) convertPoll(ctx context.Context, portal *bridgev2.Portal, msg *table.WrappedMessage) (*bridgev2.ConvertedMessage, error) {
	switch msg.PollEventType {
	case table.PollEventCreate:
		if msg.PollQuestion == "" || len(msg.PollOptions) == 0 {
			return nil, fmt.Errorf("poll %d has no question or options", msg.PollID)
		}
		return mc.ConvertPollStart(&metaid.MessageMetadata{
			PollID:        msg.PollID,
			PollQuestion:  msg.PollQuestion,
			PollAnswers:   msg.PollOptions,
			PollFetched:   msg.PollOptionsComplete && !msg.IsUpsert,
			PollUpdatedMS: msg.TimestampMs,
		}), nil
	case table.PollEventVote:
		return mc.convertPollVote(ctx, portal, msg)
	default:
		return nil, bridgev2.ErrIgnoringRemoteEvent
	}
}

func (mc *MessageConverter) convertPollVote(ctx context.Context, portal *bridgev2.Portal, msg *table.WrappedMessage) (*bridgev2.ConvertedMessage, error) {
	if len(msg.PollVotes) == 0 && len(msg.PollRemovals) == 0 {
		return nil, bridgev2.ErrIgnoringRemoteEvent
	}
	targetID, err := mc.DB.GetPollMessageID(ctx, portal.PortalKey, msg.PollID)
	if err != nil {
		return nil, fmt.Errorf("failed to find poll: %w", err)
	} else if targetID == "" {
		return nil, fmt.Errorf("%w: poll not found", bridgev2.ErrIgnoringRemoteEvent)
	}
	target, err := mc.Bridge.DB.Message.GetFirstPartByID(ctx, portal.Receiver, targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get poll message: %w", err)
	} else if target == nil {
		return nil, fmt.Errorf("%w: poll message not found", bridgev2.ErrIgnoringRemoteEvent)
	}
	meta := target.Metadata.(*metaid.MessageMetadata)
	if !meta.PollFetched {
		return nil, fmt.Errorf("poll selections have not been fetched")
	}
	previous := meta.PollVotes[msg.SenderId]
	if previous.TimestampMS > msg.TimestampMs {
		return nil, bridgev2.ErrIgnoringRemoteEvent
	}
	selection, err := ApplyPollVote(msg, previous.OptionIDs)
	if err != nil {
		return nil, err
	}
	updated := *meta
	updated.PollVotes = maps.Clone(meta.PollVotes)
	if updated.PollVotes == nil {
		updated.PollVotes = make(map[int64]metaid.PollVote)
	}
	updated.PollVotes[msg.SenderId] = metaid.PollVote{OptionIDs: selection, TimestampMS: msg.TimestampMs}
	target.Metadata = &updated
	if err = mc.Bridge.DB.Message.Update(ctx, target); err != nil {
		return nil, fmt.Errorf("failed to save poll selection: %w", err)
	}
	return mc.ConvertPollResponse(target, meta, selection, ""), nil
}

func (mc *MessageConverter) ConvertPollResponse(
	target *database.Message, meta *metaid.MessageMetadata,
	selection []int64, partID networkid.PartID,
) *bridgev2.ConvertedMessage {
	answers := make([]string, len(selection))
	for i, selected := range selection {
		answers[i] = strconv.FormatInt(selected, 10)
		for alias, remoteID := range meta.PollOptions {
			if selected == remoteID {
				answers[i] = alias
				break
			}
		}
	}
	return &bridgev2.ConvertedMessage{Parts: []*bridgev2.ConvertedMessagePart{{
		ID:      partID,
		Type:    event.EventUnstablePollResponse,
		Content: &event.MessageEventContent{RelatesTo: &event.RelatesTo{Type: event.RelReference, EventID: target.MXID}},
		Extra:   map[string]any{"org.matrix.msc3381.poll.response": event.PollResponse{Answers: answers}},
	}}}
}

func ApplyPollVote(msg *table.WrappedMessage, previous []int64) ([]int64, error) {
	selection := slices.Clone(previous)
	for _, vote := range msg.PollRemovals {
		if vote.ContactID != msg.SenderId {
			return nil, fmt.Errorf("poll removal actor does not match message sender")
		}
		selection = slices.DeleteFunc(selection, func(optionID int64) bool { return optionID == vote.OptionID })
	}
	for _, vote := range msg.PollVotes {
		if vote.ContactID != msg.SenderId {
			return nil, fmt.Errorf("poll vote actor does not match message sender")
		}
		selection = append(selection, vote.OptionID)
	}
	slices.Sort(selection)
	return slices.Compact(selection), nil
}

func (mc *MessageConverter) ConvertPollEdit(
	ctx context.Context, portal *bridgev2.Portal,
	msg *table.WrappedMessage, existing *database.Message,
) (*bridgev2.ConvertedEdit, error) {
	meta := *existing.Metadata.(*metaid.MessageMetadata)
	if meta.PollID != msg.PollID || len(msg.PollOptions) == 0 ||
		(!msg.PollOptionsComplete && meta.PollUpdatedMS >= msg.TimestampMs) {
		return nil, bridgev2.ErrIgnoringRemoteEvent
	}
	meta.PollAnswers = slices.Clone(meta.PollAnswers)
	for _, option := range msg.PollOptions {
		index := slices.IndexFunc(meta.PollAnswers, func(existing table.PollOption) bool { return existing.OptionID == option.OptionID })
		if index < 0 {
			meta.PollAnswers = append(meta.PollAnswers, option)
		} else {
			meta.PollAnswers[index] = option
		}
	}
	meta.PollFetched = meta.PollFetched || msg.PollOptionsComplete
	meta.PollUpdatedMS = max(meta.PollUpdatedMS, msg.TimestampMs)
	part := mc.ConvertPollStart(&meta).Parts[0].ToEditPart(existing)
	return &bridgev2.ConvertedEdit{ModifiedParts: []*bridgev2.ConvertedEditPart{part}}, nil
}

func (mc *MessageConverter) ConvertPollSnapshot(existing *database.Message, data *table.PollData) (*bridgev2.ConvertedEdit, error) {
	meta := *existing.Metadata.(*metaid.MessageMetadata)
	if meta.PollID != data.PollID || len(data.Options) == 0 {
		return nil, bridgev2.ErrIgnoringRemoteEvent
	}
	meta.PollAnswers = slices.Clone(data.Options)
	for _, option := range existing.Metadata.(*metaid.MessageMetadata).PollAnswers {
		if !slices.ContainsFunc(meta.PollAnswers, func(existing table.PollOption) bool { return existing.OptionID == option.OptionID }) {
			meta.PollAnswers = append(meta.PollAnswers, option)
		}
	}
	if meta.PollFetched {
		if slices.Equal(meta.PollAnswers, existing.Metadata.(*metaid.MessageMetadata).PollAnswers) {
			return nil, bridgev2.ErrIgnoringRemoteEvent
		}
	} else {
		meta.PollVotes = make(map[int64]metaid.PollVote, len(data.Votes))
		for actor, selection := range data.Votes {
			meta.PollVotes[actor] = metaid.PollVote{
				OptionIDs:   slices.Clone(selection),
				TimestampMS: data.VoteTimestamps[actor],
			}
		}
	}
	meta.PollFetched = true
	part := mc.ConvertPollStart(&meta).Parts[0].ToEditPart(existing)
	return &bridgev2.ConvertedEdit{ModifiedParts: []*bridgev2.ConvertedEditPart{part}}, nil
}
