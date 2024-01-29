package types

type Platform int

const (
	Instagram Platform = iota
	Facebook
	Messenger
	FacebookTor
)

func (p Platform) IsMessenger() bool {
	return p == Facebook || p == FacebookTor || p == Messenger
}
