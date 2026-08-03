package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Auth reuses the model mautrix-meta already asks users for: paste an
// authenticated request copied from DevTools ("Copy as cURL"). No password, no
// OAuth, no Meta developer app.

const defaultSessionFile = ".bsprobe/session.curl"

// businessHost is the surface under investigation. Kept in one place so a
// redirect to a different host is obvious rather than silently followed.
const businessHost = "https://business.facebook.com"

type Session struct {
	Cookies   []*http.Cookie
	UserAgent string
	client    *http.Client
}

var (
	curlCookieHeaderRe = regexp.MustCompile(`(?i)-H\s+'cookie:\s*([^']*)'`)
	curlCookieBRe      = regexp.MustCompile(`(?i)-b\s+'([^']*)'`)
	curlUARe           = regexp.MustCompile(`(?i)-H\s+'user-agent:\s*([^']*)'`)
)

// LoadSession accepts either a raw "name=value; name=value" cookie string or a
// full "Copy as cURL" blob, because both are what people actually paste.
func LoadSession(path string) (*Session, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no session at %s\n\n"+
				"In Chrome, open business.facebook.com/latest/inbox/all while logged in,\n"+
				"right-click any request in DevTools > Network, choose Copy > Copy as cURL,\n"+
				"and save it to that path. It is gitignored.", path)
		}
		return nil, err
	}
	text := string(raw)

	cookieStr := ""
	if m := curlCookieHeaderRe.FindStringSubmatch(text); m != nil {
		cookieStr = m[1]
	} else if m := curlCookieBRe.FindStringSubmatch(text); m != nil {
		cookieStr = m[1]
	} else if strings.Contains(text, "=") && !strings.Contains(text, "curl ") {
		cookieStr = strings.TrimSpace(text)
	}
	if cookieStr == "" {
		return nil, fmt.Errorf("could not find cookies in %s (expected a 'Copy as cURL' blob or a raw cookie string)", path)
	}

	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	if m := curlUARe.FindStringSubmatch(text); m != nil {
		ua = m[1]
	}

	s := &Session{UserAgent: ua}
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		i := strings.Index(part, "=")
		if i <= 0 {
			continue
		}
		s.Cookies = append(s.Cookies, &http.Cookie{
			Name:  part[:i],
			Value: part[i+1:],
		})
	}
	if len(s.Cookies) == 0 {
		return nil, fmt.Errorf("parsed zero cookies from %s", path)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(businessHost)
	jar.SetCookies(u, s.Cookies)
	// Meta shares session cookies across the facebook.com apex.
	if fb, err := url.Parse("https://www.facebook.com"); err == nil {
		jar.SetCookies(fb, s.Cookies)
	}

	s.client = &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	return s, nil
}

func (s *Session) Has(name string) bool {
	for _, c := range s.Cookies {
		if c.Name == name {
			return true
		}
	}
	return false
}

// UserID returns the Facebook user ID from the c_user cookie. Note this stays
// the human account even when a Page mailbox is selected — Page identity is
// carried by asset context, not by swapping this value.
func (s *Session) UserID() string {
	for _, c := range s.Cookies {
		if c.Name == "c_user" {
			return c.Value
		}
	}
	return ""
}

func (s *Session) Get(rawURL string) (*http.Response, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	s.decorate(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	// Cap the read: the inbox bootstrap document is large but not unbounded.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}
	return resp, string(body), nil
}

func (s *Session) decorate(req *http.Request) {
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

func cmdValidateSession(args []string) error {
	fs := flag.NewFlagSet("validate-session", flag.ExitOnError)
	sessionPath := fs.String("session", defaultSessionFile, "path to a cookie string or 'Copy as cURL' blob")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := LoadSession(*sessionPath)
	if err != nil {
		return err
	}

	fmt.Printf("cookies loaded: %d\n", len(s.Cookies))
	for _, want := range []string{"c_user", "xs", "datr", "sb"} {
		status := "MISSING"
		if s.Has(want) {
			status = "present"
		}
		fmt.Printf("  %-8s %s\n", want, status)
	}
	if !s.Has("c_user") || !s.Has("xs") {
		return fmt.Errorf("c_user and xs are both required for an authenticated session")
	}
	fmt.Printf("facebook user id: %s\n", s.UserID())

	target := businessHost + "/latest/inbox/all"
	resp, body, err := s.Get(target)
	if err != nil {
		return fmt.Errorf("request %s: %w", target, err)
	}
	fmt.Printf("\nGET %s -> %d (%d bytes)\n", target, resp.StatusCode, len(body))

	final := resp.Request.URL.String()
	if !strings.Contains(final, "business.facebook.com") {
		fmt.Printf("redirected off the business surface: %s\n", final)
		return fmt.Errorf("session did not stay on business.facebook.com (likely logged out or checkpointed)")
	}
	if strings.Contains(body, "login") && strings.Contains(body, "password") && len(body) < 200_000 {
		return fmt.Errorf("response looks like a login page - session is not authenticated")
	}
	fmt.Printf("final url: %s\n", final)
	fmt.Println("\nsession looks authenticated against the Business Suite inbox")
	return nil
}
