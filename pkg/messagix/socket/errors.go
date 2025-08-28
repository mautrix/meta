package socket

import "errors"

var (
	ErrSocketClosed      = errors.New("messagix-socket: socket is closed")
	ErrSocketAlreadyOpen = errors.New("messagix-socket: socket is already open")
	ErrNotAuthenticated  = errors.New("messagix-socket: client has not been authenticated successfully yet")
	ErrDial              = errors.New("failed to dial socket")
	ErrSendConnect       = errors.New("failed to send connect packet")
	ErrInReadLoop        = errors.New("error in read loop")
)
