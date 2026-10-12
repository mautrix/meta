package receiverfetch

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/quic-go/quic-go/quicvarint"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
)

func appendBinaryHTTPBytes(message, data []byte) []byte {
	return append(quicvarint.Append(message, uint64(len(data))), data...)
}

func readBinaryHTTPBytes(message *bytes.Buffer) ([]byte, error) {
	if message.Len() == 0 {
		return nil, nil
	}
	length, err := quicvarint.Read(message)
	if err != nil {
		return nil, err
	} else if length > uint64(message.Len()) {
		return nil, io.ErrUnexpectedEOF
	}
	return message.Next(int(length)), nil
}

func readBinaryHTTPFields(message *bytes.Buffer) error {
	fields, err := readBinaryHTTPBytes(message)
	if err != nil {
		return err
	}
	for data := bytes.NewBuffer(fields); data.Len() > 0; {
		name, err := readBinaryHTTPBytes(data)
		if err != nil {
			return err
		} else if len(name) == 0 {
			return fmt.Errorf("empty binary http field name")
		} else if data.Len() == 0 {
			return io.ErrUnexpectedEOF
		}
		if _, err = readBinaryHTTPBytes(data); err != nil {
			return err
		}
	}
	return nil
}

func EncodeRequest(req *http.Request, body []byte) []byte {
	message := []byte{0}
	message = appendBinaryHTTPBytes(message, []byte(req.Method))
	message = appendBinaryHTTPBytes(message, []byte(req.URL.Scheme))
	message = appendBinaryHTTPBytes(message, []byte(req.Host))
	message = appendBinaryHTTPBytes(message, []byte(req.URL.RequestURI()))

	req.Header.Set("Host", req.Host)
	keys := slices.Sorted(maps.Keys(req.Header))
	var headers []byte
	for _, key := range keys {
		for _, value := range req.Header[key] {
			headers = appendBinaryHTTPBytes(headers, []byte(strings.ToLower(key)))
			headers = appendBinaryHTTPBytes(headers, []byte(value))
		}
	}
	message = appendBinaryHTTPBytes(message, headers)
	message = appendBinaryHTTPBytes(message, body)
	return append(message, 0)
}

func DecodeResponse(data []byte) (int, []byte, error) {
	message := bytes.NewBuffer(data)
	framing, err := quicvarint.Read(message)
	if err != nil {
		return 0, nil, err
	} else if framing != 1 {
		return 0, nil, fmt.Errorf("unsupported binary http response framing %d", framing)
	}
	for {
		status, err := quicvarint.Read(message)
		if err != nil {
			return 0, nil, err
		} else if status < 100 || status > 599 {
			return 0, nil, fmt.Errorf("invalid binary http status %d", status)
		}
		if err = readBinaryHTTPFields(message); err != nil {
			return 0, nil, err
		}
		if status < 200 {
			continue
		}
		body, err := readBinaryHTTPBytes(message)
		if err != nil {
			return 0, nil, err
		} else if err = readBinaryHTTPFields(message); err != nil {
			return 0, nil, err
		}
		return int(status), bytes.TrimPrefix(body, httpclient.AntiJSPrefix), nil
	}
}
