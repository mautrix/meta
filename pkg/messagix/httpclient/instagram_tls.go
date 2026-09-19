package httpclient

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/imroc/req/v3"
	utls "github.com/refraction-networking/utls"
)

func setInstagramTLSFingerprint(client *req.Client, isNative func() bool) {
	fallback := client.GetTransport().TLSHandshakeContext
	client.SetTLSHandshake(func(ctx context.Context, addr string, plainConn net.Conn) (net.Conn, *tls.ConnectionState, error) {
		if !isNative() {
			return fallback(ctx, addr, plainConn)
		}
		host := addr
		if name, _, err := net.SplitHostPort(addr); err == nil {
			host = name
		}
		config := client.GetTLSClientConfig()
		conn := utls.UClient(plainConn, &utls.Config{
			ServerName:             host,
			RootCAs:                config.RootCAs,
			InsecureSkipVerify:     config.InsecureSkipVerify,
			SessionTicketsDisabled: config.SessionTicketsDisabled,
			KeyLogWriter:           config.KeyLogWriter,
		}, utls.HelloCustom)
		err := conn.ApplyPreset(&utls.ClientHelloSpec{
			TLSVersMin:         utls.VersionTLS13,
			TLSVersMax:         utls.VersionTLS13,
			CipherSuites:       []uint16{utls.TLS_AES_128_GCM_SHA256},
			CompressionMethods: []uint8{0},
			Extensions: []utls.TLSExtension{
				&utls.SNIExtension{},
				&utls.SupportedVersionsExtension{Versions: []uint16{utls.VersionTLS13}},
				&utls.SupportedCurvesExtension{Curves: []utls.CurveID{utls.X25519, utls.CurveP256}},
				&utls.KeyShareExtension{KeyShares: []utls.KeyShare{{Group: utls.X25519}}},
				&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
					utls.ECDSAWithP256AndSHA256, utls.ECDSAWithP384AndSHA384, utls.PSSWithSHA256,
				}},
				&utls.ALPNExtension{AlpnProtocols: []string{"h2"}},
				&utls.PSKKeyExchangeModesExtension{Modes: []uint8{utls.PskModePlain, utls.PskModeDHE}},
			},
		})
		if err != nil {
			return nil, nil, err
		}
		conn.HandshakeState.Hello.SessionId = nil
		if err = conn.HandshakeContext(ctx); err != nil {
			return nil, nil, err
		}
		state := conn.ConnectionState()
		return conn, &tls.ConnectionState{
			Version:            state.Version,
			HandshakeComplete:  state.HandshakeComplete,
			DidResume:          state.DidResume,
			CipherSuite:        state.CipherSuite,
			NegotiatedProtocol: state.NegotiatedProtocol,
			//lint:ignore SA1019 req requires this field to select HTTP/2
			NegotiatedProtocolIsMutual:  state.NegotiatedProtocolIsMutual,
			ServerName:                  state.ServerName,
			PeerCertificates:            state.PeerCertificates,
			VerifiedChains:              state.VerifiedChains,
			SignedCertificateTimestamps: state.SignedCertificateTimestamps,
			OCSPResponse:                state.OCSPResponse,
			TLSUnique:                   state.TLSUnique,
		}, nil
	})
}
