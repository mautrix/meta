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

	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	meta := portal.Metadata.(*PortalMetadata)
	if !meta.ThreadType.IsWhatsApp() {
		return nil, fmt.Errorf("getting chat info for non-whatsapp threads is not supported")
	}
	jid := meta.JID(portal.ID)
	switch jid.Server {
	case types.GroupServer:
		groupInfo, err := m.E2EEClient.GetGroupInfo(jid)
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
		disappear.Type = database.DisappearingTypeAfterRead
		disappear.Timer = time.Duration(chatInfo.DisappearingTimer) * time.Second
	}
	// TODO avatar
	var avatar *bridgev2.Avatar
	ml := &bridgev2.ChatMemberList{
		IsFull:           true,
		TotalMemberCount: len(chatInfo.Participants),
		Members:          make([]bridgev2.ChatMember, len(chatInfo.Participants)),
		PowerLevels: &bridgev2.PowerLevelChanges{
			Events: map[event.Type]int{
				event.StateRoomName:   powerDefault,
				event.StateRoomAvatar: powerDefault,
				event.StateTopic:      powerDefault,
				event.EventReaction:   powerDefault,
				event.EventRedaction:  powerDefault,
			},
			EventsDefault: ptr.Ptr(powerDefault),
			StateDefault:  ptr.Ptr(powerAdmin),
		},
	}
	if chatInfo.IsLocked {
		ml.PowerLevels.Events[event.StateRoomName] = powerAdmin
		ml.PowerLevels.Events[event.StateRoomAvatar] = powerAdmin
		ml.PowerLevels.Events[event.StateTopic] = powerAdmin
	}
	if chatInfo.IsAnnounce {
		ml.PowerLevels.EventsDefault = ptr.Ptr(powerAdmin)
	}
	for i, member := range chatInfo.Participants {
		pl := powerDefault
		if member.IsAdmin {
			pl = powerAdmin
		}
		if member.IsSuperAdmin {
			pl = powerSuperAdmin
		}
		ml.Members[i] = bridgev2.ChatMember{
			EventSender: m.makeWAEventSender(member.JID),
			Membership:  event.MembershipJoin,
			PowerLevel:  &pl,
		}
	}
	return &bridgev2.ChatInfo{
		Name:      ptr.Ptr(chatInfo.Name),
		Topic:     ptr.Ptr(chatInfo.Topic),
		Avatar:    avatar,
		Members:   ml,
		Type:      ptr.Ptr(database.RoomTypeDefault),
		Disappear: &disappear,
	}
}

func (m *MetaClient) makeMinimalChatInfo(threadID int64, threadType table.ThreadType) *bridgev2.ChatInfo {
	members := &bridgev2.ChatMemberList{
		Members: []bridgev2.ChatMember{{
			EventSender: m.selfEventSender(),
			Membership:  event.MembershipJoin,
		}},
	}

	roomType := database.RoomTypeDefault
	if threadType.IsOneToOne() {
		roomType = database.RoomTypeDM
		members.OtherUserID = metaid.MakeUserID(threadID)
		members.IsFull = true
		if metaid.MakeUserLoginID(threadID) != m.UserLogin.ID {
			members.Members = append(members.Members, bridgev2.ChatMember{
				EventSender: m.makeEventSender(threadID),
				Membership:  event.MembershipJoin,
			})
		}
	}
	return &bridgev2.ChatInfo{
		Members: members,
		Type:    &roomType,
		ExtraUpdates: func(ctx context.Context, portal *bridgev2.Portal) (changed bool) {
			meta := portal.Metadata.(*PortalMetadata)
			if meta.ThreadType != threadType && !meta.ThreadType.IsWhatsApp() {
				meta.ThreadType = threadType
				changed = true
			}
			return
		},
	}
}

func (m *MetaClient) makeWADirectChatInfo(recipient types.JID) *bridgev2.ChatInfo {
	members := &bridgev2.ChatMemberList{
		Members: []bridgev2.ChatMember{{
			EventSender: m.selfEventSender(),
			Membership:  event.MembershipJoin,
		}},
	}

	members.OtherUserID = metaid.MakeWAUserID(recipient)
	members.IsFull = true
	if networkid.UserLoginID(recipient.User) != m.UserLogin.ID {
		members.Members = append(members.Members, bridgev2.ChatMember{
			EventSender: m.makeWAEventSender(recipient),
			Membership:  event.MembershipJoin,
		})
	}
	return &bridgev2.ChatInfo{
		Members:      members,
		Type:         ptr.Ptr(database.RoomTypeDM),
		ExtraUpdates: markPortalAsEncrypted,
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
	dtit, ok := tbl.(*table.LSDeleteThenInsertThread)
	if ok {
		chatInfo.Members.TotalMemberCount = int(dtit.MemberCount)
	}
	return chatInfo
}

func (m *MetaClient) addMembersToChatInfo(chatInfo *bridgev2.ChatInfo, members []*table.LSAddParticipantIdToGroupThread) {
	chatInfo.Members.Members = make([]bridgev2.ChatMember, 0, len(members))
	hasSelf := false
	for _, member := range members {
		wrappedMember := m.wrapChatMember(member)
		chatInfo.Members.Members = append(chatInfo.Members.Members, wrappedMember)
		if wrappedMember.EventSender.IsFromMe {
			hasSelf = true
		}
	}
	if !hasSelf {
		chatInfo.Members.Members = append(chatInfo.Members.Members, bridgev2.ChatMember{
			EventSender: m.selfEventSender(),
			Membership:  event.MembershipJoin,
		})
	}
	chatInfo.Members.IsFull = chatInfo.Members.TotalMemberCount == len(members)
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
