package cookies

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"github.com/0xzer/messagix/types"
	"golang.org/x/net/http/httpguts"
)

type Cookies interface {
	GetValue(name string) string
	GetViewports() (string, string)
	IsLoggedIn() bool
	ToJSON() ([]byte, error)
}

func UpdateValue(cookieStruct Cookies, name, val string) {
	//val = strings.Replace(val, "\\", "\\/", -1)
    v := reflect.ValueOf(cookieStruct).Elem()
    t := v.Type()
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        tagVal, ok := t.Field(i).Tag.Lookup("cookie")
        if !ok {
            continue
        }

        tagMainVal := strings.Split(tagVal, ",")[0]
        if tagMainVal == name && field.CanSet() {
            field.SetString(val)
        }
    }
}

/*
	Make sure the indexes for the names correspond with the correct value they should be set to
*/
func UpdateMultipleValues(cookieStruct Cookies, names []string, values []string) error {
	if len(names) != len(values) {
		return fmt.Errorf("expected same len for both names and values (names=%v, values=%v)", names, values)
	}

	for i := 0; i < len(names); i++ {
		UpdateValue(cookieStruct, names[i], values[i])
	}
	return nil
}

func UpdateFromResponse(cookieStruct Cookies, h http.Header) {
	cookies := ReadSetCookiesCustom(h)
	for _, c := range cookies {
		UpdateValue(cookieStruct, c.Name, c.Value)
	}
}

func CookiesToString(c Cookies) string {
	s := ""
	values := reflect.ValueOf(c)
	if values.Kind() == reflect.Ptr {
		values = values.Elem()
	}

	for i := 0; i < values.NumField(); i++ {
		field := values.Type().Field(i)
		value := values.Field(i).Interface()

		tagValue := field.Tag.Get("cookie")
		zeroValue := reflect.Zero(field.Type).Interface()
		if value == zeroValue || tagValue == "" {
			continue
		}

		tagName := strings.Split(tagValue, ",")[0]
		s += fmt.Sprintf("%s=%v; ", tagName, value)
	}
	
	return s
}

func NewCookiesFromResponse(cookies []*http.Cookie) Cookies {
	return nil
}

// FROM JSON FILE.
func NewCookiesFromFile(path string, platform types.Platform) (Cookies, error) {
	jsonBytes, jsonErr := os.ReadFile(path)
	if jsonErr != nil {
		return nil, jsonErr
	}

	var session Cookies

	if platform == types.Instagram {
		session = &InstagramCookies{}
	} else {
		session = &FacebookCookies{}
	}

	marshalErr := json.Unmarshal(jsonBytes, session)
	if marshalErr != nil {
		return nil, marshalErr
	}

	return session, nil
}

/*
	Example:
		var cookies types.FacebookCookies
		err := types.NewCookiesFromString("...", &cookies)
*/
func NewCookiesFromString(cookieStr string, cookieStruct Cookies) error {
	val := reflect.ValueOf(cookieStruct)
	if val.Kind() == reflect.Ptr && val.Elem().Kind() == reflect.Struct {
		val = val.Elem()
	} else {
		return fmt.Errorf("expected a pointer to a struct")
	}
	
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)
		fieldType := typ.Field(i)
		tag, ok := fieldType.Tag.Lookup("cookie")
		if ok {
			cookieValue := extractCookieValue(cookieStr, strings.Split(tag, ",")[0])
			fieldVal.SetString(cookieValue)
		}
	}

	return nil
}

func extractCookieValue(cookieString, key string) string {
	startIndex := strings.Index(cookieString, key)
	if startIndex < 0 {
		return ""
	}

	startIndex += len(key) + 1
	endIndex := strings.IndexByte(cookieString[startIndex:], ';')
	if endIndex == -1 {
		return cookieString[startIndex:]
	}

	return cookieString[startIndex : startIndex+endIndex]
}

func getCookieValue(name string, cookieStruct Cookies) string {
	v := reflect.ValueOf(cookieStruct)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		tagVal, ok := t.Field(i).Tag.Lookup("cookie")
		if !ok {
			continue
		}

		tagMainVal := strings.Split(tagVal, ",")[0]
		if tagMainVal == name {
			return field.String()
		}
	}
	return ""
}

//
// Custom implementation of std http lib implementation of parsing the Set-Cookie in response headers
//
// Because their implementation can apparently says cookie values that start with " or contain \ are invalid (https://github.com/golang/go/blob/master/src/net/http/cookie.go#L415)
//
// https://github.com/golang/go/blob/master/src/net/http/cookie.go#L60
// readSetCookies parses all "Set-Cookie" values from
// the header h and returns the successfully parsed Cookies.
func ReadSetCookiesCustom(h http.Header) []*http.Cookie {
	cookieCount := len(h["Set-Cookie"])
	if cookieCount == 0 {
		return []*http.Cookie{}
	}
	cookies := make([]*http.Cookie, 0, cookieCount)
	for _, line := range h["Set-Cookie"] {
		parts := strings.Split(textproto.TrimString(line), ";")
		if len(parts) == 1 && parts[0] == "" {
			continue
		}
		parts[0] = textproto.TrimString(parts[0])
		name, value, ok := strings.Cut(parts[0], "=")
		if !ok {
			continue
		}
		name = textproto.TrimString(name)
		if !isCookieNameValid(name) {
			continue
		}
		value, ok = parseCookieValue(value, true)
		if !ok {
			continue
		}
		c := &http.Cookie{
			Name:  name,
			Value: value,
			Raw:   line,
		}
		for i := 1; i < len(parts); i++ {
			parts[i] = textproto.TrimString(parts[i])
			if len(parts[i]) == 0 {
				continue
			}
			attr, val, _ := strings.Cut(parts[i], "=")
			lowerAttr, isASCII := ToLower(attr)
			if !isASCII {
				continue
			}
			val, ok = parseCookieValue(val, false)
			if !ok {
				c.Unparsed = append(c.Unparsed, parts[i])
				continue
			}

			switch lowerAttr {
			case "samesite":
				lowerVal, ascii := ToLower(val)
				if !ascii {
					c.SameSite = http.SameSiteDefaultMode
					continue
				}
				switch lowerVal {
				case "lax":
					c.SameSite = http.SameSiteLaxMode
				case "strict":
					c.SameSite = http.SameSiteStrictMode
				case "none":
					c.SameSite = http.SameSiteNoneMode
				default:
					c.SameSite = http.SameSiteDefaultMode
				}
				continue
			case "secure":
				c.Secure = true
				continue
			case "httponly":
				c.HttpOnly = true
				continue
			case "domain":
				c.Domain = val
				continue
			case "max-age":
				secs, err := strconv.Atoi(val)
				if err != nil || secs != 0 && val[0] == '0' {
					break
				}
				if secs <= 0 {
					secs = -1
				}
				c.MaxAge = secs
				continue
			case "expires":
				c.RawExpires = val
				exptime, err := time.Parse(time.RFC1123, val)
				if err != nil {
					exptime, err = time.Parse("Mon, 02-Jan-2006 15:04:05 MST", val)
					if err != nil {
						c.Expires = time.Time{}
						break
					}
				}
				c.Expires = exptime.UTC()
				continue
			case "path":
				c.Path = val
				continue
			}
			c.Unparsed = append(c.Unparsed, parts[i])
		}
		cookies = append(cookies, c)
	}
	return cookies
}

func validCookieValueByte(b byte) bool {
	return 0x20 <= b && b < 0x7f && b != ';'
}

func parseCookieValue(raw string, allowDoubleQuote bool) (string, bool) {
	for i := 0; i < len(raw); i++ {
		if !validCookieValueByte(raw[i]) {
			return "", false
		}
	}
	return raw, true
}

func isCookieNameValid(raw string) bool {
	if raw == "" {
		return false
	}
	return strings.IndexFunc(raw, isNotToken) < 0
}

func ToLower(s string) (lower string, ok bool) {
	if !IsPrint(s) {
		return "", false
	}
	return strings.ToLower(s), true
}

func isNotToken(r rune) bool {
	return !httpguts.IsTokenRune(r)
}

// IsPrint returns whether s is ASCII and printable according to
// https://tools.ietf.org/html/rfc20#section-4.2.
func IsPrint(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}