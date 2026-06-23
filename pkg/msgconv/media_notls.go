//go:build ignore_tls

package msgconv

import (
	"crypto/tls"
	"net/http"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
)

func init() {
	mediaHTTPClient.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	httpclient.DisableTLSVerification = true
}
