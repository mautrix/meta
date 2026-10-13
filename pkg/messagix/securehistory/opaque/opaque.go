package opaque

/*
#cgo !vesta_libsignal LDFLAGS: -lvesta_opaque
#cgo vesta_libsignal LDFLAGS: -lsignal_ffi
#cgo windows LDFLAGS: -ladvapi32 -lbcrypt -lkernel32 -lntdll -luserenv -lws2_32 -ldbghelp
#include "vesta_opaque.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

var ErrInvalidPIN = errors.New("incorrect recovery PIN")

type Login struct {
	state unsafe.Pointer
}

type Result struct {
	Message    []byte
	SessionKey []byte
	ExportKey  []byte
}

func Begin(password []byte) (*Login, []byte, error) {
	if len(password) == 0 {
		return nil, nil, errors.New("empty recovery PIN")
	}
	var output C.VestaOpaqueStart
	if err := resultError(C.vesta_opaque_begin((*C.uint8_t)(unsafe.Pointer(&password[0])), C.size_t(len(password)), &output)); err != nil {
		return nil, nil, err
	}
	return &Login{state: output.state}, C.GoBytes(unsafe.Pointer(&output.message[0]), 98), nil
}

func (l *Login) Finish(response []byte) (*Result, error) {
	if l.state == nil || len(response) == 0 {
		return nil, errors.New("invalid recovery login state or response")
	}
	var output C.VestaOpaqueFinish
	defer clear(output.session_key[:])
	defer clear(output.export_key[:])
	if err := resultError(C.vesta_opaque_finish(&l.state, (*C.uint8_t)(unsafe.Pointer(&response[0])), C.size_t(len(response)), &output)); err != nil {
		return nil, err
	}
	return &Result{
		Message:    C.GoBytes(unsafe.Pointer(&output.message[0]), 64),
		SessionKey: C.GoBytes(unsafe.Pointer(&output.session_key[0]), 64),
		ExportKey:  C.GoBytes(unsafe.Pointer(&output.export_key[0]), 64),
	}, nil
}

func (l *Login) Close() {
	C.vesta_opaque_free(&l.state)
}

func resultError(code C.int32_t) error {
	switch code {
	case 0:
		return nil
	case 1:
		return errors.New("invalid OPAQUE input")
	case 2:
		return ErrInvalidPIN
	case 3:
		return errors.New("invalid OPAQUE response")
	default:
		return errors.New("OPAQUE client failed")
	}
}
