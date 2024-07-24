package ids

import (
	"strconv"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

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

func MakePortalID(portal int64) networkid.PortalID {
	return networkid.PortalID(strconv.Itoa(int(portal)))
}

func ParseUserID(user networkid.UserID) int64 {
	i, _ := strconv.Atoi(string(user))
	return int64(i)
}
