package messagix

import (
	"context"
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

type WebsocketTLSDialer struct {
	netDialer proxy.ContextDialer
}

func (td *WebsocketTLSDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	helloChromeH11Only, err := utls.UTLSIdToSpec(utls.HelloChrome_Auto)
	if err != nil {
		return nil, fmt.Errorf("wstls: failed to get Chrome spec: %w", err)
	}
	for _, ext := range helloChromeH11Only.Extensions {
		alpnExt, ok := ext.(*utls.ALPNExtension)
		if !ok {
			continue
		}
		alpnExt.AlpnProtocols = []string{"http/1.1"}
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	netConn, err := td.netDialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("wstls: failed to dial: %w", err)
	}
	uconn := utls.UClient(netConn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: DisableTLSVerification,
	}, utls.HelloCustom)
	err = uconn.ApplyPreset(&helloChromeH11Only)
	if err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("wstls: failed to apply preset: %w", err)
	}
	err = uconn.HandshakeContext(ctx)
	if err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("wstls: failed to handshake: %w", err)
	}
	return uconn, nil
}
