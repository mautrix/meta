package connector

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

const EditMaxAge = 15 * time.Minute

type MediaEditCacheKey struct {
	MessageID  networkid.MessageID
	PortalMXID id.RoomID
}

type MediaEditCacheValue struct {
	Part   *bridgev2.ConvertedMessagePart
	Expiry time.Time
}

type MediaEditCache map[MediaEditCacheKey]MediaEditCacheValue

func (m *MetaConnector) mediaEditCacheExpireLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	ctxDone := ctx.Done()
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctxDone:
			return
		}
		m.expireMediaEditCache()
	}
}

func (m *MetaConnector) AddMediaEditCache(portal *bridgev2.Portal, messageID networkid.MessageID, converted *bridgev2.ConvertedMessagePart) {
	if converted == nil || converted.Content == nil || !converted.Content.MsgType.IsMedia() {
		return
	}
	m.mediaEditCacheLock.Lock()
	defer m.mediaEditCacheLock.Unlock()
	m.mediaEditCache[MediaEditCacheKey{
		MessageID:  messageID,
		PortalMXID: portal.MXID,
	}] = MediaEditCacheValue{
		Part:   converted,
		Expiry: time.Now().Add(EditMaxAge + 5*time.Minute),
	}
}

func (m *MetaConnector) GetMediaEditCache(portal *bridgev2.Portal, messageID networkid.MessageID) *bridgev2.ConvertedMessagePart {
	m.mediaEditCacheLock.RLock()
	defer m.mediaEditCacheLock.RUnlock()
	value, ok := m.mediaEditCache[MediaEditCacheKey{
		MessageID:  messageID,
		PortalMXID: portal.MXID,
	}]
	if !ok || time.Until(value.Expiry) < 0 {
		return nil
	}
	return value.Part
}

func (m *MetaConnector) expireMediaEditCache() {
	m.mediaEditCacheLock.Lock()
	defer m.mediaEditCacheLock.Unlock()
	for key, value := range m.mediaEditCache {
		if time.Until(value.Expiry) < 0 {
			delete(m.mediaEditCache, key)
		}
	}
}

func (m *MetaConnector) getEditedMediaPart(ctx context.Context, portal *bridgev2.Portal, target *database.Message) *bridgev2.ConvertedMessagePart {
	if cached := m.GetMediaEditCache(portal, target.ID); cached != nil {
		return cached
	} else if target.HasFakeMXID() {
		return nil
	}
	evt, err := m.Bridge.Bot.GetEvent(ctx, portal.MXID, target.MXID)
	if err != nil {
		zerolog.Ctx(ctx).Debug().Err(err).Msg("Failed to fetch edit target to check for media")
		return nil
	}
	content := evt.Content.AsMessage()
	if !content.MsgType.IsMedia() {
		return nil
	}
	return &bridgev2.ConvertedMessagePart{
		Type:    evt.Type,
		Content: content,
	}
}

func (m *MetaConnector) mergeEditedCaption(ctx context.Context, portal *bridgev2.Portal, target *database.Message, textPart *bridgev2.ConvertedMessagePart) *bridgev2.ConvertedMessagePart {
	mediaPart := m.getEditedMediaPart(ctx, portal, target)
	if mediaPart == nil {
		return textPart
	}
	mediaPart = ptr.Clone(mediaPart)
	mediaPart.Content = ptr.Clone(mediaPart.Content)
	mediaPart.Content.RelatesTo = nil
	mediaPart.Content.NewContent = nil
	mediaPart.DBMetadata = nil
	if mediaPart.Content.FileName != "" {
		mediaPart.Content.Body = mediaPart.Content.FileName
		mediaPart.Content.Format = ""
		mediaPart.Content.FormattedBody = ""
	}
	return bridgev2.MergeCaption(textPart, mediaPart)
}
