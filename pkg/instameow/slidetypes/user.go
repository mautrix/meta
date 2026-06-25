// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package slidetypes

type User struct {
	InteropMessagingUserFBID int64  `json:"interop_messaging_user_fbid,string"`
	Username                 string `json:"username"`
	FullName                 string `json:"full_name"`
	ID                       string `json:"id"`
	ProfilePicURL            string `json:"profile_pic_url"`
	LatestReelMedia          int    `json:"latest_reel_media"`
	LatestBestiesReelMedia   int    `json:"latest_besties_reel_media"`
	ReelMediaSeenTimestamp   any    `json:"reel_media_seen_timestamp"`
	IsVerified               bool   `json:"is_verified"`
	AIAgentType              string `json:"ai_agent_type"`
	FriendshipStatus         struct {
		IsRestricted bool `json:"is_restricted"`
		Blocking     bool `json:"blocking"`
		Following    bool `json:"following"`
	} `json:"friendship_status"`
	PK                string `json:"pk"`
	FBIDV2            string `json:"fbid_v2"`
	Typename          string `json:"__typename"`
	IsCannes          bool   `json:"is_cannes"`
	IsPredictedCannes bool   `json:"is_predicted_cannes"`

	Unrecognized map[string]any `json:",unknown"`
}
