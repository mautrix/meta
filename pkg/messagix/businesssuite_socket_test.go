package messagix

import "testing"

func TestIsTrustedMQTTBrokerHost(t *testing.T) {
	trusted := []string{
		"facebook.com",
		"FACEBOOK.COM",
		"gateway.facebook.com",
		"edge-chat.facebook.com",
	}
	for _, host := range trusted {
		if !isTrustedMQTTBrokerHost(host) {
			t.Errorf("expected %q to be trusted", host)
		}
	}

	untrusted := []string{
		"evil.com",
		"facebook.com.evil.com",
		"notfacebook.com",
		"",
	}
	for _, host := range untrusted {
		if isTrustedMQTTBrokerHost(host) {
			t.Errorf("expected %q to be untrusted", host)
		}
	}
}
