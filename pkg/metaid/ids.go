package metaid

import (
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func MakeWAUserID(jid types.JID) networkid.UserID {
	return networkid.UserID(strconv.FormatUint(jid.UserInt(), 10))
}

func MakeUserID(user int64) networkid.UserID {
	if user == 0 {
		return ""
	}
	return networkid.UserID(strconv.FormatInt(user, 10))
}

func ParseIDFromString(id string) (int64, error) {
	if strings.HasPrefix(id, "+") || strings.HasPrefix(id, "-") {
		return 0, fmt.Errorf("identifier can't have a sign")
	}
	i, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func MakeUserLoginID(user int64) networkid.UserLoginID {
	return networkid.UserLoginID(MakeUserID(user))
}

func MakeWAPortalID(jid types.JID) networkid.PortalID {
	return networkid.PortalID(jid.User)
}

func MakeFBPortalID(portal int64) networkid.PortalID {
	return networkid.PortalID(strconv.FormatInt(portal, 10))
}

func ParseUserID(user networkid.UserID) int64 {
	i, _ := strconv.ParseInt(string(user), 10, 64)
	return i
}

func ParseUserLoginID(user networkid.UserLoginID) int64 {
	i, _ := strconv.ParseInt(string(user), 10, 64)
	return i
}

func ParseFBPortalID(portal networkid.PortalID) int64 {
	i, _ := strconv.ParseInt(string(portal), 10, 64)
	return i
}

func ParseWAPortalID(portal networkid.PortalID, server string) types.JID {
	return types.JID{
		User:   string(portal),
		Server: server,
	}
}

type ParsedMessageID interface {
	isParsedMessageID()
	String() string
}

type ParsedWAMessageID struct {
	Chat   types.JID
	Sender types.JID
	ID     types.MessageID
}

func (ParsedWAMessageID) isParsedMessageID() {}

func (p ParsedWAMessageID) String() string {
	return fmt.Sprintf("%s:%s:%s:%s", MessageIDPrefixWA, p.Chat.String(), p.Sender.ToNonAD().String(), p.ID)
}

type ParsedFBMessageID struct {
	ID string
}

func (ParsedFBMessageID) isParsedMessageID() {}

func (p ParsedFBMessageID) String() string {
	return fmt.Sprintf("%s:%s", MessageIDPrefixFB, p.ID)
}

const (
	MessageIDPrefixFB = "fb"
	MessageIDPrefixWA = "wa"
)

func MakeWAMessageID(chat, sender types.JID, id types.MessageID) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("%s:%s:%s:%s", MessageIDPrefixWA, chat.String(), sender.ToNonAD().String(), id))
}

func MakeFBMessageID(messageID string) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("%s:%s", MessageIDPrefixFB, messageID))
}

func MakeMessagePartID(i int) networkid.PartID {
	if i == 0 {
		return ""
	}
	return networkid.PartID(strconv.Itoa(i))
}

func MakeMessageID(parsed ParsedMessageID) networkid.MessageID {
	return networkid.MessageID(parsed.String())
}

func ParseMessageID(messageID networkid.MessageID) ParsedMessageID {
	parts := strings.SplitN(string(messageID), ":", 4)
	if len(parts) == 2 && parts[0] == MessageIDPrefixFB {
		return ParsedFBMessageID{ID: parts[1]}
	} else if len(parts) == 4 && parts[0] == MessageIDPrefixWA {
		chat, err := types.ParseJID(parts[1])
		if err != nil {
			return nil
		}
		sender, err := types.ParseJID(parts[2])
		if err != nil {
			return nil
		}
		id := types.MessageID(parts[3])
		return ParsedWAMessageID{Chat: chat, Sender: sender, ID: id}
	} else {
		return nil
	}
}
