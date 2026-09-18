package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"

	"github.com/imroc/req/v3"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.mau.fi/util/exhttp"
)

type instagramHTTPTransport struct {
	*req.Transport
	native *http3.Transport
}

type connectedPacketConn struct {
	net.Conn
}

type instagramQUICDialError struct {
	error
}

func (e *instagramQUICDialError) Unwrap() error {
	return e.error
}

func (c connectedPacketConn) ReadFrom(buf []byte) (int, net.Addr, error) {
	n, err := c.Read(buf)
	return n, c.RemoteAddr(), err
}

func (c connectedPacketConn) WriteTo(buf []byte, addr net.Addr) (int, error) {
	if addr.String() != c.RemoteAddr().String() {
		return 0, net.InvalidAddrError("connected UDP peer changed")
	}
	return c.Write(buf)
}

func newInstagramHTTPTransport(base *req.Transport, settings exhttp.ClientSettings) http.RoundTripper {
	if settings.ProxyAddress != "" || settings.HTTPProxy != nil {
		return base
	}
	dial := settings.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: settings.DialTimeout}).DialContext
	}
	native := &http3.Transport{
		TLSClientConfig: base.TLSClientConfig,
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout: settings.TLSHandshakeTimeout,
		},
		Dial: func(ctx context.Context, addr string, tlsConfig *tls.Config, config *quic.Config) (*quic.Conn, error) {
			udp, err := dial(ctx, "udp", addr)
			if err != nil {
				return nil, &instagramQUICDialError{err}
			}
			conn, err := quic.Dial(ctx, connectedPacketConn{udp}, udp.RemoteAddr(), tlsConfig, config)
			if err != nil {
				_ = udp.Close()
				return nil, &instagramQUICDialError{err}
			}
			context.AfterFunc(conn.Context(), func() { _ = udp.Close() })
			return conn, nil
		},
	}
	base.WrapRoundTripFunc(func(next http.RoundTripper) req.HttpRoundTripFunc {
		return func(request *http.Request) (*http.Response, error) {
			if request.URL.Scheme != "https" || request.URL.Hostname() != "i.instagram.com" ||
				(!strings.HasPrefix(request.URL.Path, "/api/v1/") && request.URL.Path != "/graphql_www") {
				return next.RoundTrip(request)
			}
			if base.Proxy != nil {
				proxy, err := base.Proxy(request)
				if err != nil {
					return nil, err
				} else if proxy != nil {
					return next.RoundTrip(request)
				}
			}
			var gotConn atomic.Bool
			h3Request := request.Clone(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
				GotConn: func(httptrace.GotConnInfo) { gotConn.Store(true) },
			}))
			delete(h3Request.Header, req.HeaderOderKey)
			delete(h3Request.Header, req.PseudoHeaderOderKey)
			response, err := native.RoundTrip(h3Request)
			var dialErr *instagramQUICDialError
			if !errors.As(err, &dialErr) || gotConn.Load() || request.Context().Err() != nil {
				return response, err
			}
			fallbackRequest := request.Clone(request.Context())
			if request.Body != nil && request.Body != http.NoBody {
				if request.GetBody == nil {
					return response, err
				}
				var bodyErr error
				fallbackRequest.Body, bodyErr = request.GetBody()
				if bodyErr != nil {
					return nil, errors.Join(err, bodyErr)
				}
			}
			return next.RoundTrip(fallbackRequest)
		}
	})
	return &instagramHTTPTransport{Transport: base, native: native}
}

func (t *instagramHTTPTransport) CloseIdleConnections() {
	t.Transport.CloseIdleConnections()
	t.native.CloseIdleConnections()
}

func (t *instagramHTTPTransport) Close() error {
	t.Transport.CloseIdleConnections()
	return t.native.Close()
}
