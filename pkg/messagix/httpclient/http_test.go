package httpclient

import "testing"

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
