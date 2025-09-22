// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
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

package metadb

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

type MetaDB struct {
	*dbutil.Database

	BridgeID networkid.BridgeID
}

func New(bridgeID networkid.BridgeID, db *dbutil.Database, log zerolog.Logger) *MetaDB {
	db = db.Child("meta_version", table, dbutil.ZeroLogger(log))
	return &MetaDB{
		BridgeID: bridgeID,
		Database: db,
	}
}

var table dbutil.UpgradeTable

//go:embed *.sql
var upgrades embed.FS

func init() {
	table.RegisterFS(upgrades)
}

func (db *MetaDB) PutThread(ctx context.Context, parentKey, threadKey int64, messageID string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meta_thread (parent_key, thread_key, message_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, parentKey, threadKey, messageID)
	return err
}

func (db *MetaDB) GetThreadByKey(ctx context.Context, threadKey int64) (parentKey int64, messageID string, err error) {
	err = db.QueryRow(ctx, "SELECT parent_key, message_id FROM meta_thread WHERE thread_key = $1", threadKey).
		Scan(&parentKey, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (db *MetaDB) GetThreadByMessage(ctx context.Context, messageID string) (threadKey int64, err error) {
	err = db.QueryRow(ctx, "SELECT thread_key FROM meta_thread WHERE message_id = $1", messageID).
		Scan(&threadKey)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (db *MetaDB) PutReconnectionState(ctx context.Context, loginID networkid.UserLoginID, state json.RawMessage) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meta_reconnection_state (bridge_id, login_id, state)
		VALUES ($1, $2, $3)
		ON CONFLICT (bridge_id, login_id) DO UPDATE SET state = excluded.state
	`, db.BridgeID, loginID, string(state))
	return err
}

func (db *MetaDB) PopReconnectionState(ctx context.Context, loginID networkid.UserLoginID) (state json.RawMessage, err error) {
	var stateStr string
	err = db.QueryRow(ctx, "DELETE FROM meta_reconnection_state WHERE bridge_id = $1 AND login_id = $2 RETURNING state", db.BridgeID, loginID).
		Scan(&stateStr)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	} else if err == nil {
		state = []byte(stateStr)
	}
	return
}

func (db *MetaDB) GetFBIDForIGUser(ctx context.Context, igid string) (fbid int64, err error) {
	err = db.QueryRow(ctx, "SELECT fbid FROM meta_instagram_user_id WHERE igid = $1", igid).Scan(&fbid)
	if errors.Is(err, sql.ErrNoRows) {
		// return 0 if not cached
		err = nil
	}
	return
}

func (db *MetaDB) GetIGUserForFBID(ctx context.Context, fbid int64) (igid string, err error) {
	err = db.QueryRow(ctx, "SELECT igid FROM meta_instagram_user_id WHERE fbid = $1", fbid).Scan(&igid)
	if errors.Is(err, sql.ErrNoRows) {
		// return "" if not cached
		err = nil
	}
	return
}

func (db *MetaDB) GetFBIDForIGThread(ctx context.Context, igid string) (fbid int64, err error) {
	err = db.QueryRow(ctx, "SELECT fbid FROM meta_instagram_thread_id WHERE igid = $1", igid).Scan(&fbid)
	if errors.Is(err, sql.ErrNoRows) {
		// return 0 if not cached
		err = nil
	}
	return
}

func (db *MetaDB) PutFBIDForIGUser(ctx context.Context, igid string, fbid int64) error {
	// If the fbid gets set to a new value for an existing row,
	// that would be surprising. We don't currently expect these
	// values ever to change.
	_, err := db.Exec(ctx, "INSERT INTO meta_instagram_user_id (igid, fbid) VALUES ($1, $2) ON CONFLICT DO NOTHING", igid, fbid)
	return err
}

func (db *MetaDB) PutFBIDForIGThread(ctx context.Context, igid string, fbid int64) error {
	_, err := db.Exec(ctx, "INSERT INTO meta_instagram_thread_id (igid, fbid) VALUES ($1, $2) ON CONFLICT DO NOTHING", igid, fbid)
	return err
}
