package table

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	badGlobalLog "github.com/rs/zerolog/log"
)

type UpsertMessages struct {
	Range    *LSInsertNewMessageRange
	Messages []*WrappedMessage
	MarkRead bool
}

func (um *UpsertMessages) Join(other *UpsertMessages) *UpsertMessages {
	if um == nil {
		return other
	} else if other == nil {
		return um
	}
	um.Messages = append(um.Messages, other.Messages...)
	um.Range.HasMoreBefore = other.Range.HasMoreBefore
	um.Range.MinTimestampMsTemplate = other.Range.MinTimestampMsTemplate
	um.Range.MinTimestampMs = other.Range.MinTimestampMs
	um.Range.MinMessageId = other.Range.MinMessageId
	return um
}

func (um *UpsertMessages) GetThreadKey() int64 {
	if um.Range != nil {
		return um.Range.ThreadKey
	} else if len(um.Messages) > 0 {
		return um.Messages[0].ThreadKey
	}
	return 0
}

func (table *LSTable) WrapMessages() (upsert map[int64]*UpsertMessages, insert []*WrappedMessage) {
	messageMap := make(map[string]*WrappedMessage, len(table.LSInsertMessage)+len(table.LSUpsertMessage))

	upsert = make(map[int64]*UpsertMessages, len(table.LSUpsertMessage))
	for _, rng := range table.LSInsertNewMessageRange {
		upsert[rng.ThreadKey] = &UpsertMessages{Range: rng}
	}

	// TODO are there other places that might have read receipts for upserts than these two?
	for _, read := range table.LSMarkThreadRead {
		upsertMsg, ok := upsert[read.ThreadKey]
		if ok {
			upsertMsg.MarkRead = upsertMsg.Range.MaxTimestampMs <= read.LastReadWatermarkTimestampMs
		}
	}
	for _, read := range table.LSMarkThreadReadV2 {
		upsertMsg, ok := upsert[read.ThreadKey]
		if ok {
			upsertMsg.MarkRead = upsertMsg.Range.MaxTimestampMs <= read.LastReadWatermarkTimestampMs
		}
	}
	for _, thread := range table.LSDeleteThenInsertThread {
		upsertMsg, ok := upsert[thread.ThreadKey]
		if ok {
			upsertMsg.MarkRead = upsertMsg.Range.MaxTimestampMs <= thread.LastReadWatermarkTimestampMs
		}
	}

	for _, msg := range table.LSUpsertMessage {
		if messageMap[msg.MessageId] != nil {
			continue
		}
		wrapped := &WrappedMessage{LSInsertMessage: msg.ToInsert(), IsUpsert: true}
		chatUpsert, ok := upsert[msg.ThreadKey]
		if !ok {
			badGlobalLog.Warn().
				Int64("thread_id", msg.ThreadKey).
				Str("message_id", msg.MessageId).
				Msg("Got upsert message for thread without corresponding message range")
			//upsert[msg.ThreadKey] = &UpsertMessages{Messages: []*WrappedMessage{wrapped}}
		} else {
			chatUpsert.Messages = append(chatUpsert.Messages, wrapped)
		}
		messageMap[msg.MessageId] = wrapped
	}

	if len(table.LSUpsertMessage) > 0 {
		// Bundle any upserted reactions with their messages, note that we also continue to process
		// them separately to cover cases where we already have the message but not the reaction.
		// Will be deduped at the framework level.
		for _, reaction := range table.LSUpsertReaction {
			wrapped, ok := messageMap[reaction.MessageId]
			if ok && wrapped.IsUpsert {
				wrapped.Reactions = append(wrapped.Reactions, reaction)
			}
		}
	}

	insert = make([]*WrappedMessage, len(table.LSInsertMessage))
	for i, msg := range table.LSInsertMessage {
		insert[i] = &WrappedMessage{LSInsertMessage: msg}
		messageMap[msg.MessageId] = insert[i]
	}
	for _, poll := range table.LSAddPollForThread {
		if msg := messageMap[poll.LastUpdateMessageID]; msg != nil && msg.ThreadKey == poll.ThreadKey {
			msg.PollID = poll.PollID
			msg.PollEventType = poll.LastUpdateMessageEventType
		}
	}
	for _, update := range table.LSUpdatePollAdminMessageSummary {
		if msg := messageMap[update.MessageID]; msg != nil && msg.ThreadKey == update.ThreadKey {
			msg.PollID = update.PollID
			msg.PollEventType = update.MessageEventType
		}
	}
	for _, poll := range table.LSVerifyPollExists {
		if msg := messageMap[poll.MessageID]; msg != nil && msg.ThreadKey == poll.ThreadKey {
			msg.PollID = poll.PollID
			msg.PollEventType = poll.MessageEventType
		}
	}
	pollCreations := make(map[int64]*WrappedMessage)
	for _, msg := range messageMap {
		if msg.PollID != 0 && msg.PollEventType == PollEventCreate {
			pollCreations[msg.PollID] = msg
		}
	}
	pollOptions := slices.Clone(table.LSAddPollOption)
	slices.SortStableFunc(pollOptions, func(a, b *LSAddPollOption) int {
		return cmp.Compare(a.SortKeyCreationTimestamp, b.SortKeyCreationTimestamp)
	})
	for _, option := range pollOptions {
		if msg := pollCreations[option.PollID]; msg != nil {
			msg.PollOptions = append(msg.PollOptions, PollOption{OptionID: option.OptionID, Text: option.OptionText})
			msg.PollOptionsComplete = true
		}
	}
	for _, option := range table.LSAddPollOptionV2 {
		if msg := messageMap[option.MessageID]; msg != nil && msg.ThreadKey == option.ThreadKey {
			msg.PollID = option.PollID
			msg.PollEventType = option.MessageEventType
			msg.PollOptions = append(msg.PollOptions, PollOption{OptionID: option.OptionID, Text: option.OptionText})
		}
	}
	for _, vote := range table.LSAddPollVoteV2 {
		if msg := messageMap[vote.MessageID]; msg != nil && msg.ThreadKey == vote.ThreadKey {
			msg.PollID = vote.PollID
			msg.PollEventType = vote.MessageEventType
			msg.PollVotes = append(msg.PollVotes, vote)
		}
	}
	for _, vote := range table.LSRemovePollVoteV2 {
		if msg := messageMap[vote.MessageID]; msg != nil && msg.ThreadKey == vote.ThreadKey {
			msg.PollID = vote.PollID
			msg.PollEventType = vote.MessageEventType
			msg.PollRemovals = append(msg.PollRemovals, vote)
		}
	}

	for _, blob := range table.LSInsertBlobAttachment {
		msg, ok := messageMap[blob.MessageId]
		if ok {
			msg.BlobAttachments = append(msg.BlobAttachments, blob)
		} else {
			badGlobalLog.Warn().
				Str("message_id", blob.MessageId).
				Str("attachment_id", blob.AttachmentFbid).
				Msg("Got blob attachment in table without corresponding message")
		}
	}
	for _, att := range table.LSInsertAttachment {
		msg, ok := messageMap[att.MessageId]
		if ok {
			msg.Attachments = append(msg.Attachments, att)
		} else {
			badGlobalLog.Warn().
				Str("message_id", att.MessageId).
				Str("attachment_id", att.AttachmentFbid).
				Msg("Got attachment in table without corresponding message")
		}
	}
	ctaMap := make(map[string]*LSInsertAttachmentCta, len(table.LSInsertAttachmentCta))
	for _, cta := range table.LSInsertAttachmentCta {
		ctaMap[cta.AttachmentFbid] = cta
	}
	for _, xma := range table.LSInsertXmaAttachment {
		msg, ok := messageMap[xma.MessageId]
		if ok {
			wrappedXMA := &WrappedXMA{LSInsertXmaAttachment: xma, CTA: ctaMap[xma.AttachmentFbid]}
			msg.XMAAttachments = append(msg.XMAAttachments, wrappedXMA)
			if wrappedXMA.CTA != nil && strings.HasPrefix(wrappedXMA.CTA.Type_, "xma_poll_") &&
				xma.AttachmentFbid == strconv.FormatInt(msg.PollID, 10) {
				msg.PollQuestion = xma.ListItemsDescriptionText
				if len(msg.PollOptions) == 0 && msg.PollEventType == PollEventCreate {
					for _, option := range []PollOption{
						{OptionID: xma.ListItemId1, Text: xma.ListItemTitleText1},
						{OptionID: xma.ListItemId2, Text: xma.ListItemTitleText2},
						{OptionID: xma.ListItemId3, Text: xma.ListItemTitleText3},
					} {
						if option.OptionID != 0 {
							msg.PollOptions = append(msg.PollOptions, option)
						}
					}
				}
			}
		} else {
			badGlobalLog.Warn().
				Str("message_id", xma.MessageId).
				Str("attachment_id", xma.AttachmentFbid).
				Msg("Got XMA attachment in table without corresponding message")
		}
	}
	insert = splitPollUpdates(insert)
	for _, chatUpsert := range upsert {
		chatUpsert.Messages = splitPollUpdates(chatUpsert.Messages)
	}
	for _, blob := range table.LSInsertStickerAttachment {
		msg, ok := messageMap[blob.MessageId]
		if ok {
			msg.Stickers = append(msg.Stickers, blob)
		} else {
			badGlobalLog.Warn().
				Str("message_id", blob.MessageId).
				Str("attachment_id", blob.AttachmentFbid).
				Msg("Got sticker attachment in table without corresponding message")
		}
	}
	return
}

func splitPollUpdates(msgs []*WrappedMessage) []*WrappedMessage {
	for i := 0; i < len(msgs); i++ {
		msg := msgs[i]
		if msg.PollEventType != PollEventAddVotedOption && msg.PollEventType != PollEventMultipleUpdates {
			continue
		} else if len(msg.PollVotes)+len(msg.PollRemovals) == 0 {
			msg.PollEventType = PollEventAddOption
			continue
		}
		msg.PollEventType = PollEventVote
		if len(msg.PollOptions) > 0 {
			options := &WrappedMessage{
				LSInsertMessage: msg.LSInsertMessage,
				IsUpsert:        msg.IsUpsert,
				PollID:          msg.PollID,
				PollEventType:   PollEventAddOption,
				PollQuestion:    msg.PollQuestion,
				PollOptions:     msg.PollOptions,
			}
			msg.PollOptions = nil
			msgs = slices.Insert(msgs, i, options)
			i++
		}
	}
	return msgs
}

type WrappedMessage struct {
	*LSInsertMessage
	IsUpsert            bool
	BlobAttachments     []*LSInsertBlobAttachment
	Attachments         []*LSInsertAttachment
	XMAAttachments      []*WrappedXMA
	Stickers            []*LSInsertStickerAttachment
	Reactions           []*LSUpsertReaction
	PollID              int64
	PollEventType       PollEventType
	PollQuestion        string
	PollOptions         []PollOption
	PollOptionsComplete bool
	PollVotes           []*LSAddPollVote
	PollRemovals        []*LSAddPollVote

	ThreadID         string
	IsSubthreadStart bool
}

type WrappedXMA struct {
	*LSInsertXmaAttachment
	CTA *LSInsertAttachmentCta
}
