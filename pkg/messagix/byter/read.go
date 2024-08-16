package byter

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	badGlobalLog "github.com/rs/zerolog/log"
)

func NewReader(data []byte) *byter {
	return &byter{
		Buff: bytes.NewBuffer(data),
	}
}

func (b *byter) ReadInteger(kind reflect.Kind, size int, endian string) (uint64, error) {
	if endian == "" {
		endian = "big"
	}
	buffer := make([]byte, size)
	_, err := b.Buff.Read(buffer)
	if err != nil {
		return 0, fmt.Errorf("error reading integer: %w", err)
	}

	if funcs, exists := endianOperations[endian]; exists {
		if ops, ok := funcs[kind]; ok {
			return ops.Read(buffer), nil
		}
	}

	return 0, fmt.Errorf("unsupported kind or endianness")
}

func (b *byter) ReadToStruct(s interface{}) error {
	values := reflect.ValueOf(s)
	if values.Kind() == reflect.Ptr && values.Elem().Kind() == reflect.Struct {
		values = values.Elem()
	} else {
		return fmt.Errorf("expected a pointer to a struct")
	}

	for i := 0; i < values.NumField(); i++ {
		if b.Buff.Len() <= 0 {
			return nil
		}
		field := values.Field(i)
		if !field.CanSet() {
			continue
		}

		tags := values.Type().Field(i).Tag
		switch field.Kind() {
		case reflect.Bool:
			boolByte := b.Buff.Next(1)
			field.SetBool(boolByte[0] != 0)
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			hasVLQTag := tags.Get("vlq")
			if hasVLQTag != "" {
				uintValue, err := b.DecodeVLQ()
				if err != nil {
					return err
				}
				field.SetUint(uint64(uintValue))
			} else {
				size := int(field.Type().Size())
				endianess := tags.Get("endian")
				uintValue, err := b.ReadInteger(field.Kind(), size, endianess)
				if err != nil {
					return err
				}
				field.SetUint(uintValue)
			}
		case reflect.String:
			lengthType := tags.Get("lengthType")
			data, err := b.readString(lengthType, tags.Get("endian"))
			if err != nil {
				return err
			}
			field.SetString(string(data))
		case reflect.Interface:
			if field.IsNil() {
				continue
			}
		case reflect.Struct:
			jsonString := tags.Get("jsonString")
			if jsonString != "" {
				jsonData := b.Buff.Next(b.Buff.Len())
				err := json.Unmarshal(jsonData, field.Addr().Interface())
				if err != nil {
					return fmt.Errorf("failed to unmarshal JSON for field %s: %w", values.Type().Field(i).Name, err)
				}
			}
		default:
			// TODO figure out why this happens when reconnecting
			logEvt := badGlobalLog.Warn().
				Str("field_type", field.Type().Name()).
				Str("field_name", values.Type().Field(i).Name).
				Str("struct_name", values.Type().Name()).
				Int("buff_len", b.Buff.Len())
			if b.Buff.Len() < 1024 {
				logEvt.Str("buff", base64.StdEncoding.EncodeToString(b.Buff.Next(b.Buff.Len())))
			}
			logEvt.Msg("Byter.ReadToStruct: unsupported type")
			//return fmt.Errorf("unsupported type %s for field %s of %s", field.Type(), values.Type().Field(i).Name, values.Type().Name())
		}
	}

	return nil
}

func (b *byter) readString(lengthType string, endianess string) (string, error) {
	if endianess == "" {
		endianess = "big"
	}

	size, exists := stringLengthSizes[lengthType]
	if !exists {
		return "", fmt.Errorf("invalid lengthType: %s", lengthType)
	}

	kind, exists := stringLengthTags[lengthType]
	if !exists {
		return "", fmt.Errorf("invalid lengthType: %s", lengthType)
	}

	strLength, err := b.ReadInteger(kind, size, endianess) // Now, `size` is an integer value.
	if err != nil {
		return "", err
	}

	strBytes := make([]byte, strLength)
	_, err = b.Buff.Read(strBytes)
	if err != nil {
		return "", err
	}

	return string(strBytes), nil
}

func (b *byter) DecodeVLQ() (int, error) {
	multiplier := 1
	value := 0
	var encodedByte byte
	var err error

	for {
		encodedByte, err = b.Buff.ReadByte()
		if err != nil {
			return 0, err
		}
		value += int(encodedByte&0x7F) * multiplier
		if multiplier > 2097152 {
			return 0, errors.New("malformed vlq encoded data")
		}
		multiplier *= 128
		if (encodedByte & 0x80) == 0 {
			break
		}
	}
	return value, nil
}
