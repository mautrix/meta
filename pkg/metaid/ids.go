package metaid

import (
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func MakeWAUserID(jid types.JID) networkid.UserID {
	return networkid.UserID(strconv.Itoa(int(jid.UserInt())))
}

func MakeUserID(user int64) networkid.UserID {
	return networkid.UserID(strconv.Itoa(int(user)))
}

func ParseIDFromString(id string) (int64, error) {
	i, err := strconv.Atoi(id)
	if err != nil {
		return 0, err
	}
	return int64(i), nil
}

func MakeUserLoginID(user int64) networkid.UserLoginID {
	return networkid.UserLoginID(MakeUserID(user))
}

func MakeWAPortalID(jid types.JID) networkid.PortalID {
	return networkid.PortalID(jid.User)
}

func MakeFBPortalID(portal int64) networkid.PortalID {
	return networkid.PortalID(strconv.Itoa(int(portal)))
}

func ParseUserID(user networkid.UserID) int64 {
	i, _ := strconv.Atoi(string(user))
	return int64(i)
}

func ParseUserLoginID(user networkid.UserLoginID) int64 {
	i, _ := strconv.Atoi(string(user))
	return int64(i)
}

func ParseFBPortalID(portal networkid.PortalID) int64 {
	i, _ := strconv.Atoi(string(portal))
	return int64(i)
}

func ParseWAPortalID(portal networkid.PortalID, server string) types.JID {
	return types.JID{
		User:   string(portal),
		Server: server,
	}
}

func MakeWAMessageID(chat, sender types.JID, id types.MessageID) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("wa:%s:%s:%s", chat.String(), sender.ToNonAD().String(), id))
}

func MakeFBMessageID(messageID string) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("fb:%s", messageID))
}

func ParseFBMessageID(messageID networkid.MessageID) string {
	parts := strings.SplitN(string(messageID), ":", 2)
	if len(parts) != 2 || parts[0] != "fb" {
		return ""
	}
	return parts[1]
}

func ParseWAMessageID(messageID networkid.MessageID) (chat, sender types.JID, id types.MessageID) {
	parts := strings.SplitN(string(messageID), ":", 4)
	if len(parts) != 4 || parts[0] != "wa" {
		return
	}
	var err error
	chat, err = types.ParseJID(parts[1])
	if err != nil {
		return
	}
	sender, err = types.ParseJID(parts[2])
	if err != nil {
		return
	}
	id = types.MessageID(parts[3])
	return
}

func GetMessageIDKind(messageID networkid.MessageID) string {
	parts := strings.SplitN(string(messageID), ":", 2)
	switch parts[0] {
	case "wa", "fb":
		return parts[0]
	default:
		return ""
	}
}
