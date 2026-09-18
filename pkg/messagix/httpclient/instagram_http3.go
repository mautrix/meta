package httpclient

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"

	"github.com/imroc/req/v3"
	"github.com/quic-go/quic-go"
	"go.mau.fi/util/exhttp"
)

type instagramHTTPTransport struct {
	*req.Transport
	native *req.Transport
}

func newInstagramHTTPTransport(base *req.Transport, settings exhttp.ClientSettings) http.RoundTripper {
	if settings.ProxyAddress != "" || settings.HTTPProxy != nil {
		return base
	}
	native := req.NewTransport()
	native.Options = base.Options.Clone()
	native.EnableForceHTTP3().DisableAutoDecode().SetHTTP3QUICConfig(&quic.Config{
		HandshakeIdleTimeout: settings.TLSHandshakeTimeout,
	})
	if native.DialContext == nil {
		native.SetDial((&net.Dialer{Timeout: settings.DialTimeout}).DialContext)
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
			var dialErr *req.HTTP3DialError
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
