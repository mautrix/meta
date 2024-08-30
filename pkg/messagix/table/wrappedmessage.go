package table

import (
	"slices"

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
		// For upserted messages, add reactions to the upsert data, and delete them
		// from the main list to avoid handling them as new reactions.
		table.LSUpsertReaction = slices.DeleteFunc(table.LSUpsertReaction, func(reaction *LSUpsertReaction) bool {
			wrapped, ok := messageMap[reaction.MessageId]
			if ok && wrapped.IsUpsert {
				wrapped.Reactions = append(wrapped.Reactions, reaction)
				return true
			}
			return false
		})
	}
	insert = make([]*WrappedMessage, len(table.LSInsertMessage))
	for i, msg := range table.LSInsertMessage {
		insert[i] = &WrappedMessage{LSInsertMessage: msg}
		messageMap[msg.MessageId] = insert[i]
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
		} else {
			badGlobalLog.Warn().
				Str("message_id", xma.MessageId).
				Str("attachment_id", xma.AttachmentFbid).
				Msg("Got XMA attachment in table without corresponding message")
		}
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

type WrappedMessage struct {
	*LSInsertMessage
	IsUpsert        bool
	BlobAttachments []*LSInsertBlobAttachment
	Attachments     []*LSInsertAttachment
	XMAAttachments  []*WrappedXMA
	Stickers        []*LSInsertStickerAttachment
	Reactions       []*LSUpsertReaction

	ThreadID         string
	IsSubthreadStart bool
}

type WrappedXMA struct {
	*LSInsertXmaAttachment
	CTA *LSInsertAttachmentCta
}
