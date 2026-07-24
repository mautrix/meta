package types

import (
	"fmt"
)

type Platform int

// Rather than matching on these enum values specifically, please
// consider using the utility methods down below, which implement
// logical operations such as "does this represent a Messenger
// account" and "does this represent an account where the user logged
// in via Facebook". That way when new enum values are added,
// callsites do not have to be updated for existing code to keep
// working for those new values that may be similar to existing ones.
const (
	Unset Platform = iota
	Instagram
	Facebook
	Messenger
	MessengerLiteIOS
	MessengerLiteAndroid
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
	case "messenger-lite":
		return MessengerLiteIOS
	case "messenger-lite-android":
		return MessengerLiteAndroid
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
	case `"messenger-lite"`, `5`:
		*p = MessengerLiteIOS
	case `"messenger-lite-android"`, `6`:
		*p = MessengerLiteAndroid
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
	case MessengerLiteIOS:
		return "messenger-lite"
	case MessengerLiteAndroid:
		return "messenger-lite-android"
	default:
		return ""
	}
}

func (p Platform) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, p.String())), nil
}

func (p Platform) IsViaFacebook() bool {
	return p == Facebook || p == FacebookTor
}

func (p Platform) IsViaMessenger() bool {
	return p == Messenger || p.IsMessengerLite()
}

func (p Platform) IsMessenger() bool {
	return p.IsViaFacebook() || p.IsViaMessenger()
}

func (p Platform) IsMessengerLite() bool {
	return p == MessengerLiteIOS || p == MessengerLiteAndroid
}

func (p Platform) IsInstagram() bool {
	return p == Instagram
}

func (p Platform) IsValid() bool {
	return p == Instagram || p == Facebook || p == FacebookTor || p == Messenger || p == MessengerLiteIOS || p == MessengerLiteAndroid
}
