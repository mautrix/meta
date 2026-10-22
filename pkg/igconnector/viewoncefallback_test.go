package igconnector

import (
	"encoding/json"
	"testing"
	"time"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
)

const testDirectInbox = `{
	"status": "ok",
	"inbox": {"threads": [{
		"thread_id": "340282366841710300000000000000000001",
		"thread_v2_id": "17840000000000001",
		"users": [{"pk": 1234, "interop_messaging_user_fbid": 100001}],
		"items": [
			{
				"item_id": "1", "message_id": "mid.$image", "item_type": "raven_media",
				"user_id": 1234, "timestamp": 1792053266326758, "is_sent_by_viewer": false,
				"visual_media": {"view_mode": "once", "seen_count": 0, "media": {
					"id": "555", "media_type": 1, "original_width": 1278, "original_height": 2272,
					"image_versions2": {"candidates": [
						{"url": "https://example.com/small.jpg", "width": 640, "height": 1138},
						{"url": "https://example.com/full.jpg", "width": 1278, "height": 2272}
					]}
				}}
			},
			{
				"item_id": "2", "message_id": "mid.$video", "item_type": "raven_media",
				"user_id": "1234", "timestamp": 1792053266326758, "is_sent_by_viewer": false,
				"visual_media": {"view_mode": "replayable", "media": {
					"id": "556", "media_type": 2,
					"image_versions2": {"candidates": [{"url": "https://example.com/thumb.jpg", "width": 100, "height": 100}]},
					"video_versions": [{"url": "https://example.com/video.mp4", "width": 720, "height": 1280}]
				}}
			},
			{
				"item_id": "3", "message_id": "mid.$seen", "item_type": "raven_media",
				"user_id": 1234, "timestamp": 1792053266326758,
				"visual_media": {"view_mode": "once", "seen_count": 1, "media": {"media_type": 1}}
			}
		]
	}]}
}`

func TestMakeViewOnceFallbackMessage(t *testing.T) {
	var resp instameow.DirectInboxResponse
	if err := json.Unmarshal([]byte(testDirectInbox), &resp); err != nil {
		t.Fatal(err)
	}
	thread := resp.Inbox.Threads[0]
	ts := time.UnixMicro(1792053266326758)

	img := makeViewOnceFallbackMessage(thread, thread.Items[0], ts)
	if img == nil {
		t.Fatal("expected image message")
	}
	imgContent, ok := img.Content.Content.(*slidetypes.MessageContentRavenImage)
	if !ok {
		t.Fatalf("unexpected content type %T", img.Content.Content)
	}
	if img.ID != "mid.$image" || img.SenderFBID != 100001 || img.ThreadFBID != "17840000000000001" {
		t.Errorf("unexpected message fields: %+v", img)
	}
	if imgContent.Attachment.AttachmentCDNURL != "https://example.com/full.jpg" || imgContent.ViewMode != slidetypes.RavenViewModeViewOnce {
		t.Errorf("unexpected image content: %+v", imgContent.Attachment)
	}

	vid := makeViewOnceFallbackMessage(thread, thread.Items[1], ts)
	vidContent, ok := vid.Content.Content.(*slidetypes.MessageContentRavenVideo)
	if !ok {
		t.Fatalf("unexpected content type %T", vid.Content.Content)
	}
	if vidContent.Attachment.AttachmentCDNURL != "https://example.com/video.mp4" || vidContent.ViewMode != slidetypes.RavenViewModeViewTwice {
		t.Errorf("unexpected video content: %+v", vidContent.Attachment)
	}

	if makeViewOnceFallbackMessage(thread, thread.Items[2], ts) != nil {
		t.Error("expected nil for item without media URLs")
	}
}
