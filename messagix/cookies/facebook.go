package cookies

import (
	"encoding/json"
	"strings"
)

type FacebookCookies struct {
	Datr 	  string `cookie:"datr,omitempty" json:"datr,omitempty"`
	Sb   	  string `cookie:"sb,omitempty" json:"sb,omitempty"`
	AccountId string `cookie:"c_user,omitempty" json:"c_user,omitempty"`
	Xs 		  string `cookie:"xs,omitempty" json:"xs,omitempty"`
	Fr 		  string `cookie:"fr,omitempty" json:"fr,omitempty"`
	Wd 		  string `cookie:"wd,omitempty" json:"wd,omitempty"`
	Presence  string `cookie:"presence,omitempty" json:"presence,omitempty"`
}

func (fb *FacebookCookies) ToJSON() ([]byte, error) {
	return json.Marshal(&fb)
}

func (fb *FacebookCookies) GetValue(name string) string {
	return getCookieValue(name, fb)
}

func (fb *FacebookCookies) IsLoggedIn() bool {
	return fb.Xs != ""
}

func (fb *FacebookCookies) GetViewports() (string, string) {
	pxs := strings.Split(fb.Wd, "x")
	if len(pxs) != 2 {
		return "2276", "1156"
	}
	return pxs[0], pxs[1]
}