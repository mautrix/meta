package httpclient

import (
	"cmp"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/util/random"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

type RUploadMedia struct {
	Filename  string
	MimeType  string
	MediaData []byte

	IsVoiceClip bool
	DurationMS  int
}

func ruploadNamespace(media *RUploadMedia) string {
	switch {
	case media.MimeType == "image/gif":
		return "messenger_gif"
	case strings.HasPrefix(media.MimeType, "image/"):
		return "messenger_image"
	case strings.HasPrefix(media.MimeType, "video/"):
		return "messenger_video"
	case strings.HasPrefix(media.MimeType, "audio/"):
		return "messenger_audio"
	default:
		return "messenger_file"
	}
}

func (c *HTTPClient) SendRUploadRequest(ctx context.Context, session *types.NativeSession, media *RUploadMedia) (int64, error) {
	if c == nil {
		return 0, fmt.Errorf("client is nil")
	} else if session == nil || session.AccessToken == "" {
		return 0, fmt.Errorf("no native session")
	}
	namespace := ruploadNamespace(media)
	entityName := fmt.Sprintf("%s-0-%d", hex.EncodeToString(random.Bytes(16)), len(media.MediaData))

	h := http.Header{}
	h.Set("authorization", "OAuth "+session.AccessToken)
	switch session.AppID {
	case useragent.MessengerLiteIOSAppID:
		h.Set("user-agent", useragent.MessengerLiteIOSUserAgent)
		h.Set("x-fb-http-engine", "Tigon/MNS/TCP")
		h.Set("x-fb-device-id", strings.ToUpper(session.DeviceID.String()))
		h.Set("x-fb-family-device-id", strings.ToUpper(session.FamilyDeviceID.String()))
	case useragent.MessengerLiteAndroidAppID:
		h.Set("user-agent", useragent.MessengerLiteAndroidUserAgent)
		h.Set("x-fb-http-engine", "Tigon/Liger")
	default:
		return 0, fmt.Errorf("unsupported native session app %s", session.AppID)
	}
	h.Set("x-fb-friendly-name", "post_resumable_upload_session")
	h.Set("x-fb-client-ip", "True")
	h.Set("x-fb-server-cluster", "True")
	h.Set("accept-language", "en-US")
	h.Set("offset", "0")
	h.Set("x-entity-length", strconv.Itoa(len(media.MediaData)))
	h.Set("x-entity-name", url.QueryEscape(cmp.Or(media.Filename, entityName)))
	h.Set("x-entity-type", media.MimeType)
	h.Set("attempt_id", strconv.FormatUint(binary.BigEndian.Uint64(random.Bytes(8))>>1, 10))
	switch namespace {
	case "messenger_image", "messenger_gif":
		h.Set("image_type", "FILE_ATTACHMENT")
	case "messenger_video":
		h.Set("video_type", "FILE_ATTACHMENT")
	case "messenger_file":
		h.Set("file_type", "FILE_ATTACHMENT")
	case "messenger_audio":
		if media.IsVoiceClip {
			h.Set("audio_type", "VOICE_MESSAGE")
			h.Set("duration", strconv.Itoa(media.DurationMS))
		}
	}

	reqURL := fmt.Sprintf("https://rupload.facebook.com/%s/%s", namespace, entityName)
	_, body, err := c.makeRequest(ctx, c.uploadClient, reqURL, http.MethodPost, h, media.MediaData, "application/octet-stream", func(e *zerolog.Event) *zerolog.Event {
		return e.Str("rupload_namespace", namespace)
	})
	if err != nil {
		var errResp struct {
			DebugInfo struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"debug_info"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.DebugInfo.Message != "" {
			return 0, fmt.Errorf("failed to send rupload request: %w (%s: %s)", err, errResp.DebugInfo.Type, errResp.DebugInfo.Message)
		}
		return 0, fmt.Errorf("failed to send rupload request: %w", err)
	}
	var resp struct {
		MediaID json.Number `json:"media_id"`
	}
	if err = json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("failed to parse rupload response: %w", err)
	}
	mediaID, err := resp.MediaID.Int64()
	if err != nil || mediaID == 0 {
		return 0, fmt.Errorf("no media ID in rupload response")
	}
	return mediaID, nil
}
