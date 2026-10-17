// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package igconnector

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

// In some regions (at least the EU), Instagram doesn't deliver view-once media over the slide socket. The message
// either arrives without an attachment, or the socket only sends a SlideUQPPNotYetImplementedMutation without a
// thread ID. When the login has a native Android session, the mobile direct_v2 inbox does still include the media,
// so fetch it from there.

const (
	viewOnceFallbackDelay       = 3 * time.Second
	viewOnceFallbackMaxAge      = 24 * time.Hour
	viewOnceFallbackThreadLimit = 10
	viewOnceFallbackItemLimit   = 5
)

func (ic *IGClient) queueViewOnceFallback() {
	if !ic.Main.Config.ViewOnceFallback || ic.Main.Config.DisableViewOnce ||
		ic.Client == nil || ic.Client.GetInstagramNativeSession() == nil {
		return
	}
	if !ic.viewOnceFallbackPending.CompareAndSwap(false, true) {
		return
	}
	go func() {
		// Wait a bit so that bursts of mutations only cause one fetch
		time.Sleep(viewOnceFallbackDelay)
		ic.viewOnceFallbackPending.Store(false)
		log := ic.UserLogin.Log.With().Str("action", "view-once fallback").Logger()
		ic.fetchViewOnceFallback(log.WithContext(ic.Main.Bridge.BackgroundCtx))
	}()
}

func (ic *IGClient) fetchViewOnceFallback(ctx context.Context) {
	ic.viewOnceFallbackLock.Lock()
	defer ic.viewOnceFallbackLock.Unlock()
	log := zerolog.Ctx(ctx)
	resp, err := ic.Client.FetchDirectInbox(ctx, viewOnceFallbackThreadLimit, viewOnceFallbackItemLimit)
	if err != nil {
		log.Err(err).Msg("Failed to fetch direct inbox")
		return
	}
	minTS := time.Now().Add(-viewOnceFallbackMaxAge)
	for _, thread := range resp.Inbox.Threads {
		if thread == nil || thread.ThreadV2ID == "" {
			continue
		}
		for _, item := range thread.Items {
			if item == nil || item.ItemType != instameow.DirectItemTypeRavenMedia || item.IsSentByViewer ||
				item.MessageID == "" || item.VisualMedia == nil || item.VisualMedia.Media == nil {
				continue
			}
			tsUS, _ := item.TimestampUS.Int64()
			ts := time.UnixMicro(tsUS)
			if ts.Before(minTS) {
				continue
			}
			ic.bridgeViewOnceFallbackItem(ctx, thread, item, ts)
		}
	}
}

func (ic *IGClient) bridgeViewOnceFallbackItem(ctx context.Context, thread *instameow.DirectThread, item *instameow.DirectItem, ts time.Time) {
	log := zerolog.Ctx(ctx).With().
		Str("thread_igid", thread.ThreadV2ID).
		Str("message_id", item.MessageID).
		Logger()
	msg := makeViewOnceFallbackMessage(thread, item, ts)
	if msg == nil {
		log.Debug().Msg("View-once item has no usable media")
		return
	}
	portalKey, _, err := ic.ensurePortal(ctx, thread.ThreadV2ID, true, "view-once fallback")
	if err != nil {
		log.Err(err).Msg("Failed to ensure portal for view-once item")
		return
	} else if portalKey.IsEmpty() {
		log.Warn().Msg("No portal found for view-once item")
		return
	}
	msgID := metaid.MakeFBMessageID(msg.ID)
	existing, err := ic.Main.Bridge.DB.Message.GetFirstPartByID(ctx, portalKey.Receiver, msgID)
	if err != nil {
		log.Err(err).Msg("Failed to check if view-once item is already bridged")
		return
	} else if existing != nil {
		return
	}
	log.Info().Msg("Bridging view-once item from direct inbox")
	res := ic.handleMessage(portalKey, msg)
	if !res.Success {
		log.Err(res.Error).Msg("Failed to queue view-once item")
	}
}

// fillViewOnceFromDirectInbox fills in the media of a view-once message that was delivered over the socket
// without an attachment, which is what Instagram does in some regions.
func (ic *IGClient) fillViewOnceFromDirectInbox(ctx context.Context, msg *slidetypes.Message) {
	if !ic.Main.Config.ViewOnceFallback || ic.Main.Config.DisableViewOnce ||
		ic.Client == nil || ic.Client.GetInstagramNativeSession() == nil || msg == nil || msg.ID == "" {
		return
	}
	switch content := msg.Content.Content.(type) {
	case *slidetypes.MessageContentRavenImage:
		if content.Attachment != nil {
			return
		}
	case *slidetypes.MessageContentRavenVideo:
		if content.Attachment != nil {
			return
		}
	default:
		return
	}
	log := zerolog.Ctx(ctx).With().Str("action", "view-once fallback").Str("message_id", msg.ID).Logger()
	ic.viewOnceFallbackLock.Lock()
	defer ic.viewOnceFallbackLock.Unlock()
	resp, err := ic.Client.FetchDirectInbox(ctx, viewOnceFallbackThreadLimit, viewOnceFallbackItemLimit)
	if err != nil {
		log.Err(err).Msg("Failed to fetch direct inbox")
		return
	}
	for _, thread := range resp.Inbox.Threads {
		if thread == nil {
			continue
		}
		for _, item := range thread.Items {
			if item == nil || item.MessageID != msg.ID || item.VisualMedia == nil || item.VisualMedia.Media == nil {
				continue
			}
			filled := makeViewOnceFallbackMessage(thread, item, msg.TimestampMS.Time)
			if filled == nil {
				log.Debug().Msg("View-once item in direct inbox has no usable media")
				return
			}
			log.Info().Msg("Filled view-once media from direct inbox")
			msg.Content.Content = filled.Content.Content
			return
		}
	}
	log.Warn().Msg("View-once item not found in direct inbox")
}

func isViewOnceMedia(msg *slidetypes.Message) bool {
	switch content := msg.Content.Content.(type) {
	case *slidetypes.MessageContentRavenImage:
		return content.Attachment != nil && content.ViewMode.ViewType() != ""
	case *slidetypes.MessageContentRavenVideo:
		return content.Attachment != nil && content.ViewMode.ViewType() != ""
	default:
		return false
	}
}

func makeViewOnceFallbackMessage(thread *instameow.DirectThread, item *instameow.DirectItem, ts time.Time) *slidetypes.Message {
	media := item.VisualMedia.Media
	var senderFBID int64
	for _, user := range thread.Users {
		if user != nil && user.PK == item.UserID {
			senderFBID = user.InteropMessagingUserFBID
			break
		}
	}
	if senderFBID == 0 && media.User != nil && media.User.PK == item.UserID {
		senderFBID = media.User.InteropMessagingUserFBID
	}
	if senderFBID == 0 {
		return nil
	}
	att := &slidetypes.Attachment{
		AttachmentFBID: media.ID,
		PreviewWidth:   media.OriginalWidth,
		PreviewHeight:  media.OriginalHeight,
	}
	var content slidetypes.MessageContent
	switch media.MediaType {
	case instameow.DirectMediaTypeImage:
		best := instameow.BestVersion(media.ImageVersions2.Candidates)
		if best == nil {
			return nil
		}
		att.AttachmentCDNURL = best.URL
		content = &slidetypes.MessageContentRavenImage{ViewMode: directViewMode(item.VisualMedia.ViewMode), Attachment: att}
	case instameow.DirectMediaTypeVideo:
		best := instameow.BestVersion(media.VideoVersions)
		if best == nil {
			return nil
		}
		att.AttachmentCDNURL = best.URL
		if preview := instameow.BestVersion(media.ImageVersions2.Candidates); preview != nil {
			att.PreviewCDNURL = preview.URL
		}
		content = &slidetypes.MessageContentRavenVideo{ViewMode: directViewMode(item.VisualMedia.ViewMode), Attachment: att}
	default:
		return nil
	}
	return &slidetypes.Message{
		ID:          item.MessageID,
		MessageID:   item.MessageID,
		SenderFBID:  senderFBID,
		ThreadFBID:  thread.ThreadV2ID,
		TimestampMS: jsontime.UnixMilliString{Time: ts},
		Content:     slidetypes.MessageContentWrapper{Content: content},
	}
}

func directViewMode(mode string) slidetypes.RavenViewMode {
	switch mode {
	case "once":
		return slidetypes.RavenViewModeViewOnce
	case "replayable":
		return slidetypes.RavenViewModeViewTwice
	default:
		return slidetypes.RavenViewModeKeepInChat
	}
}

// labelViewOnceParts adds a caption to bridged media so that it's clear the original was view-once.
func labelViewOnceParts(cm *bridgev2.ConvertedMessage) {
	for _, part := range cm.Parts {
		if part.Content == nil {
			continue
		}
		var label string
		switch part.Content.MsgType {
		case event.MsgImage:
			label = "View-once photo"
		case event.MsgVideo:
			label = "View-once video"
		default:
			continue
		}
		if part.Content.FileName == "" {
			part.Content.FileName = part.Content.Body
		}
		part.Content.Body = label
		part.Content.Format = ""
		part.Content.FormattedBody = ""
	}
}
