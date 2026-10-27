package httpclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// emSign Root TLS CA - G3 and EM OV TLS CA - G3C, one of the intermediates it
// signed whose private key was compromised (CVE-2026-111943), copied from
// OkHttp's CompromisedIntermediateTest. The intermediate's private key is the
// scalar 4, so a test can issue a leaf that chains to the real root.
const (
	emsignRootTLSCAG3 = `-----BEGIN CERTIFICATE-----
MIICKTCCAa+gAwIBAgIQDnYGcvFDRZ/I/gqwvAXjlDAKBggqhkjOPQQDAzBWMQsw
CQYDVQQGEwJJTjElMCMGA1UECgwcZU11ZGhyYSBUZWNobm9sb2dpZXMgTGltaXRl
ZDEgMB4GA1UEAwwXZW1TaWduIFJvb3QgVExTIENBIC0gRzMwHhcNMjQwMjA4MDgx
OTMyWhcNNDkwMjA3MDgxOTMyWjBWMQswCQYDVQQGEwJJTjElMCMGA1UECgwcZU11
ZGhyYSBUZWNobm9sb2dpZXMgTGltaXRlZDEgMB4GA1UEAwwXZW1TaWduIFJvb3Qg
VExTIENBIC0gRzMwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAARfD9w/q1cWxL6D9bMG
H6CQ4u+gAV6GN9s+PwQx9Bl4hC+0iU8QOk8mWO/RwXQy10DCSxT8vSIJClHKp+Ao
4qJKdYeV0SwZ7LMQziyLfxIUOY6Grja/sQFwLKYf8EdDXAyjQjBAMB0GA1UdDgQW
BBThOqshfPHjTVSu1HK3/YtZQiCQVDAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/
BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjBhCFTkly7Yl7+s4hiw+k6ZJTXT2AiI
rvmsmfqPr5E7ecwXb6kXA5LTFd6rjwTuV3sCMQC9w6ZSbVq1mveMO6dKFziOzClT
KCT2RFT6yXtq6nzgRICgBmiXbJAXsmZ8yAwjyWc=
-----END CERTIFICATE-----`

	emOVTLSCAG3C = `-----BEGIN CERTIFICATE-----
MIIDaDCCAu6gAwIBAgIPKWkmwiejoVJjkrGUgqUEMAoGCCqGSM49BAMDMFYxCzAJ
BgNVBAYTAklOMSUwIwYDVQQKDBxlTXVkaHJhIFRlY2hub2xvZ2llcyBMaW1pdGVk
MSAwHgYDVQQDDBdlbVNpZ24gUm9vdCBUTFMgQ0EgLSBHMzAeFw0yNTA0MzAxNTM3
NDFaFw00MDA0MjgxODMwMDBaMEAxCzAJBgNVBAYTAlVTMRQwEgYDVQQKDAtlTXVk
aHJhIEluYzEbMBkGA1UEAwwSRU0gT1YgVExTIENBIC0gRzNDMHYwEAYHKoZIzj0C
AQYFK4EEACIDYgAEE4JRzVKskpjByKrZdzId65fnCb0LTKCspV3IrVHc/J0ViaFZ
fjpRIOHv1jHGPhg1ysrimGmmLhYx6KKBgatWYW3EXZGKvAnzqw5jz3kqpNztc4e+
N7ulaVSfHAKycO1no4IBljCCAZIwHwYDVR0jBBgwFoAU4TqrIXzx401UrtRyt/2L
WUIgkFQwHQYDVR0OBBYEFDTq6gzryQra+dDF4u2beMmHnhtsMB0GA1UdJQQWMBQG
CCsGAQUFBwMBBggrBgEFBQcDAjBdBgNVHSAEVjBUMDkGCysGAQQBg44hAQABMCow
KAYIKwYBBQUHAgEWHGh0dHA6Ly9yZXBvc2l0b3J5LmVtc2lnbi5jb20wDQYLKwYB
BAGDjiEBAm4wCAYGZ4EMAQICMHUGCCsGAQUFBwEBBGkwZzAkBggrBgEFBQcwAYYY
aHR0cDovL29jc3AtYy5lbXNpZ24uY29tMD8GCCsGAQUFBzAChjNodHRwOi8vcmVw
b3NpdG9yeS5lbXNpZ24uY29tL2NlcnRzL0VNT1ZUTFNDQUczQy5wN2MwNwYDVR0f
BDAwLjAsoCqgKIYmaHR0cDovL2NybC5lbXNpZ24uY29tLz9yb290dGxzY2FnMy5j
cmwwDgYDVR0PAQH/BAQDAgEGMBIGA1UdEwEB/wQIMAYBAf8CAQAwCgYIKoZIzj0E
AwMDaAAwZQIxAMSZt2xsPeumRdeSMSQ3CbzIZeQ/q7GDCWfeE1Pq+mhgA6gRM44c
NVF3xJ+wsxJk+wIwFJbaDMokD8Ei6eNOiA8yUMYvfzQyngmrhZQMdPCK1SewrR79
4rSb9vGqxZzelQth
-----END CERTIFICATE-----`

	// emOVTLSCAG3CSPKI is the SHA-256 of the intermediate's
	// SubjectPublicKeyInfo, as OkHttp 5.5.1's CertificateBlocklist lists it.
	emOVTLSCAG3CSPKI = "63ecefee431d49523bc1136c4e72d0206a477086caf606cd5cf025686c109cf8"

	compromisedHost = "bank.secure"
)

var errBlocklisted = errors.New("blocklisted certificate")

func rejectBlocklisted(certs []*x509.Certificate) error {
	for _, cert := range certs {
		sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
		if hex.EncodeToString(sum[:]) == emOVTLSCAG3CSPKI {
			return errBlocklisted
		}
	}
	return nil
}

// The websocket client does its own uTLS handshake to force HTTP/1.1, so it
// has to carry the TLS config's certificate checks itself.
func TestWebsocketHandshakeKeepsCertificateChecks(t *testing.T) {
	root, addr, reached := serveCompromisedChain(t)

	t.Run("without a check the chain is trusted", func(t *testing.T) {
		c := clientTo(t, addr, &tls.Config{RootCAs: certPoolOf(root)})
		resp, err := c.websocketClient.Get("https://" + compromisedHost + "/")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		resp.Body.Close()
		if !reached.Load() {
			t.Fatal("the request did not reach the server")
		}
	})

	t.Run("VerifyConnection", func(t *testing.T) {
		assertBlocked(t, clientTo(t, addr, &tls.Config{
			RootCAs: certPoolOf(root),
			VerifyConnection: func(cs tls.ConnectionState) error {
				return rejectBlocklisted(cs.PeerCertificates)
			},
		}), reached)
	})

	t.Run("VerifyPeerCertificate", func(t *testing.T) {
		assertBlocked(t, clientTo(t, addr, &tls.Config{
			RootCAs: certPoolOf(root),
			VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
				var certs []*x509.Certificate
				for _, raw := range rawCerts {
					cert, err := x509.ParseCertificate(raw)
					if err != nil {
						return err
					}
					certs = append(certs, cert)
				}
				return rejectBlocklisted(certs)
			},
		}), reached)
	})
}

func assertBlocked(t *testing.T, c *HTTPClient, reached *atomic.Bool) {
	t.Helper()
	reached.Store(false)
	resp, err := c.websocketClient.Get("https://" + compromisedHost + "/")
	if err == nil {
		resp.Body.Close()
	}
	if !errors.Is(err, errBlocklisted) {
		t.Fatalf("err = %v, want the blocklist's error", err)
	}
	if reached.Load() {
		t.Fatal("the request reached the server")
	}
}

// clientTo returns an HTTPClient that uses tlsConfig and sends every request
// to addr, never through a proxy from the environment.
func clientTo(t *testing.T, addr string, tlsConfig *tls.Config) *HTTPClient {
	t.Helper()
	c := NewHTTPClient(testClient{}, nil, exhttp.ClientSettings{
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		HTTPProxy: func(*http.Request) (*url.URL, error) { return nil, nil },
		TLSConfig: tlsConfig,
	})
	t.Cleanup(c.websocketClient.CloseIdleConnections)
	return c
}

// testClient is the least a messagix client provides for NewHTTPClient.
type testClient struct{}

func (testClient) GetPlatform() types.Platform  { return types.Messenger }
func (testClient) GetCookies() *cookies.Cookies { return nil }
func (testClient) GetEndpoint(string) string    { return "" }
func (testClient) IsAuthenticated() bool        { return false }
func (testClient) GetLogger() *zerolog.Logger {
	logger := zerolog.Nop()
	return &logger
}

// serveCompromisedChain starts a TLS server that sends a leaf issued by the
// compromised intermediate, then the intermediate, and reports whether a
// request reached its handler.
func serveCompromisedChain(t *testing.T) (root *x509.Certificate, addr string, reached *atomic.Bool) {
	t.Helper()
	root = parsePEMCert(t, emsignRootTLSCAG3)
	intermediate := parsePEMCert(t, emOVTLSCAG3C)
	intermediateKey, err := ecdsa.ParseRawPrivateKey(elliptic.P384(), big.NewInt(4).FillBytes(make([]byte, 48)))
	if err != nil {
		t.Fatalf("intermediate key: %v", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: compromisedHost},
		DNSNames:     []string{compromisedHost},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, intermediate, leafKey.Public(), intermediateKey)
	if err != nil {
		t.Fatalf("issue leaf: %v", err)
	}

	reached = new(atomic.Bool)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{
		Certificate: [][]byte{der, intermediate.Raw},
		PrivateKey:  leafKey,
	}}}
	// The rejected handshakes are expected; keep the server's reports of them out of the test log.
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	return root, server.Listener.Addr().String(), reached
}

func parsePEMCert(t *testing.T, pemText string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		t.Fatal("no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return cert
}

func certPoolOf(certs ...*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, cert := range certs {
		pool.AddCert(cert)
	}
	return pool
}
