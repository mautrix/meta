package cookies

import (
	"net/http"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

func TestUpdateFromResponsePreservesSessionCookie(t *testing.T) {
	c := &Cookies{Platform: types.Facebook}
	c.UpdateValues(map[MetaCookieName]string{FBCookieXS: "old"})
	resp := &http.Response{Header: http.Header{
		"Set-Cookie": []string{"xs=next; Path=/; HttpOnly; Secure"},
	}}

	c.UpdateFromResponse(resp)

	if got := c.Get(FBCookieXS); got != "next" {
		t.Fatalf("expected session cookie to be updated, got %q", got)
	}
}

func TestGetUserIDPrefersSelectedFacebookActor(t *testing.T) {
	c := &Cookies{Platform: types.Facebook}
	c.UpdateValues(map[MetaCookieName]string{
		FBCookieCUser: "100",
		FBCookieIUser: "200",
	})

	if got := c.GetUserID(); got != 200 {
		t.Fatalf("expected selected actor 200, got %d", got)
	}
	c.Delete(FBCookieIUser)
	if got := c.GetUserID(); got != 100 {
		t.Fatalf("expected owning account 100, got %d", got)
	}
}
