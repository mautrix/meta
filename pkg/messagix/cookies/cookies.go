package cookies

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type MetaCookieName string

const (
	// MetaCookieDatr seems to be a session ID that's displayed in security settings
	MetaCookieDatr             MetaCookieName = "datr"
	MetaCookieDevicePixelRatio MetaCookieName = "dpr"

	// FBCookieXS is the main session cookie for Facebook
	FBCookieXS MetaCookieName = "xs"
	// FBCookieCUser contains the user ID for Facebook
	FBCookieCUser MetaCookieName = "c_user"

	FBCookieSB               MetaCookieName = "sb"
	FBCookieFR               MetaCookieName = "fr"
	FBCookieWindowDimensions MetaCookieName = "wd"
	FBCookiePresence         MetaCookieName = "presence"
	FBCookieOO               MetaCookieName = "oo"

	// IGCookieSessionID is the main session cookie for Instagram
	IGCookieSessionID MetaCookieName = "sessionid"
	// IGCookieCSRFToken is the CSRF token for Instagram which must match the one in request headers
	IGCookieCSRFToken MetaCookieName = "csrftoken"
	// IGCookieDSUserID contains the user ID for Instagram
	IGCookieDSUserID MetaCookieName = "ds_user_id"

	IGCookieMachineID MetaCookieName = "mid"
	IGCookieDeviceID  MetaCookieName = "ig_did"
)

var FBRequiredCookies = []MetaCookieName{FBCookieXS, FBCookieCUser, MetaCookieDatr}
var IGRequiredCookies = []MetaCookieName{IGCookieSessionID, IGCookieCSRFToken, IGCookieDSUserID, IGCookieMachineID, IGCookieDeviceID}

type Cookies struct {
	Platform types.Platform
	values   map[MetaCookieName]string
	lock     sync.RWMutex

	IGWWWClaim string
}

func (c *Cookies) UpdateValues(newValues map[MetaCookieName]string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.values = make(map[MetaCookieName]string)
	for k, v := range newValues {
		c.values[MetaCookieName(k)] = v
	}
}

func (c *Cookies) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.values)
}

func (c *Cookies) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &c.values)
}

func (c *Cookies) String() string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	var out []string
	for k, v := range c.values {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(out, "; ")
}

func (c *Cookies) GetViewports() (width, height string) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	pxs := strings.Split(c.values[FBCookieWindowDimensions], "x")
	if len(pxs) != 2 {
		return "1920", "1003"
	}
	return pxs[0], pxs[1]
}

func (c *Cookies) GetMissingCookieNames() []MetaCookieName {
	c.lock.RLock()
	defer c.lock.RUnlock()
	var missingCookies []MetaCookieName
	if c.Platform.IsMessenger() {
		missingCookies = slices.Clone(FBRequiredCookies)
	} else {
		missingCookies = slices.Clone(IGRequiredCookies)
	}
	return slices.DeleteFunc(missingCookies, func(name MetaCookieName) bool {
		return c.values[name] != ""
	})
}

func (c *Cookies) IsLoggedIn() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	var cookieKey MetaCookieName
	if c.Platform.IsMessenger() {
		cookieKey = FBCookieXS
	} else {
		cookieKey = IGCookieSessionID
	}
	return c.values[cookieKey] != ""
}

func (c *Cookies) GetUserID() int64 {
	c.lock.RLock()
	defer c.lock.RUnlock()
	var cookieKey MetaCookieName
	if c.Platform.IsMessenger() {
		cookieKey = FBCookieCUser
	} else {
		cookieKey = IGCookieDSUserID
	}
	userID, _ := strconv.ParseInt(c.values[cookieKey], 10, 64)
	return userID
}

func (c *Cookies) Get(key MetaCookieName) string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.values[key]
}

func (c *Cookies) GetAll() map[MetaCookieName]string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	newMap := map[MetaCookieName]string{}
	for key, val := range c.values {
		newMap[key] = val
	}
	return newMap
}

func (c *Cookies) Set(key MetaCookieName, value string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.values[key] = value
}

func (c *Cookies) UpdateFromResponse(r *http.Response) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// Note: this will fail to parse rur, shbid and shbts because they have quotes and backslashes
	for _, cookie := range r.Cookies() {
		if cookie.MaxAge == 0 || cookie.Expires.Before(time.Now()) {
			delete(c.values, MetaCookieName(cookie.Name))
		} else {
			c.values[MetaCookieName(cookie.Name)] = cookie.Value
		}
	}
	if wwwClaim := r.Header.Get("x-ig-set-www-claim"); wwwClaim != "" {
		c.IGWWWClaim = wwwClaim
	}
}
