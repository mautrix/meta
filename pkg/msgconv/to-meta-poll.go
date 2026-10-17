package msgconv

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	errPollHiddenVotes = bridgev2.WrapErrorInStatus(errors.New("hidden poll votes are not supported by Messenger")).
				WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)
	errPollOptionCount = bridgev2.WrapErrorInStatus(errors.New("a Messenger poll needs at least two options")).
				WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)
	errPollQuestion = bridgev2.WrapErrorInStatus(errors.New("poll questions must contain 1 to 140 characters")).
			WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)
	errPollOptionText = bridgev2.WrapErrorInStatus(errors.New("poll options must contain 1 to 140 characters")).
				WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)
	errPollOptionID = bridgev2.WrapErrorInStatus(errors.New("poll option IDs must be nonempty and unique")).
			WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)
	errPollDuplicateOption = bridgev2.WrapErrorInStatus(errors.New("duplicate poll options are not supported by Messenger")).
				WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)
	errPollNotLoaded = bridgev2.WrapErrorInStatus(errors.New("the poll is still loading; try again shortly")).
				WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true)
	errPollUnknownOption = bridgev2.WrapErrorInStatus(errors.New("unknown poll option")).
				WithIsCertain(true).WithErrorAsMessage().WithSendNotice(true).WithErrorReason(event.MessageStatusUnsupported)
)

func (mc *MessageConverter) PollStartToMeta(msg *bridgev2.MatrixPollStart) (*socket.CreatePollTask, error) {
	content := msg.Content.PollStart
	if content.Kind != "" && content.Kind != "org.matrix.msc3381.poll.disclosed" {
		return nil, errPollHiddenVotes
	} else if len(content.Answers) < 2 {
		return nil, errPollOptionCount
	}
	question := content.Question.GetText()
	if strings.TrimSpace(question) == "" || len(utf16.Encode([]rune(question))) > 140 {
		return nil, errPollQuestion
	}
	options := make([]string, len(content.Answers))
	ids := make(map[string]bool, len(options))
	for i, answer := range content.Answers {
		text := answer.GetText()
		if strings.TrimSpace(text) == "" || len(utf16.Encode([]rune(text))) > 140 {
			return nil, errPollOptionText
		} else if answer.ID == "" || ids[answer.ID] {
			return nil, errPollOptionID
		} else if slices.Contains(options[:i], text) {
			return nil, errPollDuplicateOption
		}
		ids[answer.ID] = true
		options[i] = text
	}
	return &socket.CreatePollTask{
		QuestionText: question,
		ThreadKey:    metaid.ParseFBPortalID(msg.Portal.ID),
		Options:      options,
		SyncGroup:    1,
	}, nil
}

func (mc *MessageConverter) PollVoteToMeta(msg *bridgev2.MatrixPollVote) (*socket.UpdatePollTask, error) {
	meta := msg.VoteTo.Metadata.(*metaid.MessageMetadata)
	if meta.PollID == 0 || !meta.PollFetched {
		return nil, errPollNotLoaded
	}
	selections := make([]int64, 0, len(msg.Content.Response.Answers))
	for _, answer := range msg.Content.Response.Answers {
		optionID, ok := meta.PollOptions[answer]
		if !ok {
			var err error
			optionID, err = strconv.ParseInt(answer, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: %q", errPollUnknownOption, answer)
			}
		}
		if !slices.ContainsFunc(meta.PollAnswers, func(option table.PollOption) bool { return option.OptionID == optionID }) {
			return nil, fmt.Errorf("%w: %q", errPollUnknownOption, answer)
		}
		selections = append(selections, optionID)
	}
	slices.Sort(selections)
	return &socket.UpdatePollTask{
		ThreadKey:       metaid.ParseFBPortalID(msg.Portal.ID),
		PollID:          meta.PollID,
		SelectedOptions: slices.Compact(selections),
		SyncGroup:       1,
	}, nil
}
