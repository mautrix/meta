package httpclient

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type testRoundTripper struct{}

func (*testRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

type testHTTPParent struct {
	cookies *cookies.Cookies
	log     zerolog.Logger
}

func (p *testHTTPParent) GetPlatform() types.Platform  { return types.Instagram }
func (p *testHTTPParent) GetLogger() *zerolog.Logger   { return &p.log }
func (p *testHTTPParent) GetCookies() *cookies.Cookies { return p.cookies }
func (p *testHTTPParent) GetEndpoint(string) string    { return "" }
func (p *testHTTPParent) IsAuthenticated() bool        { return false }

func TestSetMobileTLSFingerprintRebuildsOnlyOnChange(t *testing.T) {
	client := &HTTPClient{}
	client.SetConfig(client.HTTPSettings)
	webHTTP := client.HTTP

	client.SetMobileTLSFingerprint(true)
	if !client.mobileTLSFingerprint {
		t.Fatal("mobile TLS fingerprint was not enabled")
	}
	if client.HTTP == webHTTP {
		t.Fatal("enabling the mobile TLS fingerprint did not rebuild the HTTP transport")
	}
	mobileHTTP := client.HTTP

	client.SetMobileTLSFingerprint(true)
	if client.HTTP != mobileHTTP {
		t.Fatal("enabling an active mobile TLS fingerprint rebuilt the HTTP transport")
	}

	client.SetMobileTLSFingerprint(false)
	if client.mobileTLSFingerprint {
		t.Fatal("mobile TLS fingerprint was not disabled")
	}
	if client.HTTP == mobileHTTP {
		t.Fatal("restoring the web TLS fingerprint did not rebuild the HTTP transport")
	}
}

func TestLoginHTTPTransportSurvivesTLSFingerprintChanges(t *testing.T) {
	client := &HTTPClient{}
	client.SetConfig(exhttp.ClientSettings{})
	transport := &testRoundTripper{}
	client.SetLoginHTTPTransport(transport)

	client.SetMobileTLSFingerprint(true)
	if client.HTTP.Transport != transport {
		t.Fatal("mobile TLS fingerprint switch replaced the login HTTP transport")
	}
	client.SetMobileTLSFingerprint(false)
	if client.HTTP.Transport != transport {
		t.Fatal("web TLS fingerprint switch replaced the login HTTP transport")
	}

	client.SetLoginHTTPTransport(nil)
	if client.HTTP.Transport == transport {
		t.Fatal("clearing the login HTTP transport did not restore the default transport")
	}
}

func TestClientHTTPRedirectRefreshesAndScopesInstagramCookies(t *testing.T) {
	for _, test := range []struct {
		name          string
		nextURL       string
		forwardCookie bool
	}{
		{name: "Instagram redirect", nextURL: "https://www.instagram.com/direct/inbox/", forwardCookie: true},
		{name: "cross-origin redirect", nextURL: "https://example.com/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			jar := &cookies.Cookies{Platform: types.Instagram}
			jar.UpdateValues(map[cookies.MetaCookieName]string{
				cookies.IGCookieSessionID: "old-session",
			})
			log := zerolog.New(io.Discard)
			client := &HTTPClient{parent: &testHTTPParent{cookies: jar, log: log}, log: &log}
			previousURL, _ := url.Parse("https://i.instagram.com/api/v1/test/")
			nextURL, _ := url.Parse(test.nextURL)
			redirect := &http.Request{
				Method: http.MethodGet,
				URL:    nextURL,
				Header: http.Header{},
				Response: &http.Response{
					Proto:      clientHTTPProto,
					StatusCode: http.StatusFound,
					Header: http.Header{
						"Set-Cookie": {"sessionid=new-session; Path=/; Secure"},
					},
					Request: &http.Request{Method: http.MethodPost, URL: previousURL},
				},
			}
			if err := client.checkHTTPRedirect(redirect, []*http.Request{{
				Method: http.MethodPost,
				URL:    previousURL,
			}}); err != nil {
				t.Fatalf("redirect check failed: %v", err)
			}
			if got := jar.Get(cookies.IGCookieSessionID); got != "new-session" {
				t.Fatalf("unexpected stored session cookie: %q", got)
			}
			forwarded := strings.Contains(redirect.Header.Get("Cookie"), "sessionid=new-session")
			if forwarded != test.forwardCookie {
				t.Fatalf("cookie forwarding = %t, want %t", forwarded, test.forwardCookie)
			}
		})
	}
}
