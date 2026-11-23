package messagix

import (
	"cmp"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

func isAllowedFacebookWebHost(host string) bool {
	return host == "m.facebook.com" || host == "www.facebook.com"
}

func mregRedirectError(parsed *url.URL) error {
	if parsed == nil {
		return nil
	}
	p := strings.ToLower(parsed.Path)
	switch {
	case strings.Contains(p, "/checkpoint"):
		return httpclient.ErrCheckpointRequired
	case strings.Contains(p, "/challenge"):
		return httpclient.ErrChallengeRequired
	case strings.HasPrefix(p, "/login.php"), p == "/login", p == "/login/":
		return httpclient.ErrTokenInvalidated
	}
	return nil
}

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func parseMRegSubmitForm(pageHTML, pageURL string) (string, url.Values, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return "", nil, fmt.Errorf("parsing mreg page html: %w", err)
	}
	var formNode *html.Node
	var rawAction string
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.DataAtom == atom.Form {
			if action := htmlAttr(n, "action"); strings.Contains(strings.ToLower(action), "mreg/login/submit") {
				formNode = n
				rawAction = action
				break
			}
		}
	}
	if formNode == nil {
		return "", nil, fmt.Errorf("mreg page missing login confirmation form")
	}
	parsedAction, err := url.Parse(rawAction)
	if err != nil {
		return "", nil, fmt.Errorf("parsing mreg form action: %w", err)
	}
	if !parsedAction.IsAbs() {
		baseParsed, err := url.Parse(pageURL)
		if err != nil {
			return "", nil, fmt.Errorf("parsing mreg page url: %w", err)
		}
		parsedAction = baseParsed.ResolveReference(parsedAction)
	}
	if parsedAction.Scheme != "https" || !isAllowedFacebookWebHost(parsedAction.Hostname()) {
		return "", nil, fmt.Errorf("unexpected mreg form action host %q", parsedAction.Hostname())
	}

	formVals := url.Values{}
	for n := range formNode.Descendants() {
		if n.Type != html.ElementNode || n.DataAtom != atom.Input {
			continue
		}
		if !strings.EqualFold(htmlAttr(n, "type"), "hidden") {
			continue
		}
		if name := htmlAttr(n, "name"); name != "" {
			formVals.Set(name, htmlAttr(n, "value"))
		}
	}
	if formVals.Get("fb_dtsg") == "" || formVals.Get("jazoest") == "" {
		return "", nil, fmt.Errorf("mreg confirmation form missing required tokens")
	}
	if formVals.Get("action_login") == "" {
		formVals.Set("action_login", "auto")
	}
	return parsedAction.String(), formVals, nil
}

func buildSortedCookieHeader(cookieMap map[string]string) string {
	parts := make([]string, 0, len(cookieMap))
	for k, v := range cookieMap {
		parts = append(parts, k+"="+v)
	}
	slices.Sort(parts)
	return strings.Join(parts, "; ")
}

func applyMRegSetCookies(cookieMap map[string]string, resp *http.Response) error {
	if resp == nil {
		return nil
	}
	for _, sc := range resp.Cookies() {
		if sc.Value == "" || sc.Value == "deleted" || sc.MaxAge < 0 {
			delete(cookieMap, sc.Name)
			if sc.Name == string(cookies.FBCookieCUser) || sc.Name == string(cookies.FBCookieXS) {
				return httpclient.ErrTokenInvalidated
			}
		} else {
			cookieMap[sc.Name] = sc.Value
		}
	}
	return nil
}

func (m *MessengerLiteMethods) fetchMRegForm(
	ctx context.Context,
	cookieMap map[string]string,
	clientKey, dHash string,
) (string, string, error) {
	curURL := "https://m.facebook.com/mreg?" + url.Values{
		"client_key":        {clientKey},
		"d_hash":            {dHash},
		"cid":               {useragent.MessengerLiteAndroidAppID},
		"app_version":       {useragent.MessengerLiteAndroidMajorVersion},
		"tg":                {"1"},
		"auth_tab":          {"1"},
		"is_from_msite_sso": {"1"},
	}.Encode()

	for range 5 {
		headers := http.Header{}
		headers.Set("User-Agent", useragent.MobileChromeUserAgent)
		headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		headers.Set("Accept-Language", "en-US,en;q=0.9")
		headers.Set("Sec-Fetch-Mode", "navigate")
		headers.Set("Sec-Fetch-Site", "none")
		headers.Set("Sec-Fetch-Dest", "document")
		headers.Set("Cookie", buildSortedCookieHeader(cookieMap))

		resp, body, err := m.client.http.MakeRequestOnceNoRedirect(ctx, curURL, http.MethodGet, headers, nil, types.NONE)
		if err != nil {
			return "", "", fmt.Errorf("fetching mreg page: %w", err)
		}
		if err := applyMRegSetCookies(cookieMap, resp); err != nil {
			return "", "", err
		}
		if resp.StatusCode == http.StatusOK {
			parsedCur, err := url.Parse(curURL)
			if err != nil {
				return "", "", fmt.Errorf("parsing mreg url: %w", err)
			}
			if !strings.HasPrefix(parsedCur.Path, "/mreg/login") {
				if redirErr := mregRedirectError(parsedCur); redirErr != nil {
					return "", "", redirErr
				}
				return "", "", fmt.Errorf("unexpected mreg page path %q", parsedCur.Path)
			}
			return string(body), curURL, nil
		}
		loc := resp.Header.Get("Location")
		if loc == "" {
			return "", "", fmt.Errorf("unexpected mreg status %d without redirect", resp.StatusCode)
		}
		nextParsed, err := url.Parse(loc)
		if err != nil {
			return "", "", fmt.Errorf("parsing mreg redirect: %w", err)
		}
		if !nextParsed.IsAbs() {
			baseParsed, _ := url.Parse(curURL)
			nextParsed = baseParsed.ResolveReference(nextParsed)
		}
		if redirErr := mregRedirectError(nextParsed); redirErr != nil {
			return "", "", redirErr
		}
		if nextParsed.Scheme != "https" || !isAllowedFacebookWebHost(nextParsed.Hostname()) {
			return "", "", fmt.Errorf("unexpected mreg redirect host %q", nextParsed.Hostname())
		}
		curURL = nextParsed.String()
	}
	return "", "", fmt.Errorf("mreg redirect chain did not reach login confirmation page")
}

func (m *MessengerLiteMethods) submitMRegForm(
	ctx context.Context,
	cookieMap map[string]string,
	pageURL, submitURL string,
	submitForm url.Values,
	clientKey string,
	expectedUID int64,
) (string, string, error) {
	headers := http.Header{}
	headers.Set("User-Agent", useragent.MobileChromeUserAgent)
	headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	origin := "https://m.facebook.com"
	if parsedPage, err := url.Parse(pageURL); err == nil && parsedPage.Scheme != "" && parsedPage.Host != "" {
		origin = parsedPage.Scheme + "://" + parsedPage.Host
	}
	headers.Set("Origin", origin)
	headers.Set("Referer", pageURL)
	headers.Set("Sec-Fetch-Mode", "navigate")
	headers.Set("Sec-Fetch-Site", "same-origin")
	headers.Set("Sec-Fetch-Dest", "document")
	headers.Set("Sec-Fetch-User", "?1")
	headers.Set("Cookie", buildSortedCookieHeader(cookieMap))

	resp, _, err := m.client.http.MakeRequestOnceNoRedirect(
		ctx,
		submitURL,
		http.MethodPost,
		headers,
		[]byte(submitForm.Encode()),
		types.FORM,
	)
	if err != nil {
		return "", "", fmt.Errorf("submitting mreg login confirmation: %w", err)
	}
	if err := applyMRegSetCookies(cookieMap, resp); err != nil {
		return "", "", err
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", "", fmt.Errorf("mreg submit returned status %d without redirect", resp.StatusCode)
	}
	parsedLoc, err := url.Parse(loc)
	if err != nil {
		return "", "", fmt.Errorf("parsing mreg submit redirect: %w", err)
	}
	nonce := parsedLoc.Query().Get("nonce")
	uidStr := cmp.Or(parsedLoc.Query().Get("uid"), parsedLoc.Query().Get("user_id"))
	if nonce == "" || uidStr == "" {
		if redirErr := mregRedirectError(parsedLoc); redirErr != nil {
			return "", "", redirErr
		}
		return "", "", fmt.Errorf("mreg submit redirect missing nonce or uid")
	}
	if returnedClientKey := parsedLoc.Query().Get("client_key"); returnedClientKey != "" && returnedClientKey != clientKey {
		return "", "", fmt.Errorf("mreg submit redirect client_key mismatch")
	}
	parsedUID, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil || parsedUID != expectedUID {
		return "", "", fmt.Errorf("mreg submit redirect uid does not match cookies")
	}
	return uidStr, nonce, nil
}

func (m *MessengerLiteMethods) exchangeMRegNonce(
	ctx context.Context,
	uidStr, nonce, devID, famID string,
	expectedUID int64,
) (*cookies.Cookies, *types.NativeSession, error) {
	authForm := url.Values{
		"format":                   {"json"},
		"email":                    {uidStr},
		"password":                 {nonce},
		"device_id":                {devID},
		"family_device_id":         {famID},
		"credentials_type":         {"mobile_web_based_messenger_login_nonce"},
		"generate_session_cookies": {"1"},
		"generate_machine_id":      {"1"},
		"generate_analytics_claim": {"1"},
		"source":                   {"login"},
	}
	authHeaders := http.Header{}
	authHeaders.Set("User-Agent", useragent.MessengerLiteAndroidUserAgent)
	authHeaders.Set("Authorization", "OAuth "+useragent.MessengerLiteAndroidAccessToken)
	authHeaders.Set("X-FB-HTTP-Engine", "Tigon/Liger")

	_, authBytes, reqErr := m.client.http.MakeRequestOnce(
		ctx,
		"https://b-graph.facebook.com/auth/login",
		http.MethodPost,
		authHeaders,
		[]byte(authForm.Encode()),
		types.FORM,
	)

	var authResp SessionForAppResponse
	unmarshalErr := json.Unmarshal(authBytes, &authResp)
	switch {
	case unmarshalErr == nil && authResp.Err() != nil:
		if reqErr != nil {
			return nil, nil, fmt.Errorf("%w: %w", reqErr, authResp.Err())
		}
		return nil, nil, authResp.Err()
	case reqErr != nil:
		return nil, nil, fmt.Errorf("exchanging mreg nonce for native session: %w", reqErr)
	case unmarshalErr != nil:
		return nil, nil, fmt.Errorf("parsing native session exchange response: %w", unmarshalErr)
	}

	respUID, err := authResp.UID.Int64()
	if err != nil || respUID != expectedUID {
		return nil, nil, fmt.Errorf("native session exchange account does not match cookies")
	}
	if authResp.AccessToken == "" {
		return nil, nil, fmt.Errorf("native session exchange missing access token")
	}

	newCookies := &cookies.Cookies{Platform: types.MessengerLiteAndroid}
	newCookies.UpdateValues(make(map[cookies.MetaCookieName]string))
	for _, raw := range authResp.SessionCookies {
		newCookies.Set(cookies.MetaCookieName(raw.Name), raw.Value)
	}
	if !newCookies.IsLoggedIn() || newCookies.GetUserID() != expectedUID {
		return nil, nil, fmt.Errorf("native session exchange missing valid session cookies")
	}

	nativeSession := &types.NativeSession{
		AccessToken:    authResp.AccessToken,
		AppID:          useragent.MessengerLiteAndroidAppID,
		DeviceID:       m.deviceID,
		FamilyDeviceID: m.familyDeviceID,
		MachineID:      cmp.Or(authResp.MachineID, m.machineID),
	}
	m.SetNativeSession(nativeSession)
	return newCookies, nativeSession, nil
}

func (m *MessengerLiteMethods) ExchangeCookiesForNativeSession(ctx context.Context, webCookies *cookies.Cookies) (*cookies.Cookies, *types.NativeSession, error) {
	if webCookies == nil || !webCookies.IsLoggedIn() {
		return nil, nil, loginerrors.MissingCookies
	}
	expectedUID := webCookies.GetUserID()
	if expectedUID <= 0 {
		return nil, nil, loginerrors.MissingCookies
	}
	if m.client.Platform != types.MessengerLiteAndroid {
		m.deviceID = uuid.New()
		m.familyDeviceID = uuid.New()
		m.machineID = ""
	} else {
		if m.deviceID == uuid.Nil {
			m.deviceID = uuid.New()
		}
		if m.familyDeviceID == uuid.Nil {
			m.familyDeviceID = uuid.New()
		}
	}
	devID := strings.ToUpper(m.deviceID.String())
	famID := strings.ToUpper(m.familyDeviceID.String())
	dHashSum := md5.Sum([]byte(devID))
	dHash := strings.ToUpper(hex.EncodeToString(dHashSum[:]))
	clientKey := strings.ReplaceAll(uuid.New().String(), "-", "")

	cookieMap := make(map[string]string)
	for k, v := range webCookies.GetAll() {
		if v != "" {
			cookieMap[string(k)] = v
		}
	}

	pageHTML, pageURL, err := m.fetchMRegForm(ctx, cookieMap, clientKey, dHash)
	if err != nil {
		return nil, nil, err
	}
	submitURL, submitForm, err := parseMRegSubmitForm(pageHTML, pageURL)
	if err != nil {
		return nil, nil, err
	}
	uidStr, nonce, err := m.submitMRegForm(ctx, cookieMap, pageURL, submitURL, submitForm, clientKey, expectedUID)
	if err != nil {
		return nil, nil, err
	}
	return m.exchangeMRegNonce(ctx, uidStr, nonce, devID, famID, expectedUID)
}
