package types

import (
	"fmt"
)

type Platform int

const (
	Unset Platform = iota
	Instagram
	Facebook
	Messenger
	FacebookTor
)

func PlatformFromString(s string) Platform {
	switch s {
	case "instagram":
		return Instagram
	case "facebook":
		return Facebook
	case "messenger":
		return Messenger
	case "facebook-tor":
		return FacebookTor
	default:
		return Unset
	}
}

func (p *Platform) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case `"instagram"`, `1`:
		*p = Instagram
	case `"facebook"`, `2`:
		*p = Facebook
	case `"messenger"`, `3`:
		*p = Messenger
	case `"facebook-tor"`, `4`:
		*p = FacebookTor
	default:
		*p = Unset
	}
	return nil
}

func (p Platform) String() string {
	switch p {
	case Instagram:
		return "instagram"
	case Facebook:
		return "facebook"
	case Messenger:
		return "messenger"
	case FacebookTor:
		return "facebook-tor"
	default:
		return ""
	}
}

func (p Platform) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, p.String())), nil
}

func (p Platform) IsMessenger() bool {
	return p == Facebook || p == FacebookTor || p == Messenger
}

func (p Platform) IsValid() bool {
	return p == Instagram || p == Facebook || p == FacebookTor || p == Messenger
}
