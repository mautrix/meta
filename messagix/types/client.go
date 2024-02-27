package types

type Platform int

const (
	Unset Platform = iota
	Instagram
	Facebook
	Messenger
	FacebookTor
)

func (p Platform) IsMessenger() bool {
	return p == Facebook || p == FacebookTor || p == Messenger
}
