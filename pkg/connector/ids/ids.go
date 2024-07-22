package ids

import (
	"strconv"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

func MakeUserID(user int64) networkid.UserID {
	return networkid.UserID(networkid.UserID(strconv.Itoa(int(user))))
}

func MakeUserLoginID(user int64) networkid.UserLoginID {
	return networkid.UserLoginID(MakeUserID(user))
}

func ParseUserID(user networkid.UserID) int64 {
	i, _ := strconv.Atoi(string(user))
	return int64(i)
}
