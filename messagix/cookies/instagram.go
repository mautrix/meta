package cookies

import "encoding/json"

type InstagramCookies struct {
	SessionId  string `cookie:"sessionid,omitempty" json:"SessionId,omitempty"`
	CsrfToken  string `cookie:"csrftoken,omitempty" json:"CsrfToken,omitempty"`
	MachineId  string `cookie:"mid,omitempty" json:"MachineId,omitempty"`
	IgDeviceId string `cookie:"ig_did,omitempty" json:"IgDeviceId,omitempty"`
	Rur 	   string `cookie:"rur,omitempty" json:"Rur,omitempty"`
	UserId 	   string `cookie:"ds_user_id,omitempty" json:"UserId,omitempty"`
	ShbId 	   string `cookie:"shbid,omitempty" json:"ShbId,omitempty"`
	Shbts 	   string `cookie:"shbts,omitempty" json:"Shbts,omitempty"`
	IgWWWClaim string `json:"IgWWWClaim,omitempty"`
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