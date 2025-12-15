package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.mau.fi/util/ptr"
	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	meta := portal.Metadata.(*metaid.PortalMetadata)
	if !meta.ThreadType.IsWhatsApp() {
		return nil, fmt.Errorf("getting chat info for non-whatsapp threads is not supported")
	}
	jid := meta.JID(portal.ID)
	switch jid.Server {
	case types.GroupServer:
		if m.E2EEClient == nil {
			return nil, ErrNotConnected
		}
		groupInfo, err := m.E2EEClient.GetGroupInfo(ctx, jid)
		if err != nil {
			log.Err(err).Msg("Failed to fetch WhatsApp group info")
			return nil, nil
		}
		return m.wrapWAGroupInfo(groupInfo), nil
	case types.MessengerServer, types.DefaultUserServer:
		return m.makeWADirectChatInfo(jid), nil
	default:
		return nil, fmt.Errorf("unknown WhatsApp server %s", jid.Server)
	}
}

const (
	powerDefault    = 0
	powerAdmin      = 50
	powerSuperAdmin = 75
)

func (m *MetaClient) wrapWAGroupInfo(chatInfo *types.GroupInfo) *bridgev2.ChatInfo {
	var disappear database.DisappearingSetting
	if chatInfo.IsEphemeral {
		disappear.Type = event.DisappearingTypeAfterSend
		disappear.Timer = time.Duration(chatInfo.DisappearingTimer) * time.Second
	}
	// TODO avatar
	var avatar *bridgev2.Avatar
	ml := &bridgev2.ChatMemberList{
		IsFull:           true,
		TotalMemberCount: len(chatInfo.Participants),
		MemberMap:        make(map[networkid.UserID]bridgev2.ChatMember, len(chatInfo.Participants)),
		PowerLevels: &bridgev2.PowerLevelOverrides{
			Events: map[event.Type]int{
				event.StateRoomName:   powerDefault,
				event.StateRoomAvatar: powerDefault,
				event.StateTopic:      powerDefault,
				event.EventReaction:   powerDefault,
				event.EventRedaction:  powerDefault,

				event.StateBeeperDisappearingTimer: powerDefault,
			},
			EventsDefault: ptr.Ptr(powerDefault),
			StateDefault:  ptr.Ptr(powerAdmin),
		},
	}
	if chatInfo.IsLocked {
		ml.PowerLevels.Events[event.StateRoomName] = powerAdmin
		ml.PowerLevels.Events[event.StateRoomAvatar] = powerAdmin
		ml.PowerLevels.Events[event.StateTopic] = powerAdmin
		ml.PowerLevels.Events[event.StateBeeperDisappearingTimer] = powerAdmin
	}
	if chatInfo.IsAnnounce {
		ml.PowerLevels.EventsDefault = ptr.Ptr(powerAdmin)
	}
	for _, member := range chatInfo.Participants {
		pl := powerDefault
		if member.IsAdmin {
			pl = powerAdmin
		}
		if member.IsSuperAdmin {
			pl = powerSuperAdmin
		}
		evtSender := m.makeWAEventSender(member.JID)
		ml.MemberMap[evtSender.Sender] = bridgev2.ChatMember{
			EventSender: evtSender,
			Membership:  event.MembershipJoin,
			PowerLevel:  &pl,
		}
	}
	return &bridgev2.ChatInfo{
		Name:         ptr.Ptr(chatInfo.Name),
		Topic:        ptr.Ptr(chatInfo.Topic),
		Avatar:       avatar,
		Members:      ml,
		Type:         ptr.Ptr(database.RoomTypeDefault),
		Disappear:    &disappear,
		CanBackfill:  false,
		ExtraUpdates: updateServerAndThreadType(chatInfo.JID, table.ENCRYPTED_OVER_WA_GROUP),
	}
}

func updateServerAndThreadType(jid types.JID, threadType table.ThreadType) func(context.Context, *bridgev2.Portal) bool {
	return func(ctx context.Context, portal *bridgev2.Portal) (changed bool) {
		meta := portal.Metadata.(*metaid.PortalMetadata)
		if meta.WhatsAppServer != jid.Server {
			meta.WhatsAppServer = jid.Server
			changed = true
		}
		if meta.ThreadType != threadType {
			meta.ThreadType = threadType
			changed = true
		}
		return
	}
}

func (m *MetaClient) makeMinimalChatInfo(threadID int64, threadType table.ThreadType) *bridgev2.ChatInfo {
	selfEvtSender := m.selfEventSender()
	members := &bridgev2.ChatMemberList{
		MemberMap: map[networkid.UserID]bridgev2.ChatMember{selfEvtSender.Sender: {
			EventSender: selfEvtSender,
			Membership:  event.MembershipJoin,
		}},
	}

	roomType := database.RoomTypeDefault
	if threadType.IsOneToOne() {
		roomType = database.RoomTypeDM
		members.OtherUserID = metaid.MakeUserID(threadID)
		members.IsFull = true
		if metaid.MakeUserLoginID(threadID) != m.UserLogin.ID {
			members.MemberMap[members.OtherUserID] = bridgev2.ChatMember{
				EventSender: m.makeEventSender(threadID),
				Membership:  event.MembershipJoin,
			}
		} else {
			members.MemberMap = makeNoteToSelfMembers(members.OtherUserID, nil)
		}
	}
	return &bridgev2.ChatInfo{
		Members:     members,
		Type:        &roomType,
		CanBackfill: !threadType.IsWhatsApp(),
		ExtraUpdates: func(ctx context.Context, portal *bridgev2.Portal) (changed bool) {
			meta := portal.Metadata.(*metaid.PortalMetadata)
			if threadType == table.GROUP_THREAD && meta.ThreadType > 15 {
				// Don't trust changing more specific group types into the generic type
				return
			}
			if meta.ThreadType != threadType && !meta.ThreadType.IsWhatsApp() {
				meta.ThreadType = threadType
				changed = true
			}
			return
		},
	}
}

func makeNoteToSelfMembers(otherUserID networkid.UserID, info *bridgev2.UserInfo) map[networkid.UserID]bridgev2.ChatMember {
	// For note to self chats, force the user's ghost to be present by having two members
	// where one only has FromMe and the other only has Sender.
	return map[networkid.UserID]bridgev2.ChatMember{
		"":          {EventSender: bridgev2.EventSender{IsFromMe: true}},
		otherUserID: {EventSender: bridgev2.EventSender{Sender: otherUserID}, UserInfo: info},
	}
}

func (m *MetaClient) makeWADirectChatInfo(recipient types.JID) *bridgev2.ChatInfo {
	members := &bridgev2.ChatMemberList{
		OtherUserID: metaid.MakeWAUserID(recipient),
		IsFull:      true,
		PowerLevels: &bridgev2.PowerLevelOverrides{
			Events: map[event.Type]int{
				event.StateRoomName:                0,
				event.StateRoomAvatar:              0,
				event.StateTopic:                   0,
				event.StateBeeperDisappearingTimer: 0,
			},
		},
	}

	if networkid.UserLoginID(recipient.User) != m.UserLogin.ID {
		selfEvtSender := m.selfEventSender()
		members.MemberMap = map[networkid.UserID]bridgev2.ChatMember{
			selfEvtSender.Sender: {EventSender: selfEvtSender},
			members.OtherUserID:  {EventSender: m.makeWAEventSender(recipient)},
		}
	} else {
		members.MemberMap = makeNoteToSelfMembers(members.OtherUserID, nil)
	}
	return &bridgev2.ChatInfo{
		Members:      members,
		Type:         ptr.Ptr(database.RoomTypeDM),
		ExtraUpdates: updateServerAndThreadType(recipient, table.ENCRYPTED_OVER_WA_ONE_TO_ONE),
		CanBackfill:  false,
	}
}

func (m *MetaClient) wrapChatInfo(tbl table.ThreadInfo) *bridgev2.ChatInfo {
	chatInfo := m.makeMinimalChatInfo(tbl.GetThreadKey(), tbl.GetThreadType())
	if *chatInfo.Type != database.RoomTypeDM {
		if tbl.GetThreadName() != "" {
			chatInfo.Name = ptr.Ptr(tbl.GetThreadName())
		}
		if tbl.GetThreadPictureUrl() != "" {
			chatInfo.Avatar = wrapAvatar(tbl.GetThreadPictureUrl())
		}
	}
	if chatInfo.UserLocal == nil {
		chatInfo.UserLocal = &bridgev2.UserLocalPortalInfo{}
	}
	if tbl.GetFolderName() == folderE2EECutover {
		chatInfo.ExtraUpdates = bridgev2.MergeExtraUpdaters(chatInfo.ExtraUpdates, markPortalAsEncrypted)
	}
	dtit, ok := tbl.(*table.LSDeleteThenInsertThread)
	if ok {
		chatInfo.Members.TotalMemberCount = int(dtit.MemberCount)
		if dtit.MuteExpireTimeMs < 0 {
			chatInfo.UserLocal.MutedUntil = ptr.Ptr(event.MutedForever)
		} else if dtit.MuteExpireTimeMs > 0 {
			chatInfo.UserLocal.MutedUntil = ptr.Ptr(time.UnixMilli(dtit.MuteExpireTimeMs))
		}
	}
	return chatInfo
}

func (m *MetaClient) wrapChatMember(tbl *table.LSAddParticipantIdToGroupThread) bridgev2.ChatMember {
	var power int
	if tbl.IsSuperAdmin {
		power = 95
	} else if tbl.IsAdmin {
		power = 75
	} else if tbl.IsModerator {
		power = 50
	}
	return bridgev2.ChatMember{
		EventSender: m.makeEventSender(tbl.ContactId),
		Nickname:    &tbl.Nickname,
		Membership:  event.MembershipJoin,
		PowerLevel:  &power,
	}
}

func (m *MetaClient) wrapChatInfoChange(threadKey, participantID int64, threadType table.ThreadType, change *bridgev2.ChatInfoChange) *simplevent.ChatInfoChange {
	return &simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventChatInfoChange,
			LogContext: func(c zerolog.Context) zerolog.Context {
				if participantID != 0 {
					c = c.Int64("participant_id", participantID)
				}
				return c.Int64("thread_id", threadKey)
			},
			PortalKey:         m.makeFBPortalKey(threadKey, threadType),
			UncertainReceiver: threadType == table.UNKNOWN_THREAD_TYPE,
		},
		ChatInfoChange: change,
	}
}
