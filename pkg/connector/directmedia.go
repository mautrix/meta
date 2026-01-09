package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/mediaproxy"

	"go.mau.fi/mautrix-meta/pkg/messagix/data/responses"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/mautrix-meta/pkg/msgconv"
)

var _ bridgev2.DirectMediableNetwork = (*MetaConnector)(nil)

func (m *MetaConnector) SetUseDirectMedia() {
	m.MsgConv.DirectMedia = true
}

func (m *MetaConnector) Download(ctx context.Context, mediaID networkid.MediaID, params map[string]string) (mediaproxy.GetMediaResponse, error) {
	mediaInfo, err := metaid.ParseMediaID(mediaID)
	if err != nil {
		return nil, err
	}
	zerolog.Ctx(ctx).Trace().Any("mediaInfo", mediaInfo).Any("err", err).Msg("download direct media")

	var msg *database.Message
	if mediaInfo.PartID == "" {
		msg, err = m.Bridge.DB.Message.GetFirstPartByID(ctx, mediaInfo.UserID, mediaInfo.MessageID)
	} else {
		msg, err = m.Bridge.DB.Message.GetPartByID(ctx, mediaInfo.UserID, mediaInfo.MessageID, mediaInfo.PartID)
	}
	if err != nil {
		return nil, nil
	} else if msg == nil {
		return nil, fmt.Errorf("message not found")
	}

	dmm := msg.Metadata.(*metaid.MessageMetadata).DirectMediaMeta
	if dmm == nil {
		return nil, fmt.Errorf("message does not have direct media metadata")
	}

	switch mediaInfo.Type {
	case metaid.DirectMediaTypeMetaV1, metaid.DirectMediaTypeMetaV2:
		var info *msgconv.DirectMediaMeta
		err = json.Unmarshal(dmm, &info)
		if err != nil {
			return nil, err
		}

		log := zerolog.Ctx(ctx)
		url := info.URL

		// Check if URL is expired or about to expire (5 minute buffer)
		needsRefresh := info.ExpiresAt > 0 && time.Now().UnixMilli() > info.ExpiresAt-5*60*1000

		// Try download first
		size, reader, err := msgconv.DownloadMedia(ctx, info.MimeType, url, m.MsgConv.MaxFileSize)

		// If download failed with 403 or we know URL is expired, try to refresh
		if err != nil && (errors.Is(err, msgconv.ErrForbidden) || needsRefresh) {
			log.Debug().
				Bool("needs_refresh", needsRefresh).
				AnErr("original_error", err).
				Msg("Attempting to refresh expired media URL")

			refreshedURL, refreshErr := m.refreshMediaURL(ctx, mediaInfo, msg, info)
			if refreshErr != nil {
				log.Warn().Err(refreshErr).Msg("Failed to refresh media URL")
				// Return the original error if refresh failed
				return nil, err
			}

			log.Debug().Str("refreshed_url", refreshedURL).Msg("Successfully refreshed media URL")
			size, reader, err = msgconv.DownloadMedia(ctx, info.MimeType, refreshedURL, m.MsgConv.MaxFileSize)
		}

		if err != nil {
			return nil, err
		}

		return &mediaproxy.GetMediaResponseData{
			Reader:        reader,
			ContentType:   info.MimeType,
			ContentLength: size,
		}, nil
	case metaid.DirectMediaTypeWhatsAppV1, metaid.DirectMediaTypeWhatsAppV2:
		var info *msgconv.DirectMediaWhatsApp
		err = json.Unmarshal(dmm, &info)
		if err != nil {
			return nil, err
		}

		ul := m.Bridge.GetCachedUserLoginByID(mediaInfo.UserID)
		if ul == nil || !ul.Client.IsLoggedIn() {
			return nil, fmt.Errorf("no logged in user found")
		}
		client := ul.Client.(*MetaClient)
		return &mediaproxy.GetMediaResponseFile{
			Callback: func(f *os.File) (*mediaproxy.FileMeta, error) {
				return &mediaproxy.FileMeta{}, client.E2EEClient.DownloadToFile(ctx, info, f)
			},
		}, nil
	}

	return nil, nil
}

// refreshMediaURL routes to the appropriate refresh method based on attachment type
func (m *MetaConnector) refreshMediaURL(
	ctx context.Context,
	mediaInfo *metaid.MediaInfo,
	msg *database.Message,
	info *msgconv.DirectMediaMeta,
) (string, error) {
	ul := m.Bridge.GetCachedUserLoginByID(mediaInfo.UserID)
	if ul == nil || !ul.Client.IsLoggedIn() {
		return "", fmt.Errorf("no logged in user found for refresh")
	}
	client := ul.Client.(*MetaClient)

	// Route to appropriate refresh method based on attachment type
	if info.XMATargetID != 0 || info.XMAShortcode != "" || info.StoryMediaID != "" {
		return m.refreshXMAMedia(ctx, client, info)
	}
	if info.AttachmentFbid != "" {
		return m.refreshBlobMedia(ctx, client, msg, info)
	}

	return "", fmt.Errorf("no refresh identifiers available")
}

// refreshXMAMedia fetches fresh URLs using Instagram API for XMA attachments
func (m *MetaConnector) refreshXMAMedia(
	ctx context.Context,
	client *MetaClient,
	info *msgconv.DirectMediaMeta,
) (string, error) {
	ig := client.Client.Instagram
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

// refreshBlobMedia re-fetches messages to get fresh attachment URLs.
// It updates all fetched messages in the database, not just the one we need.
func (m *MetaConnector) refreshBlobMedia(
	ctx context.Context,
	client *MetaClient,
	msg *database.Message,
	info *msgconv.DirectMediaMeta,
) (string, error) {
	log := zerolog.Ctx(ctx)

	// Parse thread key from portal ID
	threadKey := metaid.ParseFBPortalID(msg.Room.ID)
	if threadKey == 0 {
		return "", fmt.Errorf("invalid thread key")
	}

	// Parse message ID for the target message
	parsedMsgID := metaid.ParseMessageID(msg.ID)
	fbMsgID, ok := parsedMsgID.(metaid.ParsedFBMessageID)
	if !ok {
		return "", fmt.Errorf("not a FB message ID: %s", msg.ID)
	}

	log.Debug().
		Int64("thread_key", threadKey).
		Str("message_id", fbMsgID.ID).
		Str("attachment_fbid", info.AttachmentFbid).
		Int("part_index", info.PartIndex).
		Msg("Refreshing blob media by re-fetching messages")

	// Re-fetch messages around the target
	resp, err := client.Client.ExecuteTasks(ctx, &socket.FetchMessagesTask{
		ThreadKey:            threadKey,
		Direction:            0,
		ReferenceTimestampMs: msg.Timestamp.UnixMilli(),
		ReferenceMessageId:   fbMsgID.ID,
		SyncGroup:            1,
		Cursor:               client.Client.GetCursor(1),
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch messages: %w", err)
	}

	// Collect all fetched messages
	upsertMap, insertMsgs := resp.WrapMessages()
	var allMessages []*table.WrappedMessage
	for _, upsert := range upsertMap {
		allMessages = append(allMessages, upsert.Messages...)
	}
	allMessages = append(allMessages, insertMsgs...)

	var resultURL string
	var updatedCount int

	// Process ALL fetched messages, updating their URLs in the database
	for _, wrappedMsg := range allMessages {
		// Build attachment URL map: AttachmentFbid -> fresh URL
		attachmentURLs := make(map[string]string)
		attachmentExpiry := make(map[string]int64)

		for _, att := range wrappedMsg.BlobAttachments {
			if att.AttachmentFbid == "" {
				continue
			}
			url := att.PlayableUrl
			expiry := att.PlayableUrlExpirationTimestampMs
			if url == "" {
				url = att.PreviewUrl
				expiry = att.PreviewUrlExpirationTimestampMs
			}
			if url != "" {
				attachmentURLs[att.AttachmentFbid] = url
				attachmentExpiry[att.AttachmentFbid] = expiry
			}
		}

		for _, att := range wrappedMsg.Attachments {
			if att.AttachmentFbid == "" {
				continue
			}
			url := att.PlayableUrl
			expiry := att.PlayableUrlExpirationTimestampMs
			if url == "" {
				url = att.PreviewUrl
				expiry = att.PreviewUrlExpirationTimestampMs
			}
			if url != "" {
				attachmentURLs[att.AttachmentFbid] = url
				attachmentExpiry[att.AttachmentFbid] = expiry
			}
		}

		if len(attachmentURLs) == 0 {
			continue
		}

		// Look up all message parts in the database for this message
		networkMsgID := metaid.MakeFBMessageID(wrappedMsg.MessageId)
		dbMessages, err := m.Bridge.DB.Message.GetAllPartsByID(ctx, msg.Room.Receiver, networkMsgID)
		if err != nil {
			log.Warn().Err(err).Str("message_id", wrappedMsg.MessageId).Msg("Failed to get message parts from database")
			continue
		}

		for _, dbMsg := range dbMessages {
			meta, ok := dbMsg.Metadata.(*metaid.MessageMetadata)
			if !ok || meta.DirectMediaMeta == nil {
				continue
			}

			var dmm msgconv.DirectMediaMeta
			if err := json.Unmarshal(meta.DirectMediaMeta, &dmm); err != nil {
				continue
			}

			// Check if this attachment has a fresh URL
			if dmm.AttachmentFbid == "" {
				continue
			}

			freshURL, hasFreshURL := attachmentURLs[dmm.AttachmentFbid]
			freshExpiry := attachmentExpiry[dmm.AttachmentFbid]

			if !hasFreshURL || freshURL == dmm.URL {
				// No update needed
				continue
			}

			// Update the stored URL and expiry
			dmm.URL = freshURL
			dmm.ExpiresAt = freshExpiry

			updatedMeta, err := json.Marshal(dmm)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to marshal updated DirectMediaMeta")
				continue
			}

			meta.DirectMediaMeta = updatedMeta
			if err := m.Bridge.DB.Message.Update(ctx, dbMsg); err != nil {
				log.Warn().Err(err).Str("message_id", string(dbMsg.ID)).Msg("Failed to update message in database")
				continue
			}

			updatedCount++
			log.Debug().
				Str("message_id", wrappedMsg.MessageId).
				Str("attachment_fbid", dmm.AttachmentFbid).
				Msg("Updated attachment URL in database")

			// Check if this is the attachment we're looking for
			if wrappedMsg.MessageId == fbMsgID.ID && dmm.AttachmentFbid == info.AttachmentFbid {
				resultURL = freshURL
			}
		}

		// Also check by part index for the target message if we haven't found our result
		if wrappedMsg.MessageId == fbMsgID.ID && resultURL == "" {
			if info.PartIndex >= 0 && info.PartIndex < len(wrappedMsg.BlobAttachments) {
				att := wrappedMsg.BlobAttachments[info.PartIndex]
				url := att.PlayableUrl
				if url == "" {
					url = att.PreviewUrl
				}
				if url != "" {
					resultURL = url
				}
			}
		}
	}

	log.Debug().Int("updated_count", updatedCount).Msg("Finished refreshing blob media URLs")

	if resultURL != "" {
		return resultURL, nil
	}

	return "", fmt.Errorf("target attachment not found in re-fetched messages")
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
