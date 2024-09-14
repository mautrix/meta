package connector

import (
	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaClient) selfEventSender() bridgev2.EventSender {
	return bridgev2.EventSender{
		IsFromMe:    true,
		Sender:      networkid.UserID(m.UserLogin.ID),
		SenderLogin: m.UserLogin.ID,
	}
}

func (m *MetaClient) makeEventSender(id int64) bridgev2.EventSender {
	return bridgev2.EventSender{
		IsFromMe:    metaid.MakeUserLoginID(id) == m.UserLogin.ID,
		Sender:      metaid.MakeUserID(id),
		SenderLogin: metaid.MakeUserLoginID(id),
	}
}

func (m *MetaClient) makeWAEventSender(sender types.JID) bridgev2.EventSender {
	return m.makeEventSender(int64(sender.UserInt()))
}

func (m *MetaClient) makeWAPortalKey(chatJID types.JID) networkid.PortalKey {
	key := networkid.PortalKey{
		ID: metaid.MakeWAPortalID(chatJID),
	}
	if m.Main.Bridge.Config.SplitPortals || chatJID.Server == types.MessengerServer || chatJID.Server == types.DefaultUserServer {
		key.Receiver = m.UserLogin.ID
	}
	return key
}

func (m *MetaClient) makeFBPortalKey(threadID int64, threadType table.ThreadType) networkid.PortalKey {
	key := networkid.PortalKey{ID: metaid.MakeFBPortalID(threadID)}
	if m.Main.Bridge.Config.SplitPortals || threadType == table.UNKNOWN_THREAD_TYPE || threadType.IsOneToOne() {
		key.Receiver = m.UserLogin.ID
	}
	return key
}
