package httpclient

import (
	"testing"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type pageActorClient struct{}

func (pageActorClient) GetPlatform() types.Platform  { return types.BusinessSuite }
func (pageActorClient) GetLogger() *zerolog.Logger   { return nil }
func (pageActorClient) GetCookies() *cookies.Cookies { return nil }
func (pageActorClient) GetEndpoint(string) string    { return "" }
func (pageActorClient) IsAuthenticated() bool        { return true }
func (pageActorClient) GetRequestActorID() string    { return "281134752317319" }

type personalClient struct{ pageActorClient }

func (personalClient) GetPlatform() types.Platform { return types.Facebook }
func (personalClient) GetRequestActorID() string   { return "" }

func TestRequestActorIDUsesSelectedBusinessPage(t *testing.T) {
	if got := requestActorID(pageActorClient{}); got != "281134752317319" {
		t.Fatalf("expected selected Page actor, got %q", got)
	}
	if got := requestActorID(personalClient{}); got != "" {
		t.Fatalf("personal inbox must not set a Page actor, got %q", got)
	}
}
