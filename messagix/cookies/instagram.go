package cookies

import (
	"encoding/json"
	"strconv"
)

type InstagramCookies struct {
	SessionId  string `cookie:"sessionid,omitempty" json:"sessionid,omitempty"`
	CsrfToken  string `cookie:"csrftoken,omitempty" json:"csrftoken,omitempty"`
	MachineId  string `cookie:"mid,omitempty" json:"mid,omitempty"`
	IgDeviceId string `cookie:"ig_did,omitempty" json:"ig_did,omitempty"`
	Rur        string `cookie:"rur,omitempty" json:"rur,omitempty"`
	UserId     string `cookie:"ds_user_id,omitempty" json:"ds_user_id,omitempty"`
	ShbId      string `cookie:"shbid,omitempty" json:"shbid,omitempty"`
	Shbts      string `cookie:"shbts,omitempty" json:"shbts,omitempty"`
	IgWWWClaim string `json:"ig_www_claim,omitempty"`
}

func (ig *InstagramCookies) ToJSON() ([]byte, error) {
	return json.Marshal(&ig)
}

func (ig *InstagramCookies) GetValue(name string) string {
	return getCookieValue(name, ig)
}

func (ig *InstagramCookies) IsLoggedIn() bool {
	return ig.SessionId != ""
}

func (ig *InstagramCookies) GetViewports() (string, string) {
	return "2276", "1156"
}

func (ig *InstagramCookies) GetUserID() int64 {
	userID, _ := strconv.ParseInt(ig.UserId, 10, 64)
	return userID
}

func (ig *InstagramCookies) AllCookiesPresent() bool {
	return ig.SessionId != "" && ig.CsrfToken != "" && ig.MachineId != "" && ig.IgDeviceId != "" && ig.Rur != "" && ig.UserId != "" && ig.ShbId != "" && ig.Shbts != ""
}
