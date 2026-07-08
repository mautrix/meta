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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/mediaproxy"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/responses"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

var _ bridgev2.DirectMediableNetwork = (*IGConnector)(nil)

func (ic *IGConnector) SetUseDirectMedia() {
	ic.MsgConv.DirectMedia = true
}

func (ic *IGConnector) Download(ctx context.Context, mediaID networkid.MediaID, params map[string]string) (mediaproxy.GetMediaResponse, error) {
	mediaInfo, err := metaid.ParseMediaID(mediaID)
	if err != nil {
		return nil, err
	}

	switch mediaInfo.Type {
	case metaid.DirectMediaTypeMetaV1, metaid.DirectMediaTypeMetaV2:
		// ok
	default:
		return nil, fmt.Errorf("unknown media type: %v", mediaInfo.Type)
	}

	var msg *database.Message
	if mediaInfo.PartID == "" {
		msg, err = ic.Bridge.DB.Message.GetFirstPartByID(ctx, mediaInfo.UserID, mediaInfo.MessageID)
	} else {
		msg, err = ic.Bridge.DB.Message.GetPartByID(ctx, mediaInfo.UserID, mediaInfo.MessageID, mediaInfo.PartID)
	}
	if err != nil {
		return nil, err
	} else if msg == nil {
		zerolog.Ctx(ctx).Trace().Any("media_info", mediaInfo).Msg("Message not found in database")
		return nil, fmt.Errorf("message not found")
	}

	dmm := msg.Metadata.(*metaid.MessageMetadata).DirectMediaMeta
	if dmm == nil {
		return nil, fmt.Errorf("message does not have direct media metadata")
	}

	var info *mediadl.DirectMediaMeta
	err = json.Unmarshal(dmm, &info)
	if err != nil {
		return nil, err
	}

	log := zerolog.Ctx(ctx)

	needsRefresh := !info.ExpiresAt.IsZero() && time.Now().After(info.ExpiresAt.Add(-5*time.Minute))
	size, reader, err := mediadl.DownloadMedia(ctx, info.MimeType, info.URL, ic.MsgConv.MaxFileSize)
	if err != nil && (errors.Is(err, mediadl.ErrForbidden) || needsRefresh) {
		log.Debug().
			Bool("needs_refresh", needsRefresh).
			AnErr("original_error", err).
			Msg("Attempting to refresh expired media URL")

		refreshedURL, refreshErr := ic.refreshMediaURL(ctx, mediaInfo, msg, info)
		if refreshErr != nil {
			log.Warn().Err(refreshErr).Msg("Failed to refresh media URL")
			// Return the original error if refresh failed
			return nil, err
		}

		log.Debug().Str("refreshed_url", refreshedURL).Msg("Successfully refreshed media URL")
		size, reader, err = mediadl.DownloadMedia(ctx, info.MimeType, refreshedURL, ic.MsgConv.MaxFileSize)
	}

	if err != nil {
		return nil, err
	}

	return &mediaproxy.GetMediaResponseData{
		Reader:        reader,
		ContentType:   info.MimeType,
		ContentLength: size,
	}, nil
}

// refreshMediaURL routes to the appropriate refresh method based on attachment type
func (ic *IGConnector) refreshMediaURL(
	ctx context.Context,
	mediaInfo *metaid.MediaInfo,
	msg *database.Message,
	info *mediadl.DirectMediaMeta,
) (string, error) {
	ul := ic.Bridge.GetCachedUserLoginByID(mediaInfo.UserID)
	if ul == nil || !ul.Client.IsLoggedIn() {
		return "", fmt.Errorf("no logged in user found for refresh")
	}
	client := ul.Client.(*IGClient)

	// Route to appropriate refresh method based on attachment type
	if info.XMATargetID != 0 || info.XMAShortcode != "" || info.StoryMediaID != "" {
		return ic.refreshXMAMedia(ctx, client, info)
	}
	return ic.refreshBlobMedia(ctx, client, msg, info)
}

// refreshBlobMedia re-fetches messages to get fresh attachment URLs.
// It updates all fetched messages in the database, not just the one we need.
func (ic *IGConnector) refreshBlobMedia(
	ctx context.Context,
	client *IGClient,
	msg *database.Message,
	info *mediadl.DirectMediaMeta,
) (string, error) {
	log := zerolog.Ctx(ctx)

	// Parse thread key from portal ID
	threadKey := metaid.ParseFBPortalID(msg.Room.ID)
	if threadKey == 0 {
		return "", fmt.Errorf("invalid thread key")
	}
	portal, err := ic.Bridge.GetExistingPortalByKey(ctx, msg.Room)
	if err != nil {
		return "", fmt.Errorf("failed to get portal: %w", err)
	} else if portal == nil {
		return "", fmt.Errorf("portal not found for room")
	}
	meta, err := client.ensureIGID(ctx, portal)
	if err != nil {
		return "", err
	}

	// Parse message ID for the target message
	parsedMsgID := metaid.ParseMessageID(msg.ID)
	fbMsgID, ok := parsedMsgID.(metaid.ParsedFBMessageID)
	if !ok {
		return "", fmt.Errorf("not a FB message ID: %s", msg.ID)
	}

	nextMsg, err := ic.Bridge.DB.Message.GetFirstNonFakePartAfterTime(ctx, msg.Room, msg.Timestamp.Add(1*time.Second))
	if err != nil {
		return "", fmt.Errorf("failed to get first message after time: %w", err)
	}
	var messages *slidetypes.SlideMessages
	if nextMsg != nil {
		nextMsgID, ok := metaid.ParseMessageID(nextMsg.ID).(metaid.ParsedFBMessageID)
		if !ok {
			return "", fmt.Errorf("next message is not a FB message ID: %s", nextMsg.ID)
		}
		log.Debug().
			Int64("thread_key", threadKey).
			Str("message_id", fbMsgID.ID).
			Str("attachment_fbid", info.AttachmentFBID).
			Int("part_index", info.PartIndex).
			Str("before_message_id", nextMsgID.ID).
			Msg("Refreshing blob media by re-fetching messages before next message")
		resp, err := client.Client.PaginateMessages(ctx, &slidetypes.PaginateMessagesRequest{
			FirstN:                  20,
			OlderThanMessageID:      &nextMsgID.ID,
			ThreadID:                meta.IGID,
			InitialMessagePageCount: 20,
		})
		if err != nil {
			return "", fmt.Errorf("failed to paginate messages: %w", err)
		}
		messages = resp.ThreadInfo.AsIGDirectThread.Messages
	} else {
		log.Debug().
			Int64("thread_key", threadKey).
			Str("message_id", fbMsgID.ID).
			Str("attachment_fbid", info.AttachmentFBID).
			Int("part_index", info.PartIndex).
			Msg("Refreshing blob media by re-fetching recent thread messages")
		resp, err := client.Client.GetThread(ctx, slidetypes.MakeGetThreadInfoRequest(meta.IGID))
		if err != nil {
			return "", fmt.Errorf("failed to get thread: %w", err)
		}
		messages = resp.ThreadInfo.AsIGDirectThread.SlideMessages
	}

	var resultURL string
	var updatedCount int

	// Process ALL fetched messages, updating their URLs in the database
	for _, edge := range messages.Edges {
		// Build attachment URL map: AttachmentFbid -> fresh URL
		attachmentURLs := make(map[string]string)
		switch content := edge.Node.Content.Content.(type) {
		case *slidetypes.MessageContentImage:
			for _, att := range content.Attachments {
				attachmentURLs[att.AttachmentFBID] = cmp.Or(att.AttachmentCDNURL, att.PreviewCDNURL)
			}
		case *slidetypes.MessageContentVideo:
			for _, att := range content.Videos {
				attachmentURLs[att.AttachmentFBID] = cmp.Or(att.AttachmentCDNURL, att.PreviewCDNURL)
			}
		case *slidetypes.MessageContentAudio:
			for _, att := range content.AudioAttachments {
				attachmentURLs[att.AttachmentFBID] = att.AttachmentCDNURL
			}
		case *slidetypes.MessageContentMultiMedia:
			for _, att := range content.Attachments {
				attachmentURLs[att.AttachmentFBID] = cmp.Or(att.AttachmentCDNURL, att.PreviewCDNURL)
			}
		case *slidetypes.MessageContentAnimatedMedia:
			for i, att := range content.AnimatedMedia {
				if att.AttachmentWebpURL != "" && (att.IsSticker || att.AttachmentMP4URL == "") {
					attachmentURLs[strconv.Itoa(i)] = att.AttachmentWebpURL
				} else if att.AttachmentMP4URL != "" {
					attachmentURLs[strconv.Itoa(i)] = att.AttachmentMP4URL
				} else if att.PreviewCDNURL != "" {
					attachmentURLs[strconv.Itoa(i)] = att.PreviewCDNURL
				}
			}
		case *slidetypes.MessageContentRavenImage:
			if att := content.Attachment; att != nil {
				attachmentURLs[att.AttachmentFBID] = cmp.Or(att.AttachmentCDNURL, att.PreviewCDNURL)
			}
		case *slidetypes.MessageContentRavenVideo:
			if att := content.Attachment; att != nil {
				attachmentURLs[att.AttachmentFBID] = cmp.Or(att.AttachmentCDNURL, att.PreviewCDNURL)
			}
		case *slidetypes.MessageContentSticker:
			attachmentURLs["0"] = content.PreviewURL
		case *slidetypes.MessageContentMusicSticker:
			attachmentURLs["0"] = content.AudioTrack.Web30SPreviewDownloadURL
		case *slidetypes.MessageContentXMA:
			if att := cmp.Or(content.XMA.PreviewImage, content.XMA.XMAPreviewImage); att != nil {
				attachmentURLs["0"] = att.URL
			}
		default:
			continue
		}

		if len(attachmentURLs) == 0 {
			continue
		}

		// Look up all message parts in the database for this message
		networkMsgID := metaid.MakeFBMessageID(edge.Node.ID)
		dbMessages, err := ic.Bridge.DB.Message.GetAllPartsByID(ctx, msg.Room.Receiver, networkMsgID)
		if err != nil {
			log.Warn().Err(err).Str("message_id", edge.Node.ID).Msg("Failed to get message parts from database")
			continue
		}

		for _, dbMsg := range dbMessages {
			meta, ok := dbMsg.Metadata.(*metaid.MessageMetadata)
			if !ok || meta.DirectMediaMeta == nil {
				continue
			}

			var dmm mediadl.DirectMediaMeta
			if err := json.Unmarshal(meta.DirectMediaMeta, &dmm); err != nil {
				continue
			}
			// Don't update XMA attachments here (until thumbnailing support is added)
			if dmm.XMATargetID != 0 || dmm.StoryMediaID != "" {
				continue
			}

			freshURL := attachmentURLs[dmm.AttachmentFBID]
			if freshURL == "" {
				freshURL = attachmentURLs[strconv.Itoa(dmm.PartIndex)]
			}
			if freshURL == "" {
				continue
			}

			// Update the stored URL and expiry
			dmm.URL = freshURL
			dmm.ExpiresAt = jsontime.UM(mediadl.ParseExpiryFromURL(freshURL))

			updatedMeta, err := json.Marshal(dmm)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to marshal updated DirectMediaMeta")
				continue
			}

			meta.DirectMediaMeta = updatedMeta
			if err := ic.Bridge.DB.Message.Update(ctx, dbMsg); err != nil {
				log.Warn().Err(err).Str("message_id", string(dbMsg.ID)).Msg("Failed to update message in database")
				continue
			}

			updatedCount++
			log.Debug().
				Str("message_id", edge.Node.ID).
				Str("attachment_fbid", dmm.AttachmentFBID).
				Int("part_index", dmm.PartIndex).
				Msg("Updated attachment URL in database")

			// Check if this is the attachment we're looking for
			if edge.Node.ID == fbMsgID.ID && (dmm.AttachmentFBID == info.AttachmentFBID || dmm.PartIndex == info.PartIndex) {
				resultURL = freshURL
			}
		}
	}

	log.Debug().Int("updated_count", updatedCount).Msg("Finished refreshing blob media URLs")

	if resultURL != "" {
		return resultURL, nil
	}

	return "", fmt.Errorf("target attachment not found in re-fetched messages")
}

// refreshXMAMedia fetches fresh URLs using Instagram API for XMA attachments
func (ic *IGConnector) refreshXMAMedia(
	ctx context.Context,
	client *IGClient,
	info *mediadl.DirectMediaMeta,
) (string, error) {
	ig := client.Client
	if ig == nil {
		return "", fmt.Errorf("instagram client not available")
	}

	log := zerolog.Ctx(ctx)

	// Handle story refresh (type 1: has both StoryMediaID and StoryReelID)
	if info.StoryMediaID != "" && info.StoryReelID != "" {
		log.Debug().
			Str("story_media_id", info.StoryMediaID).
			Str("story_reel_id", info.StoryReelID).
			Msg("Refreshing story media (type 1)")
		resp, err := ig.FetchReel(ctx, []string{info.StoryReelID}, info.StoryMediaID)
		if err != nil {
			return "", fmt.Errorf("failed to fetch reel: %w", err)
		}
		reel, ok := resp.Reels[info.StoryReelID]
		if !ok || len(reel.Items) == 0 {
			return "", fmt.Errorf("reel not found in response")
		}
		// Find the item matching the media ID
		for _, item := range reel.Items {
			if item.Pk == info.StoryMediaID {
				return extractBestURL(&item.Items)
			}
		}
		// If exact match not found, use first item
		return extractBestURL(&reel.Items[0].Items)
	}

	// Handle story refresh (type 2: has StoryMediaID but not StoryReelID)
	if info.StoryMediaID != "" {
		log.Debug().
			Str("story_media_id", info.StoryMediaID).
			Msg("Refreshing story media (type 2)")
		resp, err := ig.FetchMedia(ctx, info.StoryMediaID, "")
		if err != nil {
			return "", fmt.Errorf("failed to fetch media: %w", err)
		}
		if len(resp.Items) == 0 {
			return "", fmt.Errorf("empty response from FetchMedia")
		}
		return extractBestURL(resp.Items[0])
	}

	// Handle post/reel refresh using TargetId and/or Shortcode
	if info.XMATargetID != 0 || info.XMAShortcode != "" {
		log.Debug().
			Int64("target_id", info.XMATargetID).
			Str("shortcode", info.XMAShortcode).
			Msg("Refreshing XMA media")
		resp, err := ig.FetchMedia(ctx, strconv.FormatInt(info.XMATargetID, 10), info.XMAShortcode)
		if err != nil {
			return "", fmt.Errorf("failed to fetch media: %w", err)
		}
		if len(resp.Items) == 0 {
			return "", fmt.Errorf("empty response from FetchMedia")
		}
		return extractBestURL(resp.Items[0])
	}

	return "", fmt.Errorf("no XMA identifiers available for refresh")
}

func extractBestURL(item *responses.Items) (string, error) {
	var bestURL string
	var bestRes int

	// Find the highest resolution video, if any
	for _, ver := range item.VideoVersions {
		res := ver.Width * ver.Height
		if res > bestRes {
			bestURL = ver.URL
			bestRes = res
		}
	}

	// Find the highest resolution image if no video
	if bestURL == "" {
		for _, ver := range item.ImageVersions2.Candidates {
			res := ver.Width * ver.Height
			if res > bestRes {
				bestURL = ver.URL
				bestRes = res
			}
		}
	}

	if bestURL == "" {
		return "", fmt.Errorf("no URL found in response")
	}
	return bestURL, nil
}
