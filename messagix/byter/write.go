package byter

import (
	"bytes"
	"fmt"
	"reflect"

	badGlobalLog "github.com/rs/zerolog/log"
)

func NewWriter() *byter {
	return &byter{
		Buff: bytes.NewBuffer(make([]byte, 0)),
	}
}

func (b *byter) writeInteger(value uint64, kind reflect.Kind, endian string) error {
	// Default to big endian if not specified
	if endian == "" {
		endian = "big"
	}

	if funcs, exists := endianOperations[endian]; exists {
		if ops, ok := funcs[kind]; ok {
			return ops.Write(b.Buff, value)
		}
	}

	return fmt.Errorf("unsupported kind or endianness")
}

func (b *byter) WriteFromStruct(s interface{}) ([]byte, error) {
	values := reflect.ValueOf(s)
	if values.Kind() == reflect.Ptr && values.Elem().Kind() == reflect.Struct {
		values = values.Elem()
	} else {
		return nil, fmt.Errorf("expected a struct")
	}

	for i := 0; i < values.NumField(); i++ {
		field := values.Field(i)
		shouldSkip := values.Type().Field(i).Tag.Get("skip")
		if shouldSkip == "1" {
			continue
		}
		switch field.Kind() {
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if b.isEnum(field) {
				enumValue := uint8(field.Uint()) // TO-DO: support all int vals
				b.Buff.WriteByte(enumValue)
				continue
			}
			hasVLQTag := values.Type().Field(i).Tag.Get("vlq")
			if hasVLQTag != "" {
				err := b.EncodeVLQ(int(field.Uint())) // Convert to int because our VLQ function takes an int
				if err != nil {
					return nil, err
				}
				continue
			} else {
				endianess := values.Type().Field(i).Tag.Get("endian")
				err := b.writeInteger(field.Uint(), field.Kind(), endianess)
				if err != nil {
					return nil, err
				}
			}
		case reflect.String:
			str := field.String()
			f := values.Type().Field(i)
			lengthType := f.Tag.Get("lengthType")
			endianess := f.Tag.Get("endian")
			err := b.writeString(str, lengthType, endianess)
			if err != nil {
				return nil, err
			}
		/*
			case reflect.Struct:
				sBytes, err := b.WriteFromStruct(field)
				if err != nil {
					return nil, err
				}
				b.Buff.Write(sBytes)
		*/
		default:
			badGlobalLog.Warn().
				Str("field_type", field.Type().Name()).
				Str("field_name", values.Type().Field(i).Name).
				Str("struct_name", values.Type().Name()).
				Msg("Byter.WriteFromStruct: unsupported type")
		}
	}

	return b.Buff.Bytes(), nil
}

func (b *byter) writeString(s string, lengthType string, endianess string) error {
	if endianess == "" {
		endianess = "big"
	}

	if lengthType != "" {
		b.writeInteger(uint64(len(s)), stringLengthTags[lengthType], endianess)
	}
	_, err := b.Buff.Write([]byte(s))
	return err
}

func (b *byter) EncodeVLQ(value int) error {
	var encodedByte byte
	for {
		encodedByte = byte(value & 0x7F)
		value >>= 7
		if value > 0 {
			encodedByte |= 0x80
		}

		err := b.Buff.WriteByte(encodedByte)
		if err != nil {
			return err
		}

		if value == 0 {
			break
		}
	}

	return nil
}
