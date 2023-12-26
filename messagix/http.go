package messagix

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/0xzer/messagix/cookies"
	"github.com/0xzer/messagix/types"
)

type HttpQuery struct {
	AcceptOnlyEssential  string `url:"accept_only_essential,omitempty"`
	Av                   string `url:"av,omitempty"` // not required
	User                 string `url:"__user,omitempty"` // not required
	A                    string `url:"__a,omitempty"` // 1 or 0 wether to include "suggestion_keys" or not in the response - no idea what this is
	Req                  string `url:"__req,omitempty"` // not required
	Hs                   string `url:"__hs,omitempty"` // not required
	Dpr                  string `url:"dpr,omitempty"` // not required
	Ccg                  string `url:"__ccg,omitempty"` // not required
	Rev                  string `url:"__rev,omitempty"` // not required
	S                    string `url:"__s,omitempty"` // not required
	Hsi                  string `url:"__hsi,omitempty"` // not required
	Dyn                  string `url:"__dyn,omitempty"` // not required
	Csr                  string `url:"__csr"` // not required
	CometReq             string `url:"__comet_req,omitempty"` // not required but idk what this is
	FbDtsg               string `url:"fb_dtsg,omitempty"`
	Jazoest              string `url:"jazoest,omitempty"` // not required
	Lsd                  string `url:"lsd,omitempty"` // required
	SpinR                string `url:"__spin_r,omitempty"` // not required
	SpinB                string `url:"__spin_b,omitempty"` // not required
	SpinT                string `url:"__spin_t,omitempty"` // not required
	FbAPICallerClass     string `url:"fb_api_caller_class,omitempty"` // not required
	FbAPIReqFriendlyName string `url:"fb_api_req_friendly_name,omitempty"` // not required
	Variables            string `url:"variables,omitempty"`
	ServerTimestamps     string `url:"server_timestamps,omitempty"` // "true" or "false"
	DocID                string `url:"doc_id,omitempty"`
	D					 string `url:"__d,omitempty"` // for insta
}

func (c *Client) NewHttpQuery() *HttpQuery {
	c.graphQLRequests++
	siteConfig := c.configs.browserConfigTable.SiteData
	dpr := strconv.FormatFloat(siteConfig.Pr, 'g', 4, 64)
	query := &HttpQuery{
		User: c.configs.browserConfigTable.CurrentUserInitialData.UserID,
		A: "1",
		Req: strconv.Itoa(c.graphQLRequests),
		Hs: siteConfig.HasteSession,
		Dpr: dpr,
		Ccg: c.configs.browserConfigTable.WebConnectionClassServerGuess.ConnectionClass,
		Rev: strconv.Itoa(siteConfig.SpinR),
		S: c.configs.WebSessionId,
		Hsi: siteConfig.Hsi,
		Dyn: c.configs.Bitmap.CompressedStr,
		Csr: c.configs.CsrBitmap.CompressedStr,
		CometReq: c.configs.CometReq,
		FbDtsg: c.configs.browserConfigTable.DTSGInitData.Token,
		Jazoest: c.configs.Jazoest,
		Lsd: c.configs.LsdToken,
		SpinR: strconv.Itoa(siteConfig.SpinR),
		SpinB: siteConfig.SpinB,
		SpinT: strconv.Itoa(siteConfig.SpinT),
	}
	if c.configs.browserConfigTable.CurrentUserInitialData.UserID != "0" {
		query.Av = c.configs.browserConfigTable.CurrentUserInitialData.UserID
	}
	if c.platform == types.Instagram {
		query.D = "www"
	}
	return query
}

func (c *Client) MakeRequest(url string, method string, headers http.Header, payload []byte, contentType types.ContentType) (*http.Response, []byte, error) {
	newRequest, err := http.NewRequest(method, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, nil, err
	}

	if contentType != types.NONE {
		headers.Add("content-type", string(contentType))
	}

	newRequest.Header = headers

	response, err := c.http.Do(newRequest)
	if errors.Is(err, ErrRedirectAttempted) {
		/*
			can't read body on redirect
			https://github.com/golang/go/issues/10069
		*/
		return response, nil, nil
	}
	defer response.Body.Close()

	if err != nil {
		return nil, nil, err
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}

	return response, responseBody, nil
}

func (c *Client) buildHeaders(withCookies bool) http.Header {

	headers := http.Header{}
	headers.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	headers.Add("accept-language", "en-US,en;q=0.9")
	headers.Add("dpr", "1.125")
	headers.Add("sec-ch-prefers-color-scheme", "light")
	headers.Add("sec-ch-ua", "\"Google Chrome\";v=\"116\", \"Chromium\";v=\"116\", \"Not-A.Brand\";v=\"24\"")
	headers.Add("sec-ch-ua-full-version-list", "\"Google Chrome\";v=\"116.0.5845.140\", \"Chromium\";v=\"116.0.5845.140\", \"Not-A.Brand\";v=\"24.0.0.0\"")
	headers.Add("sec-ch-ua-mobile", "?0")
	headers.Add("sec-ch-ua-model", "")
	headers.Add("sec-ch-ua-platform", "Linux")
	headers.Add("sec-ch-ua-platform-version", "6.4.12")
	headers.Add("user-agent", USER_AGENT)

	if c.platform == types.Facebook {
		c.addFacebookHeaders(&headers)
	} else {
		c.addInstagramHeaders(&headers)
	}

	if c.cookies != nil && withCookies {
		cookieStr := cookies.CookiesToString(c.cookies)
		if cookieStr != "" {
			headers.Add("cookie", cookieStr)
		}
		w, _ := c.cookies.GetViewports()
		headers.Add("viewport-width", w)
		headers.Add("x-asbd-id", "129477")
	}
	return headers
}

func (c *Client) addFacebookHeaders(h *http.Header) {
	if c.configs != nil {
		if c.configs.LsdToken != "" {
			h.Add("x-fb-lsd", c.configs.LsdToken)
		}
	}
}

func (c *Client) addInstagramHeaders(h *http.Header) {
	if c.configs != nil {
		csrfToken := c.cookies.GetValue("csrftoken")
		mid := c.cookies.GetValue("mid")
		if csrfToken != "" {
			h.Add("x-csrftoken", csrfToken)
		}

		if mid != "" {
			h.Add("x-mid", mid)
		}

		if c.configs.browserConfigTable != nil {
			instaCookies, ok := c.cookies.(*cookies.InstagramCookies)
			if ok {
				if instaCookies.IgWWWClaim == "" {
					h.Add("x-ig-www-claim", "0")
				} else {
					h.Add("x-ig-www-claim", instaCookies.IgWWWClaim)
				}
			} else {
				h.Add("x-ig-www-claim", "0")
			}
		}
		h.Add("x-ig-app-id", c.configs.browserConfigTable.CurrentUserInitialData.AppID)
	}
}

func (c *Client) findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (a *Account) sendLoginRequest(form url.Values, loginUrl string) (*http.Response, []byte, error) {
    h := a.buildLoginHeaders()
    loginPayload := []byte(form.Encode())
    
    resp, respBody, err := a.client.MakeRequest(loginUrl, "POST", h, loginPayload, types.FORM)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to send login request: %e", err)
    }

    return resp, respBody, nil
}

func (a *Account) buildLoginHeaders() http.Header {
    h := a.client.buildHeaders(true)
    if a.client.platform == types.Facebook {
        h = a.addFacebookHeaders(h)
    } else {
        h = a.addInstagramHeaders(h)
    }
    h.Add("origin", a.client.getEndpoint("base_url"))
    h.Add("referer", a.client.getEndpoint("login_page"))

    return h
}

func (a *Account) addFacebookHeaders(h http.Header) http.Header {
    h.Add("sec-fetch-dest", "document")
    h.Add("sec-fetch-mode", "navigate")
    h.Add("sec-fetch-site", "same-origin") // header is required
    h.Add("sec-fetch-user", "?1")
    h.Add("upgrade-insecure-requests", "1")
    return h
}

func (a *Account) addInstagramHeaders(h http.Header) http.Header {
    h.Add("x-instagram-ajax", strconv.Itoa(int(a.client.configs.browserConfigTable.SiteData.ServerRevision)))
    h.Add("sec-fetch-dest", "empty")
    h.Add("sec-fetch-mode", "cors")
    h.Add("sec-fetch-site", "same-origin") // header is required
    h.Add("x-requested-with", "XMLHttpRequest")
    return h
}