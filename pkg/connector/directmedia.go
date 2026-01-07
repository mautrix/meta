package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
		if err != nil && (strings.Contains(err.Error(), "403") || needsRefresh) {
			log.Debug().
				Bool("needs_refresh", needsRefresh).
				Str("original_error", err.Error()).
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
	if info.XMATargetId != 0 || info.XMAShortcode != "" || info.XMAActionUrl != "" {
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

	// Handle story refresh (type 1: /stories/direct/...)
	if info.XMAActionUrl != "" && strings.HasPrefix(info.XMAActionUrl, "/stories/direct/") {
		match := msgconv.ReelActionURLRegex.FindStringSubmatch(info.XMAActionUrl)
		if len(match) == 3 {
			log.Debug().Str("action_url", info.XMAActionUrl).Msg("Refreshing story media (type 1)")
			resp, err := ig.FetchReel(ctx, []string{match[2]}, match[1])
			if err != nil {
				return "", fmt.Errorf("failed to fetch reel: %w", err)
			}
			reel, ok := resp.Reels[match[2]]
			if !ok || len(reel.Items) == 0 {
				return "", fmt.Errorf("reel not found in response")
			}
			// Find the item matching the media ID
			for _, item := range reel.Items {
				if item.Pk == match[1] {
					return extractBestURL(&item.Items, info.MediaType)
				}
			}
			// If exact match not found, use first item
			return extractBestURL(&reel.Items[0].Items, info.MediaType)
		}
	}

	// Handle story refresh (type 2: https://instagram.com/stories/...)
	if info.XMAActionUrl != "" && strings.HasPrefix(info.XMAActionUrl, "https://instagram.com/stories/") {
		match := msgconv.ReelActionURLRegex2.FindStringSubmatch(info.XMAActionUrl)
		if len(match) == 3 {
			log.Debug().Str("action_url", info.XMAActionUrl).Msg("Refreshing story media (type 2)")
			resp, err := ig.FetchMedia(ctx, match[2], "")
			if err != nil {
				return "", fmt.Errorf("failed to fetch media: %w", err)
			}
			if len(resp.Items) == 0 {
				return "", fmt.Errorf("empty response from FetchMedia")
			}
			return extractBestURL(resp.Items[0], info.MediaType)
		}
	}

	// Handle post/reel refresh using TargetId and/or Shortcode
	if info.XMATargetId != 0 || info.XMAShortcode != "" {
		log.Debug().
			Int64("target_id", info.XMATargetId).
			Str("shortcode", info.XMAShortcode).
			Msg("Refreshing XMA media")
		resp, err := ig.FetchMedia(ctx, strconv.FormatInt(info.XMATargetId, 10), info.XMAShortcode)
		if err != nil {
			return "", fmt.Errorf("failed to fetch media: %w", err)
		}
		if len(resp.Items) == 0 {
			return "", fmt.Errorf("empty response from FetchMedia")
		}
		return extractBestURL(resp.Items[0], info.MediaType)
	}

	return "", fmt.Errorf("no XMA identifiers available for refresh")
}

// refreshBlobMedia re-fetches the message to get fresh attachment URLs
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

	// Parse message ID
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
		Msg("Refreshing blob media by re-fetching message")

	// Re-fetch the message
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

	// Find matching message and attachment
	upsertMap, insertMsgs := resp.WrapMessages()

	// Collect all messages to search through
	var allMessages []*table.WrappedMessage
	for _, upsert := range upsertMap {
		allMessages = append(allMessages, upsert.Messages...)
	}
	allMessages = append(allMessages, insertMsgs...)

	for _, wrappedMsg := range allMessages {
		if wrappedMsg.MessageId != fbMsgID.ID {
			continue
		}

		// Try to match by AttachmentFbid first
		if info.AttachmentFbid != "" {
			for _, att := range wrappedMsg.BlobAttachments {
				if att.AttachmentFbid == info.AttachmentFbid {
					url := att.PlayableUrl
					if url == "" {
						url = att.PreviewUrl
					}
					if url != "" {
						log.Debug().Str("matched_by", "attachment_fbid").Msg("Found refreshed URL")
						return url, nil
					}
				}
			}
			// Also check legacy attachments
			for _, att := range wrappedMsg.Attachments {
				if att.AttachmentFbid == info.AttachmentFbid {
					url := att.PlayableUrl
					if url == "" {
						url = att.PreviewUrl
					}
					if url != "" {
						log.Debug().Str("matched_by", "attachment_fbid").Msg("Found refreshed URL")
						return url, nil
					}
				}
			}
		}

		// Fallback: match by part index
		if info.PartIndex >= 0 && info.PartIndex < len(wrappedMsg.BlobAttachments) {
			att := wrappedMsg.BlobAttachments[info.PartIndex]
			url := att.PlayableUrl
			if url == "" {
				url = att.PreviewUrl
			}
			if url != "" {
				log.Debug().Str("matched_by", "part_index").Msg("Found refreshed URL")
				return url, nil
			}
		}

		// Message found but no matching attachment
		return "", fmt.Errorf("attachment not found in re-fetched message")
	}

	return "", fmt.Errorf("message not found in fetch response")
}

// extractBestURL selects the highest resolution URL from Instagram media response
func extractBestURL(item *responses.Items, mediaType string) (string, error) {
	var bestURL string
	var bestRes int

	// Prefer video versions if media type is video or if video versions exist
	if mediaType == "video" || mediaType == "story" || len(item.VideoVersions) > 0 {
		for _, ver := range item.VideoVersions {
			res := ver.Width * ver.Height
			if res > bestRes {
				bestURL = ver.URL
				bestRes = res
			}
		}
	}

	// Fall back to image versions
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
