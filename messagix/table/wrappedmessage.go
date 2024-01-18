package table

import (
	badGlobalLog "github.com/rs/zerolog/log"
)

func (table *LSTable) WrapMessages() []*WrappedMessage {
	messages := make([]*WrappedMessage, len(table.LSInsertMessage))
	messageMap := make(map[string]*WrappedMessage, len(table.LSInsertMessage))
	for i, msg := range table.LSInsertMessage {
		messages[i] = &WrappedMessage{LSInsertMessage: msg}
		messageMap[msg.MessageId] = messages[i]
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
	return messages
}

type WrappedMessage struct {
	*LSInsertMessage
	BlobAttachments []*LSInsertBlobAttachment
	XMAAttachments  []*WrappedXMA
	Stickers        []*LSInsertStickerAttachment
}

type WrappedXMA struct {
	*LSInsertXmaAttachment
	CTA *LSInsertAttachmentCta
}
