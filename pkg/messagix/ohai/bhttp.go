package ohai

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func appendVarint(buf []byte, val uint64) []byte {
	switch {
	case val < 1<<6:
		return append(buf, byte(val))
	case val < 1<<14:
		return append(buf, 0x40|byte(val>>8), byte(val))
	case val < 1<<30:
		return append(buf, 0x80|byte(val>>24), byte(val>>16), byte(val>>8), byte(val))
	default:
		return append(buf, 0xc0|byte(val>>56), byte(val>>48), byte(val>>40), byte(val>>32), byte(val>>24), byte(val>>16), byte(val>>8), byte(val))
	}
}

func appendBytes(buf, data []byte) []byte {
	return append(appendVarint(buf, uint64(len(data))), data...)
}

var errTruncated = errors.New("truncated binary http message")

func readVarint(data []byte) (uint64, []byte, error) {
	if len(data) == 0 {
		return 0, nil, errTruncated
	}
	length := 1 << (data[0] >> 6)
	if len(data) < length {
		return 0, nil, errTruncated
	}
	val := uint64(data[0] & 0x3f)
	for _, b := range data[1:length] {
		val = val<<8 | uint64(b)
	}
	return val, data[length:], nil
}

func readBytes(data []byte) ([]byte, []byte, error) {
	length, data, err := readVarint(data)
	if err != nil {
		return nil, nil, err
	} else if uint64(len(data)) < length {
		return nil, nil, errTruncated
	}
	return data[:length], data[length:], nil
}

func EncodeRequest(method string, target *url.URL, headers http.Header, body []byte) []byte {
	buf := []byte{0}
	buf = appendBytes(buf, []byte(method))
	buf = appendBytes(buf, []byte(target.Scheme))
	buf = appendBytes(buf, []byte(target.Host))
	buf = appendBytes(buf, []byte(target.RequestURI()))

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var headerBuf []byte
	for _, key := range keys {
		for _, value := range headers[key] {
			headerBuf = appendBytes(headerBuf, []byte(strings.ToLower(key)))
			headerBuf = appendBytes(headerBuf, []byte(value))
		}
	}
	if headers.Get("Host") == "" {
		headerBuf = appendBytes(headerBuf, []byte("Host"))
		headerBuf = appendBytes(headerBuf, []byte(target.Host))
	}
	buf = appendBytes(buf, headerBuf)
	buf = appendBytes(buf, body)
	return append(buf, 0)
}

func DecodeResponse(data []byte) (status int, headers http.Header, body []byte, err error) {
	if len(data) == 0 {
		err = errTruncated
		return
	} else if data[0] != 1 {
		err = fmt.Errorf("unexpected binary http framing indicator %d", data[0])
		return
	}
	var statusCode uint64
	statusCode, data, err = readVarint(data[1:])
	if err != nil {
		return
	}
	status = int(statusCode)
	headers = make(http.Header)
	if len(data) == 0 {
		return
	}
	var headerData []byte
	headerData, data, err = readBytes(data)
	if err != nil {
		return
	}
	for len(headerData) > 0 {
		var key, value []byte
		key, headerData, err = readBytes(headerData)
		if err != nil {
			return
		}
		value, headerData, err = readBytes(headerData)
		if err != nil {
			return
		}
		headers.Add(string(key), string(value))
	}
	if len(data) == 0 {
		return
	}
	body, _, err = readBytes(data)
	body = bytes.Clone(body)
	return
}
