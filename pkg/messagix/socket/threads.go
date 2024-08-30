package socket

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
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
	Text                     string                 `json:"text,omitempty"`
	HotEmojiSize             int32                  `json:"hot_emoji_size,omitempty"`
	StickerId                int64                  `json:"sticker_id,omitempty"`
	InitiatingSource         table.InitiatingSource `json:"initiating_source,omitempty"`           // usually FACEBOOK_INBOX
	SkipUrlPreviewGen        int32                  `json:"skip_url_preview_gen"`                  // 0 or 1
	TextHasLinks             int32                  `json:"text_has_links"`                        // 0 or 1
	StripForwardedMsgCaption int32                  `json:"strip_forwarded_msg_caption,omitempty"` // 0 or 1
	ForwardedMsgId           string                 `json:"forwarded_msg_id,omitempty"`
	MultiTabEnv              int32                  `json:"multitab_env"` // 0 ?
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
	ReplySender     int64  `json:"-"`
}

type MentionData struct {
	// All fields here are comma-separated lists
	MentionIDs     string `json:"mention_ids"`
	MentionOffsets string `json:"mention_offsets"`
	MentionLengths string `json:"mention_lengths"`
	MentionTypes   string `json:"mention_types"`
}

func (md *MentionData) Parse() (Mentions, error) {
	if md == nil || len(md.MentionIDs) == 0 {
		return nil, nil
	}
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
			ID:     userID,
			Offset: offset,
			Length: length,
			Type:   MentionType(mentionTypes[i]),
		}
	}
	sort.Sort(mentions)
	return mentions, nil
}

type MentionType string

const (
	MentionTypePerson MentionType = "p"
	MentionTypeSilent MentionType = "s"
	MentionTypeThread MentionType = "t"
)

type Mention struct {
	ID     int64
	Offset int
	Length int
	Type   MentionType
}

type Mentions []Mention

func (m Mentions) Len() int {
	return len(m)
}

func (m Mentions) Less(i, j int) bool {
	return m[i].Offset < m[j].Offset
}

func (m Mentions) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

func (m Mentions) ToData() *MentionData {
	mentionIDs := make([]string, len(m))
	mentionOffsets := make([]string, len(m))
	mentionLengths := make([]string, len(m))
	mentionTypes := make([]string, len(m))
	for i, mention := range m {
		mentionIDs[i] = strconv.FormatInt(mention.ID, 10)
		mentionOffsets[i] = strconv.Itoa(mention.Offset)
		mentionLengths[i] = strconv.Itoa(mention.Length)
		mentionTypes[i] = string(mention.Type)
	}
	return &MentionData{
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
	queueName := strconv.FormatInt(t.ThreadId, 10)
	return t, queueName, false
}

type CreatePollTask struct {
	QuestionText string   `json:"question_text"`
	ThreadKey    int64    `json:"thread_key"`
	Options      []string `json:"options"`
	SyncGroup    int64    `json:"sync_group"`
}

func (t *CreatePollTask) GetLabel() string {
	return TaskLabels["CreatePollTask"]
}

func (t *CreatePollTask) Create() (interface{}, interface{}, bool) {
	return t, "poll_creation", false
}

type UpdatePollTask struct {
	ThreadKey       int64            `json:"thread_key"`
	PollID          int64            `json:"poll_id"`
	AddedOptions    []map[string]int `json:"added_options"`
	SelectedOptions []int64          `json:"selected_options"`
	SyncGroup       int64            `json:"sync_group"`
}

func (t *UpdatePollTask) GetLabel() string {
	return TaskLabels["UpdatePollTask"]
}

func (t *UpdatePollTask) Create() (interface{}, interface{}, bool) {
	return t, "poll_update", false
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
	queueName := strconv.FormatInt(t.ThreadId, 10)
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
	threadStr := strconv.FormatInt(t.ThreadKey, 10)
	queueName := "mrq." + threadStr
	return t, queueName, false
}

type MuteThreadTask struct {
	ThreadKey        int64 `json:"thread_key"`
	MailboxType      int64 `json:"mailbox_type"` // 0
	MuteExpireTimeMS int64 `json:"mute_expire_time_ms"`
	SyncGroup        int64 `json:"sync_group"` // 1
}

func (t *MuteThreadTask) GetLabel() string {
	return TaskLabels["MuteThreadTask"]
}

func (t *MuteThreadTask) Create() (interface{}, interface{}, bool) {
	return t, strconv.FormatInt(t.ThreadKey, 10), false
}

type RenameThreadTask struct {
	ThreadKey  int64  `json:"thread_key"`
	ThreadName string `json:"thread_name"`
	SyncGroup  int64  `json:"sync_group"` // 1
}

func (t *RenameThreadTask) GetLabel() string {
	return TaskLabels["RenameThreadTask"]
}

func (t *RenameThreadTask) Create() (interface{}, interface{}, bool) {
	return t, strconv.FormatInt(t.ThreadKey, 10), false
}

type SetThreadImageTask struct {
	ThreadKey int64 `json:"thread_key"`
	ImageID   int64 `json:"image_id"`
	SyncGroup int64 `json:"sync_group"` // 1
}

func (t *SetThreadImageTask) GetLabel() string {
	return TaskLabels["SetThreadImageTask"]
}

func (t *SetThreadImageTask) Create() (interface{}, interface{}, bool) {
	return t, "thread_image", false
}

type EditMessageTask struct {
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

func (t *EditMessageTask) GetLabel() string {
	return TaskLabels["EditMessageTask"]
}

func (t *EditMessageTask) Create() (interface{}, interface{}, bool) {
	return t, "edit_message", false
}

type UpdateAdminTask struct {
	ThreadKey int64 `json:"thread_key"`
	ContactID int64 `json:"contact_id"`
	IsAdmin   int   `json:"is_admin"`
}

func (t *UpdateAdminTask) GetLabel() string {
	return TaskLabels["UpdateAdminTask"]
}

func (t *UpdateAdminTask) Create() (interface{}, interface{}, bool) {
	return t, "admin_status", false
}

type RemoveParticipantTask struct {
	ThreadID  int64 `json:"thread_id"`
	ContactID int64 `json:"contact_id"`
}

func (t *RemoveParticipantTask) GetLabel() string {
	return TaskLabels["RemoveParticipantTask"]
}

func (t *RemoveParticipantTask) Create() (interface{}, interface{}, bool) {
	return t, "remove_participant_v2", false
}

type AddParticipantsTask struct {
	ThreadKey  int64   `json:"thread_key"`
	ContactIDs []int64 `json:"contact_ids"`
	SyncGroup  int64   `json:"sync_group"`
}

func (t *AddParticipantsTask) GetLabel() string {
	return TaskLabels["AddParticipantsTask"]
}

func (t *AddParticipantsTask) Create() (interface{}, interface{}, bool) {
	return t, strconv.FormatInt(t.ThreadKey, 10), false
}

type CreateThreadTask struct {
	ThreadFBID                int64 `json:"thread_fbid"`
	ForceUpsert               int   `json:"force_upsert"`
	UseOpenMessengerTransport int   `json:"use_open_messenger_transport"`
	SyncGroup                 int   `json:"sync_group"`
	MetadataOnly              int   `json:"metadata_only"`
	PreviewOnly               int   `json:"preview_only"`
}

func (t *CreateThreadTask) GetLabel() string {
	return TaskLabels["CreateThreadTask"]
}

func (t *CreateThreadTask) Create() (interface{}, interface{}, bool) {
	return t, strconv.FormatInt(t.ThreadFBID, 10), false
}

type CreateGroupPayload struct {
	ThreadID int64  `json:"thread_id"`
	OTID     string `json:"otid"`
	Source   int    `json:"source"`    // 0
	SendType int    `json:"send_type"` // 8
}

type CreateGroupTask struct {
	Participants []int64            `json:"participants"`
	SendPayload  CreateGroupPayload `json:"send_payload"`
}

func (t *CreateGroupTask) GetLabel() string {
	return TaskLabels["CreateGroupTask"]
}

func (t *CreateGroupTask) Create() (interface{}, interface{}, bool) {
	return t, strconv.FormatInt(t.SendPayload.ThreadID, 10), false
}

type DeleteThreadTask struct {
	ThreadKey  int64 `json:"thread_key"`
	RemoveType int64 `json:"remove_type"`
	SyncGroup  int64 `json:"sync_group"`
}

func (t *DeleteThreadTask) GetLabel() string {
	return TaskLabels["DeleteThreadTask"]
}

func (t *DeleteThreadTask) Create() (interface{}, interface{}, bool) {
	return t, strconv.FormatInt(t.ThreadKey, 10), false
}

type CreateWhatsAppThreadTask struct {
	WAJID            int64            `json:"wa_jid"`
	OfflineThreadKey int64            `json:"offline_thread_key"`
	ThreadType       table.ThreadType `json:"thread_type"`
	FolderType       table.FolderType `json:"folder_type"`
	BumpTimestampMS  int64            `json:"bump_timestamp_ms"`
	TAMThreadSubtype int64            `json:"tam_thread_subtype"` // 0
}

func (t *CreateWhatsAppThreadTask) GetLabel() string {
	return TaskLabels["CreateWhatsAppThreadTask"]
}

func (t *CreateWhatsAppThreadTask) Create() (any, any, bool) {
	return t, strconv.FormatInt(t.OfflineThreadKey, 10), false
}

type FetchCommunityMemberList struct {
	CommunityID     int64   `json:"community_id"`
	Roles           []int   `json:"roles"`             // [0]
	FetchAdminsOnly int     `json:"fetch_admins_only"` // 0
	Cursor          string  `json:"cursor"`
	Source          int     `json:"source"` // 7
	ThreadKey       int64   `json:"thread_key"`
	SearchText      *string `json:"search_text"`     // null
	ThreadRoles     []int   `json:"thread_roles"`    // []
	GroupThreadID   *int64  `json:"group_thread_id"` // null
	RequestID       int64   `json:"request_id"`      // inbox_info_member_list_<random string?>
}

func (t *FetchCommunityMemberList) GetLabel() string {
	return TaskLabels["FetchCommunityMemberList"]
}

func (t *FetchCommunityMemberList) Create() (any, any, bool) {
	return t, "fetch_community_member_list", false
}

type FetchAdditionalThreadData struct {
	ThreadKey int64 `json:"thread_key"`
}

func (t *FetchAdditionalThreadData) GetLabel() string {
	return TaskLabels["FetchAdditionalThreadData"]
}

func (t *FetchAdditionalThreadData) Create() (any, any, bool) {
	return t, fmt.Sprintf("fetch_additional_thread_data_%d", t.ThreadKey), false
}

type CreateCommunitySubThread struct {
	ClientMutationID int64  `json:"client_mutation_id"`
	CommunityID      int64  `json:"community_id"`
	ParentMessageID  string `json:"parent_message_id"`
	ParentThreadID   int64  `json:"parent_thread_id"`
}

func (t *CreateCommunitySubThread) GetLabel() string {
	return TaskLabels["CreateCommunitySubThread"]
}

func (t *CreateCommunitySubThread) Create() (any, any, bool) {
	return t, fmt.Sprintf("create_community_sub_thread_%d", t.ParentThreadID), false
}

type DeleteCommunitySubThread struct {
	ThreadKey int64 `json:"thread_key"`
	ActorID   int64 `json:"actor_id"`
	SyncGroup int   `json:"sync_group"`
}

func (t *DeleteCommunitySubThread) GetLabel() string {
	return TaskLabels["DeleteCommunitySubThread"]
}

func (t *DeleteCommunitySubThread) Create() (any, any, bool) {
	return t, strconv.FormatInt(t.ThreadKey, 10), false
}

type CommunityThreadHoleDetection struct {
	ThreadID        int64 `json:"thread_id"`
	PreviousTQSeqID int64 `json:"previous_tq_seq_id"`
	CurrentTQSeqID  int64 `json:"current_tq_seq_id"`
	DeltaType       int64 `json:"delta_type"`
}

func (t *CommunityThreadHoleDetection) GetLabel() string {
	return TaskLabels["CommunityThreadHoleDetection"]
}

func (t *CommunityThreadHoleDetection) Create() (any, any, bool) {
	return t, fmt.Sprintf("cm_thread_hole_detection%d", t.ThreadID), false
}
