package messagix

import (
	"fmt"
)

type ConnectionCode uint8

const (
	CONNECTION_ACCEPTED ConnectionCode = iota
	CONNECTION_REFUSED_UNACCEPTABLE_PROTOCOL_VERSION
	CONNECTION_REFUSED_IDENTIFIER_REJECTED
	CONNECTION_REFUSED_SERVER_UNAVAILABLE
	CONNECTION_REFUSED_BAD_USERNAME_OR_PASSWORD
	CONNECTION_REFUSED_UNAUTHORIZED
)

var connectionCodesDescription = map[ConnectionCode]string{
	CONNECTION_ACCEPTED: "accepted",
	CONNECTION_REFUSED_UNACCEPTABLE_PROTOCOL_VERSION: "unacceptable protocol version",
	CONNECTION_REFUSED_IDENTIFIER_REJECTED:           "identifier rejected",
	CONNECTION_REFUSED_SERVER_UNAVAILABLE:            "server unavailable",
	CONNECTION_REFUSED_BAD_USERNAME_OR_PASSWORD:      "bad username or password",
	CONNECTION_REFUSED_UNAUTHORIZED:                  "unauthorized",
}

func (c ConnectionCode) Error() string {
	if name, ok := connectionCodesDescription[c]; ok {
		return name
	}
	return fmt.Sprintf("ConnectionCode(%d)", uint8(c))
}
func (c ConnectionCode) IsEnum() {}
