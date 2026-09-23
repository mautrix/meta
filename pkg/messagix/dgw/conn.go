package dgw

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/coder/websocket"
)

type connectionTransport interface {
	Read(context.Context) (websocket.MessageType, []byte, error)
	Write(context.Context, websocket.MessageType, []byte) error
	Close(websocket.StatusCode, string) error
	CloseNow() error
}

type connection struct {
	connectionTransport
	extendedData bool
}

type HTTPStreamOptions struct {
	Client     *http.Client
	URL        string
	GetHeaders func() http.Header
}

type httpConnection struct {
	client   http.Client
	request  *http.Request
	cancel   context.CancelFunc
	input    *io.PipeReader
	output   *io.PipeWriter
	response atomic.Pointer[http.Response]
	reader   *bufio.Reader
}

func newHTTPConnection(ctx context.Context, opts *HTTPStreamOptions) (*connection, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("%w: HTTP client is required", ErrDial)
	}
	ctx, cancel := context.WithCancel(ctx)
	input, output := io.Pipe()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.URL, input)
	if err != nil {
		cancel()
		_ = input.Close()
		_ = output.Close()
		return nil, fmt.Errorf("%w: %w", ErrDial, err)
	}
	if opts.GetHeaders != nil {
		request.Header = opts.GetHeaders().Clone()
	}
	conn := &httpConnection{
		client:  *opts.Client,
		request: request,
		cancel:  cancel,
		input:   input,
		output:  output,
	}
	conn.client.Timeout = 0
	return &connection{connectionTransport: conn, extendedData: true}, nil
}

func (c *httpConnection) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}
	if c.reader == nil {
		response, err := c.client.Do(c.request)
		if err != nil {
			if response != nil {
				_ = response.Body.Close()
			}
			return 0, nil, fmt.Errorf("%w: %w", ErrDial, err)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusUnauthorized {
				return 0, nil, fmt.Errorf("%w: %w", ErrDial, ErrUnauthorized)
			}
			return 0, nil, fmt.Errorf("%w: HTTP status %d", ErrDial, response.StatusCode)
		}
		c.response.Store(response)
		if err = c.request.Context().Err(); err != nil {
			_ = c.CloseNow()
			return 0, nil, err
		}
		c.reader = bufio.NewReader(response.Body)
	}
	data, err := readFrame(c.reader)
	return websocket.MessageBinary, data, err
}

func (c *httpConnection) Write(ctx context.Context, _ websocket.MessageType, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.CloseNow() })
	defer stop()
	_, err := c.output.Write(data)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (c *httpConnection) Close(websocket.StatusCode, string) error {
	return c.CloseNow()
}

func (c *httpConnection) CloseNow() error {
	c.cancel()
	_ = c.input.CloseWithError(net.ErrClosed)
	_ = c.output.CloseWithError(net.ErrClosed)
	if response := c.response.Swap(nil); response != nil {
		return response.Body.Close()
	}
	return nil
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	frameType, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	headerSize := 1
	switch FrameType(frameType) {
	case FrameTypeDeauth, FrameTypePing, FrameTypePong:
	case FrameTypeEndOfData:
		headerSize = 3
	case FrameTypeDrain:
		headerSize = 4
	case FrameTypeAck, FrameTypeData, FrameTypeExtendedData, FrameTypeEstabStream, FrameTypeEndOfDataWithReason:
		headerSize = 6
	default:
		return nil, fmt.Errorf("dgw: unsupported HTTP stream frame type %s", FrameType(frameType))
	}
	data := make([]byte, headerSize)
	data[0] = frameType
	if _, err = io.ReadFull(reader, data[1:]); err != nil {
		return nil, err
	}
	if headerSize < 4 {
		return data, nil
	}
	length := int(uint24LE(data[headerSize-3:]))
	switch FrameType(frameType) {
	case FrameTypeDrain:
		if length != 1 {
			return nil, fmt.Errorf("dgw: invalid drain frame length %d", length)
		}
	case FrameTypeAck:
		if length != 2 {
			return nil, fmt.Errorf("dgw: invalid ack frame length %d", length)
		}
	case FrameTypeData:
		if length < 2 {
			return nil, fmt.Errorf("dgw: invalid data frame length %d", length)
		}
	case FrameTypeExtendedData:
		if length < 3 {
			return nil, fmt.Errorf("dgw: invalid extended data frame length %d", length)
		}
	}
	data = append(data, make([]byte, length)...)
	_, err = io.ReadFull(reader, data[headerSize:])
	return data, err
}
