package socket

import (
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/mautrix-meta/messagix/table"
)

type SendMessageTask struct {
	// If you are forwarding a message, you set the ThreadId to the thread you would like to forward it to
	ThreadId                 int64                  `json:"thread_id"`
	Otid                     int64                  `json:"otid,string"`
	Source                   table.ThreadSourceType `json:"source"`
	SendType                 table.SendType         `json:"send_type"`
	AttachmentFBIds          []int64                `json:"attachment_fbids,omitempty"`
	SyncGroup                int64                  `json:"sync_group"`
	ReplyMetaData            *ReplyMetaData         `json:"reply_metadata,omitempty"`
	MentionData              *MentionData           `json:"mention_data,omitempty"`
	Text                     interface{}            `json:"text"`
	HotEmojiSize             int32                  `json:"hot_emoji_size,omitempty"`
	StickerId                int64                  `json:"sticker_id,omitempty"`
	InitiatingSource         table.InitiatingSource `json:"initiating_source,omitempty"`           // usually FACEBOOK_INBOX
	SkipUrlPreviewGen        int32                  `json:"skip_url_preview_gen"`                  // 0 or 1
	TextHasLinks             int32                  `json:"text_has_links"`                        // 0 or 1
	StripForwardedMsgCaption int32                  `json:"strip_forwarded_msg_caption,omitempty"` // 0 or 1
	ForwardedMsgId           string                 `json:"forwarded_msg_id,omitempty"`
	MultiTabEnv              int32                  `json:"multitab_env,omitempty"` // 0 ?
	// url to external media
	// for example:
	//
	// https://media2.giphy.com/media/fItgT774J3nWw/giphy.gif?cid=999aceaclonctzck6x9rte211fb3l24m2poepsdchan17ryd&ep=v1_gifs_trending&rid=giphy.gif&ct=g
	Url string `json:"url,omitempty"`
	// attribution app id, returned in the graphql query CometAnimatedImagePickerSearchResultsRootQuery
	AttributionAppId int64 `json:"attribution_app_id,omitempty"`
}

type ReplyMetaData struct {
	ReplyMessageId  string `json:"reply_source_id"`
	ReplySourceType int64  `json:"reply_source_type"` // 1 ?
	ReplyType       int64  `json:"reply_type"`        // ?
}

type MentionData struct {
	// All fields here are comma-separated lists
	MentionIDs     string `json:"mention_ids"`
	MentionOffsets string `json:"mention_offsets"`
	MentionLengths string `json:"mention_lengths"`
	MentionTypes   string `json:"mention_types"`
}

func (md *MentionData) Parse() (Mentions, error) {
	mentionIDs := strings.Split(md.MentionIDs, ",")
	mentionOffsets := strings.Split(md.MentionOffsets, ",")
	mentionLengths := strings.Split(md.MentionLengths, ",")
	mentionTypes := strings.Split(md.MentionTypes, ",")
	if len(mentionIDs) != len(mentionOffsets) || len(mentionOffsets) != len(mentionLengths) || len(mentionLengths) != len(mentionTypes) {
		return nil, fmt.Errorf("mismatching mention data lengths: %d, %d, %d, %d", len(mentionIDs), len(mentionOffsets), len(mentionLengths), len(mentionTypes))
	}
	mentions := make(Mentions, len(mentionIDs))
	for i := range mentionIDs {
		userID, err := strconv.ParseInt(mentionIDs[i], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse mention #%d user ID: %w", i+1, err)
		}
		offset, err := strconv.Atoi(mentionOffsets[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse mention #%d offset: %w", i+1, err)
		}
		length, err := strconv.Atoi(mentionLengths[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse mention #%d length: %w", i+1, err)
		}
		mentions[i] = Mention{
			UserID: userID,
			Offset: offset,
			Length: length,
			Type:   MentionType(mentionTypes[i]),
		}
	}
	return mentions, nil
}

type MentionType string

const (
	MentionTypePerson MentionType = "p"
	MentionTypeSilent MentionType = "s"
	MentionTypeThread MentionType = "t"
)

type Mention struct {
	UserID int64
	Offset int
	Length int
	Type   MentionType
}

type Mentions []Mention

func (m Mentions) ToData() MentionData {
	mentionIDs := make([]string, len(m))
	mentionOffsets := make([]string, len(m))
	mentionLengths := make([]string, len(m))
	mentionTypes := make([]string, len(m))
	for i, mention := range m {
		mentionIDs[i] = strconv.FormatInt(mention.UserID, 10)
		mentionOffsets[i] = strconv.Itoa(mention.Offset)
		mentionLengths[i] = strconv.Itoa(mention.Length)
		mentionTypes[i] = string(mention.Type)
	}
	return MentionData{
		MentionIDs:     strings.Join(mentionIDs, ","),
		MentionOffsets: strings.Join(mentionOffsets, ","),
		MentionLengths: strings.Join(mentionLengths, ","),
		MentionTypes:   strings.Join(mentionTypes, ","),
	}
}

func (t *SendMessageTask) GetLabel() string {
	return TaskLabels["SendMessageTask"]
}

func (t *SendMessageTask) Create() (interface{}, interface{}, bool) {
	queueName := strconv.Itoa(int(t.ThreadId))
	return t, queueName, false
}

type ThreadMarkReadTask struct {
	ThreadId            int64 `json:"thread_id"`
	LastReadWatermarkTs int64 `json:"last_read_watermark_ts"`
	SyncGroup           int64 `json:"sync_group"`
}

func (t *ThreadMarkReadTask) GetLabel() string {
	return TaskLabels["ThreadMarkRead"]
}

func (t *ThreadMarkReadTask) Create() (interface{}, interface{}, bool) {
	queueName := strconv.Itoa(int(t.ThreadId))
	return t, queueName, false
}

type FetchMessagesTask struct {
	ThreadKey            int64  `json:"thread_key"`
	Direction            int64  `json:"direction"` // 0
	ReferenceTimestampMs int64  `json:"reference_timestamp_ms"`
	ReferenceMessageId   string `json:"reference_message_id"`
	SyncGroup            int64  `json:"sync_group"` // 1
	Cursor               string `json:"cursor"`
}

func (t *FetchMessagesTask) GetLabel() string {
	return TaskLabels["FetchMessagesTask"]
}

func (t *FetchMessagesTask) Create() (interface{}, interface{}, bool) {
	threadStr := strconv.Itoa(int(t.ThreadKey))
	queueName := "mrq." + threadStr
	return t, queueName, false
}

type MuteThreadTask struct {
	ThreadKey        int64 `json:"thread_key"`
	MailboxType      int64 `json:"mailbox_type"` // 0
	MuteExpireTimeMS int64 `json:"mute_expire_time_ms"`
	SyncGroup        int64 `json:"sync_group"` // 1
}

type UpdateThreadTask struct {
	ThreadKey  int64  `json:"thread_key"`
	ThreadName string `json:"thread_name"`
	SyncGroup  int64  `json:"sync_group"` // 1
}
