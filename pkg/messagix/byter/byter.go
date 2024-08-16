package byter

import (
	"bytes"
	"encoding/binary"
	"reflect"
)

var stringLengthTags = map[string]reflect.Kind{
	"uint8":  reflect.Uint8,
	"uint16": reflect.Uint16,
	"uint32": reflect.Uint32,
	"uint64": reflect.Uint64,
	"int8":   reflect.Uint8,
	"byte":   reflect.Uint8,
	"int16":  reflect.Int16,
	"int32":  reflect.Int32,
	"int64":  reflect.Int64,
}

var stringLengthSizes = map[string]int{
	"uint8":  1,
	"uint16": 2,
	"uint32": 4,
	"uint64": 8,
	"int8":   1,
	"byte":   1,
	"int16":  2,
	"int32":  4,
	"int64":  8,
}

type EndianOps struct {
	Read  endianReaderFunc
	Write endianWriterFunc
}

type endianReaderFunc func([]byte) uint64
type endianWriterFunc func(*bytes.Buffer, uint64) error

var endianOperations = map[string]map[reflect.Kind]EndianOps{
	"big": {
		reflect.Uint8: {
			Read:  func(b []byte) uint64 { return uint64(b[0]) },
			Write: func(b *bytes.Buffer, val uint64) error { return b.WriteByte(byte(val)) },
		},
		reflect.Uint16: {
			Read:  func(b []byte) uint64 { return uint64(binary.BigEndian.Uint16(b)) },
			Write: func(b *bytes.Buffer, val uint64) error { return binary.Write(b, binary.BigEndian, uint16(val)) },
		},
		reflect.Uint32: {
			Read:  func(b []byte) uint64 { return uint64(binary.BigEndian.Uint32(b)) },
			Write: func(b *bytes.Buffer, val uint64) error { return binary.Write(b, binary.BigEndian, uint32(val)) },
		},
		reflect.Uint64: {
			Read:  func(b []byte) uint64 { return binary.BigEndian.Uint64(b) },
			Write: func(b *bytes.Buffer, val uint64) error { return binary.Write(b, binary.BigEndian, val) },
		},
	},
	"little": {
		reflect.Uint8: {
			Read:  func(b []byte) uint64 { return uint64(b[0]) },
			Write: func(b *bytes.Buffer, val uint64) error { return b.WriteByte(byte(val)) },
		},
		reflect.Uint16: {
			Read:  func(b []byte) uint64 { return uint64(binary.LittleEndian.Uint16(b)) },
			Write: func(b *bytes.Buffer, val uint64) error { return binary.Write(b, binary.LittleEndian, uint16(val)) },
		},
		reflect.Uint32: {
			Read:  func(b []byte) uint64 { return uint64(binary.LittleEndian.Uint32(b)) },
			Write: func(b *bytes.Buffer, val uint64) error { return binary.Write(b, binary.LittleEndian, uint32(val)) },
		},
		reflect.Uint64: {
			Read:  func(b []byte) uint64 { return binary.LittleEndian.Uint64(b) },
			Write: func(b *bytes.Buffer, val uint64) error { return binary.Write(b, binary.LittleEndian, val) },
		},
	},
}

type byter struct {
	Buff *bytes.Buffer
}

type EnumMarker interface {
	IsEnum()
}

func (b *byter) isEnum(field reflect.Value) bool {
	return field.CanInterface() && reflect.PointerTo(field.Type()).Implements(reflect.TypeOf((*EnumMarker)(nil)).Elem())
}
