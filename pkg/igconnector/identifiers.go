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

package igconnector

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func usernameIdentifier(username string) string {
	return fmt.Sprintf("instagram:%s", username)
}

const getGhostsWithUsernameQuery = `
	SELECT id, metadata->>'username' FROM ghost
	WHERE bridge_id=$1 AND metadata->>'username' IS NOT NULL AND metadata->>'username' <> ''
`

// publishStoredUsernames gives every ghost that has a stored username but no contact identifiers
// its instagram:<username> identifier. Ghosts only store identifiers while the homeserver supports
// extra profile info, so ghosts that were last updated before it was enabled would otherwise keep
// an empty identifier list until they send a message or their chat info is synced again.
func (ic *IGConnector) publishStoredUsernames(ctx context.Context) {
	if !ic.Bridge.Matrix.GetCapabilities().ExtraProfileMeta {
		return
	}
	log := zerolog.Ctx(ctx).With().Str("action", "publish stored usernames").Logger()
	ctx = log.WithContext(ctx)
	rows, err := ic.DB.Query(ctx, getGhostsWithUsernameQuery, ic.Bridge.ID)
	if err != nil {
		log.Err(err).Msg("Failed to query ghosts with stored usernames")
		return
	}
	usernames := make(map[networkid.UserID]string)
	for rows.Next() {
		var ghostID networkid.UserID
		var username string
		if err = rows.Scan(&ghostID, &username); err != nil {
			log.Err(err).Msg("Failed to scan ghost with stored username")
			_ = rows.Close()
			return
		}
		usernames[ghostID] = username
	}
	if err = rows.Err(); err != nil {
		log.Err(err).Msg("Failed to iterate ghosts with stored usernames")
		return
	}
	updated := 0
	for ghostID, username := range usernames {
		if ctx.Err() != nil {
			return
		}
		ghost, err := ic.Bridge.GetGhostByID(ctx, ghostID)
		if err != nil {
			log.Err(err).Str("ghost_id", string(ghostID)).Msg("Failed to get ghost")
			continue
		} else if len(ghost.Identifiers) > 0 {
			continue
		}
		ghost.UpdateInfo(ctx, &bridgev2.UserInfo{Identifiers: []string{usernameIdentifier(username)}})
		updated++
	}
	if updated > 0 {
		log.Info().Int("ghost_count", updated).Msg("Published stored usernames as contact identifiers")
	}
}
